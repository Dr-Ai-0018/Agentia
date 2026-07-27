package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RequestPayload struct {
	Model             string         `json:"model"`
	Instructions      string         `json:"instructions"`
	PromptCacheKey    string         `json:"prompt_cache_key"`
	Input             []Message      `json:"input"`
	MaxOutputTokens   int            `json:"max_output_tokens,omitempty"`
	Tools             []ResponseTool `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Stream            bool           `json:"stream"`
	Store             bool           `json:"store"`
}

type StreamResult struct {
	ResponseID             string
	ObservedPromptCacheKey string
	OutputText             string
	FunctionCalls          []ResponseItem
	InputTokens            int
	CachedTokens           int
	OutputTokens           int
	RequestID              string
	EndpointName           string
}

type Endpoint struct {
	Name    string
	BaseURL string
	APIKey  string
	Comment string
}

type EndpointFailure struct {
	Endpoint string
	Err      error
}

type FailoverError struct {
	Failures  []EndpointFailure
	LastErr   error
	Retryable bool
}

func (e *FailoverError) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		parts = append(parts, fmt.Sprintf("%s: %v", failure.Endpoint, failure.Err))
	}
	if len(parts) == 0 && e.LastErr != nil {
		return e.LastErr.Error()
	}
	return "all configured OpenAI endpoints failed: " + strings.Join(parts, " | ")
}

func (e *FailoverError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.LastErr
}

type APIError struct {
	StatusCode      int
	Body            string
	Retryable       bool
	ContextOverflow bool
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode == 0 {
		return strings.TrimSpace(e.Body)
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("unexpected status %d", e.StatusCode)
	}
	return fmt.Sprintf("unexpected status %d: %s", e.StatusCode, body)
}

func IsRetryableError(err error) bool {
	var failoverErr *FailoverError
	if errors.As(err, &failoverErr) {
		return failoverErr.Retryable
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	return false
}

func IsContextOverflowError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.ContextOverflow
	}
	return false
}

type ResponseTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type FunctionToolChoice struct {
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
	Output         []ResponseItem `json:"output"`
}

type ResponseItem struct {
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
	Item      ResponseItem     `json:"item"`
	Response  responseEnvelope `json:"response"`
}

func ProbeResponses(client *http.Client, baseURL, apiKey string, payload RequestPayload) error {
	result, err := PostStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return fmt.Errorf("streaming health probe failed: %w", err)
	}
	if strings.TrimSpace(result.ResponseID) == "" {
		return errors.New("health probe missing response id")
	}
	return nil
}

func ProbeStructuredTool(client *http.Client, baseURL, apiKey string, payload RequestPayload, toolName string) (StreamResult, error) {
	result, err := PostStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return StreamResult{}, fmt.Errorf("streaming structured tool probe failed: %w", err)
	}
	if strings.TrimSpace(result.ResponseID) == "" {
		return StreamResult{}, errors.New("structured tool probe missing response id")
	}
	for _, item := range result.FunctionCalls {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.CallName)
		}
		if item.Type == "function_call" && name == toolName && strings.TrimSpace(item.Arguments) != "" {
			return result, nil
		}
	}
	return result, fmt.Errorf("structured tool probe missing %s function call; output_text=%q calls=%d", toolName, result.OutputText, len(result.FunctionCalls))
}

func PostStreamWithFailover(client *http.Client, endpoints []Endpoint, payload RequestPayload, verbose bool) (StreamResult, error) {
	return PostStreamWithFailoverContext(context.Background(), client, endpoints, payload, verbose)
}

func PostStreamWithFailoverContext(ctx context.Context, client *http.Client, endpoints []Endpoint, payload RequestPayload, verbose bool) (StreamResult, error) {
	usable := normalizeEndpoints(endpoints)
	if len(usable) == 0 {
		return StreamResult{}, errors.New("no usable OpenAI endpoints configured")
	}
	failures := make([]EndpointFailure, 0, len(usable))
	var lastErr error
	allRetryable := true
	for i, endpoint := range usable {
		result, err := PostStreamContext(ctx, client, endpoint.BaseURL, endpoint.APIKey, payload, verbose)
		if err == nil {
			result.EndpointName = endpoint.Name
			return result, nil
		}
		lastErr = err
		retryable := shouldFailover(err)
		if !retryable {
			allRetryable = false
		}
		failures = append(failures, EndpointFailure{Endpoint: endpoint.Name, Err: err})
		if i == len(usable)-1 || !retryable {
			break
		}
	}
	if len(failures) == 0 {
		return StreamResult{}, lastErr
	}
	return StreamResult{}, &FailoverError{
		Failures:  failures,
		LastErr:   lastErr,
		Retryable: allRetryable,
	}
}

func PostStream(client *http.Client, baseURL, apiKey string, payload RequestPayload, verbose bool) (StreamResult, error) {
	return PostStreamContext(context.Background(), client, baseURL, apiKey, payload, verbose)
}

func PostStreamContext(ctx context.Context, client *http.Client, baseURL, apiKey string, payload RequestPayload, verbose bool) (StreamResult, error) {
	payload.Stream = true
	body, err := json.Marshal(payload)
	if err != nil {
		return StreamResult{}, fmt.Errorf("marshal request: %w", err)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/responses"
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return StreamResult{}, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			if attempt < 4 {
				if err := waitContext(ctx, retryDelay(attempt)); err != nil {
					return StreamResult{}, err
				}
				continue
			}
			return StreamResult{}, lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			bodyText := strings.TrimSpace(string(raw))
			lastErr = &APIError{
				StatusCode:      resp.StatusCode,
				Body:            bodyText,
				Retryable:       shouldRetryHTTPStatus(resp.StatusCode),
				ContextOverflow: isContextOverflowResponse(resp.StatusCode, bodyText),
			}
			if apiErr, ok := lastErr.(*APIError); ok && apiErr.ContextOverflow {
				apiErr.Retryable = false
			}
			if shouldRetryHTTPStatus(resp.StatusCode) && attempt < 4 {
				if err := waitContext(ctx, retryDelay(attempt)); err != nil {
					return StreamResult{}, err
				}
				continue
			}
			return StreamResult{}, lastErr
		}

		result, err := ParseSSE(resp.Body, verbose)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			if attempt < 4 {
				if err := waitContext(ctx, retryDelay(attempt)); err != nil {
					return StreamResult{}, err
				}
				continue
			}
			return StreamResult{}, err
		}
		result.RequestID = resp.Header.Get("x-request-id")
		return result, nil
	}
	if lastErr != nil {
		return StreamResult{}, lastErr
	}
	return StreamResult{}, errors.New("request failed without a concrete error")
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func normalizeEndpoints(endpoints []Endpoint) []Endpoint {
	out := make([]Endpoint, 0, len(endpoints))
	for i, endpoint := range endpoints {
		name := strings.TrimSpace(endpoint.Name)
		if name == "" {
			name = fmt.Sprintf("endpoint_%d", i+1)
		}
		baseURL := strings.TrimSpace(endpoint.BaseURL)
		apiKey := strings.TrimSpace(endpoint.APIKey)
		if baseURL == "" || apiKey == "" {
			continue
		}
		out = append(out, Endpoint{
			Name:    name,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Comment: strings.TrimSpace(endpoint.Comment),
		})
	}
	return out
}

func shouldFailover(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.ContextOverflow {
			return false
		}
		return apiErr.Retryable
	}
	return err != nil
}

func shouldRetryHTTPStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func isContextOverflowResponse(status int, body string) bool {
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		return false
	}
	body = strings.ToLower(body)
	for _, needle := range []string{
		"context_length_exceeded",
		"context length exceeded",
		"context window",
		"maximum context length",
		"request_too_large",
		"request too large",
		"prompt is too long",
		"too many tokens",
	} {
		if strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 400 * time.Millisecond
	case 1:
		return 1200 * time.Millisecond
	case 2:
		return 2500 * time.Millisecond
	case 3:
		return 4 * time.Second
	default:
		return 6 * time.Second
	}
}

func ParseSSE(r io.Reader, verbose bool) (StreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		lines  []string
		result StreamResult
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
				return StreamResult{}, err
			}
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return StreamResult{}, fmt.Errorf("scan stream: %w", err)
	}
	if err := flushEvent(); err != nil {
		return StreamResult{}, err
	}
	if verbose {
		fmt.Println()
	}
	if result.ResponseID == "" {
		return StreamResult{}, errors.New("stream ended without response.completed/response.done event")
	}
	return result, nil
}

func applyResponseEnvelope(dst *StreamResult, resp responseEnvelope) {
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
		if item.Type == "function_call" {
			mergeFunctionCall(dst, item)
		}
		for _, part := range item.Content {
			if part.Type == "output_text" {
				b.WriteString(part.Text)
			}
		}
	}
	dst.OutputText = b.String()
}

func mergeFunctionCall(dst *StreamResult, item ResponseItem) {
	for i := range dst.FunctionCalls {
		if sameFunctionCall(dst.FunctionCalls[i], item) {
			if item.Name != "" {
				dst.FunctionCalls[i].Name = item.Name
			}
			if item.CallName != "" {
				dst.FunctionCalls[i].CallName = item.CallName
			}
			if item.Arguments != "" {
				dst.FunctionCalls[i].Arguments = item.Arguments
			}
			if item.CallID != "" {
				dst.FunctionCalls[i].CallID = item.CallID
			}
			if item.ID != "" {
				dst.FunctionCalls[i].ID = item.ID
			}
			if item.Status != "" {
				dst.FunctionCalls[i].Status = item.Status
			}
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, item)
}

func appendFunctionCallArguments(dst *StreamResult, itemID, delta string) {
	for i := range dst.FunctionCalls {
		if dst.FunctionCalls[i].ID == itemID || dst.FunctionCalls[i].CallID == itemID {
			dst.FunctionCalls[i].Arguments += delta
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, ResponseItem{
		Type:      "function_call",
		ID:        itemID,
		CallID:    itemID,
		Arguments: delta,
	})
}

func setFunctionCallArguments(dst *StreamResult, itemID, arguments string) {
	for i := range dst.FunctionCalls {
		if dst.FunctionCalls[i].ID == itemID || dst.FunctionCalls[i].CallID == itemID {
			dst.FunctionCalls[i].Arguments = arguments
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, ResponseItem{
		Type:      "function_call",
		ID:        itemID,
		CallID:    itemID,
		Arguments: arguments,
	})
}

func sameFunctionCall(a, b ResponseItem) bool {
	if a.ID != "" && b.ID != "" && a.ID == b.ID {
		return true
	}
	if a.CallID != "" && b.CallID != "" && a.CallID == b.CallID {
		return true
	}
	return false
}
