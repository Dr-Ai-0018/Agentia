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
		!hasRecommendedStep(out, "final_acceptance_manual_pass") ||
		!hasRecommendedStep(out, "ultra_long_soak_pre_release") {
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

func TestBuildV0ReadinessUsesManualValidationEvidence(t *testing.T) {
	now := time.Date(2026, 6, 18, 6, 30, 0, 0, time.UTC)
	out := BuildV0ReadinessWithEvidence(HostInspectSummary{
		CollectedAt:      "2026-06-18T06:30:00Z",
		ResidentCount:    3,
		ResidentsRunning: 3,
		Capacity:         HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 4, AllocatableFree: 2, Unit: "vcpu"}}},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-20260617T041222.823174786Z",
		},
	}, now, "test", []V0AcceptanceEvidenceRecord{
		passedAcceptanceEvidence("host_only_maintenance_smoke", "2026-06-18T08:00:00Z"),
		passedAcceptanceEvidence("cpu_maintenance_regression", "2026-06-18T08:01:00Z"),
		passedAcceptanceEvidence("disk_maintenance_regression", "2026-06-18T08:02:00Z"),
		passedAcceptanceEvidence("checkpoint_cleanup_apply_regression", "2026-06-18T08:03:00Z"),
	})

	if hasRecommendedStep(out, "cpu_maintenance_regression") ||
		hasRecommendedStep(out, "disk_maintenance_regression") ||
		hasRecommendedStep(out, "checkpoint_cleanup_apply_regression") {
		t.Fatalf("completed manual validations should not remain recommended steps: %#v", out.Completion.RecommendedSteps)
	}
	if !hasBlockingRecommendedStep(out, "final_acceptance_manual_pass") {
		t.Fatalf("expected final acceptance blocker to remain: %#v", out.Completion.RecommendedSteps)
	}
	if !hasBlockingRecommendedStep(out, "ultra_long_soak_pre_release") {
		t.Fatalf("expected ultra-long soak blocker to remain: %#v", out.Completion.RecommendedSteps)
	}
	item := findReadinessItem(out, "known_manual_gaps")
	if item.Status != v0ReadinessWarn || len(item.Evidence) != 2 {
		t.Fatalf("expected known manual gaps to reflect final acceptance and ultra-long soak: %#v", item)
	}
	if !strings.Contains(strings.Join(item.Evidence, "\n"), "v0 should only be declared") ||
		!strings.Contains(strings.Join(item.Evidence, "\n"), "administrator-layer intervention is forbidden") {
		t.Fatalf("expected final acceptance and ultra-long soak evidence text: %#v", item)
	}
}

func TestBuildV0ReadinessTreatsNonBlockingBacklogAsReleaseCandidate(t *testing.T) {
	now := time.Date(2026, 6, 26, 10, 56, 25, 0, time.UTC)
	out := BuildV0ReadinessWithEvidence(HostInspectSummary{
		CollectedAt:               "2026-06-26T10:56:25Z",
		ResidentCount:             3,
		ResidentsRunning:          3,
		PendingChatResidents:      3,
		MemoryItemsAttention:      375,
		MemoryResidentReviewQueue: 375,
		Capacity: HostCapacityReport{Pools: []ResourcePoolSummary{
			{Resource: "cpu", AllocatableTotal: 4, AllocatableFree: 1, Unit: "vcpu"},
			{Resource: "memory", AllocatableTotal: 19991, AllocatableFree: 13589, Unit: "MiB"},
			{Resource: "disk", AllocatableTotal: 107, AllocatableFree: 66, Unit: "GiB"},
		}},
		RecentMaintenanceRuns: []MaintenanceRunRecord{
			{ID: "maintenance-amber-1", ResidentID: "amber", Resource: "memory", Amount: "noop", State: "completed", CreatedAt: "2026-06-26T09:00:00Z", InventoryRefreshed: true},
		},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-20260621T153938.239228513Z",
		},
	}, now, "test", []V0AcceptanceEvidenceRecord{
		passedAcceptanceEvidence("host_only_maintenance_smoke", "2026-06-20T12:40:00Z"),
		passedAcceptanceEvidence("cpu_maintenance_regression", "2026-06-20T12:41:00Z"),
		passedAcceptanceEvidence("disk_maintenance_regression", "2026-06-20T12:42:00Z"),
		passedAcceptanceEvidence("checkpoint_cleanup_apply_regression", "2026-06-20T12:43:00Z"),
		passedAcceptanceEvidence("final_acceptance_manual_pass", "2026-06-20T12:44:00Z"),
		passedAcceptanceEvidence("ultra_long_soak_pre_release", "2026-06-20T12:45:00Z"),
	})

	if out.Status != "ready_with_warnings" {
		t.Fatalf("expected non-blocking backlog to keep warning state, got %#v", out)
	}
	if out.Completion.WeightedPercent < 90 {
		t.Fatalf("expected non-blocking backlog to remain above 90 weighted completion, got %#v", out.Completion)
	}
	if out.Completion.ReleaseGate != "release_candidate" {
		t.Fatalf("expected release candidate gate once blockers and manual gaps are closed, got %#v", out.Completion)
	}
	if findReadinessItem(out, "world_followups").Status != v0ReadinessWarn {
		t.Fatalf("expected world followups to remain a warning: %#v", out)
	}
	if findReadinessItem(out, "memory_governance").Status != v0ReadinessWarn {
		t.Fatalf("expected resident-owned memory queue to remain a warning: %#v", out)
	}
	if stream, ok := findWorkstream(out, "s5_memory"); !ok || stream.Status != "resident_queue_pending" || stream.Percent < 90 {
		t.Fatalf("expected resident-owned memory backlog to stay high-progress: %#v", out.Completion.Workstreams)
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

	item := findReadinessItem(out, "memory_governance")
	if item.Status != v0ReadinessWarn {
		t.Fatalf("expected stale memory reviews to warn without blocking: %#v", out)
	}
	joinedEvidence := strings.Join(item.Evidence, " ")
	if !strings.Contains(joinedEvidence, "host_actionable=0") || !strings.Contains(joinedEvidence, "resident_owned=12") {
		t.Fatalf("expected memory governance evidence to split host-actionable and resident-owned work: %#v", item)
	}
	step, ok := findRecommendedStep(out, "review_memory_governance_queue")
	if !ok {
		t.Fatalf("expected memory governance review step: %#v", out.Completion.RecommendedSteps)
	}
	if step.Title != "Review resident-owned memory governance queue" {
		t.Fatalf("expected resident-owned memory step title, got %#v", step)
	}
	if !strings.Contains(step.Reason, "Host-actionable memory cleanup is clear") {
		t.Fatalf("expected step reason to avoid implying host rewrite work: %#v", step)
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
	if step.Title != "Estimate and issue test allowance before another orchestrator probe" {
		t.Fatalf("expected budget-blocked soak step to point at budget estimate first, got %#v", step)
	}
	if !strings.Contains(step.Reason, "not valid evidence for the no-admin ultra-long pre-release test") {
		t.Fatalf("expected allowance boundary in reason, got %#v", step)
	}
	if !strings.Contains(step.Command, "orchestrator-budget-estimate") || !strings.Contains(step.Command, "probe_allowance_by_resident") || !strings.Contains(step.Command, "--duration 45s") {
		t.Fatalf("expected budget estimate plus short probe command, got %#v", step)
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

func findWorkstream(out V0ReadinessOutput, id string) (V0WorkstreamProgress, bool) {
	for _, stream := range out.Completion.Workstreams {
		if stream.ID == id {
			return stream, true
		}
	}
	return V0WorkstreamProgress{}, false
}
