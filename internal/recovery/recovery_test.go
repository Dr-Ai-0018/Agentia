package recovery

import (
	"testing"
	"time"

	"ai-arena/internal/tokenledger"
)

func TestRecoveryPaysDebtFirst(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	result := Apply(Policy{
		SparkRecoveryPerHour:  0.2,
		StrainRecoveryPerHour: 100,
		DayRecoveryPerHour:    50,
		WeekRecoveryPerHour:   25,
	}, State{
		SparkBalance: -0.3875,
		Fatigue:      600,
		SleepDebt:    6,
		DebtActive:   true,
		DebtAmount:   0.3875,
		Quota: tokenledger.QuotaState{
			Window6HCap:  4000,
			Window6HUsed: 1520,
			DayCap:       20000,
			DayUsed:      2720,
			WeekCap:      150000,
			WeekUsed:     9220,
		},
		LastTickAt: start,
	}, start.Add(2*time.Hour))

	if result.DebtActive {
		t.Fatalf("expected debt to be cleared")
	}
	if result.NewSparkBalance != 0.0125 {
		t.Fatalf("new balance = %.4f, want 0.0125", result.NewSparkBalance)
	}
	if result.QuotaAfter.Window6HUsed != 1320 {
		t.Fatalf("6h used = %d, want 1320", result.QuotaAfter.Window6HUsed)
	}
	if result.QuotaAfter.DayUsed != 2620 {
		t.Fatalf("day used = %d, want 2620", result.QuotaAfter.DayUsed)
	}
	if result.QuotaAfter.WeekUsed != 9170 {
		t.Fatalf("week used = %d, want 9170", result.QuotaAfter.WeekUsed)
	}
	if result.FatigueAfter != 600 {
		t.Fatalf("fatigue after = %d, want 600 with default zero fatigue recovery policy", result.FatigueAfter)
	}
	if result.SleepDebtAfter != 6 {
		t.Fatalf("sleep debt after = %d, want 6 with default zero sleep debt recovery policy", result.SleepDebtAfter)
	}
}

func TestRecoveryAlsoReducesFatigueAndSleepDebtWhenConfigured(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	result := Apply(Policy{
		SparkRecoveryPerHour:     0.2,
		StrainRecoveryPerHour:    100,
		DayRecoveryPerHour:       50,
		WeekRecoveryPerHour:      25,
		FatigueRecoveryPerHour:   180,
		SleepDebtRecoveryPerHour: 2,
	}, State{
		SparkBalance: 0.5,
		Fatigue:      600,
		SleepDebt:    6,
		Quota: tokenledger.QuotaState{
			Window6HCap:  4000,
			Window6HUsed: 1520,
			DayCap:       20000,
			DayUsed:      2720,
			WeekCap:      150000,
			WeekUsed:     9220,
		},
		LastTickAt: start,
	}, start.Add(2*time.Hour))

	if result.FatigueAfter != 240 {
		t.Fatalf("fatigue after = %d, want 240", result.FatigueAfter)
	}
	if result.SleepDebtAfter != 2 {
		t.Fatalf("sleep debt after = %d, want 2", result.SleepDebtAfter)
	}
	if result.NextRecoveryAt != "2026-06-05T02:15:00Z" {
		t.Fatalf("next recovery at = %s", result.NextRecoveryAt)
	}
	if result.RecoveryTickMinutes != 15 {
		t.Fatalf("recovery tick minutes = %d", result.RecoveryTickMinutes)
	}
}

func TestRecoveryModeMultiplierAffectsRecovery(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	policy := Policy{
		SparkRecoveryPerHour:     0.2,
		StrainRecoveryPerHour:    100,
		DayRecoveryPerHour:       50,
		WeekRecoveryPerHour:      25,
		FatigueRecoveryPerHour:   180,
		SleepDebtRecoveryPerHour: 2,
		ActivityMultipliers: map[string]float64{
			"idle": 1.0,
			"rest": 1.5,
		},
	}
	baseState := State{
		SparkBalance: 0.5,
		Fatigue:      600,
		SleepDebt:    6,
		Quota: tokenledger.QuotaState{
			Window6HCap:  4000,
			Window6HUsed: 1520,
			DayCap:       20000,
			DayUsed:      2720,
			WeekCap:      150000,
			WeekUsed:     9220,
		},
		LastTickAt: start,
	}

	idle := Apply(policy, baseState, start.Add(2*time.Hour))
	rest := Apply(policy, State{
		SparkBalance: baseState.SparkBalance,
		Fatigue:      baseState.Fatigue,
		SleepDebt:    baseState.SleepDebt,
		Quota:        baseState.Quota,
		LastTickAt:   baseState.LastTickAt,
		RecoveryMode: "rest",
	}, start.Add(2*time.Hour))

	if idle.RecoveryMode != "idle" {
		t.Fatalf("idle recovery mode = %s", idle.RecoveryMode)
	}
	if rest.RecoveryMode != "rest" {
		t.Fatalf("rest recovery mode = %s", rest.RecoveryMode)
	}
	if rest.RecoveryMultiplier <= idle.RecoveryMultiplier {
		t.Fatalf("expected rest multiplier > idle multiplier")
	}
	if rest.RecoveredFatigue <= idle.RecoveredFatigue {
		t.Fatalf("expected rest fatigue recovery to be stronger")
	}
	if rest.QuotaAfter.Window6HUsed >= idle.QuotaAfter.Window6HUsed {
		t.Fatalf("expected rest to recover more 6h strain")
	}
}
