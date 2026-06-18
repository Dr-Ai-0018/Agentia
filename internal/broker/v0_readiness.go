package broker

import (
	"fmt"
	"strings"
	"time"
)

const (
	v0ReadinessPass = "pass"
	v0ReadinessWarn = "warn"
	v0ReadinessFail = "fail"
)

func (a *App) RunV0Readiness(limit int) (V0ReadinessOutput, error) {
	summary, err := a.RunHostInspectSummary(limit)
	if err != nil {
		return V0ReadinessOutput{}, err
	}
	return BuildV0Readiness(summary, time.Now().UTC(), "live"), nil
}

func (a *App) RunV0ReadinessFromSnapshot(limit int) (V0ReadinessOutput, error) {
	summary, err := a.RunHostInspectSummaryFromSnapshot(limit)
	if err != nil {
		return V0ReadinessOutput{}, err
	}
	return BuildV0Readiness(summary, time.Now().UTC(), "cached"), nil
}

func BuildV0Readiness(summary HostInspectSummary, now time.Time, source string) V0ReadinessOutput {
	decision := BuildHostDecisionAssist(summary)
	draft := BuildHostMaintenanceDraft(decision, now, true)
	out := V0ReadinessOutput{
		GeneratedAt: now.Format(time.RFC3339),
		Source:      strings.TrimSpace(source),
	}
	if out.Source == "" {
		out.Source = "summary"
	}
	addReadinessItem(&out, inventoryReadiness(summary))
	addReadinessItem(&out, capacityReadiness(summary))
	addReadinessItem(&out, orchestratorReadiness(summary))
	addReadinessItem(&out, followupReadiness(summary))
	addReadinessItem(&out, maintenanceReadiness(summary))
	addReadinessItem(&out, memoryReadiness(summary))
	addReadinessItem(&out, decisionBoundaryReadiness(decision))
	addReadinessItem(&out, maintenanceDraftReadiness(draft))
	addReadinessItem(&out, knownManualGapReadiness())
	out.Completion = buildV0Completion(out.Items)
	finalizeV0Readiness(&out)
	return out
}

func addReadinessItem(out *V0ReadinessOutput, item V0ReadinessItem) {
	out.Items = append(out.Items, item)
	switch item.Status {
	case v0ReadinessPass:
		out.Passed++
	case v0ReadinessWarn:
		out.Warnings++
	case v0ReadinessFail:
		out.Failed++
	}
}

func inventoryReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "inventory_facts", Title: "Inventory and resident facts are available", Required: true}
	if summary.ResidentsMissingInventory > 0 {
		item.Status = v0ReadinessFail
		item.Evidence = append(item.Evidence, fmt.Sprintf("%d residents are missing inventory facts", summary.ResidentsMissingInventory))
		return item
	}
	item.Status = v0ReadinessPass
	item.Evidence = append(item.Evidence, fmt.Sprintf("%d residents inspected; %d running", summary.ResidentCount, summary.ResidentsRunning))
	if summary.ResidentsWithDrift > 0 {
		item.Status = v0ReadinessWarn
		item.Evidence = append(item.Evidence, fmt.Sprintf("%d residents have resource drift versus configured baseline", summary.ResidentsWithDrift))
	}
	return item
}

func capacityReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "capacity_headroom", Title: "Host allocatable capacity has safe headroom", Required: true, Status: v0ReadinessPass}
	if len(summary.Capacity.Pools) == 0 {
		item.Status = v0ReadinessFail
		item.Evidence = append(item.Evidence, "capacity pools are missing")
		return item
	}
	for _, pool := range summary.Capacity.Pools {
		item.Evidence = append(item.Evidence, fmt.Sprintf("%s free=%d%s total=%d%s", pool.Resource, pool.AllocatableFree, pool.Unit, pool.AllocatableTotal, pool.Unit))
		if pool.AllocatableTotal <= 0 || pool.AllocatableFree <= 0 {
			item.Status = v0ReadinessFail
			continue
		}
		if pool.AllocatableFree*100 < pool.AllocatableTotal*10 && item.Status != v0ReadinessFail {
			item.Status = v0ReadinessWarn
		}
	}
	return item
}

func orchestratorReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "orchestrator_registry", Title: "Orchestrator run registry and latest report are readable", Required: true}
	if summary.LatestOrchestrator == nil {
		item.Status = v0ReadinessFail
		item.Evidence = append(item.Evidence, "latest orchestrator report is missing")
		return item
	}
	item.Status = v0ReadinessPass
	item.Evidence = append(item.Evidence, fmt.Sprintf("latest run %s", summary.LatestOrchestrator.RunID))
	if summary.LatestOrchestrator.ResidentsErrored > 0 || summary.LatestOrchestrator.BudgetBlockedRuns > 0 || summary.RecentRunsNeedingAttention > 0 {
		item.Status = v0ReadinessWarn
		item.Evidence = append(item.Evidence, fmt.Sprintf("recent runs needing attention=%d latest errors=%d latest budget_blocked=%d", summary.RecentRunsNeedingAttention, summary.LatestOrchestrator.ResidentsErrored, summary.LatestOrchestrator.BudgetBlockedRuns))
	}
	return item
}

func followupReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "world_followups", Title: "World-facing followups are visible and bounded", Required: false, Status: v0ReadinessPass}
	total := summary.PendingChatResidents + summary.OpenTicketResidents + summary.InterventionCount
	item.Evidence = append(item.Evidence, fmt.Sprintf("pending_chat_residents=%d open_ticket_residents=%d host_interventions=%d", summary.PendingChatResidents, summary.OpenTicketResidents, summary.InterventionCount))
	if total > 0 {
		item.Status = v0ReadinessWarn
	}
	return item
}

func maintenanceReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "maintenance_state", Title: "Maintenance state is auditable", Required: true, Status: v0ReadinessPass}
	if summary.MaintenanceInterventions != nil {
		m := summary.MaintenanceInterventions
		item.Evidence = append(item.Evidence, fmt.Sprintf("interventions total=%d planned=%d in_progress=%d failed=%d rolled_back=%d unknown=%d", m.Total, m.Planned, m.InProgress, m.Failed, m.RolledBack, m.Unknown))
		if m.InProgress > 0 || m.Failed > 0 || m.RolledBack > 0 || m.Unknown > 0 {
			item.Status = v0ReadinessWarn
		}
	}
	count, stale := maintenanceRunsNeedingReview(summary.RecentMaintenanceRuns)
	item.Evidence = append(item.Evidence, fmt.Sprintf("recent maintenance runs=%d need_review=%d stale_inventory=%d", len(summary.RecentMaintenanceRuns), count, stale))
	if count > 0 {
		item.Status = v0ReadinessWarn
	}
	return item
}

func memoryReadiness(summary HostInspectSummary) V0ReadinessItem {
	item := V0ReadinessItem{ID: "memory_governance", Title: "Memory governance boundaries are explicit", Required: true, Status: v0ReadinessPass}
	item.Evidence = append(item.Evidence, fmt.Sprintf("attention=%d operator_decay=%d resident_review=%d operator_review=%d duplicates=%d", summary.MemoryItemsAttention, summary.MemoryOperatorDecayCandidates, summary.MemoryResidentReviewQueue, summary.MemoryOperatorReviewRequired, summary.MemoryDuplicateHistoryGroups))
	if summary.MemoryDuplicateHistoryGroups > 0 {
		item.Status = v0ReadinessFail
		return item
	}
	if summary.MemoryItemsAttention > 0 || summary.MemoryOperatorDecayCandidates > 0 || summary.MemoryResidentReviewQueue > 0 || summary.MemoryOperatorReviewRequired > 0 {
		item.Status = v0ReadinessWarn
	}
	return item
}

func decisionBoundaryReadiness(decision HostDecisionAssist) V0ReadinessItem {
	item := V0ReadinessItem{ID: "decision_boundaries", Title: "Host decision separates operator-only and world-facing outputs", Required: true, Status: v0ReadinessPass}
	for _, action := range decision.Actions {
		if strings.HasPrefix(action.Kind, "memory_") || action.Kind == "resident_memory_review_queue" || action.Kind == "maintenance_run_review" {
			if action.Visibility != decisionVisibilityOperatorOnly {
				item.Status = v0ReadinessFail
				item.Evidence = append(item.Evidence, fmt.Sprintf("%s should be operator-only but is %s", action.Kind, action.Visibility))
			}
		}
	}
	if item.Status == v0ReadinessPass {
		item.Evidence = append(item.Evidence, fmt.Sprintf("%d actions checked", len(decision.Actions)))
	}
	return item
}

func maintenanceDraftReadiness(draft HostMaintenanceDraftOutput) V0ReadinessItem {
	item := V0ReadinessItem{ID: "maintenance_draft_safety", Title: "Maintenance draft is side-effect free", Required: true, Status: v0ReadinessPass}
	item.Evidence = append(item.Evidence, fmt.Sprintf("apply=%v drafts=%d", draft.Apply, len(draft.Drafts)))
	if draft.Apply {
		item.Status = v0ReadinessFail
	}
	return item
}

func knownManualGapReadiness() V0ReadinessItem {
	return V0ReadinessItem{
		ID:       "known_manual_gaps",
		Title:    "Known v0 manual validation gaps are explicit",
		Status:   v0ReadinessWarn,
		Required: false,
		Evidence: []string{
			"CPU and disk maintenance-style real regressions still require approved maintenance windows.",
			"checkpoint cleanup apply is intentionally not run without explicit approval.",
			"operator runbook and final acceptance checklist still need final docs pass.",
		},
	}
}

func finalizeV0Readiness(out *V0ReadinessOutput) {
	switch {
	case out.Failed > 0:
		out.Status = "not_ready"
	case out.Warnings > 0:
		out.Status = "ready_with_warnings"
	default:
		out.Status = "ready"
	}
	for _, item := range out.Items {
		if item.Status == v0ReadinessFail || item.Status == v0ReadinessWarn {
			out.NextActions = append(out.NextActions, fmt.Sprintf("%s: %s", item.ID, item.Title))
		}
	}
}

func buildV0Completion(items []V0ReadinessItem) V0CompletionSummary {
	byID := map[string]V0ReadinessItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	workstreams := []V0WorkstreamProgress{
		v0FoundationProgress(byID),
		v0OrchestratorProgress(byID),
		v0HostDecisionProgress(byID),
		v0MemoryProgress(byID),
		v0FinalAcceptanceProgress(byID),
	}
	weighted := 0
	totalWeight := 0
	for _, item := range workstreams {
		weighted += item.Weight * item.Percent
		totalWeight += item.Weight
	}
	if totalWeight > 0 {
		weighted = (weighted + totalWeight/2) / totalWeight
	}
	checklist := foldedReadinessPercent(items)
	summary := V0CompletionSummary{
		WeightedPercent:  weighted,
		WeightedRange:    fmt.Sprintf("%d-%d%%", clampInt(weighted-2, 0, 100), clampInt(weighted+2, 0, 100)),
		ChecklistPercent: checklist,
		ReleaseGate:      "ready_with_warnings",
		Workstreams:      workstreams,
	}
	for _, item := range items {
		if item.ID == "known_manual_gaps" {
			summary.ManualValidationGaps = append(summary.ManualValidationGaps, item.Evidence...)
		}
		if item.Status == v0ReadinessFail {
			summary.ReleaseGate = "blocked_by_required_failures"
		}
	}
	if summary.ReleaseGate != "blocked_by_required_failures" && weighted >= 90 {
		summary.ReleaseGate = "release_candidate"
	}
	if summary.ReleaseGate != "blocked_by_required_failures" && weighted < 80 {
		summary.ReleaseGate = "continue_development"
	}
	summary.CompletionNotes = append(summary.CompletionNotes,
		"weighted_percent uses workstream weights, not raw checklist item count",
		"manual CPU/disk/checkpoint apply validations are intentionally not auto-run by readiness",
	)
	return summary
}

func v0FoundationProgress(items map[string]V0ReadinessItem) V0WorkstreamProgress {
	progress := V0WorkstreamProgress{
		ID:       "s0_s2_foundation",
		Title:    "Broker state, host control, inventory, resource maintenance foundation",
		Weight:   30,
		Percent:  82,
		Status:   "mostly_complete",
		Evidence: evidenceFor(items, "inventory_facts", "capacity_headroom", "maintenance_state"),
	}
	if hasFailure(items, "inventory_facts", "capacity_headroom") {
		progress.Percent = 55
		progress.Status = "blocked"
		return progress
	}
	if hasWarning(items, "inventory_facts", "capacity_headroom", "maintenance_state") {
		progress.Percent = 78
		progress.Status = "needs_validation"
	}
	return progress
}

func v0OrchestratorProgress(items map[string]V0ReadinessItem) V0WorkstreamProgress {
	progress := V0WorkstreamProgress{
		ID:       "s3_orchestrator",
		Title:    "Multi-resident runtime, pause/resume, run registry and reports",
		Weight:   20,
		Percent:  88,
		Status:   "mostly_complete",
		Evidence: evidenceFor(items, "orchestrator_registry"),
	}
	if hasFailure(items, "orchestrator_registry") {
		progress.Percent = 45
		progress.Status = "blocked"
		return progress
	}
	if hasWarning(items, "orchestrator_registry") {
		progress.Percent = 82
		progress.Status = "needs_soak"
	}
	return progress
}

func v0HostDecisionProgress(items map[string]V0ReadinessItem) V0WorkstreamProgress {
	progress := V0WorkstreamProgress{
		ID:       "s4_host_decision",
		Title:    "Host inspect, decision assist, maintenance draft and world/operator boundary",
		Weight:   18,
		Percent:  76,
		Status:   "usable_manual_assist",
		Evidence: evidenceFor(items, "world_followups", "decision_boundaries", "maintenance_draft_safety"),
	}
	if hasFailure(items, "decision_boundaries", "maintenance_draft_safety") {
		progress.Percent = 45
		progress.Status = "blocked"
		return progress
	}
	if hasWarning(items, "world_followups") {
		progress.Percent = 74
		progress.Status = "followups_pending"
	}
	return progress
}

func v0MemoryProgress(items map[string]V0ReadinessItem) V0WorkstreamProgress {
	progress := V0WorkstreamProgress{
		ID:       "s5_memory",
		Title:    "Memory governance, compaction, lifecycle and private/operator separation",
		Weight:   16,
		Percent:  90,
		Status:   "mostly_complete",
		Evidence: evidenceFor(items, "memory_governance"),
	}
	if hasFailure(items, "memory_governance") {
		progress.Percent = 55
		progress.Status = "blocked"
		return progress
	}
	if hasWarning(items, "memory_governance") {
		progress.Percent = 82
		progress.Status = "review_queue_pending"
	}
	return progress
}

func v0FinalAcceptanceProgress(items map[string]V0ReadinessItem) V0WorkstreamProgress {
	progress := V0WorkstreamProgress{
		ID:       "s6_acceptance",
		Title:    "Final runbook, end-to-end regression and release declaration",
		Weight:   16,
		Percent:  35,
		Status:   "not_closed",
		Evidence: evidenceFor(items, "known_manual_gaps"),
	}
	if hasFailure(items, "inventory_facts", "capacity_headroom", "orchestrator_registry", "memory_governance", "decision_boundaries", "maintenance_draft_safety") {
		progress.Percent = 20
		progress.Status = "blocked"
		return progress
	}
	if !hasWarning(items, "known_manual_gaps") {
		progress.Percent = 80
		progress.Status = "ready_for_final_review"
	}
	return progress
}

func foldedReadinessPercent(items []V0ReadinessItem) int {
	if len(items) == 0 {
		return 0
	}
	score := 0
	for _, item := range items {
		switch item.Status {
		case v0ReadinessPass:
			score += 100
		case v0ReadinessWarn:
			score += 50
		}
	}
	return (score + len(items)/2) / len(items)
}

func hasFailure(items map[string]V0ReadinessItem, ids ...string) bool {
	for _, id := range ids {
		if items[id].Status == v0ReadinessFail {
			return true
		}
	}
	return false
}

func hasWarning(items map[string]V0ReadinessItem, ids ...string) bool {
	for _, id := range ids {
		if items[id].Status == v0ReadinessWarn {
			return true
		}
	}
	return false
}

func evidenceFor(items map[string]V0ReadinessItem, ids ...string) []string {
	var evidence []string
	for _, id := range ids {
		item, ok := items[id]
		if !ok {
			continue
		}
		prefix := item.ID + "=" + item.Status
		if len(item.Evidence) == 0 {
			evidence = append(evidence, prefix)
			continue
		}
		evidence = append(evidence, prefix+": "+strings.Join(item.Evidence, "; "))
	}
	return evidence
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
