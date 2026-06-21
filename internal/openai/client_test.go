package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPostStreamReturnsRetryableAPIErrorAfter429Exhausted(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Too many pending requests"}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	_, err := PostStream(client, "http://example.invalid", "key", RequestPayload{
		Model:        "test-model",
		Instructions: "test",
		Input:        []Message{{Role: "user", Content: "hello"}},
	}, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 5 {
		t.Fatalf("expected 5 attempts, got %d", attempts)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T %[1]v", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests || !apiErr.Retryable {
		t.Fatalf("unexpected api error: %#v", apiErr)
	}
	if !IsRetryableError(err) {
		t.Fatalf("expected retryable classification for %v", err)
	}
}

func TestPostStreamWithFailoverUsesBackupAfterRetryablePrimaryFailure(t *testing.T) {
	attemptsByHost := map[string]int{}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attemptsByHost[req.URL.Host]++
		if req.URL.Host == "primary.invalid" {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporarily unavailable"}}`)),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(renderCompletedEvent(t, map[string]any{
				"id": "resp-backup",
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 2,
				},
				"output_text": "ok",
			}))),
			Header: make(http.Header),
		}, nil
	})}

	result, err := PostStreamWithFailover(client, []Endpoint{
		{Name: "primary", BaseURL: "http://primary.invalid", APIKey: "primary-key"},
		{Name: "backup_1", BaseURL: "http://backup.invalid", APIKey: "backup-key"},
	}, RequestPayload{
		Model:        "test-model",
		Instructions: "test",
		Input:        []Message{{Role: "user", Content: "hello"}},
	}, false)
	if err != nil {
		t.Fatalf("failover request: %v", err)
	}
	if result.EndpointName != "backup_1" || result.ResponseID != "resp-backup" {
		t.Fatalf("unexpected failover result: %#v", result)
	}
	if attemptsByHost["primary.invalid"] != 5 || attemptsByHost["backup.invalid"] != 1 {
		t.Fatalf("unexpected attempts by host: %#v", attemptsByHost)
	}
}

func TestPostStreamWithFailoverPreservesRetryableClassificationWhenAllEndpointsFail(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporarily unavailable"}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	_, err := PostStreamWithFailover(client, []Endpoint{
		{Name: "primary", BaseURL: "http://primary.invalid", APIKey: "primary-key"},
		{Name: "backup_1", BaseURL: "http://backup.invalid", APIKey: "backup-key"},
	}, RequestPayload{
		Model:        "test-model",
		Instructions: "test",
		Input:        []Message{{Role: "user", Content: "hello"}},
	}, false)
	if err == nil {
		t.Fatal("expected failover error")
	}
	var failoverErr *FailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("expected FailoverError, got %T %[1]v", err)
	}
	if len(failoverErr.Failures) != 2 || !IsRetryableError(err) {
		t.Fatalf("unexpected failover classification: %#v", failoverErr)
	}
}

func TestParseSSECapturesFunctionCallNameAndCallName(t *testing.T) {
	result, err := ParseSSE(strings.NewReader(renderCompletedEvent(t, map[string]any{
		"id": "resp-tool",
		"usage": map[string]any{
			"input_tokens":  12,
			"output_tokens": 4,
		},
		"output": []map[string]any{
			{
				"type":      "function_call",
				"call_name": "guest_exec",
				"call_id":   "call_1",
				"arguments": `{"situation":"Need a probe.","reason":"Observe.","command":"whoami"}`,
			},
		},
	})), false)
	if err != nil {
		t.Fatalf("parse sse: %v", err)
	}
	if len(result.FunctionCalls) != 1 {
		t.Fatalf("expected one function call, got %#v", result.FunctionCalls)
	}
	call := result.FunctionCalls[0]
	if call.CallName != "guest_exec" || call.Arguments == "" {
		t.Fatalf("unexpected function call: %#v", call)
	}
}

func renderCompletedEvent(t *testing.T, response map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": response,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return fmt.Sprintf("data: %s\n\n", raw)
}
