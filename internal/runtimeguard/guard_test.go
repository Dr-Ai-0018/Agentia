package runtimeguard

import (
	"testing"

	"ai-arena/internal/tokenledger"
)

func TestAllowsWorkToConsumeReserveAndEnterDebt(t *testing.T) {
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

	if !got.Allowed || !got.AllowDebt || !got.LockAfterThisCall {
		t.Fatalf("expected work call to be allowed into debt and lock after call: %#v", got)
	}
	if !got.WouldEnterDebt || !got.WouldExceedQuota {
		t.Fatalf("expected work call to report debt and quota overrun risk: %#v", got)
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

func TestExhaustedEffectiveQuotaBlocksWork(t *testing.T) {
	state := State{
		SparkBalance: 1.0,
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
		t.Fatalf("expected exhausted effective quota to block work")
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "effective_window_exhausted" {
		t.Fatalf("unexpected reasons: %#v", got.Reasons)
	}
}

func TestFatigueAndSleepDebtShrinkEffectiveQuota(t *testing.T) {
	state := State{
		SparkBalance: 5.0,
		Fatigue:      2200,
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
	if !got.Allowed || !got.WouldExceedQuota || !got.LockAfterThisCall {
		t.Fatalf("expected reduced effective quota to allow one overrun and then lock: %#v", got)
	}
}
