package runtimecore

import (
	"testing"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

func TestFinalNoticeCreatesDebtAndRecoveryUnlocksLater(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:     0.2,
			StrainRecoveryPerHour:    100,
			DayRecoveryPerHour:       50,
			WeekRecoveryPerHour:      25,
			FatigueRecoveryPerHour:   180,
			SleepDebtRecoveryPerHour: 2,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}, "jade", tokenledger.QuotaState{
		Window6HCap:  4000,
		Window6HUsed: 300,
		DayCap:       20000,
		DayUsed:      1500,
		WeekCap:      150000,
		WeekUsed:     8000,
	}, start)

	_, err := engine.SparkLedger().Credit("grant", 0.62, "allowance", start)
	if err != nil {
		t.Fatalf("credit: %v", err)
	}

	prepared, err := engine.PrepareCall(runtimeguard.CallKindFinalNotice, tokenledger.Usage{
		InputTokens:  700,
		CachedTokens: 300,
		OutputTokens: 600,
		Model:        "gpt-5.4",
		FinishedAt:   start.Add(time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 1})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prepared.Decision.Allowed {
		t.Fatalf("final notice should be allowed")
	}

	applied, err := engine.ApplyCall(prepared, tokenledger.ActivityNormalWork)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied.State.DebtActive {
		t.Fatalf("expected debt active after final notice")
	}
	if applied.State.Fatigue <= 0 {
		t.Fatalf("expected fatigue to increase after work")
	}
	if applied.State.SleepDebt <= 0 {
		t.Fatalf("expected sleep debt to increase after work")
	}

	engine.TickRecovery(start.Add(2 * time.Hour))
	workPrepared, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  100,
		CachedTokens: 80,
		OutputTokens: 50,
		Model:        "gpt-5.4-mini",
		FinishedAt:   start.Add(2*time.Hour + time.Minute),
	}, tokenledger.Penalties{})
	if err != nil {
		t.Fatalf("prepare work: %v", err)
	}
	if !workPrepared.Decision.Allowed {
		t.Fatalf("work should be allowed after debt is cleared")
	}

	engine.TickRecovery(start.Add(3 * time.Hour))
	workPrepared, err = engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  100,
		CachedTokens: 80,
		OutputTokens: 50,
		Model:        "gpt-5.4-mini",
		FinishedAt:   start.Add(3*time.Hour + time.Minute),
	}, tokenledger.Penalties{})
	if err != nil {
		t.Fatalf("prepare work after 3h: %v", err)
	}
	if !workPrepared.Decision.Allowed {
		t.Fatalf("work should be allowed after sufficient recovery")
	}
}

func TestWorkCallCanEnterDebtAndLocksFurtherWork(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}, "amber", tokenledger.QuotaState{
		Window6HCap:  1000,
		Window6HUsed: 990,
		DayCap:       5000,
		WeekCap:      20000,
	}, start)

	_, err := engine.SparkLedger().Credit("grant", 0.001, "tight allowance", start)
	if err != nil {
		t.Fatalf("credit: %v", err)
	}

	prepared, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  2000,
		CachedTokens: 0,
		OutputTokens: 2000,
		Model:        "gpt-5.4",
		FinishedAt:   start.Add(time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 1})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prepared.Decision.Allowed || !prepared.Decision.AllowDebt || !prepared.Decision.LockAfterThisCall {
		t.Fatalf("expected work to be allowed into debt and then lock: %#v", prepared.Decision)
	}

	applied, err := engine.ApplyCall(prepared, tokenledger.ActivityNormalWork)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied.State.DebtActive {
		t.Fatalf("expected debt after over-budget work call")
	}
	if applied.State.DebtAmount <= 0 {
		t.Fatalf("expected positive debt amount")
	}

	nextPrepared, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  100,
		OutputTokens: 50,
		Model:        "gpt-5.4-mini",
		FinishedAt:   start.Add(2 * time.Minute),
	}, tokenledger.Penalties{})
	if err != nil {
		t.Fatalf("prepare next work: %v", err)
	}
	if nextPrepared.Decision.Allowed {
		t.Fatalf("expected debt to block ordinary work")
	}
}

func TestRecoveryPartiallyRepaysDebtInSparkLedger(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour: 0.2,
		},
	}, "jade", tokenledger.QuotaState{Window6HCap: 4000}, start)

	if _, err := engine.SparkLedger().DebitAllowDebt("charge", 1.0, "test debt", start); err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	engine.state.DebtActive = true
	engine.state.DebtAmount = 1.0

	tick := engine.TickRecovery(start.Add(time.Hour))
	if tick.NewSparkBalance != -0.8 {
		t.Fatalf("tick new balance = %.4f, want -0.8000", tick.NewSparkBalance)
	}
	if engine.SparkLedger().Account().Balance != tick.NewSparkBalance {
		t.Fatalf("ledger balance %.4f does not match tick %.4f", engine.SparkLedger().Account().Balance, tick.NewSparkBalance)
	}
	if engine.State().DebtAmount != 0.8 || !engine.State().DebtActive {
		t.Fatalf("unexpected debt state after partial recovery: %#v", engine.State())
	}
}

func TestReconcileSparkDebtClearsDebtAfterGrant(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{TokenPolicy: tokenledger.DefaultConfig()}, "jade", tokenledger.QuotaState{Window6HCap: 4000}, start)

	if _, err := engine.SparkLedger().DebitAllowDebt("charge", 1.0, "test debt", start); err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	engine.ReconcileSparkDebt()
	if !engine.State().DebtActive || engine.State().DebtAmount != 1.0 {
		t.Fatalf("expected active debt before grant: %#v", engine.State())
	}
	if _, err := engine.SparkLedger().Credit("grant", 1.25, "test allowance", start.Add(time.Minute)); err != nil {
		t.Fatalf("grant spark: %v", err)
	}
	engine.ReconcileSparkDebt()
	if engine.State().DebtActive || engine.State().DebtAmount != 0 {
		t.Fatalf("expected debt cleared after positive grant: %#v", engine.State())
	}
}

func TestSnapshotAndRestore(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	cfg := Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:     0.2,
			StrainRecoveryPerHour:    100,
			DayRecoveryPerHour:       50,
			WeekRecoveryPerHour:      25,
			FatigueRecoveryPerHour:   180,
			SleepDebtRecoveryPerHour: 2,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}

	engine := New(cfg, "jade", tokenledger.QuotaState{
		Window6HCap: 4000,
	}, start)
	_, err := engine.SparkLedger().Credit("grant", 1.2345, "boot", start)
	if err != nil {
		t.Fatalf("credit: %v", err)
	}

	snapshot := engine.Snapshot(start.Add(time.Minute))
	restored := Restore(cfg, snapshot)

	if restored.State().ResidentID != "jade" {
		t.Fatalf("resident id mismatch after restore")
	}
	if restored.SparkLedger().Account().Balance != 1.2345 {
		t.Fatalf("spark balance mismatch after restore")
	}
}

func TestTickRecoveryPersistsMode(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{
		TokenPolicy:    tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{ActivityMultipliers: map[string]float64{"idle": 1.0, "rest": 1.5}},
	}, "jade", tokenledger.QuotaState{Window6HCap: 4000}, start)

	engine.SetRecoveryMode("rest")
	tick := engine.TickRecovery(start.Add(time.Hour))
	if tick.RecoveryMode != "rest" {
		t.Fatalf("tick recovery mode = %s", tick.RecoveryMode)
	}
	if engine.State().RecoveryMode != "rest" {
		t.Fatalf("engine recovery mode = %s", engine.State().RecoveryMode)
	}

	snapshot := engine.Snapshot(start.Add(time.Hour))
	restored := Restore(engine.cfg, snapshot)
	if restored.State().RecoveryMode != "rest" {
		t.Fatalf("restored recovery mode = %s", restored.State().RecoveryMode)
	}
}

func TestAdjustQuotaCaps(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	engine := New(Config{}, "jade", tokenledger.QuotaState{
		Window6HCap: 4000,
		DayCap:      20000,
		WeekCap:     150000,
	}, start)

	engine.AdjustQuotaCaps(1000, 2000, -50000)
	state := engine.State()
	if state.Quota.Window6HCap != 5000 || state.Quota.DayCap != 22000 || state.Quota.WeekCap != 100000 {
		t.Fatalf("unexpected adjusted quota: %#v", state.Quota)
	}

	engine.AdjustQuotaCaps(-10000, -100000, -100000)
	state = engine.State()
	if state.Quota.Window6HCap != 0 || state.Quota.DayCap != 0 || state.Quota.WeekCap != 0 {
		t.Fatalf("quota caps should not go below zero: %#v", state.Quota)
	}
}
