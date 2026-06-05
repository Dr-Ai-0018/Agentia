package runtimeguard

import (
	"testing"

	"ai-arena/internal/tokenledger"
)

func TestBlocksWorkToPreserveFinalNoticeReserve(t *testing.T) {
	state := State{
		SparkBalance: 1.0,
		Quota: tokenledger.QuotaState{
			Window6HCap:  1000,
			Window6HUsed: 850,
		},
		ReserveSpark:  0.2,
		ReserveStrain: 120,
	}

	got := Evaluate(state, Request{
		Kind:       CallKindWork,
		SparkCost:  0.9,
		StrainCost: 50,
	})

	if got.Allowed {
		t.Fatalf("expected work call to be blocked")
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
