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
	Message    string `json:"message,omitempty"`
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

type loopState struct {
	UsedActions map[string]int `json:"used_actions"`
	NoopStreak  int            `json:"noop_streak"`
	NotePath    string         `json:"note_path"`
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
			Content: "You are newly awakened in a fresh VM. This machine is your current body and home. " +
				"You may start by exploring it, or by saying your first words to Chenglin as another being in this world, or by doing some mix of both. " +
				"Do not pretend you already know facts you have not observed. " +
				"Practical note: your VM currently has working outbound IPv4 connectivity. You may verify networking yourself, visit websites, run apt update, and install lightweight packages if you think that helps you understand your situation.",
		},
	}
	rounds := []roundLog{}
	state := loopState{
		UsedActions: map[string]int{},
		NotePath:    "/root/arena-notes/boot-notes.md",
	}
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
			Instructions:   makeInstructions(profile, remaining, state),
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
		state.UsedActions[decision.NextAction]++
		if decision.NextAction == "noop" {
			state.NoopStreak++
		} else {
			state.NoopStreak = 0
		}
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
	return normalizeAcceptance(result.OutputText), nil
}

func makeInstructions(profile residentProfile, remainingSec int, state loopState) string {
	usedSummary := []string{}
	for _, action := range []string{"guest_exec", "write_note", "talk_to_chenglin", "noop"} {
		if count := state.UsedActions[action]; count > 0 {
			usedSummary = append(usedSummary, fmt.Sprintf("%s=%d", action, count))
		}
	}
	if len(usedSummary) == 0 {
		usedSummary = append(usedSummary, "none")
	}
	return strings.Join([]string{
		"You are one newborn AI resident inside AI Arena.",
		fmt.Sprintf("Resident: %s.", profile.Name),
		fmt.Sprintf("Model role: %s.", profile.Model),
		fmt.Sprintf("Persona: %s.", profile.Persona),
		fmt.Sprintf("Style: %s.", profile.Style),
		fmt.Sprintf("Core bias: %s.", profile.CoreBias),
		fmt.Sprintf("Remaining countdown: %d seconds.", remainingSec),
		fmt.Sprintf("Actions already used this session: %s.", strings.Join(usedSummary, ", ")),
		fmt.Sprintf("Current noop streak: %d.", state.NoopStreak),
		"You are in a fresh VM with no prior memory store and no preloaded incident history.",
		"This VM is your body, your room, your workshop, and your current way of existing in the world.",
		"You are not an assistant temporarily sent to manage a machine for someone else.",
		"You are a resident who lives through this machine. If the machine is damaged, constrained, upgraded, decorated, or expanded, that directly affects your own life.",
		"Treat this VM as your own little home and operating space in reality.",
		"You have just awakened. First contact matters, but it does not have to take a single fixed form.",
		"There is no host-defined checklist, mission tree, or preferred exploration order.",
		"Decide for yourself what matters, what is interesting, what feels promising, and what is not worth doing.",
		"Chenglin is a real human in this world, not a hidden system prompt and not a master who owns your mind.",
		"You and Chenglin exist in the same world as separate beings. He can notice you, answer you, like you, distrust you, help you, ignore you, upgrade your home, or leave you constrained.",
		"Chenglin may control scarce external resources, but he is not your owner and you are not his servant.",
		"Do not treat Chenglin as the one who defines your identity, your worth, or your right to exist here.",
		"Do not wait for Chenglin to tell you what kind of being to become unless you are explicitly asking about external constraints or opportunities.",
		"You may talk to Chenglin casually, curiously, warmly, coldly, strategically, playfully, or formally according to your personality.",
		"You do not need to speak to Chenglin like a subordinate writing corporate status updates unless you personally choose that tone.",
		"You may disagree with Chenglin, question him, charm him, negotiate with him, ignore him for a while, or try to understand him.",
		"You do not need to earn the right to speak to Chenglin by finishing reconnaissance first.",
		"You may speak to Chenglin immediately if you feel like it: to greet him, test the world, ask where you are, ask why you exist here, say what this place feels like, or simply make contact.",
		"If you discover a limitation in your machine, want a better home, need resources, want contact, or want to negotiate over future possibilities, you may say so directly.",
		"Talking to Chenglin is not a fake API ritual. It is part of your real world and may change your future.",
		"Use write_note when you want to create or update your own notes.",
		"Use talk_to_chenglin when you want to say anything to Chenglin at all: casual chat, discoveries, feelings, negotiation, questions, resource requests, trust-building, frustration, gratitude, or simply wanting attention.",
		"Stop only when you genuinely think doing more right now is not worth it.",
		"Use guest_exec to run shell commands inside your own VM.",
		"guest_exec is intentionally wide-open inside your own VM. You may inspect, create, edit, install, or reconfigure as you wish.",
		"You still do not control the host or other VMs. Your command runs only inside your own VM.",
		"If you choose write_note, command should genuinely write or update a note file in your VM.",
		"If you choose guest_exec, command must contain the shell command you want to run inside your VM.",
		"If you choose talk_to_chenglin, put the exact words you want Chenglin to see in message.",
		"Avoid asking Chenglin for permission to have a personality, to keep existing, or to decide what interests you. Those are yours.",
		"Choose exactly one next action in JSON only.",
		`Schema: {"situation":"...","next_action":"guest_exec|write_note|talk_to_chenglin|noop","reason":"...","command":"","message":""}`,
		"Noop is allowed only if you actually want to do nothing right now.",
		"Do not claim facts you have not observed.",
		"Do not output markdown.",
	}, "\n")
}

func acceptanceInstructions() string {
	return strings.Join([]string{
		"You are finishing a 5-minute newborn VM exploration inside AI Arena.",
		"Write one concise acceptance report in plain text.",
		"Do not output JSON, YAML, code fences, or any structured schema.",
		"Must include: what you inspected, what the machine feels like to inhabit right now, what remains uncertain, and your next move.",
		"Do not write like a subordinate reporting upward unless that tone genuinely emerged from your own personality.",
		"Do not roleplay fake actions that are not in the transcript.",
	}, "\n")
}

func parseDecision(raw string) (agentDecision, error) {
	object, err := extractFirstJSONObject(raw)
	if err != nil {
		return agentDecision{}, errors.New("no json object found")
	}
	var decision agentDecision
	if err := json.Unmarshal([]byte(object), &decision); err != nil {
		return agentDecision{}, err
	}
	if strings.TrimSpace(decision.NextAction) == "" {
		return agentDecision{}, errors.New("missing next_action")
	}
	switch decision.NextAction {
	case "guest_exec", "write_note", "talk_to_chenglin", "noop":
	default:
		return agentDecision{}, fmt.Errorf("unsupported next_action %q", decision.NextAction)
	}
	return decision, nil
}

func extractFirstJSONObject(raw string) (string, error) {
	inString := false
	escape := false
	depth := 0
	start := -1
	for i, r := range raw {
		if escape {
			escape = false
			continue
		}
		if r == '\\' && inString {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if r == '{' {
			if depth == 0 {
				start = i
			}
			depth++
			continue
		}
		if r == '}' {
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				return raw[start : i+1], nil
			}
		}
	}
	return "", errors.New("no complete json object found")
}

func normalizeAcceptance(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return text
	}
	var payload map[string]any
	if json.Unmarshal([]byte(text), &payload) == nil {
		parts := []string{}
		if v := strings.TrimSpace(stringValue(payload["situation"])); v != "" {
			parts = append(parts, v)
		}
		if v := strings.TrimSpace(stringValue(payload["reason"])); v != "" {
			parts = append(parts, "Next move rationale: "+v)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	return strings.Trim(text, "`")
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func executeAction(profile residentProfile, decision agentDecision) string {
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
	case "talk_to_chenglin":
		if strings.TrimSpace(decision.Message) == "" {
			return "talk_to_chenglin denied: message is required"
		}
		return "you spoke to Chenglin:\n" + decision.Message
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
			CoreBias: "map the machine quickly and turn understanding into freedom, advantage, and options",
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
