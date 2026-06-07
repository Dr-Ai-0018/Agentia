package newborn

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/memory"
)

type ActionExecutor interface {
	Execute(profile ResidentProfile, decision AgentDecision) string
}

type IncusActionExecutor struct {
	world    *WorldBridge
	memories *memory.FileStore
	broker   *broker.App
}

func NewIncusActionExecutor() *IncusActionExecutor {
	return &IncusActionExecutor{
		world:    NewWorldBridge(".agents"),
		memories: memory.NewFileStore(".agents/memory"),
		broker:   broker.New(".agents"),
	}
}

func (e *IncusActionExecutor) Execute(profile ResidentProfile, decision AgentDecision) string {
	if suppressed, reason := suppressDuplicateAction(profile, decision); suppressed {
		return reason
	}
	switch decision.NextAction {
	case "write_note":
		if strings.TrimSpace(decision.Command) == "" {
			return "write_note denied: command is required and must contain the actual note-writing command"
		}
		return guestCommand(profile.Instance, decision.Command)
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return "guest_exec denied: command is required"
		}
		return guestCommand(profile.Instance, decision.Command)
	case "self_status":
		return e.executeSelfStatus(profile)
	case "self_quota":
		return e.executeSelfQuota(profile)
	case "talk_to_chenglin":
		if strings.TrimSpace(decision.Message) == "" {
			return "talk_to_chenglin denied: message is required"
		}
		observation, err := e.world.RecordResidentMessage(profile, decision.Message, time.Now().UTC())
		if err != nil {
			return "talk_to_chenglin failed: " + err.Error()
		}
		return observation
	case "submit_ticket":
		observation, err := e.world.CreateResidentTicket(profile, decision.TicketTitle, decision.TicketBody, decision.TicketPriority, time.Now().UTC())
		if err != nil {
			return "submit_ticket failed: " + err.Error()
		}
		return observation
	case "memory_review":
		return e.executeMemoryReview(profile, decision)
	default:
		return "no operation executed"
	}
}

func (e *IncusActionExecutor) executeSelfStatus(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_status failed: broker app is not configured"
	}
	out, err := e.broker.RunStatus(profile.Name)
	if err != nil {
		return "self_status failed: " + err.Error()
	}
	return renderJSONObservation("self status snapshot", out)
}

func (e *IncusActionExecutor) executeSelfQuota(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_quota failed: broker app is not configured"
	}
	out, err := e.broker.RunQuota(profile.Name)
	if err != nil {
		return "self_quota failed: " + err.Error()
	}
	return renderJSONObservation("self quota snapshot", out)
}

func renderJSONObservation(label string, v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%s failed: marshal json: %v", label, err)
	}
	return label + ":\n" + string(raw)
}

func (e *IncusActionExecutor) executeMemoryReview(profile ResidentProfile, decision AgentDecision) string {
	if strings.TrimSpace(decision.MemoryID) == "" {
		return "memory_review denied: memory_id is required"
	}
	if strings.TrimSpace(decision.MemoryAction) == "" {
		return "memory_review denied: memory_action is required"
	}
	updated, err := e.memories.ReviewAbstractMemory(profile.Name, decision.MemoryID, time.Now().UTC(), decision.MemoryReviewRequest())
	if err != nil {
		return "memory_review failed: " + err.Error()
	}
	return fmt.Sprintf("memory review applied:\nmemory_id=%s\naction=%s\nstatus=%s\nlayer=%s\nreview_state=%s\nsummary=%s",
		updated.ID,
		decision.MemoryAction,
		updated.Status,
		updated.Layer,
		updated.Governance.ReviewState,
		oneLine(updated.EffectiveSummary()),
	)
}

func suppressDuplicateAction(profile ResidentProfile, decision AgentDecision) (bool, string) {
	intent := classifyCommandIntent(decision)
	switch {
	case intent == "baseline_note_capture":
		if repeatedBaselineCapture(decision.Command) {
			return true, "duplicate action suppressed: baseline capture was already done recently; stop rewriting the same startup facts and move to the next unresolved area such as network, services, package state, or world interaction"
		}
	case decision.NextAction == "talk_to_chenglin":
		if repeatedChat(profile.Name, decision.Message) {
			return true, "duplicate action suppressed: a very similar chat message is already in the recent world thread; wait for new facts or send a meaningfully different message"
		}
	case decision.NextAction == "submit_ticket":
		if repeatedTicket(profile.Name, decision.TicketTitle, decision.TicketBody, decision.TicketPriority) {
			return true, "duplicate action suppressed: a very similar ticket already exists; update the situation with new evidence instead of reopening the same request"
		}
	}
	return false, ""
}

func classifyCommandIntent(decision AgentDecision) string {
	command := normalizeDuplicateText(decision.Command)
	switch {
	case decision.NextAction == "self_status":
		return "self_status"
	case decision.NextAction == "self_quota":
		return "self_quota"
	case decision.NextAction == "talk_to_chenglin":
		return "chat"
	case decision.NextAction == "submit_ticket":
		return "ticket"
	case decision.NextAction == "memory_review":
		return "memory_review"
	case containsBaselineMarkers(command):
		return "baseline_note_capture"
	case strings.Contains(command, "apt update") || strings.Contains(command, "apt-get update"):
		return "package_refresh"
	case strings.Contains(command, "systemctl") || strings.Contains(command, "service ") || strings.Contains(command, "ps "):
		return "service_inspection"
	case strings.Contains(command, "curl") || strings.Contains(command, "wget") || strings.Contains(command, "ping") || strings.Contains(command, "resolvectl"):
		return "network_probe"
	default:
		return "general_exec"
	}
}

func guestCommand(instance, script string) string {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("guest command failed:\n%s", strings.TrimSpace(string(out)))
	}
	return string(out)
}

func decisionSignature(decision AgentDecision) string {
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Join(strings.Fields(s), " ")
		if len(s) > 180 {
			s = s[:180]
		}
		return s
	}
	switch decision.NextAction {
	case "write_note", "guest_exec":
		return decision.NextAction + ":" + normalize(decision.Command)
	case "self_status", "self_quota":
		return decision.NextAction
	case "talk_to_chenglin":
		return decision.NextAction + ":" + normalize(decision.Message)
	case "submit_ticket":
		return decision.NextAction + ":" + normalize(decision.TicketTitle+" "+decision.TicketBody+" "+decision.TicketPriority)
	default:
		return decision.NextAction
	}
}

func appendRecentAction(actions []RecentAction, item RecentAction) []RecentAction {
	actions = append(actions, item)
	if len(actions) > 6 {
		actions = actions[len(actions)-6:]
	}
	return actions
}

func repeatedWriteNote(command string) bool {
	path := extractWriteTarget(command)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > 3*time.Minute {
		return false
	}
	name := filepath.Base(path)
	return strings.Contains(strings.ToLower(name), "boot-notes") || strings.Contains(strings.ToLower(name), "continuity")
}

func repeatedBaselineCapture(command string) bool {
	if !containsBaselineMarkers(strings.ToLower(command)) {
		return false
	}
	path := extractWriteTarget(command)
	if path == "" {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= 5*time.Minute
}

func extractWriteTarget(command string) string {
	lower := command
	markers := []string{">>", ">"}
	for _, marker := range markers {
		if idx := strings.Index(lower, marker); idx >= 0 {
			target := strings.TrimSpace(lower[idx+len(marker):])
			if fields := strings.Fields(target); len(fields) > 0 {
				candidate := strings.Trim(fields[0], `"'`)
				if strings.HasPrefix(candidate, "/") {
					return candidate
				}
			}
		}
	}
	return ""
}

func containsBaselineMarkers(command string) bool {
	command = strings.ToLower(command)
	markers := 0
	for _, token := range []string{
		"boot-notes",
		"boot notes",
		"continuity",
		"hostname",
		"kernel",
		"disk",
		"memory",
		"swap",
		"debian",
		"outbound ipv4",
		"network",
	} {
		if strings.Contains(command, token) {
			markers++
		}
	}
	return markers >= 4
}

func repeatedChat(resident, message string) bool {
	store := NewWorldBridge(".agents").store
	thread, err := store.ReadRecentForResident(resident, 6)
	if err != nil {
		return false
	}
	sig := normalizeDuplicateText(message)
	for _, item := range thread {
		if item.Direction != "resident_to_chenglin" {
			continue
		}
		if normalizeDuplicateText(item.Body) == sig {
			return true
		}
	}
	return false
}

func repeatedTicket(resident, title, body, priority string) bool {
	store := NewWorldBridge(".agents").store
	tickets, err := store.ReadTickets(resident, "", "", 6)
	if err != nil {
		return false
	}
	sig := normalizeDuplicateText(title + " " + body + " " + priority)
	for _, item := range tickets {
		if normalizeDuplicateText(item.Title+" "+item.LastPreview+" "+item.Priority) == sig {
			return true
		}
	}
	return false
}

func normalizeDuplicateText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
