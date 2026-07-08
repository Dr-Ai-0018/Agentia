package runtimeguard

import (
	"testing"

	"ai-arena/internal/tokenledger"
)

func TestBlocksWorkBeforeDebtOrReserveConsumption(t *testing.T) {
	state := State{
		SparkBalance: 0.05,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 990,
		},
		ReserveSpark:  0.2,
		ReserveStrain: 120,
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.2,
		StrainCost: 120,
	})

	if got.Allowed || got.AllowDebt {
		t.Fatalf("expected work call to be blocked before debt: %#v", got)
	}
	if !got.WouldEnterDebt || got.WouldExceedQuota {
		t.Fatalf("expected work call to report debt risk without 6h hard quota risk: %#v", got)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "work_would_enter_debt" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestBlocksWorkBeforeSparkReserveConsumption(t *testing.T) {
	state := State{
		SparkBalance: 0.25,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 100,
		},
		ReserveSpark:  0.2,
		ReserveStrain: 120,
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if got.Allowed {
		t.Fatalf("expected work call to be blocked before consuming spark reserve: %#v", got)
	}
	if !got.ConsumesReserve || len(got.Reasons) != 1 || got.Reasons[0] != "work_would_consume_spark_reserve" {
		t.Fatalf("unexpected reserve decision: %#v", got)
	}
}

func TestAllowsFinalNoticeIntoDebt(t *testing.T) {
	state := State{
		SparkBalance: 0.05,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 990,
		},
		ReserveSpark:  0.2,
		ReserveStrain: 120,
	}

	got := Evaluate(state, Request{
		Kind:       CallKindFinalNotice,
		SparkCost:  0.2,
		StrainCost: 120,
	})

	if !got.Allowed || !got.AllowDebt || !got.LockAfterThisCall {
		t.Fatalf("expected final notice to be allowed into debt and lock after call: %#v", got)
	}
}

func TestDebtBlocksFurtherWork(t *testing.T) {
	state := State{
		SparkBalance:    -0.15,
		DebtActive:      true,
		DebtAmount:      0.15,
		FinalNoticeUsed: true,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 1000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if got.Allowed {
		t.Fatalf("expected work call to be blocked during debt")
	}
}

func TestAcceptanceIgnoresPriorFinalNoticeWhenNotInDebt(t *testing.T) {
	state := State{
		SparkBalance:    2,
		FinalNoticeUsed: true,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 100,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindAcceptance,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if !got.Allowed {
		t.Fatalf("expected normal acceptance to ignore prior final notice state: %#v", got)
	}
}

func TestAcceptanceBlockedDuringDebt(t *testing.T) {
	state := State{
		SparkBalance: -0.15,
		DebtActive:   true,
		DebtAmount:   0.15,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 100,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindAcceptance,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if got.Allowed {
		t.Fatalf("expected acceptance to be blocked during active debt: %#v", got)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "spark_debt_active" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestExhaustedBalanceBlocksWork(t *testing.T) {
	state := State{
		SparkBalance: 0,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 100,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if got.Allowed {
		t.Fatalf("expected exhausted spark to block work")
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "spark_exhausted" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestExhaustedEffectiveWindowDoesNotBlockWork(t *testing.T) {
	state := State{
		SparkBalance: 1.0,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 1000,
			DayCap:       10000,
			WeekCap:      50000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if !got.Allowed {
		t.Fatalf("expected exhausted 6h observation window not to block work: %#v", got)
	}
	if got.WouldExceedQuota {
		t.Fatalf("6h exhaustion must not be reported as hard quota risk: %#v", got)
	}
}

func TestRollingDayQuotaBlocksProjectedWork(t *testing.T) {
	state := State{
		SparkBalance:      1.0,
		RollingUsageValid: true,
		RollingDayUsed:    950,
		RollingWeekUsed:   1000,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 1000,
			DayCap:       1000,
			WeekCap:      5000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 100,
	})

	if got.Allowed {
		t.Fatalf("expected projected rolling day quota to block work")
	}
	if !got.WouldExceedQuota || !got.WouldExceedDay || got.WouldExceedWeek {
		t.Fatalf("unexpected quota projection: %#v", got)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "work_would_exceed_day_quota" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestRollingWeekQuotaBlocksProjectedWork(t *testing.T) {
	state := State{
		SparkBalance:      1.0,
		RollingUsageValid: true,
		RollingDayUsed:    100,
		RollingWeekUsed:   4950,
		Quota: tokenledger.QuotaState{
			Window6HCap: 1000,
			DayCap:      1000,
			WeekCap:     5000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindAcceptance,
		SparkCost:  0.1,
		StrainCost: 100,
	})

	if got.Allowed {
		t.Fatalf("expected projected rolling week quota to block work")
	}
	if !got.WouldExceedQuota || got.WouldExceedDay || !got.WouldExceedWeek {
		t.Fatalf("unexpected quota projection: %#v", got)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "acceptance_would_exceed_week_quota" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestFatigueHardCapBlocksOrdinaryWork(t *testing.T) {
	state := State{
		SparkBalance: 1.0,
		Fatigue:      1000,
		FatigueCap:   1000,
		Quota: tokenledger.QuotaState{
			DayCap:  10000,
			WeekCap: 50000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if got.Allowed {
		t.Fatalf("expected fatigue hard cap to block ordinary work")
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "fatigue_exhausted" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestFinalNoticeStillAllowedAtFatigueHardCap(t *testing.T) {
	state := State{
		SparkBalance: 0.05,
		Fatigue:      1000,
		FatigueCap:   1000,
		Quota: tokenledger.QuotaState{
			DayCap:  10000,
			WeekCap: 50000,
		},
	}

	got := Evaluate(state, Request{
		Kind:       CallKindFinalNotice,
		SparkCost:  0.1,
		StrainCost: 10,
	})

	if !got.Allowed {
		t.Fatalf("expected final notice to remain allowed at fatigue hard cap: %#v", got)
	}
}

func TestFatigueZoneAndMultiplier(t *testing.T) {
	cases := []struct {
		fatigue    int
		wantZone   string
		wantFactor float64
	}{
		{fatigue: 0, wantZone: "fresh", wantFactor: 1.0},
		{fatigue: 300, wantZone: "warming_up", wantFactor: 1.05},
		{fatigue: 550, wantZone: "tired", wantFactor: 1.15},
		{fatigue: 800, wantZone: "exhausted", wantFactor: 1.35},
		{fatigue: 1000, wantZone: "hard_cap", wantFactor: 1.35},
	}

	for _, tc := range cases {
		if got := FatigueZone(tc.fatigue, 1000); got != tc.wantZone {
			t.Fatalf("FatigueZone(%d) = %s, want %s", tc.fatigue, got, tc.wantZone)
		}
		if got := FatigueStrainMultiplier(tc.fatigue, 1000); got != tc.wantFactor {
			t.Fatalf("FatigueStrainMultiplier(%d) = %.2f, want %.2f", tc.fatigue, got, tc.wantFactor)
		}
	}
}

func TestFatigueAndSleepDebtShrinkEffectiveQuota(t *testing.T) {
	state := State{
		SparkBalance: 5.0,
		Fatigue:      800,
		FatigueCap:   1000,
		SleepDebt:    12,
		Quota: tokenledger.QuotaState{
			Window6HCap:  10000,
			Window6HUsed: 5400,
		},
		ReserveSpark:  0.2,
		ReserveStrain: 300,
	}

	effective := DeriveEffectiveQuota(state)
	if effective.Window6HCap >= state.Quota.Window6HCap {
		t.Fatalf("expected effective 6h cap to shrink")
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.2,
		StrainCost: 900,
	})
	if !got.Allowed {
		t.Fatalf("fatigue-shrunk 6h quota should not hard-stop admission now: %#v", got)
	}
	if got.WouldExceedQuota || got.LockAfterThisCall {
		t.Fatalf("6h projection must not be treated as hard quota risk: %#v", got)
	}
}

func TestEffectiveQuotaUsesNormalizedFatigue(t *testing.T) {
	state := State{
		Fatigue:    139441,
		FatigueCap: 2500000,
		Quota: tokenledger.QuotaState{
			Window6HCap: 240000,
			DayCap:      1400000,
			WeekCap:     5005000,
		},
	}

	effective := DeriveEffectiveQuota(state)
	if effective.Window6HCap != state.Quota.Window6HCap {
		t.Fatalf("low normalized fatigue should not shrink 6h cap: %#v", effective)
	}
	if effective.DayCap != state.Quota.DayCap || effective.WeekCap != state.Quota.WeekCap {
		t.Fatalf("low normalized fatigue should not shrink day/week caps: %#v", effective)
	}
}
