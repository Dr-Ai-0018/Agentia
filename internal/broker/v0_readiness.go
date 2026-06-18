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
