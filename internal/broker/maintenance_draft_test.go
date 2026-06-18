package broker

import (
	"testing"
	"time"

	"ai-arena/internal/worldstate"
)

func TestBuildHostMaintenanceDraftIsDryRunOnly(t *testing.T) {
	decision := BuildHostDecisionAssist(HostInspectSummary{
		CollectedAt:                       "2026-06-17T12:00:00Z",
		Capacity:                          HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 4, AllocatableFree: 0, Unit: "vcpu"}}},
		RuntimeMemoryObservationResidents: 1,
		MemoryResidentReviewQueue:         3,
		PendingChatResidents:              1,
		TopPendingChats: []worldstate.HostFollowup{
			{Kind: "chat_reply", Resident: "onyx", TargetID: "msg-1", Preview: "pending chat preview", Status: worldstate.StatusPending},
		},
		TopHostInterventions: []worldstate.HostFollowup{
			{Kind: "host_intervention", Resident: "onyx", TargetID: "host-1", Title: "Planned maintenance", Preview: "maintenance preview", Status: "in_progress", Maintenance: map[string]string{"maintenance_state": "in_progress"}},
		},
		RecentMaintenanceRuns: []MaintenanceRunRecord{
			{ID: "maintenance-1", State: "in_progress", ResidentID: "onyx", TicketID: "ticket-1", Resource: "cpu", Amount: "2"},
		},
		MaintenanceInterventions: &MaintenanceInterventionSummary{
			Total:      2,
			InProgress: 1,
			Unknown:    1,
		},
		ResidentRisk: []ResidentInspectRisk{
			{
				ResidentID:               "onyx",
				Status:                   "Running",
				HostRSSHighGuestUsageLow: true,
				HostQEMURSSMiB:           2225,
				IncusMemoryCurrentMiB:    221,
				GuestMemAvailableMiB:     1712,
				HasPendingChat:           true,
				NeedsAttention:           true,
			},
			{
				ResidentID:                "jade",
				Status:                    "Running",
				MemoryResidentReviewQueue: 3,
				NeedsAttention:            true,
			},
		},
	})

	out := BuildHostMaintenanceDraft(decision, time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC), true)
	if out.Apply {
		t.Fatalf("draft output must remain dry-run only even when apply=true: %#v", out)
	}
	if len(out.Drafts) == 0 {
		t.Fatalf("expected maintenance drafts, got %#v", out)
	}
	if out.Policy == "" {
		t.Fatalf("expected explicit policy")
	}

	runtimeDraft := findDraft(out.Drafts, "runtime_memory_observation")
	if runtimeDraft.ID == "" {
		t.Fatalf("expected runtime memory draft: %#v", out.Drafts)
	}
	if !runtimeDraft.ManualOnly || !runtimeDraft.RequiresMaintenanceWindow || !runtimeDraft.RequiresStopStart {
		t.Fatalf("runtime draft must require manual maintenance window and stop/start: %#v", runtimeDraft)
	}
	if len(runtimeDraft.ResidentIDs) != 1 || runtimeDraft.ResidentIDs[0] != "onyx" {
		t.Fatalf("expected onyx runtime draft focus, got %#v", runtimeDraft.ResidentIDs)
	}

	memoryDraft := findDraft(out.Drafts, "resident_memory_review_queue")
	if memoryDraft.ID == "" {
		t.Fatalf("expected resident memory review queue draft: %#v", out.Drafts)
	}
	if memoryDraft.RequiresStopStart || memoryDraft.RequiresMaintenanceWindow {
		t.Fatalf("resident memory self-review should not require stop/start maintenance: %#v", memoryDraft)
	}
	if memoryDraft.Visibility != decisionVisibilityOperatorOnly {
		t.Fatalf("resident memory review queue must remain operator-only: %#v", memoryDraft)
	}
	if len(memoryDraft.ResidentIDs) != 1 || memoryDraft.ResidentIDs[0] != "jade" {
		t.Fatalf("expected jade memory draft focus, got %#v", memoryDraft.ResidentIDs)
	}
	if len(memoryDraft.RelatedPendingChats) != 0 || len(memoryDraft.RelatedHostInterventions) != 0 {
		t.Fatalf("operator-only memory draft should not carry world followup context: %#v", memoryDraft)
	}

	runDraft := findDraft(out.Drafts, "maintenance_run_review")
	if runDraft.ID == "" {
		t.Fatalf("expected maintenance run review draft: %#v", out.Drafts)
	}
	if runDraft.Kind != "maintenance_run_review" || runDraft.Visibility != decisionVisibilityOperatorOnly {
		t.Fatalf("unexpected maintenance run draft classification: %#v", runDraft)
	}
	if runDraft.RequiresMaintenanceWindow || runDraft.RequiresStopStart {
		t.Fatalf("maintenance run review should not require maintenance operation by itself: %#v", runDraft)
	}
	if len(runDraft.RelatedMaintenanceRuns) != 1 || runDraft.RelatedMaintenanceRuns[0].ID != "maintenance-1" {
		t.Fatalf("expected maintenance run draft context: %#v", runDraft.RelatedMaintenanceRuns)
	}

	maintenanceDraft := findDraft(out.Drafts, "maintenance_state_review")
	if maintenanceDraft.ID == "" {
		t.Fatalf("expected maintenance state draft: %#v", out.Drafts)
	}
	if maintenanceDraft.Kind != "maintenance_intervention_followup" {
		t.Fatalf("unexpected maintenance state draft kind: %#v", maintenanceDraft)
	}
	if !maintenanceDraft.ManualOnly || !maintenanceDraft.RequiresMaintenanceWindow || maintenanceDraft.RequiresStopStart {
		t.Fatalf("maintenance state draft should be manual window review without stop/start by itself: %#v", maintenanceDraft)
	}
	if maintenanceDraft.Visibility != decisionVisibilityWorldEventCandidate {
		t.Fatalf("maintenance state draft should remain a world event candidate: %#v", maintenanceDraft)
	}
	if maintenanceDraft.Reason == "" {
		t.Fatalf("expected maintenance state draft reason: %#v", maintenanceDraft)
	}
	if len(maintenanceDraft.RelatedHostInterventions) != 1 || maintenanceDraft.RelatedHostInterventions[0].TargetID != "host-1" {
		t.Fatalf("expected maintenance draft to carry related intervention context: %#v", maintenanceDraft.RelatedHostInterventions)
	}

	chatDraft := findDraft(out.Drafts, "chat_reply")
	if chatDraft.ID == "" {
		t.Fatalf("expected chat draft: %#v", out.Drafts)
	}
	if chatDraft.RequiresMaintenanceWindow || chatDraft.RequiresStopStart {
		t.Fatalf("chat follow-up should not be a maintenance operation: %#v", chatDraft)
	}
	if len(chatDraft.RelatedPendingChats) != 1 || chatDraft.RelatedPendingChats[0].Preview != "pending chat preview" {
		t.Fatalf("expected chat draft to carry related pending chat preview: %#v", chatDraft.RelatedPendingChats)
	}
}

func findDraft(items []HostMaintenanceDraft, sourceAction string) HostMaintenanceDraft {
	for _, item := range items {
		if item.SourceActionKind == sourceAction {
			return item
		}
	}
	return HostMaintenanceDraft{}
}
