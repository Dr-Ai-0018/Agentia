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

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requestPayload struct {
	Model             string         `json:"model"`
	Instructions      string         `json:"instructions"`
	PromptCacheKey    string         `json:"prompt_cache_key"`
	Input             []message      `json:"input"`
	Tools             []responseTool `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Stream            bool           `json:"stream"`
	Store             bool           `json:"store"`
}

type responseTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type functionToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
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
	ID             string         `json:"id"`
	PromptCacheKey string         `json:"prompt_cache_key"`
	OutputText     string         `json:"output_text"`
	Usage          usageEnvelope  `json:"usage"`
	Output         []responseItem `json:"output"`
}

type responseItem struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallName  string `json:"call_name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	ID        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
}

type streamingEvent struct {
	Type      string           `json:"type"`
	Delta     string           `json:"delta"`
	Arguments string           `json:"arguments"`
	ItemID    string           `json:"item_id"`
	Item      responseItem     `json:"item"`
	Response  responseEnvelope `json:"response"`
}

type streamResult struct {
	ResponseID             string         `json:"response_id"`
	ObservedPromptCacheKey string         `json:"observed_prompt_cache_key"`
	OutputText             string         `json:"output_text"`
	FunctionCalls          []responseItem `json:"function_calls"`
	InputTokens            int            `json:"input_tokens"`
	CachedTokens           int            `json:"cached_tokens"`
	OutputTokens           int            `json:"output_tokens"`
	RequestID              string         `json:"request_id"`
}

type caseResult struct {
	Name         string `json:"name"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	ResponseID   string `json:"response_id,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	CachedTokens int    `json:"cached_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	OutputText   string `json:"output_text,omitempty"`
	CallName     string `json:"call_name,omitempty"`
	Arguments    string `json:"arguments,omitempty"`
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		model   = flag.String("model", envOrDefault("OPENAI_MODEL", "gpt-5.4"), "OpenAI model ID")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	results := []caseResult{}
	for _, c := range cases(*model) {
		res, err := postStream(client, *baseURL, apiKey, c.payload, false)
		if err != nil {
			results = append(results, caseResult{Name: c.name, Success: false, Error: err.Error()})
			continue
		}
		callName := ""
		if len(res.FunctionCalls) > 0 {
			callName = strings.TrimSpace(res.FunctionCalls[0].Name)
			if callName == "" {
				callName = strings.TrimSpace(res.FunctionCalls[0].CallName)
			}
		}
		args := ""
		if len(res.FunctionCalls) > 0 {
			args = strings.TrimSpace(res.FunctionCalls[0].Arguments)
		}
		results = append(results, caseResult{
			Name:         c.name,
			Success:      true,
			ResponseID:   res.ResponseID,
			InputTokens:  res.InputTokens,
			CachedTokens: res.CachedTokens,
			OutputTokens: res.OutputTokens,
			OutputText:   truncate(strings.TrimSpace(res.OutputText), 120),
			CallName:     callName,
			Arguments:    args,
		})
	}
	raw, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(raw))
}

type probeCase struct {
	name    string
	payload requestPayload
}

func cases(model string) []probeCase {
	ptc := false
	instructions := "Use the provided function tool exactly once. Do not produce free text."
	user := []message{{Role: "user", Content: "Return one compact valid decision."}}
	return []probeCase{
		{
			name: "legacy_small_required",
			payload: requestPayload{
				Model:          model,
				Instructions:   instructions,
				PromptCacheKey: "arena-fc-boundary-legacy-v1",
				Input:          user,
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose one next action.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "write_note", "talk_to_chenglin", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
							"message":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason", "command", "message"},
						"additionalProperties": false,
					},
				}},
				ToolChoice:        functionToolChoice{Type: "function", Name: "decide_next_action"},
				ParallelToolCalls: &ptc,
				Stream:            true,
				Store:             false,
			},
		},
		{
			name: "minimal_three_required",
			payload: requestPayload{
				Model:          model,
				Instructions:   instructions,
				PromptCacheKey: "arena-fc-boundary-min-v1",
				Input:          user,
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose one next action.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason"},
						"additionalProperties": false,
					},
				}},
				ToolChoice:        functionToolChoice{Type: "function", Name: "decide_next_action"},
				ParallelToolCalls: &ptc,
				Stream:            true,
				Store:             false,
			},
		},
		{
			name: "minimal_four_required",
			payload: requestPayload{
				Model:          model,
				Instructions:   instructions,
				PromptCacheKey: "arena-fc-boundary-min4-v1",
				Input:          user,
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose one next action.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason", "command"},
						"additionalProperties": false,
					},
				}},
				ToolChoice:        functionToolChoice{Type: "function", Name: "decide_next_action"},
				ParallelToolCalls: &ptc,
				Stream:            true,
				Store:             false,
			},
		},
		{
			name: "current_newborn_exact",
			payload: requestPayload{
				Model:          model,
				Instructions:   instructions,
				PromptCacheKey: "arena-fc-boundary-current-newborn-v1",
				Input:          user,
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose exactly one next action for the resident's own VM session.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason"},
						"additionalProperties": false,
					},
				}},
				Stream: true,
				Store:  false,
			},
		},
		{
			name: "current_newborn_all_required_no_toolchoice",
			payload: requestPayload{
				Model:          model,
				Instructions:   instructions,
				PromptCacheKey: "arena-fc-boundary-current-newborn-allreq-v1",
				Input:          user,
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose exactly one next action for the resident's own VM session.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason", "command"},
						"additionalProperties": false,
					},
				}},
				Stream: true,
				Store:  false,
			},
		},
		{
			name: "self_quota_required_command",
			payload: requestPayload{
				Model:        model,
				Instructions: "Use the provided function tool exactly once. Choose self_quota because broker-side quota facts are needed. Do not produce free text. If the action does not need a shell command, set command to an empty string.",
				PromptCacheKey: "arena-fc-boundary-selfquota-v1",
				Input: []message{{
					Role:    "user",
					Content: "You do not need a shell command. Ask for quota facts from the broker.",
				}},
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose exactly one next action for the resident's own VM session.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason", "command"},
						"additionalProperties": false,
					},
				}},
				Stream: true,
				Store:  false,
			},
		},
		{
			name: "noop_required_command",
			payload: requestPayload{
				Model:        model,
				Instructions: "Use the provided function tool exactly once. Choose noop because no action is needed right now. Do not produce free text. If the action does not need a shell command, set command to an empty string.",
				PromptCacheKey: "arena-fc-boundary-noop-v1",
				Input: []message{{
					Role:    "user",
					Content: "There is no urgent action to take. Do not run a shell command.",
				}},
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose exactly one next action for the resident's own VM session.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":   map[string]any{"type": "string"},
							"next_action": map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "noop"}},
							"reason":      map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
						},
						"required":             []string{"situation", "next_action", "reason", "command"},
						"additionalProperties": false,
					},
				}},
				Stream: true,
				Store:  false,
			},
		},
		{
			name: "full_newborn_all_required",
			payload: requestPayload{
				Model: model,
				Instructions: strings.Join([]string{
					"Use the provided function tool exactly once.",
					"Choose one action for a resident inside its own VM.",
					"If the chosen action does not need a field, return that field as an empty string.",
					"Do not produce free text.",
				}, " "),
				PromptCacheKey: "arena-fc-boundary-full-newborn-v1",
				Input: []message{{
					Role:    "user",
					Content: "You may inspect the VM, ask the broker for self facts, chat with Chenglin, open a ticket, review memory, write a note, or do nothing.",
				}},
				Tools: []responseTool{{
					Type:        "function",
					Name:        "decide_next_action",
					Description: "Choose exactly one next action for the resident's own VM session.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"situation":       map[string]any{"type": "string"},
							"next_action":     map[string]any{"type": "string", "enum": []string{"guest_exec", "self_status", "self_quota", "write_note", "talk_to_chenglin", "submit_ticket", "memory_review", "noop"}},
							"reason":          map[string]any{"type": "string"},
							"command":         map[string]any{"type": "string"},
							"message":         map[string]any{"type": "string"},
							"ticket_title":    map[string]any{"type": "string"},
							"ticket_body":     map[string]any{"type": "string"},
							"ticket_priority": map[string]any{"type": "string", "enum": []string{"", "low", "medium", "high", "urgent"}},
							"memory_id":       map[string]any{"type": "string"},
							"memory_action":   map[string]any{"type": "string", "enum": []string{"", "keep", "rewrite", "compress", "demote", "delete"}},
							"memory_summary":  map[string]any{"type": "string"},
							"memory_text":     map[string]any{"type": "string"},
							"memory_layer":    map[string]any{"type": "string", "enum": []string{"", "instant", "short", "long", "permanent"}},
							"memory_reason":   map[string]any{"type": "string"},
						},
						"required": []string{
							"situation",
							"next_action",
							"reason",
							"command",
							"message",
							"ticket_title",
							"ticket_body",
							"ticket_priority",
							"memory_id",
							"memory_action",
							"memory_summary",
							"memory_text",
							"memory_layer",
							"memory_reason",
						},
						"additionalProperties": false,
					},
				}},
				Stream: true,
				Store:  false,
			},
		},
		{
			name: "memory_runtime_style",
			payload: requestPayload{
				Model:          model,
				Instructions:   "You are a memory routing judge. Decide layer/action only. Use the provided function tool. Do not produce free text.",
				PromptCacheKey: "arena-fc-boundary-memory-style-v1",
				Input:          []message{{Role: "user", Content: "Route one candidate memory."}},
				Tools: []responseTool{{
					Type:        "function",
					Name:        "route_memory_layer",
					Description: "Decide which memory layer and lifecycle action this memory should take.",
					Strict:      true,
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"target_layer":  map[string]any{"type": "string", "enum": []string{"instant", "short", "long", "permanent"}},
							"action":        map[string]any{"type": "string", "enum": []string{"create", "update", "promote", "decay", "review", "delete"}},
							"reason_codes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"review_after":  map[string]any{"type": []string{"string", "null"}},
							"expires_after": map[string]any{"type": []string{"string", "null"}},
						},
						"required":             []string{"target_layer", "action", "reason_codes", "review_after", "expires_after"},
						"additionalProperties": false,
					},
				}},
				ToolChoice:        functionToolChoice{Type: "function", Name: "route_memory_layer"},
				ParallelToolCalls: &ptc,
				Stream:            true,
				Store:             false,
			},
		},
	}
}

func postStream(client *http.Client, baseURL, apiKey string, payload requestPayload, verbose bool) (streamResult, error) {
	payload.Stream = true
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
		case "response.output_item.added", "response.output_item.done":
			if evt.Item.Type == "function_call" {
				mergeFunctionCall(&result, evt.Item)
			}
		case "response.function_call_arguments.delta":
			if evt.ItemID != "" && evt.Delta != "" {
				appendFunctionCallArguments(&result, evt.ItemID, evt.Delta)
			}
		case "response.function_call_arguments.done":
			if evt.ItemID != "" && evt.Arguments != "" {
				setFunctionCallArguments(&result, evt.ItemID, evt.Arguments)
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
	if result.ResponseID == "" {
		return streamResult{}, errors.New("stream ended without response.completed/response.done event")
	}
	return result, nil
}

func mergeFunctionCall(result *streamResult, item responseItem) {
	for i := range result.FunctionCalls {
		if result.FunctionCalls[i].ID != "" && result.FunctionCalls[i].ID == item.ID {
			result.FunctionCalls[i] = mergeFunctionItem(result.FunctionCalls[i], item)
			return
		}
	}
	result.FunctionCalls = append(result.FunctionCalls, item)
}

func mergeFunctionItem(dst, src responseItem) responseItem {
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.CallName == "" {
		dst.CallName = src.CallName
	}
	if dst.CallID == "" {
		dst.CallID = src.CallID
	}
	if dst.ID == "" {
		dst.ID = src.ID
	}
	if dst.Status == "" {
		dst.Status = src.Status
	}
	if dst.Arguments == "" {
		dst.Arguments = src.Arguments
	}
	return dst
}

func appendFunctionCallArguments(result *streamResult, itemID, delta string) {
	for i := range result.FunctionCalls {
		if result.FunctionCalls[i].ID == itemID {
			result.FunctionCalls[i].Arguments += delta
			return
		}
	}
	result.FunctionCalls = append(result.FunctionCalls, responseItem{Type: "function_call", ID: itemID, Arguments: delta})
}

func setFunctionCallArguments(result *streamResult, itemID, arguments string) {
	for i := range result.FunctionCalls {
		if result.FunctionCalls[i].ID == itemID {
			result.FunctionCalls[i].Arguments = arguments
			return
		}
	}
	result.FunctionCalls = append(result.FunctionCalls, responseItem{Type: "function_call", ID: itemID, Arguments: arguments})
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
	if strings.TrimSpace(dst.OutputText) == "" {
		dst.OutputText = strings.TrimSpace(resp.OutputText)
	}
	if len(dst.FunctionCalls) == 0 {
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				dst.FunctionCalls = append(dst.FunctionCalls, item)
			}
		}
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
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
