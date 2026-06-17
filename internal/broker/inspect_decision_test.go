package broker

import (
	"strings"
	"testing"
)

func TestBuildHostDecisionAssist(t *testing.T) {
	out := BuildHostDecisionAssist(HostInspectSummary{
		CollectedAt:                       "2026-06-12T03:00:00Z",
		InventoryPath:                     ".agents/inventory/incus-inventory.json",
		Capacity:                          HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "memory", AllocatableTotal: 8192, AllocatableFree: 0, Unit: "MiB"}}},
		ResidentsWithDrift:                1,
		ResidentsMissingInventory:         1,
		PendingChatResidents:              1,
		OpenTicketResidents:               1,
		InterventionCount:                 1,
		MemoryResidentsAttention:          1,
		MemoryItemsAttention:              2,
		MemoryMaintenanceResidents:        1,
		MemoryDuplicateHistoryGroups:      3,
		RuntimeMemoryObservationResidents: 1,
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID:             "orchestrator-20260616T082449.311075354Z",
			ResidentsErrored:  1,
			BudgetBlockedRuns: 1,
		},
		RecentRunsNeedingAttention: 2,
		ResidentRisk: []ResidentInspectRisk{
			{ResidentID: "amber", DriftFields: []string{"memory"}, HasOpenTicket: true, HasIntervention: true, MemoryAttention: 2, MemoryDuplicateHistoryGroups: 3, MemoryRecommendedAction: "lifecycle_then_compaction_dry_run", OrchestratorBudgetBlocked: true, OrchestratorStoppedReason: "broker_preflight_denied: effective_window_exhausted", NeedsAttention: true, Status: "Running"},
			{ResidentID: "onyx", NeedsAttention: true, Status: "", HostRSSHighGuestUsageLow: true, HostQEMURSSMiB: 2225, IncusMemoryCurrentMiB: 134, GuestMemAvailableMiB: 1806},
		},
	})
	if out.Severity != "high" {
		t.Fatalf("expected high severity, got %#v", out)
	}
	if len(out.Actions) < 4 {
		t.Fatalf("expected multiple suggested actions, got %#v", out)
	}
	if len(out.ResidentFocus) != 2 {
		t.Fatalf("expected two resident focus entries, got %#v", out)
	}
	if len(out.Capacity.Pools) != 1 {
		t.Fatalf("expected capacity to be carried into decision assist: %#v", out)
	}
	if len(out.OperatorOnlyObservations) == 0 {
		t.Fatalf("expected operator-only observations, got %#v", out)
	}
	if len(out.WorldEventCandidates) == 0 {
		t.Fatalf("expected world event candidates, got %#v", out)
	}
	foundMemoryLifecycleAction := false
	foundMemoryCompactionAction := false
	foundOrchestratorBudgetAction := false
	foundOrchestratorFailureAction := false
	foundRuntimeMemoryAction := false
	foundWorldVisibleChatAction := false
	for _, action := range out.Actions {
		if action.Kind == "memory_lifecycle_review" {
			foundMemoryLifecycleAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected memory lifecycle action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "memory_compaction_review" {
			foundMemoryCompactionAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected memory compaction action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "runtime_budget_review" {
			foundOrchestratorBudgetAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected runtime budget action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "orchestrator_failure_review" {
			foundOrchestratorFailureAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected orchestrator failure action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "runtime_memory_observation" {
			foundRuntimeMemoryAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected runtime memory action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "chat_reply" {
			foundWorldVisibleChatAction = true
			if action.Visibility != decisionVisibilityWorldEventCandidate {
				t.Fatalf("expected chat action to be a world event candidate, got %#v", action)
			}
		}
	}
	if !foundMemoryLifecycleAction {
		t.Fatalf("expected memory lifecycle action, got %#v", out.Actions)
	}
	if !foundMemoryCompactionAction {
		t.Fatalf("expected memory compaction action, got %#v", out.Actions)
	}
	if !foundOrchestratorBudgetAction {
		t.Fatalf("expected orchestrator budget action, got %#v", out.Actions)
	}
	if !foundOrchestratorFailureAction {
		t.Fatalf("expected orchestrator failure action, got %#v", out.Actions)
	}
	if !foundRuntimeMemoryAction {
		t.Fatalf("expected runtime memory observation action, got %#v", out.Actions)
	}
	if !foundWorldVisibleChatAction {
		t.Fatalf("expected world-visible chat action, got %#v", out.Actions)
	}
	var onyx ResidentDecisionFocus
	var amber ResidentDecisionFocus
	for _, focus := range out.ResidentFocus {
		if focus.ResidentID == "amber" {
			amber = focus
		}
		if focus.ResidentID == "onyx" {
			onyx = focus
		}
	}
	foundOnyxRuntimeReason := false
	for _, reason := range onyx.OperatorOnlyObservations {
		if strings.Contains(reason, "host QEMU RSS") {
			foundOnyxRuntimeReason = true
		}
	}
	if !foundOnyxRuntimeReason {
		t.Fatalf("expected onyx runtime memory reason, got %#v", onyx)
	}
	foundAmberTicketCandidate := false
	for _, reason := range amber.WorldEventCandidates {
		if strings.Contains(reason, "open ticket") {
			foundAmberTicketCandidate = true
		}
	}
	if !foundAmberTicketCandidate {
		t.Fatalf("expected amber ticket follow-up to be a world event candidate, got %#v", amber)
	}
}
