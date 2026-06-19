package main

import (
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
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

func TestSplitResidentIDs(t *testing.T) {
	got := splitResidentIDs(" jade,amber, , onyx ")
	if len(got) != 3 || got[0] != "jade" || got[1] != "amber" || got[2] != "onyx" {
		t.Fatalf("unexpected resident ids: %#v", got)
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

func TestRunAdmitAllowsOneWorkOverrunThenLocks(t *testing.T) {
	app := broker.New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("onyx", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	args := defaultAdmitArgs()
	args.model = "gpt-5.4"
	args.inputTokens = 100
	args.cachedTokens = 0
	args.outputTokens = 3_000_000
	args.totalTokens = 3_000_100
	args.toolCalls = 0
	args.responseID = "resp_work_overrun"

	raw, err := runAdmit(app, "onyx", runtimeguard.CallKindWork, true, now.Add(time.Minute), args)
	if err != nil {
		t.Fatalf("run admit: %v", err)
	}
	resp, ok := raw.(brokerstate.AdmitResponse)
	if !ok {
		t.Fatalf("unexpected response type %T", raw)
	}
	if resp.Denied || !resp.Applied {
		t.Fatalf("expected overrun work call to apply: %#v", resp)
	}
	if resp.AfterStatus == nil || !resp.AfterStatus.DebtActive {
		t.Fatalf("expected overrun work call to create debt")
	}
	if !resp.Prepared.Decision.WouldEnterDebt || !resp.Prepared.Decision.WouldExceedQuota {
		t.Fatalf("expected overrun decision flags: %#v", resp.Prepared.Decision)
	}

	next, err := app.RunAdmit("onyx", runtimeguard.CallKindWork, false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("next admit: %v", err)
	}
	if !next.Denied {
		t.Fatalf("expected debt to deny next ordinary work")
	}
	if len(next.DeniedReason) == 0 || next.DeniedReason[0] != "spark_debt_active" {
		t.Fatalf("unexpected next denied reason: %#v", next.DeniedReason)
	}
}
