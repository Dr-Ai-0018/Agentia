package broker

import (
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
}

func findReadinessItem(out V0ReadinessOutput, id string) V0ReadinessItem {
	for _, item := range out.Items {
		if item.ID == id {
			return item
		}
	}
	return V0ReadinessItem{}
}
