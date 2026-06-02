package memory

import (
	"testing"
	"time"
)

func TestEvaluateRoutesWeakSignalsToInstant(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.Evaluate(EventSignal{
		Confidence:     0.3,
		DecisionImpact: 0.2,
		ImpactRounds:   1,
	})

	if decision.TargetLayer != LayerInstant || decision.Action != ActionCreate {
		t.Fatalf("expected instant create, got %s %s", decision.Action, decision.TargetLayer)
	}
}

func TestEvaluatePromotesStableOperationalMemoryToLong(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.Evaluate(EventSignal{
		Confidence:     0.8,
		DecisionImpact: 0.8,
		ImpactRounds:   4,
		Recurrence:     2,
		ResourceWeight: 0.6,
	})

	if decision.TargetLayer != LayerLong || decision.Action != ActionPromote {
		t.Fatalf("expected long promote, got %s %s", decision.Action, decision.TargetLayer)
	}
}

func TestEvaluatePromotesIdentityAnchorsToPermanent(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.Evaluate(EventSignal{
		Confidence:     0.9,
		DecisionImpact: 0.6,
		IdentityWeight: 0.9,
	})

	if decision.TargetLayer != LayerPermanent || decision.Action != ActionPromote {
		t.Fatalf("expected permanent promote, got %s %s", decision.Action, decision.TargetLayer)
	}
}

func TestDecayShortToInstantAfterExpiry(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateDecay(LayerShort, EventSignal{
		AgeSinceTouch: 80 * time.Hour,
	})

	if decision.Action != ActionDecay || decision.TargetLayer != LayerInstant {
		t.Fatalf("expected short to decay to instant, got %s %s", decision.Action, decision.TargetLayer)
	}
}

func TestPermanentReviewDue(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.EvaluateDecay(LayerPermanent, EventSignal{
		AgeSinceCreation: 31 * 24 * time.Hour,
	})

	if decision.Action != ActionReview || !decision.NeedsConfirmation {
		t.Fatalf("expected permanent review with confirmation, got %s confirmation=%v", decision.Action, decision.NeedsConfirmation)
	}
}

func TestDailySummaryDue(t *testing.T) {
	policy := DefaultPolicy()
	now := time.Now()
	if !policy.DailySummaryDue(now.Add(-25*time.Hour), now) {
		t.Fatal("expected daily summary to be due")
	}
	if policy.DailySummaryDue(now.Add(-2*time.Hour), now) {
		t.Fatal("did not expect daily summary to be due")
	}
}
