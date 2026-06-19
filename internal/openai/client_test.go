package openai

import (
	"errors"
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
