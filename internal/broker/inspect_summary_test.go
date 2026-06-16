package broker

import (
	"testing"

	"ai-arena/internal/memory"
	"ai-arena/internal/worldstate"
)

func TestSummarizeHostInspect(t *testing.T) {
	out := HostInspectOutput{
		Inventory: InventorySnapshot{CollectedAt: "2026-06-12T03:00:00Z"},
		Path:      ".agents/inventory/incus-inventory.json",
		Capacity: HostCapacityReport{Pools: []ResourcePoolSummary{
			{Resource: "cpu", AllocatableTotal: 10, AllocatableFree: 4, Unit: "vcpu"},
		}},
		ResidentFacts: []ResidentRuntimeFact{
			{ResidentID: "jade", Status: "Running"},
			{ResidentID: "amber", Status: "Running", DriftFields: []string{"memory"}},
			{ResidentID: "onyx", Status: "", HostRSSHighGuestUsageLow: true, HostQEMURSSMiB: 2225, IncusMemoryCurrentMiB: 134, GuestMemAvailableMiB: 1806, GuestBuffCacheMiB: 52, GuestTopMemoryProcess: "608 incus-agent 21784"},
		},
		Memory: []memory.LifecycleReport{
			{Resident: "amber", NeedsAttention: 2},
		},
		MemoryMaintenance: []ResidentMemoryMaintenance{
			{ResidentID: "amber", LifecycleAttention: 2, DuplicateHistoryGroups: 3, RecommendedAction: "lifecycle_then_compaction_dry_run", Summary: "inspect lifecycle first", NeedsAttention: true},
		},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID:             "orchestrator-20260616T082449.311075354Z",
			ResidentsErrored:  1,
			BudgetBlockedRuns: 1,
			Residents: []OrchestratorResidentInspectionDigest{
				{Resident: "amber", Status: "ok", StoppedReason: "broker_preflight_denied: effective_window_exhausted", BudgetBlocked: true},
				{Resident: "onyx", Status: "error", Error: "model parse failure"},
			},
		},
		RecentOrchestrators: []OrchestratorInspectionDigest{
			{RunID: "orchestrator-20260616T082449.311075354Z", BudgetBlockedRuns: 1},
			{RunID: "orchestrator-20260616T074926.897967380Z"},
		},
		Followups: []worldstate.HostFollowup{
			{Kind: "chat_reply", Resident: "jade"},
			{Kind: "ticket_reply", Resident: "amber"},
			{Kind: "host_intervention", Resident: "amber"},
		},
	}

	summary := SummarizeHostInspect(out)
	if summary.ResidentCount != 3 {
		t.Fatalf("unexpected resident count: %#v", summary)
	}
	if summary.ResidentsRunning != 2 {
		t.Fatalf("unexpected running count: %#v", summary)
	}
	if summary.ResidentsWithDrift != 1 {
		t.Fatalf("unexpected drift count: %#v", summary)
	}
	if summary.ResidentsMissingInventory != 1 {
		t.Fatalf("unexpected missing inventory count: %#v", summary)
	}
	if summary.RuntimeMemoryObservationResidents != 1 {
		t.Fatalf("expected one runtime memory observation, got %#v", summary)
	}
	if summary.PendingChatResidents != 1 || summary.OpenTicketResidents != 1 || summary.InterventionCount != 1 {
		t.Fatalf("unexpected followup aggregation: %#v", summary)
	}
	if len(summary.ResidentRisk) != 3 {
		t.Fatalf("unexpected resident risk count: %#v", summary)
	}
	if len(summary.Capacity.Pools) != 1 || summary.Capacity.Pools[0].Resource != "cpu" {
		t.Fatalf("expected capacity to be carried into summary: %#v", summary.Capacity)
	}
	if summary.MemoryResidentsAttention != 1 || summary.MemoryItemsAttention != 2 {
		t.Fatalf("unexpected memory attention summary: %#v", summary)
	}
	if summary.MemoryMaintenanceResidents != 1 || summary.MemoryDuplicateHistoryGroups != 3 {
		t.Fatalf("unexpected memory maintenance summary: %#v", summary)
	}
	if summary.LatestOrchestrator == nil || summary.LatestOrchestrator.RunID == "" {
		t.Fatalf("expected latest orchestrator report in summary: %#v", summary)
	}
	if !summary.LatestRunNeedsAttention {
		t.Fatalf("expected latest run to need attention: %#v", summary)
	}
	if len(summary.RecentOrchestrators) != 2 || summary.RecentRunsNeedingAttention != 1 {
		t.Fatalf("unexpected recent orchestrator summary: %#v", summary)
	}
	var amber ResidentInspectRisk
	var onyx ResidentInspectRisk
	for _, item := range summary.ResidentRisk {
		if item.ResidentID == "amber" {
			amber = item
		}
		if item.ResidentID == "onyx" {
			onyx = item
		}
	}
	if amber.MemoryAttention != 2 {
		t.Fatalf("expected amber memory attention, got %#v", amber)
	}
	if amber.MemoryDuplicateHistoryGroups != 3 {
		t.Fatalf("expected amber duplicate history groups, got %#v", amber)
	}
	if !amber.OrchestratorBudgetBlocked {
		t.Fatalf("expected amber orchestrator budget block, got %#v", amber)
	}
	if amber.MemoryRecommendedAction != "lifecycle_then_compaction_dry_run" || amber.MemorySummary == "" {
		t.Fatalf("expected amber memory recommendation, got %#v", amber)
	}
	if onyx.OrchestratorError == "" {
		t.Fatalf("expected onyx orchestrator error, got %#v", onyx)
	}
	if !onyx.HostRSSHighGuestUsageLow || onyx.HostQEMURSSMiB != 2225 || onyx.GuestTopMemoryProcess == "" {
		t.Fatalf("expected onyx runtime memory observation, got %#v", onyx)
	}
}
