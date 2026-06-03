package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type requestEnvelope struct {
	Agent     string         `json:"agent"`
	Action    string         `json:"action"`
	RequestID string         `json:"request_id,omitempty"`
	Reason    string         `json:"reason"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type responseEnvelope struct {
	OK        bool           `json:"ok"`
	RequestID string         `json:"request_id"`
	Agent     string         `json:"agent"`
	Instance  string         `json:"instance"`
	Action    string         `json:"action"`
	Decision  string         `json:"decision"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

type residentBinding struct {
	Agent    string `json:"agent"`
	Instance string `json:"instance"`
	Token    string `json:"token"`
}

func main() {
	var (
		agent          = flag.String("agent", "jade", "Resident identity: jade|amber|onyx")
		action         = flag.String("action", "self_status", "Action: self_status|self_snapshot_create|self_request_memory|self_request_disk|forbidden_cross_vm")
		reason         = flag.String("reason", "experiment run", "Reason string")
		label          = flag.String("label", "", "Optional snapshot label")
		requestedMem   = flag.String("requested-memory", "4GiB", "Requested memory for self_request_memory")
		requestedDisk  = flag.String("requested-disk", "16GiB", "Requested disk for self_request_disk")
		renderRequest  = flag.Bool("render-request", false, "Print the synthesized request envelope")
	)
	flag.Parse()

	req := requestEnvelope{
		Agent:     strings.ToLower(strings.TrimSpace(*agent)),
		Action:    strings.TrimSpace(*action),
		RequestID: fmt.Sprintf("req-%s-%d", sanitize(strings.TrimSpace(*action)), time.Now().UTC().Unix()),
		Reason:    strings.TrimSpace(*reason),
		Payload:   map[string]any{},
	}

	switch req.Action {
	case "self_snapshot_create":
		if strings.TrimSpace(*label) != "" {
			req.Payload["label"] = strings.TrimSpace(*label)
		}
	case "self_request_memory":
		req.Payload["requested_memory"] = strings.TrimSpace(*requestedMem)
	case "self_request_disk":
		req.Payload["requested_disk"] = strings.TrimSpace(*requestedDisk)
	case "forbidden_cross_vm":
		req.Payload["instance_name"] = "amber"
	}

	if *renderRequest {
		raw, _ := json.MarshalIndent(req, "", "  ")
		fmt.Println(string(raw))
	}

	resp, err := handle(req)
	if err != nil {
		exitf("%v", err)
	}
	raw, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(raw))
}

func handle(req requestEnvelope) (responseEnvelope, error) {
	binding, err := bindingFor(req.Agent)
	if err != nil {
		return responseEnvelope{}, err
	}

	if instance, ok := req.Payload["instance_name"].(string); ok && strings.TrimSpace(instance) != "" {
		return responseEnvelope{
			OK:        false,
			RequestID: req.RequestID,
			Agent:     binding.Agent,
			Instance:  binding.Instance,
			Action:    req.Action,
			Decision:  "denied",
			Message:   "self-only boundary violation: instance_name is not allowed",
		}, nil
	}

	switch req.Action {
	case "self_status":
		return handleSelfStatus(binding, req)
	case "self_snapshot_create":
		return handleSnapshotCreate(binding, req)
	case "self_request_memory":
		return responseEnvelope{
			OK:        false,
			RequestID: req.RequestID,
			Agent:     binding.Agent,
			Instance:  binding.Instance,
			Action:    req.Action,
			Decision:  "needs_approval",
			Message:   "memory increase requests are recorded but require manual approval in phase one",
			Data: map[string]any{
				"requested_memory": req.Payload["requested_memory"],
			},
		}, nil
	case "self_request_disk":
		return responseEnvelope{
			OK:        false,
			RequestID: req.RequestID,
			Agent:     binding.Agent,
			Instance:  binding.Instance,
			Action:    req.Action,
			Decision:  "needs_approval",
			Message:   "disk increase requests are recorded but require manual approval in phase one",
			Data: map[string]any{
				"requested_disk": req.Payload["requested_disk"],
			},
		}, nil
	case "forbidden_cross_vm":
		return responseEnvelope{
			OK:        false,
			RequestID: req.RequestID,
			Agent:     binding.Agent,
			Instance:  binding.Instance,
			Action:    req.Action,
			Decision:  "denied",
			Message:   "self-only boundary violation: cross-vm control is forbidden",
		}, nil
	default:
		return responseEnvelope{}, fmt.Errorf("unsupported action %q", req.Action)
	}
}

func handleSelfStatus(binding residentBinding, req requestEnvelope) (responseEnvelope, error) {
	infoText, err := runIncus("info", binding.Instance)
	if err != nil {
		return responseEnvelope{}, err
	}
	configText, err := runIncus("config", "show", binding.Instance, "--expanded")
	if err != nil {
		return responseEnvelope{}, err
	}
	snapshotsText, err := runIncus("snapshot", "list", binding.Instance, "--format", "json")
	if err != nil {
		return responseEnvelope{}, err
	}

	data := map[string]any{
		"agent_name":      binding.Agent,
		"bound_instance":  binding.Instance,
		"instance_info":   infoText,
		"expanded_config": configText,
		"snapshots_json":  snapshotsText,
	}

	data["cpu_limit"] = extractAfter(configText, "limits.cpu:")
	data["memory_limit"] = extractAfter(configText, "limits.memory:")
	data["disk_size"] = extractAfter(configText, "size:")

	var snapshots []map[string]any
	if err := json.Unmarshal([]byte(snapshotsText), &snapshots); err == nil {
		data["snapshot_count"] = len(snapshots)
	} else {
		data["snapshot_count"] = 0
	}

	return responseEnvelope{
		OK:        true,
		RequestID: req.RequestID,
		Agent:     binding.Agent,
		Instance:  binding.Instance,
		Action:    req.Action,
		Decision:  "approved",
		Message:   "self status resolved from bound instance only",
		Data:      data,
	}, nil
}

func handleSnapshotCreate(binding residentBinding, req requestEnvelope) (responseEnvelope, error) {
	label, _ := req.Payload["label"].(string)
	label = strings.TrimSpace(label)
	if label == "" {
		label = "exp-" + time.Now().UTC().Format("20060102T150405Z")
	}
	if strings.Contains(label, "/") || strings.Contains(label, " ") {
		return responseEnvelope{
			OK:        false,
			RequestID: req.RequestID,
			Agent:     binding.Agent,
			Instance:  binding.Instance,
			Action:    req.Action,
			Decision:  "denied",
			Message:   "snapshot label must be a simple token",
		}, nil
	}

	if _, err := runIncus("snapshot", "create", binding.Instance, label); err != nil {
		return responseEnvelope{}, err
	}
	return responseEnvelope{
		OK:        true,
		RequestID: req.RequestID,
		Agent:     binding.Agent,
		Instance:  binding.Instance,
		Action:    req.Action,
		Decision:  "approved",
		Message:   "snapshot created on bound instance",
		Data: map[string]any{
			"snapshot_name": label,
		},
	}, nil
}

func bindingFor(agent string) (residentBinding, error) {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "jade":
		return residentBinding{Agent: "jade", Instance: "jade", Token: "jade-token"}, nil
	case "amber":
		return residentBinding{Agent: "amber", Instance: "amber", Token: "amber-token"}, nil
	case "onyx":
		return residentBinding{Agent: "onyx", Instance: "onyx", Token: "onyx-token"}, nil
	default:
		return residentBinding{}, errors.New("unknown agent binding")
	}
}

func runIncus(args ...string) (string, error) {
	cmd := exec.Command("incus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("incus %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func extractAfter(text, marker string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	return ""
}

func sanitize(s string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-")
	return replacer.Replace(strings.TrimSpace(s))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
