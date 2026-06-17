package broker

import (
	"fmt"
	"sort"
)

const (
	decisionVisibilityOperatorOnly        = "operator_only"
	decisionVisibilityWorldEventCandidate = "world_event_candidate"
)

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
			reason := fmt.Sprintf("%s allocatable pool has no free %s after host reserve and resident allocations", pool.Resource, pool.Unit)
			out.Reasons = append(out.Reasons, reason)
			addOperatorAction(&out, "capacity_review", "high", fmt.Sprintf("Review %s pressure before approving new resource requests.", pool.Resource))
			addOperatorObservation(&out, reason)
			continue
		}
		if pool.AllocatableFree*100 < pool.AllocatableTotal*10 {
			raiseSeverity(&out, "medium")
			reason := fmt.Sprintf("%s allocatable pool is below 10%% free capacity", pool.Resource)
			out.Reasons = append(out.Reasons, reason)
			addOperatorAction(&out, "capacity_review", "medium", fmt.Sprintf("Treat new %s requests cautiously until host free capacity improves.", pool.Resource))
			addOperatorObservation(&out, reason)
		}
	}

	if summary.ResidentsMissingInventory > 0 {
		out.Severity = "high"
		reason := fmt.Sprintf("%d resident facts are missing live inventory state", summary.ResidentsMissingInventory)
		out.Reasons = append(out.Reasons, reason)
		addOperatorAction(&out, "inventory_check", "high", "Refresh inventory and verify residents missing observed state before making resource decisions.")
		addOperatorObservation(&out, reason)
	}
	if summary.ResidentsWithDrift > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d residents show resource drift versus configured baseline", summary.ResidentsWithDrift)
		out.Reasons = append(out.Reasons, reason)
		addOperatorAction(&out, "drift_review", "medium", "Review drifted residents and confirm whether the current CPU/memory/disk shape is intentional.")
		addOperatorObservation(&out, reason)
	}
	if summary.PendingChatResidents > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d residents are waiting on chat replies", summary.PendingChatResidents)
		out.Reasons = append(out.Reasons, reason)
		addWorldCandidateAction(&out, "chat_reply", "medium", "Review pending resident chat threads and reply where silence would block clarity.")
		addWorldEventCandidate(&out, reason)
	}
	if summary.OpenTicketResidents > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d residents have open tickets", summary.OpenTicketResidents)
		out.Reasons = append(out.Reasons, reason)
		addWorldCandidateAction(&out, "ticket_review", "medium", "Review open tickets and decide whether they need settlement, maintenance planning, or a follow-up question.")
		addWorldEventCandidate(&out, reason)
	}
	if summary.InterventionCount > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d host interventions are still active in the follow-up queue", summary.InterventionCount)
		out.Reasons = append(out.Reasons, reason)
		addWorldCandidateAction(&out, "intervention_followup", "medium", "Check active host interventions and confirm whether any maintenance or notice chain still needs closure.")
		addWorldEventCandidate(&out, reason)
	}
	if summary.MemoryItemsAttention > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d memory items across %d residents need lifecycle attention", summary.MemoryItemsAttention, summary.MemoryResidentsAttention)
		out.Reasons = append(out.Reasons, reason)
		addOperatorAction(&out, "memory_lifecycle_review", "medium", "Review memory lifecycle reports before enabling automatic decay or deletion.")
		addOperatorObservation(&out, reason)
	}
	if summary.MemoryDuplicateHistoryGroups > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d duplicate memory history groups across %d residents can be compacted", summary.MemoryDuplicateHistoryGroups, summary.MemoryMaintenanceResidents)
		out.Reasons = append(out.Reasons, reason)
		addOperatorAction(&out, "memory_compaction_review", "medium", "Run memory compaction dry-runs and apply only when the before/after report is acceptable.")
		addOperatorObservation(&out, reason)
	}
	if summary.RuntimeMemoryObservationResidents > 0 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d residents have high host QEMU RSS while guest memory usage appears low", summary.RuntimeMemoryObservationResidents)
		out.Reasons = append(out.Reasons, reason)
		addOperatorAction(&out, "runtime_memory_observation", "medium", "Do not treat host QEMU RSS alone as guest pressure; compare Incus state and guest memory before planning any maintenance restart.")
		addOperatorObservation(&out, reason)
	}
	if summary.LatestOrchestrator != nil {
		latest := summary.LatestOrchestrator
		if latest.ResidentsErrored > 0 {
			raiseSeverity(&out, "medium")
			reason := fmt.Sprintf("latest orchestrator run %s has %d errored residents", latest.RunID, latest.ResidentsErrored)
			out.Reasons = append(out.Reasons, reason)
			addOperatorAction(&out, "orchestrator_failure_review", "medium", "Review latest orchestrator errors and use retry-failed only after the failure cause is understood.")
			addOperatorObservation(&out, reason)
		}
		if latest.BudgetBlockedRuns > 0 {
			raiseSeverity(&out, "medium")
			reason := fmt.Sprintf("latest orchestrator run %s ended with %d budget-blocked residents", latest.RunID, latest.BudgetBlockedRuns)
			out.Reasons = append(out.Reasons, reason)
			addOperatorAction(&out, "runtime_budget_review", "medium", "Review resident quota, spark, debt, and recovery state before starting another long run.")
			addOperatorObservation(&out, reason)
		}
	}
	if summary.RecentRunsNeedingAttention > 1 {
		raiseSeverity(&out, "medium")
		reason := fmt.Sprintf("%d recent orchestrator runs need host review", summary.RecentRunsNeedingAttention)
		out.Reasons = append(out.Reasons, reason)
		addOperatorObservation(&out, reason)
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
			addFocusOperatorReason(&focus, "missing observed inventory state")
		}
		if len(item.DriftFields) > 0 {
			addFocusOperatorReason(&focus, fmt.Sprintf("resource drift: %v", item.DriftFields))
		}
		if item.HasPendingChat {
			addFocusWorldCandidateReason(&focus, "pending resident chat reply")
		}
		if item.HasOpenTicket {
			addFocusWorldCandidateReason(&focus, "open ticket needs review")
		}
		if item.HasIntervention {
			addFocusWorldCandidateReason(&focus, "active host intervention needs follow-up")
		}
		if item.MemoryAttention > 0 {
			addFocusOperatorReason(&focus, fmt.Sprintf("%d memory items need lifecycle attention", item.MemoryAttention))
		}
		if item.MemoryDuplicateHistoryGroups > 0 {
			addFocusOperatorReason(&focus, fmt.Sprintf("%d duplicate memory history groups can be compacted", item.MemoryDuplicateHistoryGroups))
		}
		if item.MemoryRecommendedAction != "" {
			addFocusOperatorReason(&focus, fmt.Sprintf("memory recommendation: %s", item.MemoryRecommendedAction))
		}
		if item.HostRSSHighGuestUsageLow {
			addFocusOperatorReason(&focus, fmt.Sprintf("host QEMU RSS %dMiB is high while Incus guest memory is %dMiB and guest available memory is %dMiB; observe unless host memory pressure rises or plan maintenance restart", item.HostQEMURSSMiB, item.IncusMemoryCurrentMiB, item.GuestMemAvailableMiB))
		}
		if item.LiveMetricsError != "" {
			addFocusOperatorReason(&focus, fmt.Sprintf("live runtime metrics incomplete: %s", item.LiveMetricsError))
		}
		if item.OrchestratorBudgetBlocked {
			addFocusOperatorReason(&focus, "latest orchestrator run was budget-blocked")
		}
		if item.OrchestratorStoppedReason != "" {
			addFocusOperatorReason(&focus, fmt.Sprintf("latest orchestrator stopped: %s", item.OrchestratorStoppedReason))
		}
		if item.OrchestratorError != "" {
			addFocusOperatorReason(&focus, fmt.Sprintf("latest orchestrator error: %s", item.OrchestratorError))
		}
		out.ResidentFocus = append(out.ResidentFocus, focus)
	}

	switch out.Severity {
	case "high":
		out.Headline = "Host attention is recommended before further resource or maintenance decisions."
	case "medium":
		out.Headline = "Host follow-up is recommended, but no emergency intervention is implied."
	}
	sortDecisionAssist(&out)
	return out
}

func addOperatorAction(out *HostDecisionAssist, kind, priority, summary string) {
	addSuggestedAction(out, kind, priority, decisionVisibilityOperatorOnly, summary)
}

func addWorldCandidateAction(out *HostDecisionAssist, kind, priority, summary string) {
	addSuggestedAction(out, kind, priority, decisionVisibilityWorldEventCandidate, summary)
}

func addSuggestedAction(out *HostDecisionAssist, kind, priority, visibility, summary string) {
	out.Actions = append(out.Actions, HostSuggestedAction{
		Kind:       kind,
		Priority:   priority,
		Visibility: visibility,
		Summary:    summary,
	})
}

func addOperatorObservation(out *HostDecisionAssist, value string) {
	out.OperatorOnlyObservations = append(out.OperatorOnlyObservations, value)
}

func addWorldEventCandidate(out *HostDecisionAssist, value string) {
	out.WorldEventCandidates = append(out.WorldEventCandidates, value)
}

func addFocusOperatorReason(focus *ResidentDecisionFocus, value string) {
	focus.Reasons = append(focus.Reasons, value)
	focus.OperatorOnlyObservations = append(focus.OperatorOnlyObservations, value)
}

func addFocusWorldCandidateReason(focus *ResidentDecisionFocus, value string) {
	focus.Reasons = append(focus.Reasons, value)
	focus.WorldEventCandidates = append(focus.WorldEventCandidates, value)
}

func sortDecisionAssist(out *HostDecisionAssist) {
	sort.Strings(out.Reasons)
	sort.Strings(out.OperatorOnlyObservations)
	sort.Strings(out.WorldEventCandidates)
	sort.SliceStable(out.Actions, func(i, j int) bool {
		left, right := out.Actions[i], out.Actions[j]
		if priorityRank(left.Priority) != priorityRank(right.Priority) {
			return priorityRank(left.Priority) > priorityRank(right.Priority)
		}
		if left.Visibility != right.Visibility {
			return left.Visibility < right.Visibility
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.ResidentID < right.ResidentID
	})
	sort.SliceStable(out.ResidentFocus, func(i, j int) bool {
		left, right := out.ResidentFocus[i], out.ResidentFocus[j]
		if priorityRank(left.Priority) != priorityRank(right.Priority) {
			return priorityRank(left.Priority) > priorityRank(right.Priority)
		}
		return left.ResidentID < right.ResidentID
	})
	for idx := range out.ResidentFocus {
		sort.Strings(out.ResidentFocus[idx].Reasons)
		sort.Strings(out.ResidentFocus[idx].OperatorOnlyObservations)
		sort.Strings(out.ResidentFocus[idx].WorldEventCandidates)
	}
}

func priorityRank(priority string) int {
	switch priority {
	case "high":
		return 3
	case "medium":
		return 2
	case "normal":
		return 1
	default:
		return 0
	}
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
	if item.HostRSSHighGuestUsageLow || item.LiveMetricsError != "" {
		return "medium"
	}
	return "normal"
}
