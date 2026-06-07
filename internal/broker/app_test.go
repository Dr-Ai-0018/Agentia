package broker

import (
	"testing"
	"time"

	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

func TestAppResetStatusAndAdmitFlow(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	reset, err := app.RunReset("amber", now)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset.Status.ResidentID != "amber" {
		t.Fatalf("unexpected resident id after reset")
	}
	if reset.Status.SparkBalance != 8.0 {
		t.Fatalf("unexpected reset spark balance: %v", reset.Status.SparkBalance)
	}

	status, err := app.RunStatus("amber")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.SparkBalance != 8.0 {
		t.Fatalf("unexpected persisted spark balance: %v", status.SparkBalance)
	}

	admit, err := app.RunAdmit("amber", runtimeguard.CallKindWork, true, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !admit.Applied {
		t.Fatalf("expected admit apply success")
	}
	if admit.AfterStatus == nil {
		t.Fatalf("expected after status")
	}
	if admit.AfterStatus.SparkBalance >= admit.BeforeStatus.SparkBalance {
		t.Fatalf("expected spark balance to decrease")
	}
}

func TestAppFinalNoticeDebtAndRecovery(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	reset, err := app.RunReset("onyx", now)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset.Status.SparkBalance != 3.0 {
		t.Fatalf("unexpected reset spark balance")
	}

	admit, err := app.RunAdmit("onyx", runtimeguard.CallKindFinalNotice, true, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("final notice admit: %v", err)
	}
	if !admit.Applied {
		t.Fatalf("expected final notice to apply")
	}
	if admit.AfterStatus == nil {
		t.Fatalf("expected after status")
	}
	if !admit.AfterStatus.FinalNoticeUsed {
		t.Fatalf("expected final notice flag")
	}
	beforeDebt := admit.AfterStatus.DebtAmount

	recoveryOut, err := app.RunRecover("onyx", 1, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recoveryOut.Status.Window6HUsed > admit.AfterStatus.Window6HUsed {
		t.Fatalf("expected recovery to reduce 6h usage pressure")
	}
	if recoveryOut.Status.DebtAmount > beforeDebt {
		t.Fatalf("expected recovery not to increase debt")
	}
}

func TestAppRunAdmitSpecUsesProvidedUsage(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("amber", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	spec := CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  100,
			CachedTokens: 0,
			OutputTokens: 50,
			TotalTokens:  150,
			Model:        "gpt-5.4-mini",
			ResponseID:   "resp_custom",
			StartedAt:    now.Add(time.Minute),
			FinishedAt:   now.Add(time.Minute + 2*time.Second),
		},
		Penalties: tokenledger.Penalties{},
		Activity:  tokenledger.ActivityNormalWork,
	}

	resp, err := app.RunAdmitSpec("amber", spec, true)
	if err != nil {
		t.Fatalf("run admit spec: %v", err)
	}
	if !resp.Applied {
		t.Fatalf("expected custom spec to apply")
	}
	if resp.ApplyResult == nil {
		t.Fatalf("expected apply result")
	}
	if resp.ApplyResult.SparkEntry.Reason != "work call via gpt-5.4-mini" {
		t.Fatalf("unexpected model in spark entry reason: %s", resp.ApplyResult.SparkEntry.Reason)
	}
	if resp.Prepared.Usage.ResponseID != "resp_custom" {
		t.Fatalf("unexpected response id: %s", resp.Prepared.Usage.ResponseID)
	}
}

func TestAppRunPrepareSpec(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	prepared, err := app.RunPrepareSpec("jade", CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  300,
			CachedTokens: 100,
			OutputTokens: 120,
			TotalTokens:  420,
			Model:        "gpt-5.4",
			ResponseID:   "resp_preflight",
			StartedAt:    now.Add(time.Minute),
			FinishedAt:   now.Add(time.Minute + 2*time.Second),
		},
		Penalties: tokenledger.Penalties{},
		Activity:  tokenledger.ActivityNormalWork,
	})
	if err != nil {
		t.Fatalf("prepare spec: %v", err)
	}
	if prepared.Denied {
		t.Fatalf("expected preflight spec to be allowed")
	}
	if prepared.Prepared.Usage.ResponseID != "resp_preflight" {
		t.Fatalf("unexpected prepared response id: %s", prepared.Prepared.Usage.ResponseID)
	}
}

func TestAppRunQuota(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunQuota("jade")
	if err != nil {
		t.Fatalf("run quota: %v", err)
	}
	if out.Quota.Window6HCap <= 0 {
		t.Fatalf("expected 6h cap")
	}
	if out.Quota.NextRecoveryAt == "" {
		t.Fatalf("expected next recovery at")
	}
	if out.Quota.RecoveryTickMinutes != 15 {
		t.Fatalf("expected 15 minute recovery tick")
	}
}

func TestAppRunRecoverToNow(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunRecoverToNow("jade", now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("recover to now: %v", err)
	}
	if out.Status.LastRecoveryAt.Before(now.Add(30 * time.Minute)) {
		t.Fatalf("expected last recovery at to advance to or beyond target time")
	}
	if out.Status.NextRecoveryAt == "" {
		t.Fatalf("expected next recovery timestamp")
	}
}

func TestAppRunRecoverWithMode(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunRecoverWithMode("jade", 1, now, "rest")
	if err != nil {
		t.Fatalf("recover with mode: %v", err)
	}
	if out.Recovery.RecoveryMode != "rest" {
		t.Fatalf("expected recovery mode rest, got %s", out.Recovery.RecoveryMode)
	}
	if out.Status.RecoveryMode != "rest" {
		t.Fatalf("expected status recovery mode rest, got %s", out.Status.RecoveryMode)
	}
}
