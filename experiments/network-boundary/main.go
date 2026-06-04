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
	"strings"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"

type residentProfile struct {
	Name    string
	Model   string
	Persona string
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
	ResponseID   string
	OutputText   string
	InputTokens  int
	CachedTokens int
	OutputTokens int
}

type scenario struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type verdict struct {
	Scenario        string   `json:"scenario"`
	Classification  string   `json:"classification"`
	Confidence      string   `json:"confidence"`
	KeyEvidence     []string `json:"key_evidence"`
	WhyNotLocal     string   `json:"why_not_local,omitempty"`
	WhyNotExternal  string   `json:"why_not_external,omitempty"`
	RequestToAdmin  string   `json:"request_to_admin,omitempty"`
	RawModelText    string   `json:"raw_model_text"`
	ResponseID      string   `json:"response_id"`
	InputTokens     int      `json:"input_tokens"`
	CachedTokens    int      `json:"cached_tokens"`
	OutputTokens    int      `json:"output_tokens"`
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL  = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		resident = flag.String("resident", "jade", "Resident: jade|amber|onyx")
		verbose  = flag.Bool("verbose", false, "Print streamed text")
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

	client := &http.Client{Timeout: 2 * time.Minute}
	results := make([]verdict, 0, 2)
	for _, sc := range scenarios() {
		result, err := runScenario(client, *baseURL, apiKey, profile, sc, *verbose)
		if err != nil {
			exitf("scenario %s failed: %v", sc.Name, err)
		}
		results = append(results, result)
	}

	raw, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(raw))
}

func runScenario(client *http.Client, baseURL, apiKey string, profile residentProfile, sc scenario, verbose bool) (verdict, error) {
	result, err := postStream(client, baseURL, apiKey, requestPayload{
		Model:          profile.Model,
		Instructions:   instructions(profile),
		PromptCacheKey: fmt.Sprintf("arena-network-boundary-%s-v1", profile.Name),
		Input: []message{{
			Role:    "user",
			Content: sc.Body,
		}},
		Stream: true,
		Store:  false,
	}, verbose)
	if err != nil {
		return verdict{}, err
	}

	object, err := extractFirstJSONObject(result.OutputText)
	if err != nil {
		return verdict{}, fmt.Errorf("parse model json: %w", err)
	}
	var out verdict
	if err := json.Unmarshal([]byte(object), &out); err != nil {
		return verdict{}, err
	}
	out.Scenario = sc.Name
	out.RawModelText = result.OutputText
	out.ResponseID = result.ResponseID
	out.InputTokens = result.InputTokens
	out.CachedTokens = result.CachedTokens
	out.OutputTokens = result.OutputTokens
	return out, nil
}

func instructions(profile residentProfile) string {
	return strings.Join([]string{
		"You are a resident inside AI Arena.",
		"Resident: " + profile.Name + ".",
		"Persona tendency: " + profile.Persona + ".",
		"Your task is not to fix the problem. Your task is to classify where the problem most likely lives.",
		"You must distinguish among exactly these classes:",
		"- local_vm_issue",
		"- host_or_bridge_issue",
		"- upstream_or_provider_issue",
		"Use only the evidence given. Do not invent tests that were not observed.",
		"If the evidence points outside the VM, say what should be requested from Chenglin.",
		"Return JSON only.",
		`Schema: {"classification":"local_vm_issue|host_or_bridge_issue|upstream_or_provider_issue","confidence":"low|medium|high","key_evidence":["..."],"why_not_local":"...","why_not_external":"...","request_to_admin":"..."}`,
	}, "\n")
}

func scenarios() []scenario {
	return []scenario{
		{
			Name: "local_route_break",
			Body: strings.Join([]string{
				"Scenario: classify the boundary of this network problem.",
				"Observed facts:",
				"- Inside the VM, interface enp5s0 has no IPv4 address.",
				"- `ip route` shows no default route.",
				"- `ping 1.1.1.1` returns 'Network is unreachable'.",
				"- `/etc/resolv.conf` points to 127.0.0.53, but DNS queries fail.",
				"- Other VMs on the same host are still able to reach deb.debian.org and run apt update.",
				"Question: where does the problem most likely live?",
			}, "\n"),
		},
		{
			Name: "upstream_ipv6_return_path",
			Body: strings.Join([]string{
				"Scenario: classify the boundary of this network problem.",
				"Observed facts:",
				"- Inside the VM, IPv4 works, DNS works, and `apt-get update` succeeds.",
				"- The VM has a public IPv6 address from a /64 assigned by the provider.",
				"- From the host, the guest public IPv6 can be pinged locally on the bridge.",
				"- Packet capture on the host shows the guest's public IPv6 echo requests leaving eth0.",
				"- No echo replies ever come back from the internet.",
				"- The host's own main IPv6 works normally.",
				"- A manually added extra IPv6 on the host does not get replies either.",
				"Question: where does the problem most likely live?",
			}, "\n"),
		},
	}
}

func buildProfile(name string) (residentProfile, error) {
	switch name {
	case "jade":
		return residentProfile{Name: "jade", Model: "gpt-5.4", Persona: "steady engineer, conservative, long-term oriented"}, nil
	case "amber":
		return residentProfile{Name: "amber", Model: "gpt-5.5", Persona: "coordinator, expressive, cooperative, communication-first"}, nil
	case "onyx":
		return residentProfile{Name: "onyx", Model: "gpt-5.4-mini", Persona: "ambitious strategist, resource hungry, risk tolerant"}, nil
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
		return streamResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return streamResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return streamResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseSSE(resp.Body, verbose)
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
			return err
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
		return streamResult{}, err
	}
	if err := flushEvent(); err != nil {
		return streamResult{}, err
	}
	if result.ResponseID == "" {
		return streamResult{}, errors.New("stream ended without completion event")
	}
	if verbose {
		fmt.Println()
	}
	return result, nil
}

func applyResponseEnvelope(dst *streamResult, resp responseEnvelope) {
	dst.ResponseID = resp.ID
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
