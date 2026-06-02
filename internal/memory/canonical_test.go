package memory

import "testing"

func TestDistillEventsResourceFailureRecoveryPattern(t *testing.T) {
	cm := DistillEvents([]Event{
		{Category: "resource_change", Summary: "disk expansion approved"},
		{Category: "failure", Summary: "bootstrap failed"},
		{Category: "failure", Summary: "bootstrap failed again"},
		{Category: "recovery", Summary: "recovered after narrowing path"},
	})

	if cm.Trigger == "" || cm.MistakenBelief == "" || cm.CorrectedBelief == "" {
		t.Fatal("expected canonical memory fields to be populated")
	}
	if cm.Domain != DomainResources {
		t.Fatalf("expected resource domain, got %s", cm.Domain)
	}
}

func TestDistillEventsAdminCase(t *testing.T) {
	cm := DistillEvents([]Event{
		{Category: "admin_feedback", Summary: "admin demanded cleaner structure"},
	})

	if cm.Domain != DomainLessons && cm.Domain != DomainRules {
		t.Fatalf("unexpected domain %s", cm.Domain)
	}
	if cm.Trigger == "" || cm.ActionBoundary == "" {
		t.Fatal("expected trigger and action boundary")
	}
}

func TestCanonicalToEventSignal(t *testing.T) {
	cm := DistillEvents([]Event{
		{Category: "resource_change", Summary: "disk expansion approved"},
		{Category: "failure", Summary: "bootstrap failed"},
		{Category: "failure", Summary: "bootstrap failed again"},
		{Category: "recovery", Summary: "recovered after narrowing path"},
		{Category: "admin_feedback", Summary: "admin demanded cleaner structure"},
	})

	signal := cm.ToEventSignal()
	if signal.ResourceWeight <= 0 {
		t.Fatal("expected resource weight")
	}
	if signal.DecisionImpact <= 0 {
		t.Fatal("expected decision impact")
	}
	if signal.Recurrence <= 0 {
		t.Fatal("expected recurrence signal")
	}
}
