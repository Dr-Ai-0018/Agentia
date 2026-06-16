package broker

import "fmt"

func BuildHostDecisionAssist(summary HostInspectSummary) HostDecisionAssist {
	out := HostDecisionAssist{
		CollectedAt:   summary.CollectedAt,
		InventoryPath: summary.InventoryPath,
		Capacity:      summary.Capacity,
		Severity:      "normal",
		Headline:      "No immediate host action is suggested.",
	}

	for _, pool := range summary.Capacity.Pools {
		if pool.AllocatableTotal <= 0 {
			continue
		}
		if pool.AllocatableFree <= 0 {
			out.Severity = "high"
			out.Reasons = append(out.Reasons, fmt.Sprintf("%s allocatable pool has no free %s after host reserve and resident allocations", pool.Resource, pool.Unit))
			out.Actions = append(out.Actions, HostSuggestedAction{
				Kind:     "capacity_review",
				Priority: "high",
				Summary:  fmt.Sprintf("Review %s pressure before approving new resource requests.", pool.Resource),
			})
			continue
		}
		if pool.AllocatableFree*100 < pool.AllocatableTotal*10 {
			raiseSeverity(&out, "medium")
			out.Reasons = append(out.Reasons, fmt.Sprintf("%s allocatable pool is below 10%% free capacity", pool.Resource))
			out.Actions = append(out.Actions, HostSuggestedAction{
				Kind:     "capacity_review",
				Priority: "medium",
				Summary:  fmt.Sprintf("Treat new %s requests cautiously until host free capacity improves.", pool.Resource),
			})
		}
	}

	if summary.ResidentsMissingInventory > 0 {
		out.Severity = "high"
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d resident facts are missing live inventory state", summary.ResidentsMissingInventory))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "inventory_check",
			Priority: "high",
			Summary:  "Refresh inventory and verify residents missing observed state before making resource decisions.",
		})
	}
	if summary.ResidentsWithDrift > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d residents show resource drift versus configured baseline", summary.ResidentsWithDrift))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "drift_review",
			Priority: "medium",
			Summary:  "Review drifted residents and confirm whether the current CPU/memory/disk shape is intentional.",
		})
	}
	if summary.PendingChatResidents > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d residents are waiting on chat replies", summary.PendingChatResidents))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "chat_reply",
			Priority: "medium",
			Summary:  "Review pending resident chat threads and reply where silence would block clarity.",
		})
	}
	if summary.OpenTicketResidents > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d residents have open tickets", summary.OpenTicketResidents))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "ticket_review",
			Priority: "medium",
			Summary:  "Review open tickets and decide whether they need settlement, maintenance planning, or a follow-up question.",
		})
	}
	if summary.InterventionCount > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d host interventions are still active in the follow-up queue", summary.InterventionCount))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "intervention_followup",
			Priority: "medium",
			Summary:  "Check active host interventions and confirm whether any maintenance or notice chain still needs closure.",
		})
	}
	if summary.MemoryItemsAttention > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d memory items across %d residents need lifecycle attention", summary.MemoryItemsAttention, summary.MemoryResidentsAttention))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "memory_lifecycle_review",
			Priority: "medium",
			Summary:  "Review memory lifecycle reports before enabling automatic decay or deletion.",
		})
	}
	if summary.MemoryDuplicateHistoryGroups > 0 {
		raiseSeverity(&out, "medium")
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d duplicate memory history groups across %d residents can be compacted", summary.MemoryDuplicateHistoryGroups, summary.MemoryMaintenanceResidents))
		out.Actions = append(out.Actions, HostSuggestedAction{
			Kind:     "memory_compaction_review",
			Priority: "medium",
			Summary:  "Run memory compaction dry-runs and apply only when the before/after report is acceptable.",
		})
	}
	if summary.LatestOrchestrator != nil {
		latest := summary.LatestOrchestrator
		if latest.ResidentsErrored > 0 {
			raiseSeverity(&out, "medium")
			out.Reasons = append(out.Reasons, fmt.Sprintf("latest orchestrator run %s has %d errored residents", latest.RunID, latest.ResidentsErrored))
			out.Actions = append(out.Actions, HostSuggestedAction{
				Kind:     "orchestrator_failure_review",
				Priority: "medium",
				Summary:  "Review latest orchestrator errors and use retry-failed only after the failure cause is understood.",
			})
		}
		if latest.BudgetBlockedRuns > 0 {
			raiseSeverity(&out, "medium")
			out.Reasons = append(out.Reasons, fmt.Sprintf("latest orchestrator run %s ended with %d budget-blocked residents", latest.RunID, latest.BudgetBlockedRuns))
			out.Actions = append(out.Actions, HostSuggestedAction{
				Kind:     "runtime_budget_review",
				Priority: "medium",
				Summary:  "Review resident quota, spark, debt, and recovery state before starting another long run.",
			})
		}
	}

	for _, item := range summary.ResidentRisk {
		if !item.NeedsAttention {
			continue
		}
		focus := ResidentDecisionFocus{
			ResidentID: item.ResidentID,
			Priority:   residentPriority(item),
		}
		if item.Status == "" {
			focus.Reasons = append(focus.Reasons, "missing observed inventory state")
		}
		if len(item.DriftFields) > 0 {
			focus.Reasons = append(focus.Reasons, fmt.Sprintf("resource drift: %v", item.DriftFields))
		}
		if item.HasPendingChat {
			focus.Reasons = append(focus.Reasons, "pending resident chat reply")
		}
		if item.HasOpenTicket {
			focus.Reasons = append(focus.Reasons, "open ticket needs review")
		}
		if item.HasIntervention {
			focus.Reasons = append(focus.Reasons, "active host intervention needs follow-up")
		}
		if item.MemoryAttention > 0 {
			focus.Reasons = append(focus.Reasons, fmt.Sprintf("%d memory items need lifecycle attention", item.MemoryAttention))
		}
		if item.MemoryDuplicateHistoryGroups > 0 {
			focus.Reasons = append(focus.Reasons, fmt.Sprintf("%d duplicate memory history groups can be compacted", item.MemoryDuplicateHistoryGroups))
		}
		if item.OrchestratorBudgetBlocked {
			focus.Reasons = append(focus.Reasons, "latest orchestrator run was budget-blocked")
		}
		if item.OrchestratorStoppedReason != "" {
			focus.Reasons = append(focus.Reasons, fmt.Sprintf("latest orchestrator stopped: %s", item.OrchestratorStoppedReason))
		}
		if item.OrchestratorError != "" {
			focus.Reasons = append(focus.Reasons, fmt.Sprintf("latest orchestrator error: %s", item.OrchestratorError))
		}
		out.ResidentFocus = append(out.ResidentFocus, focus)
	}

	switch out.Severity {
	case "high":
		out.Headline = "Host attention is recommended before further resource or maintenance decisions."
	case "medium":
		out.Headline = "Host follow-up is recommended, but no emergency intervention is implied."
	}
	return out
}

func raiseSeverity(out *HostDecisionAssist, next string) {
	if out == nil {
		return
	}
	order := map[string]int{"normal": 0, "medium": 1, "high": 2}
	if order[next] > order[out.Severity] {
		out.Severity = next
	}
}

func residentPriority(item ResidentInspectRisk) string {
	if item.Status == "" {
		return "high"
	}
	if len(item.DriftFields) > 0 || item.HasIntervention {
		return "medium"
	}
	if item.OrchestratorBudgetBlocked || item.OrchestratorError != "" {
		return "medium"
	}
	return "normal"
}
