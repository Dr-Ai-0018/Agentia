package broker

import (
	"strings"
	"testing"
	"time"

	"ai-arena/internal/worldstate"
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
		MemoryOperatorDecayCandidates:     1,
		MemoryResidentReviewQueue:         1,
		MemoryOperatorReviewRequired:      1,
		MemoryDuplicateHistoryGroups:      3,
		RuntimeMemoryObservationResidents: 1,
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID:             "orchestrator-20260616T082449.311075354Z",
			ResidentsErrored:  1,
			BudgetBlockedRuns: 1,
			TransientBlocked:  1,
		},
		RecentRunsNeedingAttention: 2,
		RecentMaintenanceRuns: []MaintenanceRunRecord{
			{ID: "maintenance-1", State: "in_progress", ResidentID: "amber", TicketID: "ticket-1", Resource: "cpu", Amount: "2"},
			{ID: "maintenance-2", State: "completed", ResidentID: "onyx", TicketID: "ticket-2", Resource: "disk", Amount: "20GiB"},
		},
		MaintenanceInterventions: &MaintenanceInterventionSummary{
			Total:      2,
			InProgress: 1,
			Failed:     1,
		},
		ResidentRisk: []ResidentInspectRisk{
			{ResidentID: "amber", DriftFields: []string{"memory"}, HasOpenTicket: true, HasIntervention: true, MemoryAttention: 2, MemoryOperatorDecayCandidates: 1, MemoryResidentReviewQueue: 1, MemoryOperatorReviewRequired: 1, MemoryDuplicateHistoryGroups: 3, MemoryRecommendedAction: "lifecycle_then_compaction_dry_run", OrchestratorBudgetBlocked: true, OrchestratorTransientBlocked: true, OrchestratorStoppedReason: "broker_preflight_denied: effective_window_exhausted", NeedsAttention: true, Status: "Running"},
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
	foundMemorySafeDecayAction := false
	foundMemoryCompactionAction := false
	foundResidentMemoryReviewQueueAction := false
	foundMemoryOperatorReviewAction := false
	foundOrchestratorBudgetAction := false
	foundOrchestratorFailureAction := false
	foundOrchestratorTransientAction := false
	foundRuntimeMemoryAction := false
	foundMaintenanceRunAction := false
	foundMaintenanceStateAction := false
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
		if action.Kind == "memory_safe_decay_review" {
			foundMemorySafeDecayAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected safe decay action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "resident_memory_review_queue" {
			foundResidentMemoryReviewQueueAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected resident memory review queue action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "memory_operator_review" {
			foundMemoryOperatorReviewAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected memory operator review action to be operator-only, got %#v", action)
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
		if action.Kind == "orchestrator_transient_retry_review" {
			foundOrchestratorTransientAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected orchestrator transient action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "runtime_memory_observation" {
			foundRuntimeMemoryAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected runtime memory action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "maintenance_run_review" {
			foundMaintenanceRunAction = true
			if action.Visibility != decisionVisibilityOperatorOnly {
				t.Fatalf("expected maintenance run action to be operator-only, got %#v", action)
			}
		}
		if action.Kind == "maintenance_state_review" {
			foundMaintenanceStateAction = true
			if action.Visibility != decisionVisibilityWorldEventCandidate {
				t.Fatalf("expected maintenance state action to be a world event candidate, got %#v", action)
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
	if !foundMemorySafeDecayAction {
		t.Fatalf("expected memory safe decay action, got %#v", out.Actions)
	}
	if !foundResidentMemoryReviewQueueAction {
		t.Fatalf("expected resident memory review queue action, got %#v", out.Actions)
	}
	if !foundMemoryOperatorReviewAction {
		t.Fatalf("expected memory operator review action, got %#v", out.Actions)
	}
	if !foundOrchestratorBudgetAction {
		t.Fatalf("expected orchestrator budget action, got %#v", out.Actions)
	}
	if !foundOrchestratorFailureAction {
		t.Fatalf("expected orchestrator failure action, got %#v", out.Actions)
	}
	if !foundOrchestratorTransientAction {
		t.Fatalf("expected orchestrator transient retry action, got %#v", out.Actions)
	}
	if !foundRuntimeMemoryAction {
		t.Fatalf("expected runtime memory observation action, got %#v", out.Actions)
	}
	if !foundMaintenanceRunAction {
		t.Fatalf("expected maintenance run review action, got %#v", out.Actions)
	}
	if !foundMaintenanceStateAction {
		t.Fatalf("expected maintenance state review action, got %#v", out.Actions)
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
	foundAmberResidentReviewOperatorOnly := false
	foundAmberTransientOperatorOnly := false
	for _, reason := range amber.WorldEventCandidates {
		if strings.Contains(reason, "open ticket") {
			foundAmberTicketCandidate = true
		}
	}
	for _, reason := range amber.OperatorOnlyObservations {
		if strings.Contains(reason, "resident self-review") {
			foundAmberResidentReviewOperatorOnly = true
		}
		if strings.Contains(reason, "retryable upstream/API") {
			foundAmberTransientOperatorOnly = true
		}
	}
	if !foundAmberTicketCandidate {
		t.Fatalf("expected amber ticket follow-up to be a world event candidate, got %#v", amber)
	}
	if !foundAmberResidentReviewOperatorOnly {
		t.Fatalf("expected amber resident memory self-review to remain operator-only, got %#v", amber)
	}
	if !foundAmberTransientOperatorOnly {
		t.Fatalf("expected amber transient upstream reason to remain operator-only, got %#v", amber)
	}
}

func TestBuildHostDecisionAssistCarriesCategorizedFollowupPreviews(t *testing.T) {
	rawChat := strings.Repeat("resident private long pending chat body ", 8)
	store := worldstate.New(t.TempDir())
	now := time.Date(2026, 6, 17, 14, 0, 0, 0, time.UTC)
	if _, err := store.AppendResidentToChenglin("jade", rawChat, now); err != nil {
		t.Fatalf("append chat: %v", err)
	}
	if _, err := store.CreateResidentTicket("jade", "Need disk", strings.Repeat("ticket body ", 20), worldstate.TicketPriorityMedium, now.Add(time.Second)); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := store.CreateHostIntervention("jade", "maintenance", "Planned maintenance", strings.Repeat("maintenance body ", 20), "chenglin", now.Add(2*time.Second)); err != nil {
		t.Fatalf("create intervention: %v", err)
	}
	inbox, err := store.ReadHostInboxSummary(10, 10)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	followups, err := store.ReadHostFollowups(10)
	if err != nil {
		t.Fatalf("read followups: %v", err)
	}
	summary := SummarizeHostInspect(HostInspectOutput{
		Inventory: InventorySnapshot{CollectedAt: "2026-06-17T14:00:00Z"},
		ResidentFacts: []ResidentRuntimeFact{
			{ResidentID: "jade", Status: "Running"},
		},
		Inbox:     inbox,
		Followups: followups,
	})

	out := BuildHostDecisionAssist(summary)
	if len(out.TopPendingChats) != 1 {
		t.Fatalf("expected categorized pending chat preview in decision assist: %#v", out.TopPendingChats)
	}
	if len(out.TopOpenTickets) != 1 || out.TopOpenTickets[0].Title != "Need disk" {
		t.Fatalf("expected categorized open ticket in decision assist: %#v", out.TopOpenTickets)
	}
	if len(out.TopHostInterventions) != 1 || out.TopHostInterventions[0].Title != "Planned maintenance" {
		t.Fatalf("expected categorized host intervention in decision assist: %#v", out.TopHostInterventions)
	}
	if strings.Contains(strings.Join(out.WorldEventCandidates, "\n"), rawChat) {
		t.Fatalf("world event candidates must not contain raw pending chat body: %#v", out.WorldEventCandidates)
	}
	if out.TopPendingChats[0].Preview == rawChat {
		t.Fatalf("decision assist should carry preview, not raw pending chat body")
	}
}

func TestMaintenanceRunsNeedingReviewUsesLatestRecordPerChain(t *testing.T) {
	records := []MaintenanceRunRecord{
		{
			ID:             "maintenance-20260619T032818Z",
			CreatedAt:      "2026-06-19T03:28:18Z",
			State:          "planned",
			ResidentID:     "amber",
			InterventionID: "host-amber-smoke",
			Resource:       "memory",
			Amount:         "smoke-noop",
		},
		{
			ID:             "maintenance-20260619T032826Z",
			CreatedAt:      "2026-06-19T03:28:26Z",
			State:          "in_progress",
			ResidentID:     "amber",
			InterventionID: "host-amber-smoke",
			Resource:       "memory",
			Amount:         "smoke-noop",
		},
		{
			ID:                 "maintenance-20260619T032832Z",
			CreatedAt:          "2026-06-19T03:28:32Z",
			State:              "completed",
			ResidentID:         "amber",
			InterventionID:     "host-amber-smoke",
			Resource:           "memory",
			Amount:             "smoke-noop",
			InventoryRefreshed: true,
		},
		{
			ID:             "maintenance-20260619T033000Z",
			CreatedAt:      "2026-06-19T03:30:00Z",
			State:          "completed",
			ResidentID:     "jade",
			InterventionID: "host-jade-stale",
			Resource:       "disk",
			Amount:         "36GiB",
		},
	}

	count, stale := maintenanceRunsNeedingReview(records)
	if count != 1 || stale != 1 {
		t.Fatalf("expected only latest stale completed chain to need review, got count=%d stale=%d", count, stale)
	}
}

func TestSortDecisionAssistUsesStableActionOrder(t *testing.T) {
	out := HostDecisionAssist{
		Actions: []HostSuggestedAction{
			{Kind: "chat_reply", Priority: "medium", Visibility: decisionVisibilityWorldEventCandidate},
			{Kind: "resident_memory_review_queue", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "runtime_budget_review", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "inventory_check", Priority: "high", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "ticket_review", Priority: "medium", Visibility: decisionVisibilityWorldEventCandidate},
			{Kind: "maintenance_run_review", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "maintenance_state_review", Priority: "medium", Visibility: decisionVisibilityWorldEventCandidate},
			{Kind: "capacity_review", Priority: "high", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "memory_operator_review", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "orchestrator_failure_review", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
			{Kind: "intervention_followup", Priority: "medium", Visibility: decisionVisibilityWorldEventCandidate},
			{Kind: "drift_review", Priority: "medium", Visibility: decisionVisibilityOperatorOnly},
		},
	}

	sortDecisionAssist(&out)

	got := make([]string, 0, len(out.Actions))
	for _, action := range out.Actions {
		got = append(got, action.Kind)
	}
	want := []string{
		"capacity_review",
		"inventory_check",
		"drift_review",
		"orchestrator_failure_review",
		"runtime_budget_review",
		"memory_operator_review",
		"resident_memory_review_queue",
		"ticket_review",
		"maintenance_run_review",
		"maintenance_state_review",
		"intervention_followup",
		"chat_reply",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected action order:\n got %v\nwant %v", got, want)
	}
}
