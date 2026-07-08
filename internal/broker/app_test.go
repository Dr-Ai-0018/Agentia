package broker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/brokerstate"
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

func TestRunOrchestratorBudgetEstimateUsesHistoricalSpark(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260621T025415.332200526Z")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	summary := `{
  "run_id": "orchestrator-20260621T025415.332200526Z",
  "runs": [
    {
      "resident": "amber",
      "report": {
        "resident": "amber",
        "model": "gpt-5.5",
        "started_at": "2026-06-21T02:54:15Z",
        "ended_at": "2026-06-21T02:56:15Z",
        "rounds": 3,
        "round_logs": [
          {"broker": {"applied": true, "spark_delta": -2.5}},
          {"broker": {"applied": true, "spark_delta": -3.0}},
          {"broker": {"applied": true, "spark_delta": -3.5}}
        ]
      }
    },
    {
      "resident": "jade",
      "report": {
        "resident": "jade",
        "model": "gpt-5.4",
        "started_at": "2026-06-21T02:54:15Z",
        "ended_at": "2026-06-21T02:55:15Z",
        "rounds": 2,
        "round_logs": [
          {"broker": {"applied": true, "spark_delta": -1.0}},
          {"broker": {"applied": true, "spark_delta": -1.2}}
        ]
      }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	out, err := New(root).RunOrchestratorBudgetEstimate(8)
	if err != nil {
		t.Fatalf("budget estimate: %v", err)
	}
	if out.GeneratedFromRuns != 1 {
		t.Fatalf("unexpected generated runs: %#v", out)
	}
	if len(out.Residents) != 2 {
		t.Fatalf("expected two resident estimates: %#v", out.Residents)
	}
	amber := findBudgetEstimate(out, "amber")
	if amber == nil {
		t.Fatalf("missing amber estimate: %#v", out.Residents)
	}
	if amber.SoakSparkRecommended <= amber.ProbeSparkRecommended {
		t.Fatalf("expected soak budget to exceed probe budget: %#v", amber)
	}
	if amber.ProbeSparkRecommended <= defaultMinimumProbeSpark {
		t.Fatalf("expected amber probe budget to use history, got %#v", amber)
	}
	if len(out.Recommended.SoakAllowanceByResident) != 2 {
		t.Fatalf("expected per-resident soak commands, got %#v", out.Recommended)
	}
	if !strings.Contains(out.Recommended.SoakAllowanceByResident[0], "test-allowance-card") || !strings.Contains(out.Recommended.SoakRun, "--duration 10m") {
		t.Fatalf("expected recommended commands, got %#v", out.Recommended)
	}
	if !strings.Contains(strings.Join(out.Notes, "\n"), "must not be used as evidence for ultra_long_soak_pre_release") {
		t.Fatalf("expected no-admin ultra-long boundary note, got %#v", out.Notes)
	}
	if strings.Contains(out.Recommended.SoakAllowanceByResident[0], "jade,amber") {
		t.Fatalf("allowance command must be per-resident, got %#v", out.Recommended.SoakAllowanceByResident)
	}
}

func findBudgetEstimate(out OrchestratorBudgetEstimateOutput, resident string) *ResidentBudgetEstimate {
	for i := range out.Residents {
		if out.Residents[i].Resident == resident {
			return &out.Residents[i]
		}
	}
	return nil
}

func TestRunBudgetStatusSummarizesResidents(t *testing.T) {
	app := New(t.TempDir())
	if _, err := app.RunReset("amber", time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("reset amber: %v", err)
	}
	if _, err := app.RunReset("jade", time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("reset jade: %v", err)
	}

	out, err := app.RunBudgetStatus([]string{"amber", "jade"})
	if err != nil {
		t.Fatalf("budget status: %v", err)
	}
	if out.ResidentCount != 2 || len(out.Residents) != 2 {
		t.Fatalf("expected two residents: %#v", out)
	}
	if out.Totals.SparkBalance != 12.5 {
		t.Fatalf("unexpected total spark: %#v", out.Totals)
	}
	if out.Totals.WorkAllowedCount != 2 || out.Totals.BlockedCount != 0 {
		t.Fatalf("unexpected allowed/blocked counts: %#v", out.Totals)
	}
	for _, resident := range out.Residents {
		if resident.Fatigue.Mood == "" {
			t.Fatalf("expected fatigue status for %s: %#v", resident.ResidentID, resident)
		}
		if resident.Sleep.Depth == "" {
			t.Fatalf("expected sleep status for %s: %#v", resident.ResidentID, resident)
		}
	}
}

func TestRunBudgetStatusIncludesSixHourBurnSamples(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	if _, err := app.RunReset("amber", time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("reset amber: %v", err)
	}
	store := brokerstate.New(filepath.Join(root, "brokerstate"))
	now := time.Now().UTC()
	for _, event := range []brokerstate.QuotaEvent{
		{ResidentID: "amber", Kind: brokerstate.QuotaEventWorkCall, StrainCost: 12, CreatedAt: now.Add(-5*time.Hour - 45*time.Minute)},
		{ResidentID: "amber", Kind: brokerstate.QuotaEventAcceptanceCall, StrainCost: 18, CreatedAt: now.Add(-10 * time.Minute)},
		{ResidentID: "amber", Kind: brokerstate.QuotaEventTestAllowance, StrainCost: 999, CreatedAt: now.Add(-5 * time.Minute)},
	} {
		if _, err := store.AppendQuotaEvent(event); err != nil {
			t.Fatalf("append quota event: %v", err)
		}
	}

	out, err := app.RunBudgetStatus([]string{"amber"})
	if err != nil {
		t.Fatalf("budget status: %v", err)
	}
	if len(out.Residents) != 1 {
		t.Fatalf("expected one resident: %#v", out)
	}
	burn := out.Residents[0].SixHourBurn
	if len(burn) != brokerstate.SixHourBurnSampleCount {
		t.Fatalf("burn sample count = %d, want %d: %v", len(burn), brokerstate.SixHourBurnSampleCount, burn)
	}
	total := 0
	for _, sample := range burn {
		total += sample
	}
	if total != 30 {
		t.Fatalf("burn sample total = %d, want 30: %v", total, burn)
	}
}

func TestRunOrchestratorBudgetReportSummarizesRunSpend(t *testing.T) {
	root := t.TempDir()
	runID := "orchestrator-20260621T093703.615042509Z"
	runDir := filepath.Join(root, "orchestrator-runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	summary := `{
  "run_id": "orchestrator-20260621T093703.615042509Z",
  "runs": [
    {
      "resident": "amber",
      "report": {
        "resident": "amber",
        "model": "gpt-5.5",
        "rounds": 2,
        "acceptance_broker": {"applied": true, "spark_delta": -0.5},
        "round_logs": [
          {"input_tokens": 100, "cached_tokens": 0, "output_tokens": 10, "broker": {"applied": true, "spark_delta": -2.5}},
          {"input_tokens": 200, "cached_tokens": 50, "output_tokens": 20, "broker": {"applied": true, "spark_delta": -3.0}}
        ]
      }
    },
    {
      "resident": "jade",
      "report": {
        "resident": "jade",
        "model": "gpt-5.4",
        "rounds": 1,
        "acceptance_broker": {"applied": true, "spark_delta": -0.25},
        "round_logs": [
          {"input_tokens": 80, "cached_tokens": 0, "output_tokens": 8, "broker": {"applied": true, "spark_delta": -1.25}}
        ]
      }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	out, err := New(root).RunOrchestratorBudgetReport(runID, map[string]float64{"amber": 10, "jade": 5})
	if err != nil {
		t.Fatalf("budget report: %v", err)
	}
	if out.ResidentCount != 2 {
		t.Fatalf("unexpected resident count: %#v", out)
	}
	if out.Totals.SpentSpark != 7.5 || out.InternalUSD != 0.075 {
		t.Fatalf("unexpected totals: %#v", out)
	}
	amber := findBudgetReport(out, "amber")
	if amber == nil {
		t.Fatalf("missing amber: %#v", out.Residents)
	}
	if amber.SpentSpark != 6.0 || amber.TotalTokens != 330 || amber.CacheHitRatio != 0.1667 || amber.AllowanceUsedRatio != 0.6 {
		t.Fatalf("unexpected amber report: %#v", amber)
	}
	if out.Totals.CacheHitRatio != 0.1316 {
		t.Fatalf("unexpected total cache hit ratio: %#v", out.Totals)
	}
	if out.CacheHealth != "poor" || len(out.Warnings) == 0 {
		t.Fatalf("expected poor cache health warning: %#v", out)
	}
}

func findBudgetReport(out OrchestratorBudgetReportOutput, resident string) *ResidentOrchestratorBudgetReport {
	for i := range out.Residents {
		if out.Residents[i].ResidentID == resident {
			return &out.Residents[i]
		}
	}
	return nil
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
			Model:        "gpt-5.4",
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
	if resp.ApplyResult.SparkEntry.Reason != "work call via gpt-5.4" {
		t.Fatalf("unexpected model in spark entry reason: %s", resp.ApplyResult.SparkEntry.Reason)
	}
	if resp.Prepared.Usage.ResponseID != "resp_custom" {
		t.Fatalf("unexpected response id: %s", resp.Prepared.Usage.ResponseID)
	}
}

func TestAppRunTestAllowanceCardBatch(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	out, err := app.RunTestAllowanceCard([]string{"jade", "amber"}, 2.5, 100, 200, 300, true, true, true, "temporary_test_allowance", "tester", now)
	if err != nil {
		t.Fatalf("run test allowance card: %v", err)
	}
	if out.ResidentCount != 2 || len(out.Residents) != 2 {
		t.Fatalf("unexpected resident count: %#v", out)
	}
	if len(out.RevertNotes) != 2 {
		t.Fatalf("expected revert notes: %#v", out.RevertNotes)
	}
	if out.Residents[0].AfterStatus.SparkBalance <= out.Residents[0].BeforeStatus.SparkBalance {
		t.Fatalf("expected spark balance boost")
	}
	if out.Residents[1].AfterStatus.Window6HCap != out.Residents[1].BeforeStatus.Window6HCap+100 {
		t.Fatalf("expected quota boost")
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

func TestAppRunQuotaGrant(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunQuotaGrant("jade", 5000, 10000, 20000, "test long run grant")
	if err != nil {
		t.Fatalf("quota grant: %v", err)
	}
	if out.BeforeStatus.Window6HCap != 12000 {
		t.Fatalf("unexpected before quota: %#v", out.BeforeStatus)
	}
	if out.AfterStatus.Window6HCap != 17000 {
		t.Fatalf("unexpected after 6h cap: %#v", out.AfterStatus)
	}
	if out.AfterStatus.DayCap != out.BeforeStatus.DayCap+10000 {
		t.Fatalf("unexpected after day cap: %#v", out.AfterStatus)
	}
	if out.AfterStatus.WeekCap != out.BeforeStatus.WeekCap+20000 {
		t.Fatalf("unexpected after week cap: %#v", out.AfterStatus)
	}
	if out.Reason != "test long run grant" {
		t.Fatalf("expected reason to be carried, got %q", out.Reason)
	}
}

func TestAppRunQuotaGrantRejectsNoop(t *testing.T) {
	app := New(t.TempDir())
	if _, err := app.RunQuotaGrant("jade", 0, 0, 0, "noop"); err == nil {
		t.Fatalf("expected no-op grant to fail")
	}
}

func TestAppRunSparkGrant(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunSparkGrant("jade", 12.5, "test long run spark grant")
	if err != nil {
		t.Fatalf("spark grant: %v", err)
	}
	if out.BeforeStatus.SparkBalance != 4.5 {
		t.Fatalf("unexpected before spark: %#v", out.BeforeStatus)
	}
	if out.AfterStatus.SparkBalance != 17 {
		t.Fatalf("unexpected after spark: %#v", out.AfterStatus)
	}
	if out.Entry.Kind != "grant" {
		t.Fatalf("unexpected entry kind: %#v", out.Entry)
	}
	if out.Reason != "test long run spark grant" {
		t.Fatalf("expected reason to be carried, got %q", out.Reason)
	}
}

func TestAppRunSparkGrantRejectsNonPositive(t *testing.T) {
	app := New(t.TempDir())
	if _, err := app.RunSparkGrant("jade", 0, "noop"); err == nil {
		t.Fatalf("expected zero spark grant to fail")
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

func TestAppRunQuotaRecoversToNowBeforeSnapshot(t *testing.T) {
	app := New(t.TempDir())
	now := time.Now().UTC().Add(-30 * time.Minute)

	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset: %v", err)
	}

	out, err := app.RunQuota("jade")
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if out.Status.LastRecoveryAt.Before(now.Add(20 * time.Minute)) {
		t.Fatalf("expected quota query to recover toward now, last_recovery_at=%s reset_at=%s", out.Status.LastRecoveryAt, now)
	}
	if out.Quota.NextRecoveryAt == "" || out.Quota.RecoveryTickMinutes == 0 {
		t.Fatalf("expected quota recovery timing in snapshot: %#v", out.Quota)
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

func TestAppRunRecoverAllToNow(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	for _, resident := range []string{"jade", "amber", "onyx"} {
		if _, err := app.RunReset(resident, now); err != nil {
			t.Fatalf("reset %s: %v", resident, err)
		}
	}

	out, err := app.RunRecoverAllToNow(now.Add(45*time.Minute), "rest")
	if err != nil {
		t.Fatalf("recover all: %v", err)
	}
	if out.ResidentCount != 3 || len(out.Residents) != 3 {
		t.Fatalf("expected three recovered residents, got %#v", out)
	}
	if out.RecoveryMode != "rest" {
		t.Fatalf("expected rest recovery mode, got %q", out.RecoveryMode)
	}
	if out.WorkAllowedCount != 3 || out.BlockedCount != 0 {
		t.Fatalf("expected all residents work-allowed after baseline recovery, got %#v", out)
	}
	for _, item := range out.Residents {
		if item.Recovery.RecoveryMode != "rest" {
			t.Fatalf("expected rest tick for %s, got %#v", item.ResidentID, item.Recovery)
		}
		if item.AfterWindow6HUsed > item.BeforeWindow6HUsed {
			t.Fatalf("expected recovery not to increase 6h usage for %s: %#v", item.ResidentID, item)
		}
	}
}
