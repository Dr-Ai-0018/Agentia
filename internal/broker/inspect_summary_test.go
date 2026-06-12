package broker

import (
	"testing"

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
			{ResidentID: "onyx", Status: ""},
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
	if summary.PendingChatResidents != 1 || summary.OpenTicketResidents != 1 || summary.InterventionCount != 1 {
		t.Fatalf("unexpected followup aggregation: %#v", summary)
	}
	if len(summary.ResidentRisk) != 3 {
		t.Fatalf("unexpected resident risk count: %#v", summary)
	}
	if len(summary.Capacity.Pools) != 1 || summary.Capacity.Pools[0].Resource != "cpu" {
		t.Fatalf("expected capacity to be carried into summary: %#v", summary.Capacity)
	}
}
