package main

import (
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtimeguard"
)

func TestAdmitArgsHasCustomUsage(t *testing.T) {
	if defaultAdmitArgs().hasCustomUsage() {
		t.Fatalf("expected empty admit args to be non-custom")
	}
	args := defaultAdmitArgs()
	args.inputTokens = 0
	if !args.hasCustomUsage() {
		t.Fatalf("expected explicit input token override to be custom")
	}
	args = defaultAdmitArgs()
	args.responseID = "resp"
	if !args.hasCustomUsage() {
		t.Fatalf("expected response id override to be custom")
	}
}

func TestRunAdmitRejectsInvalidCachedTokens(t *testing.T) {
	app := broker.New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("amber", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	args := defaultAdmitArgs()
	args.inputTokens = 100
	args.cachedTokens = 101
	args.outputTokens = 50
	args.totalTokens = 150
	_, err := runAdmit(app, "amber", runtimeguard.CallKindWork, true, now, args)
	if err == nil {
		t.Fatalf("expected invalid cached token combination to fail")
	}
}
