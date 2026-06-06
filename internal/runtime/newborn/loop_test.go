package newborn

import (
	"strings"
	"testing"
	"time"

	"ai-arena/internal/openai"
)

func TestBuildProfile(t *testing.T) {
	profile, err := BuildProfile("amber")
	if err != nil {
		t.Fatalf("build profile: %v", err)
	}
	if profile.Model != "gpt-5.5" {
		t.Fatalf("unexpected model: %s", profile.Model)
	}
}

func TestPreflightSpecBootstrap(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "amber", Model: "gpt-5.5"}, loopState{}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC))
	if spec.Usage.InputTokens != 1400 {
		t.Fatalf("unexpected bootstrap input: %d", spec.Usage.InputTokens)
	}
	if spec.Usage.OutputTokens != 350 {
		t.Fatalf("unexpected bootstrap output: %d", spec.Usage.OutputTokens)
	}
}

func TestPreflightSpecFromLastUsage(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, loopState{
		LastRealUsage: &openai.StreamResult{
			InputTokens:  1000,
			CachedTokens: 200,
			OutputTokens: 300,
		},
	}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC))
	if spec.Usage.InputTokens < 1000 {
		t.Fatalf("expected inflated input tokens")
	}
	if spec.Usage.CachedTokens != 200 {
		t.Fatalf("unexpected cached tokens: %d", spec.Usage.CachedTokens)
	}
	if spec.Usage.OutputTokens < 300 {
		t.Fatalf("expected inflated output tokens")
	}
}

func TestFallbackAcceptance(t *testing.T) {
	got := fallbackAcceptance(nil, "broker_preflight_denied")
	if got == "" {
		t.Fatalf("expected fallback acceptance text")
	}
}

func TestParseDecisionResultFromFunctionCall(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"fresh boot","next_action":"guest_exec","reason":"inspect first","command":"whoami","message":""}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "guest_exec" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "whoami" {
		t.Fatalf("unexpected command: %s", decision.Command)
	}
}

func TestCompactObservationForHistory(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 120; i++ {
		b.WriteString("line\n")
	}

	compacted := compactObservationForHistory(b.String())
	if !strings.Contains(compacted, "[observation truncated for context reuse:") {
		t.Fatalf("expected truncation marker, got %q", compacted)
	}
	if len(compacted) > maxObservationHistoryChars+200 {
		t.Fatalf("compacted observation too large: %d", len(compacted))
	}
}
