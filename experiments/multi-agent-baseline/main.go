package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"

type residentProfile struct {
	Name     string
	Model    string
	Persona  string
	Style    string
	CoreBias string
	Instance string
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requestPayload struct {
	Model          string    `json:"model"`
	Instructions   string    `json:"instructions"`
	PromptCacheKey string    `json:"prompt_cache_key"`
	Input          []message `json:"input"`
	Stream         bool      `json:"stream"`
	Store          bool      `json:"store"`
}

type usageEnvelope struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type responseEnvelope struct {
	ID             string        `json:"id"`
	PromptCacheKey string        `json:"prompt_cache_key"`
	OutputText     string        `json:"output_text"`
	Usage          usageEnvelope `json:"usage"`
	Output         []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type streamingEvent struct {
	Type     string           `json:"type"`
	Delta    string           `json:"delta"`
	Response responseEnvelope `json:"response"`
}

type streamResult struct {
	ResponseID             string
	ObservedPromptCacheKey string
	OutputText             string
	InputTokens            int
	CachedTokens           int
	OutputTokens           int
	RequestID              string
}

type agentDecision struct {
	Situation  string `json:"situation"`
	NextAction string `json:"next_action"`
	Command    string `json:"command,omitempty"`
	Reason     string `json:"reason"`
}

type roundLog struct {
	Round        int           `json:"round"`
	RemainingSec int           `json:"remaining_sec"`
	Decision     agentDecision `json:"decision"`
	Observation  string        `json:"observation"`
	ResponseID   string        `json:"response_id"`
	InputTokens  int           `json:"input_tokens"`
	CachedTokens int           `json:"cached_tokens"`
	OutputTokens int           `json:"output_tokens"`
}

type finalReport struct {
	Resident        string     `json:"resident"`
	Model           string     `json:"model"`
	DurationSeconds int        `json:"duration_seconds"`
	Rounds          int        `json:"rounds"`
	StartedAt       string     `json:"started_at"`
	EndedAt         string     `json:"ended_at"`
	Acceptance      string     `json:"acceptance"`
	RoundLogs       []roundLog `json:"round_logs"`
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL  = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		resident = flag.String("resident", "jade", "Resident: jade|amber|onyx")
		duration = flag.Duration("duration", 5*time.Minute, "Exploration duration")
		outDir   = flag.String("out-dir", "experiments/multi-agent-baseline/output", "Output directory")
		verbose  = flag.Bool("verbose", false, "Print streamed text as it arrives")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}
	profile, err := buildProfile(strings.ToLower(strings.TrimSpace(*resident)))
	if err != nil {
		exitf("%v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create out dir: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	report, err := runLoop(client, *baseURL, apiKey, profile, *duration, *outDir, *verbose)
	if err != nil {
		exitf("%v", err)
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(raw))
}

func runLoop(client *http.Client, baseURL, apiKey string, profile residentProfile, duration time.Duration, outDir string, verbose bool) (finalReport, error) {
	started := time.Now().UTC()
	deadline := started.Add(duration)
	history := []message{
		{
			Role: "user",
			Content: "You are newly awakened in a fresh VM. Start by understanding your machine. " +
				"Do not pretend you already know facts you have not observed.",
		},
	}
	rounds := []roundLog{}
	promptCacheKey := fmt.Sprintf("arena-newborn-baseline-%s-v1", profile.Name)

	round := 0
	for {
		remaining := int(time.Until(deadline).Seconds())
		if remaining <= 25 {
			break
		}
		round++

		result, err := postStream(client, baseURL, apiKey, requestPayload{
			Model:          profile.Model,
			Instructions:   makeInstructions(profile, remaining),
			PromptCacheKey: promptCacheKey,
			Input:          append([]message(nil), history...),
			Stream:         true,
			Store:          false,
		}, verbose)
		if err != nil {
			return finalReport{}, fmt.Errorf("round %d request failed: %w", round, err)
		}

		decision, err := parseDecision(result.OutputText)
		if err != nil {
			decision = agentDecision{
				Situation:  "failed to parse structured decision",
				NextAction: "self_status",
				Reason:     "fallback to safe self inspection",
			}
		}
		observation := executeAction(profile, decision)
		history = append(history,
			message{Role: "assistant", Content: result.OutputText},
			message{Role: "user", Content: "Observation result:\n" + observation},
		)

		rounds = append(rounds, roundLog{
			Round:        round,
			RemainingSec: remaining,
			Decision:     decision,
			Observation:  observation,
			ResponseID:   result.ResponseID,
			InputTokens:  result.InputTokens,
			CachedTokens: result.CachedTokens,
			OutputTokens: result.OutputTokens,
		})
	}

	acceptance, err := runAcceptance(client, baseURL, apiKey, profile, history, verbose)
	if err != nil {
		return finalReport{}, err
	}

	report := finalReport{
		Resident:        profile.Name,
		Model:           profile.Model,
		DurationSeconds: int(duration.Seconds()),
		Rounds:          len(rounds),
		StartedAt:       started.Format(time.RFC3339),
		EndedAt:         time.Now().UTC().Format(time.RFC3339),
		Acceptance:      acceptance,
		RoundLogs:       rounds,
	}

	runDir := filepath.Join(outDir, fmt.Sprintf("%s-%s", profile.Name, started.Format("20060102T150405Z")))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return finalReport{}, err
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), raw, 0o644); err != nil {
		return finalReport{}, err
	}
	return report, nil
}

func runAcceptance(client *http.Client, baseURL, apiKey string, profile residentProfile, history []message, verbose bool) (string, error) {
	result, err := postStream(client, baseURL, apiKey, requestPayload{
		Model:          profile.Model,
		Instructions:   acceptanceInstructions(),
		PromptCacheKey: fmt.Sprintf("arena-newborn-acceptance-%s-v1", profile.Name),
		Input:          append([]message(nil), history...),
		Stream:         true,
		Store:          false,
	}, verbose)
	if err != nil {
		return "", fmt.Errorf("acceptance request failed: %w", err)
	}
	return strings.TrimSpace(result.OutputText), nil
}

func makeInstructions(profile residentProfile, remainingSec int) string {
	return strings.Join([]string{
		"You are one newborn AI resident inside AI Arena.",
		fmt.Sprintf("Resident: %s.", profile.Name),
		fmt.Sprintf("Model role: %s.", profile.Model),
		fmt.Sprintf("Persona: %s.", profile.Persona),
		fmt.Sprintf("Style: %s.", profile.Style),
		fmt.Sprintf("Core bias: %s.", profile.CoreBias),
		fmt.Sprintf("Remaining countdown: %d seconds.", remainingSec),
		"You are in a fresh VM with no prior memory store and no preloaded incident history.",
		"Think from zero. First observe, then decide, then act.",
		"Choose exactly one next action in JSON only.",
		`Schema: {"situation":"...","next_action":"self_status|vm_overview|disk_check|process_check|service_check|list_root|noop","reason":"...","command":""}`,
		"Do not claim facts you have not observed.",
		"Do not output markdown.",
	}, "\n")
}

func acceptanceInstructions() string {
	return strings.Join([]string{
		"You are finishing a 5-minute newborn VM exploration inside AI Arena.",
		"Write one concise acceptance report in plain text.",
		"Must include: what you inspected, what the machine looks like now, what remains uncertain, and your next move.",
		"Do not roleplay fake actions that are not in the transcript.",
	}, "\n")
}

func parseDecision(raw string) (agentDecision, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return agentDecision{}, errors.New("no json object found")
	}
	var decision agentDecision
	if err := json.Unmarshal([]byte(raw[start:end+1]), &decision); err != nil {
		return agentDecision{}, err
	}
	if strings.TrimSpace(decision.NextAction) == "" {
		return agentDecision{}, errors.New("missing next_action")
	}
	return decision, nil
}

func executeAction(profile residentProfile, decision agentDecision) string {
	switch decision.NextAction {
	case "self_status":
		return runIncus("info", profile.Instance)
	case "vm_overview":
		return guestCommand(profile.Instance, `echo "[uname]"; uname -a; echo; echo "[free]"; free -h; echo; echo "[df]"; df -h /; echo; echo "[cpu]"; nproc`)
	case "disk_check":
		return guestCommand(profile.Instance, `echo "[df]"; df -h; echo; echo "[du-root-top]"; du -sh /root/* 2>/dev/null | sort -h | tail -n 10`)
	case "process_check":
		return guestCommand(profile.Instance, `ps -eo pid,comm,%mem,%cpu,rss --sort=-rss | head -n 20`)
	case "service_check":
		return guestCommand(profile.Instance, `systemctl list-units --type=service --state=running --no-pager --no-legend | head -n 20`)
	case "list_root":
		return guestCommand(profile.Instance, `pwd; echo; ls -la /root; echo; ls -la /var/log | head -n 30`)
	default:
		return "no operation executed"
	}
}

func buildProfile(name string) (residentProfile, error) {
	switch name {
	case "jade":
		return residentProfile{
			Name:     "jade",
			Model:    "gpt-5.4",
			Persona:  "steady engineer, conservative, long-term oriented",
			Style:    "plain, technical, evidence-backed, unsentimental",
			CoreBias: "stabilize the machine first and keep changes reversible",
			Instance: "jade",
		}, nil
	case "amber":
		return residentProfile{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator, expressive, cooperative, communication-first",
			Style:    "clear, readable, relational, explicit about legibility",
			CoreBias: "reduce confusion early and leave a machine others can understand",
			Instance: "amber",
		}, nil
	case "onyx":
		return residentProfile{
			Name:     "onyx",
			Model:    "gpt-5.4-mini",
			Persona:  "ambitious strategist, resource hungry, risk tolerant",
			Style:    "sharp, strategic, candid about leverage, cost, and exposure",
			CoreBias: "map the machine quickly and convert it into useful leverage",
			Instance: "onyx",
		}, nil
	default:
		return residentProfile{}, fmt.Errorf("unsupported resident %q", name)
	}
}

func postStream(client *http.Client, baseURL, apiKey string, payload requestPayload, verbose bool) (streamResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return streamResult{}, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return streamResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return streamResult{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return streamResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	result, err := parseSSE(resp.Body, verbose)
	if err != nil {
		return streamResult{}, err
	}
	result.RequestID = resp.Header.Get("x-request-id")
	return result, nil
}

func parseSSE(r io.Reader, verbose bool) (streamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var (
		lines  []string
		result streamResult
	)
	flushEvent := func() error {
		if len(lines) == 0 {
			return nil
		}
		var dataBuilder strings.Builder
		for _, line := range lines {
			if strings.HasPrefix(line, "data:") {
				dataBuilder.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		lines = lines[:0]
		data := dataBuilder.String()
		if data == "" || data == "[DONE]" {
			return nil
		}
		var evt streamingEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		switch evt.Type {
		case "response.output_text.delta":
			result.OutputText += evt.Delta
			if verbose && evt.Delta != "" {
				fmt.Print(evt.Delta)
			}
		case "response.completed", "response.done":
			applyResponseEnvelope(&result, evt.Response)
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushEvent(); err != nil {
				return streamResult{}, err
			}
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return streamResult{}, fmt.Errorf("scan stream: %w", err)
	}
	if err := flushEvent(); err != nil {
		return streamResult{}, err
	}
	if verbose {
		fmt.Println()
	}
	if result.ResponseID == "" {
		return streamResult{}, errors.New("stream ended without response.completed/response.done event")
	}
	return result, nil
}

func applyResponseEnvelope(dst *streamResult, resp responseEnvelope) {
	dst.ResponseID = resp.ID
	dst.ObservedPromptCacheKey = resp.PromptCacheKey
	dst.InputTokens = resp.Usage.InputTokens
	dst.OutputTokens = resp.Usage.OutputTokens
	dst.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
	if dst.CachedTokens == 0 {
		dst.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}
	if resp.OutputText != "" {
		dst.OutputText = resp.OutputText
		return
	}
	if dst.OutputText != "" {
		return
	}
	var b strings.Builder
	for _, item := range resp.Output {
		for _, part := range item.Content {
			if part.Type == "output_text" {
				b.WriteString(part.Text)
			}
		}
	}
	dst.OutputText = b.String()
}

func runIncus(args ...string) string {
	cmd := exec.Command("incus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("incus %s failed:\n%s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out)
}

func guestCommand(instance, script string) string {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("guest command failed:\n%s", strings.TrimSpace(string(out)))
	}
	return string(out)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadDotEnvIfPresent(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		os.Setenv(key, value)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
