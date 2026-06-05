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
		TokenPolicy:    tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
			DayRecoveryPerHour:    50,
			WeekRecoveryPerHour:   25,
		},
		ReserveSpark:   0.08,
		ReserveStrain:  300,
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
	if workPrepared.Decision.Allowed {
		t.Fatalf("work should still be blocked after only 2h recovery")
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

func TestSnapshotAndRestore(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	cfg := Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
			DayRecoveryPerHour:    50,
			WeekRecoveryPerHour:   25,
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
