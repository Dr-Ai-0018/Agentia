package broker

import (
	"strings"
	"testing"
	"time"
)

func TestBuildV0ReadinessReportsWarningsForKnownGaps(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		CollectedAt:      "2026-06-18T06:30:00Z",
		ResidentCount:    3,
		ResidentsRunning: 3,
		Capacity:         HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 4, AllocatableFree: 2, Unit: "vcpu"}}},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-20260617T041222.823174786Z",
		},
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	if out.Status != "ready_with_warnings" {
		t.Fatalf("expected ready_with_warnings from known manual gaps, got %#v", out)
	}
	if out.Failed != 0 || out.Warnings == 0 || out.Passed == 0 {
		t.Fatalf("unexpected readiness counts: %#v", out)
	}
	if out.Completion.WeightedPercent <= 0 || out.Completion.ChecklistPercent <= 0 {
		t.Fatalf("expected completion percentages: %#v", out.Completion)
	}
	if out.Completion.ReleaseGate == "" {
		t.Fatalf("expected release gate: %#v", out.Completion)
	}
	if len(out.Completion.Workstreams) != 5 {
		t.Fatalf("expected five weighted workstreams: %#v", out.Completion.Workstreams)
	}
	if len(out.Completion.RecommendedSteps) == 0 {
		t.Fatalf("expected recommended steps for warning state: %#v", out.Completion)
	}
	if !hasRecommendedStep(out, "cpu_maintenance_regression") ||
		!hasRecommendedStep(out, "disk_maintenance_regression") ||
		!hasRecommendedStep(out, "checkpoint_cleanup_apply_regression") ||
		!hasRecommendedStep(out, "final_acceptance_manual_pass") {
		t.Fatalf("expected split manual validation steps: %#v", out.Completion.RecommendedSteps)
	}
	cpuStep, ok := findRecommendedStep(out, "cpu_maintenance_regression")
	if !ok {
		t.Fatalf("expected cpu maintenance regression step")
	}
	if cpuStep.Command != "arena-broker --mode v0-runbook" {
		t.Fatalf("expected cpu regression to point at host-only runbook path, got %#v", cpuStep)
	}
	if !strings.Contains(cpuStep.Reason, "host-plan/start/complete") {
		t.Fatalf("expected cpu regression reason to mention host-only lifecycle, got %#v", cpuStep)
	}
	if findReadinessItem(out, "maintenance_draft_safety").Status != v0ReadinessPass {
		t.Fatalf("expected dry-run maintenance draft safety pass: %#v", out)
	}
	if findReadinessItem(out, "known_manual_gaps").Status != v0ReadinessWarn {
		t.Fatalf("expected known manual gaps warning: %#v", out)
	}
}

func TestBuildV0ReadinessFailsWhenRequiredStateMissing(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		ResidentsMissingInventory: 1,
		Capacity:                  HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "memory", AllocatableTotal: 8192, AllocatableFree: 0, Unit: "MiB"}}},
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	if out.Status != "not_ready" {
		t.Fatalf("expected not_ready, got %#v", out)
	}
	if out.Failed == 0 {
		t.Fatalf("expected failed readiness items: %#v", out)
	}
	if out.Completion.ReleaseGate != "blocked_by_required_failures" {
		t.Fatalf("expected blocked release gate: %#v", out.Completion)
	}
	if !hasBlockingRecommendedStep(out, "refresh_inventory_and_capacity") {
		t.Fatalf("expected blocking inventory/capacity step: %#v", out.Completion.RecommendedSteps)
	}
	if findReadinessItem(out, "inventory_facts").Status != v0ReadinessFail {
		t.Fatalf("expected inventory failure: %#v", out)
	}
	if findReadinessItem(out, "orchestrator_registry").Status != v0ReadinessFail {
		t.Fatalf("expected orchestrator failure: %#v", out)
	}
}

func TestBuildV0ReadinessFailsOnDuplicateMemoryDebt(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		ResidentCount:                3,
		ResidentsRunning:             3,
		Capacity:                     HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "disk", AllocatableTotal: 100, AllocatableFree: 50, Unit: "GiB"}}},
		LatestOrchestrator:           &OrchestratorInspectionDigest{RunID: "orchestrator-ok"},
		MemoryDuplicateHistoryGroups: 2,
		MemoryMaintenanceResidents:   1,
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	if out.Status != "not_ready" {
		t.Fatalf("expected not_ready from duplicate memory debt, got %#v", out)
	}
	if findReadinessItem(out, "memory_governance").Status != v0ReadinessFail {
		t.Fatalf("expected memory governance failure: %#v", out)
	}
	if out.Completion.WeightedPercent >= 80 {
		t.Fatalf("expected duplicate memory debt to reduce weighted completion: %#v", out.Completion)
	}
}

func TestBuildV0ReadinessFailsOnOperatorMemoryReview(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		ResidentCount:                3,
		ResidentsRunning:             3,
		Capacity:                     HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "disk", AllocatableTotal: 100, AllocatableFree: 50, Unit: "GiB"}}},
		LatestOrchestrator:           &OrchestratorInspectionDigest{RunID: "orchestrator-ok"},
		MemoryOperatorReviewRequired: 1,
		MemoryMaintenanceResidents:   1,
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	if findReadinessItem(out, "memory_governance").Status != v0ReadinessFail {
		t.Fatalf("expected operator review to fail memory governance: %#v", out)
	}
}

func TestBuildV0ReadinessWarnsOnStaleMemoryReviews(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		ResidentCount:              3,
		ResidentsRunning:           3,
		Capacity:                   HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "disk", AllocatableTotal: 100, AllocatableFree: 50, Unit: "GiB"}}},
		LatestOrchestrator:         &OrchestratorInspectionDigest{RunID: "orchestrator-ok"},
		MemoryStaleReviewItems:     12,
		MemoryMaintenanceResidents: 1,
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	if findReadinessItem(out, "memory_governance").Status != v0ReadinessWarn {
		t.Fatalf("expected stale memory reviews to warn without blocking: %#v", out)
	}
}

func TestBuildV0ReadinessMarksLongSoakAsApprovalRequired(t *testing.T) {
	out := BuildV0Readiness(HostInspectSummary{
		ResidentCount:                 3,
		ResidentsRunning:              3,
		Capacity:                      HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 8, AllocatableFree: 4, Unit: "vcpu"}}},
		RecentRunsNeedingAttention:    1,
		LatestRunNeedsAttention:       true,
		LatestOrchestrator:            &OrchestratorInspectionDigest{RunID: "orchestrator-budget-blocked", BudgetBlockedRuns: 1, TransientBlocked: 1},
		MemoryDuplicateHistoryGroups:  0,
		MemoryOperatorDecayCandidates: 0,
	}, time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC), "test")

	item := findReadinessItem(out, "orchestrator_registry")
	if !strings.Contains(strings.Join(item.Evidence, " "), "latest_transient_blocked=1") {
		t.Fatalf("expected transient block evidence, got %#v", item)
	}
	step, ok := findRecommendedStep(out, "longer_orchestrator_soak")
	if !ok {
		t.Fatalf("expected longer orchestrator soak step: %#v", out.Completion.RecommendedSteps)
	}
	if !step.RequiresApproval {
		t.Fatalf("expected longer soak to require approval: %#v", step)
	}
	if step.BlocksRelease {
		t.Fatalf("longer soak should not be a hard release blocker by itself: %#v", step)
	}
}

func findReadinessItem(out V0ReadinessOutput, id string) V0ReadinessItem {
	for _, item := range out.Items {
		if item.ID == id {
			return item
		}
	}
	return V0ReadinessItem{}
}

func hasRecommendedStep(out V0ReadinessOutput, id string) bool {
	_, ok := findRecommendedStep(out, id)
	return ok
}

func hasBlockingRecommendedStep(out V0ReadinessOutput, id string) bool {
	step, ok := findRecommendedStep(out, id)
	if !ok {
		return false
	}
	return step.BlocksRelease
}

func findRecommendedStep(out V0ReadinessOutput, id string) (V0RecommendedStep, bool) {
	for _, step := range out.Completion.RecommendedSteps {
		if step.ID == id {
			return step, true
		}
	}
	return V0RecommendedStep{}, false
}
