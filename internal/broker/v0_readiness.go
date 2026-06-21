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
	evidence, err := LoadRecentV0AcceptanceEvidence(a.root, 64)
	if err != nil {
		return V0ReadinessOutput{}, err
	}
	return BuildV0ReadinessWithEvidence(summary, time.Now().UTC(), "live", evidence), nil
}

func (a *App) RunV0ReadinessFromSnapshot(limit int) (V0ReadinessOutput, error) {
	summary, err := a.RunHostInspectSummaryFromSnapshot(limit)
	if err != nil {
		return V0ReadinessOutput{}, err
	}
	evidence, err := LoadRecentV0AcceptanceEvidence(a.root, 64)
	if err != nil {
		return V0ReadinessOutput{}, err
	}
	return BuildV0ReadinessWithEvidence(summary, time.Now().UTC(), "cached", evidence), nil
}

func BuildV0Readiness(summary HostInspectSummary, now time.Time, source string) V0ReadinessOutput {
	return BuildV0ReadinessWithEvidence(summary, now, source, nil)
}

func BuildV0ReadinessWithEvidence(summary HostInspectSummary, now time.Time, source string, evidence []V0AcceptanceEvidenceRecord) V0ReadinessOutput {
	evidenceByCheck := latestPassingV0AcceptanceEvidence(evidence)
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
	addReadinessItem(&out, knownManualGapReadiness(evidenceByCheck))
	out.Completion = buildV0Completion(out.Items, evidenceByCheck)
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
	if summary.LatestOrchestrator.ResidentsErrored > 0 || summary.LatestOrchestrator.BudgetBlockedRuns > 0 || summary.LatestOrchestrator.TransientBlocked > 0 || summary.RecentRunsNeedingAttention > 0 {
		item.Status = v0ReadinessWarn
		item.Evidence = append(item.Evidence, fmt.Sprintf("recent runs needing attention=%d latest errors=%d latest budget_blocked=%d latest_transient_blocked=%d", summary.RecentRunsNeedingAttention, summary.LatestOrchestrator.ResidentsErrored, summary.LatestOrchestrator.BudgetBlockedRuns, summary.LatestOrchestrator.TransientBlocked))
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
	hostActionable := summary.MemoryOperatorDecayCandidates + summary.MemoryOperatorReviewRequired + summary.MemoryDuplicateHistoryGroups
	residentOwned := summary.MemoryResidentReviewQueue + summary.MemoryStaleReviewItems
	item.Evidence = append(item.Evidence,
		fmt.Sprintf("attention=%d operator_decay=%d resident_review=%d operator_review=%d stale_review=%d duplicates=%d", summary.MemoryItemsAttention, summary.MemoryOperatorDecayCandidates, summary.MemoryResidentReviewQueue, summary.MemoryOperatorReviewRequired, summary.MemoryStaleReviewItems, summary.MemoryDuplicateHistoryGroups),
		fmt.Sprintf("host_actionable=%d resident_owned=%d", hostActionable, residentOwned),
	)
	if summary.MemoryDuplicateHistoryGroups > 0 || summary.MemoryOperatorReviewRequired > 0 {
		item.Status = v0ReadinessFail
		item.Evidence = append(item.Evidence, "operator-owned memory governance work must be resolved by safe maintenance or explicit review before release")
		return item
	}
	if summary.MemoryItemsAttention > 0 || summary.MemoryOperatorDecayCandidates > 0 || summary.MemoryResidentReviewQueue > 0 || summary.MemoryStaleReviewItems > 0 {
		item.Status = v0ReadinessWarn
		if hostActionable == 0 && residentOwned > 0 {
			item.Evidence = append(item.Evidence, "remaining warning is resident-owned self-review or stale retain visibility; host must not rewrite protected resident memories")
		}
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

type v0ManualValidationGap struct {
	ID               string
	Title            string
	Reason           string
	Command          string
	RequiresApproval bool
	BlocksRelease    bool
}

func v0ManualValidationGaps() []v0ManualValidationGap {
	return []v0ManualValidationGap{
		{
			ID:               "host_only_maintenance_smoke",
			Title:            "Run host-only maintenance notice lifecycle smoke test",
			Reason:           "Host-only maintenance should prove plan/start/fail-or-complete notices, intervention status updates, and maintenance run records without requiring a resident ticket or real VM resource change.",
			Command:          "arena-broker --mode host-plan-maintenance --resident amber --resource memory --amount smoke-noop --window '<test window>' --operator <operator> --body '<smoke notice>'; then host-start-maintenance and host-fail-maintenance for the returned intervention id",
			RequiresApproval: true,
		},
		{
			ID:               "cpu_maintenance_regression",
			Title:            "Run CPU maintenance-style regression with an approved window",
			Reason:           "CPU maintenance must prove host-plan/start/complete or rollback, approved window, stop/change/start if needed, resident-facing notices, maintenance run record, and inventory refresh.",
			Command:          "arena-broker --mode v0-runbook",
			RequiresApproval: true,
			BlocksRelease:    true,
		},
		{
			ID:               "disk_maintenance_regression",
			Title:            "Run disk maintenance-style regression with an approved window",
			Reason:           "Disk maintenance must prove host-plan/start/complete or rollback, approved window, stop/change/start if needed, resident-facing notices, maintenance run record, and inventory refresh.",
			Command:          "arena-broker --mode v0-runbook",
			RequiresApproval: true,
			BlocksRelease:    true,
		},
		{
			ID:               "checkpoint_cleanup_apply_regression",
			Title:            "Run checkpoint cleanup apply regression after dry-run review",
			Reason:           "Checkpoint cleanup apply is intentionally not automated and must prove protected baseline/self snapshots are preserved.",
			Command:          "arena-broker --mode checkpoint-cleanup --resident jade --keep 2",
			RequiresApproval: true,
			BlocksRelease:    true,
		},
		{
			ID:               "final_acceptance_manual_pass",
			Title:            "Perform final manual acceptance pass after validation gaps close",
			Reason:           "v0 should only be declared after automatic checks pass and approved manual validation evidence is recorded.",
			Command:          "arena-broker --mode v0-acceptance-cached --limit 5",
			RequiresApproval: true,
			BlocksRelease:    true,
		},
	}
}

func knownManualGapReadiness(evidenceByCheck map[string]*V0AcceptanceEvidenceRecord) V0ReadinessItem {
	item := V0ReadinessItem{
		ID:       "known_manual_gaps",
		Title:    "Known v0 manual validation gaps are explicit",
		Status:   v0ReadinessWarn,
		Required: false,
	}
	for _, gap := range v0ManualValidationGaps() {
		if evidenceByCheck[gap.ID] != nil {
			continue
		}
		item.Evidence = append(item.Evidence, gap.Reason)
	}
	if len(item.Evidence) == 0 {
		item.Status = v0ReadinessPass
		item.Evidence = append(item.Evidence, "manual validation evidence is recorded for all v0 acceptance checks")
	}
	return item
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

func buildV0Completion(items []V0ReadinessItem, evidenceByCheck map[string]*V0AcceptanceEvidenceRecord) V0CompletionSummary {
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
	summary.RecommendedSteps = buildV0RecommendedSteps(byID, evidenceByCheck)
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

func buildV0RecommendedSteps(items map[string]V0ReadinessItem, evidenceByCheck map[string]*V0AcceptanceEvidenceRecord) []V0RecommendedStep {
	var steps []V0RecommendedStep
	if hasFailure(items, "inventory_facts", "capacity_headroom") {
		steps = append(steps, V0RecommendedStep{
			ID:            "refresh_inventory_and_capacity",
			Title:         "Refresh inventory and capacity facts",
			Reason:        "Required host facts are missing or unsafe; v0 cannot be declared until host view is reliable.",
			Command:       "arena-broker --mode host-inspect-summary --limit 5",
			BlocksRelease: true,
			RelatedItems:  []string{"inventory_facts", "capacity_headroom"},
		})
	}
	if hasFailure(items, "orchestrator_registry") {
		steps = append(steps, V0RecommendedStep{
			ID:            "run_orchestrator_smoke",
			Title:         "Run a fresh orchestrator smoke test",
			Reason:        "The latest orchestrator report is missing, so runtime registry readiness is not proven.",
			Command:       "arena-orchestrator --help",
			BlocksRelease: true,
			RelatedItems:  []string{"orchestrator_registry"},
		})
	}
	if hasFailure(items, "memory_governance") {
		steps = append(steps, V0RecommendedStep{
			ID:            "run_memory_auto_maintenance",
			Title:         "Run safe memory auto-maintenance before release",
			Reason:        "Duplicate history groups or low-risk expired instant memories can bury useful resident memory; safe auto-maintenance handles mechanical cleanup while preserving protected and sensitive memories.",
			Command:       "arena-broker --mode memory-auto-maintain --resident <resident>; review dry-run, then repeat with --apply",
			BlocksRelease: true,
			RelatedItems:  []string{"memory_governance"},
		})
	}
	if hasFailure(items, "decision_boundaries", "maintenance_draft_safety") {
		steps = append(steps, V0RecommendedStep{
			ID:            "fix_operator_world_boundary",
			Title:         "Fix operator-only and world-facing boundary failures",
			Reason:        "Readiness found a required boundary or side-effect safety failure.",
			Command:       "arena-broker --mode host-decision-assist-cached --limit 5",
			BlocksRelease: true,
			RelatedItems:  []string{"decision_boundaries", "maintenance_draft_safety"},
		})
	}
	if hasWarning(items, "orchestrator_registry") {
		step := V0RecommendedStep{
			ID:               "longer_orchestrator_soak",
			Title:            "Run a longer multi-resident orchestrator soak",
			Reason:           "Recent orchestrator runs are readable but still need attention, usually budget-blocked or runtime stop reasons.",
			Command:          "arena-orchestrator --mode run --run-mode parallel --residents jade,amber,onyx --duration 10m",
			RequiresApproval: true,
			RelatedItems:     []string{"orchestrator_registry"},
		}
		if itemEvidenceContains(items["orchestrator_registry"], "latest budget_blocked=") && !itemEvidenceContains(items["orchestrator_registry"], "latest budget_blocked=0") {
			step.Title = "Estimate and issue test allowance before another orchestrator probe"
			step.Reason = "The latest orchestrator run was budget-blocked. During controlled testing, estimate per-resident spark from historical runs, issue the probe allowance per resident, then run a short probe before any full 10m soak."
			step.Command = "arena-broker --mode orchestrator-budget-estimate --limit 8; issue the listed probe_allowance_by_resident commands; confirm work_allowed_now, then arena-orchestrator --mode run --run-mode parallel --residents jade --duration 45s"
		}
		steps = append(steps, step)
	}
	if hasWarning(items, "world_followups") {
		steps = append(steps, V0RecommendedStep{
			ID:           "review_world_followups",
			Title:        "Review pending world-facing followups",
			Reason:       "There are pending chats, tickets, or host interventions that should be handled through in-world boundaries.",
			Command:      "arena-broker --mode host-decision-assist-cached --limit 5",
			RelatedItems: []string{"world_followups"},
		})
	}
	if hasWarning(items, "memory_governance") {
		step := V0RecommendedStep{
			ID:           "review_memory_governance_queue",
			Title:        "Review memory governance queue without leaking operator-only context",
			Reason:       "Memory governance has remaining review work; v0 can continue, but operator-only and resident-owned memory boundaries must stay visible.",
			Command:      "arena-broker --mode memory-maintenance",
			RelatedItems: []string{"memory_governance"},
		}
		if itemEvidenceContains(items["memory_governance"], "host_actionable=0") {
			step.Title = "Review resident-owned memory governance queue"
			step.Reason = "Host-actionable memory cleanup is clear; remaining work is resident self-review or stale retain visibility, so the host must not rewrite protected resident memories."
		}
		steps = append(steps, step)
	}
	if hasWarning(items, "known_manual_gaps") {
		for _, gap := range v0ManualValidationGaps() {
			if evidenceByCheck[gap.ID] != nil {
				continue
			}
			steps = append(steps, V0RecommendedStep{
				ID:               gap.ID,
				Title:            gap.Title,
				Reason:           gap.Reason,
				Command:          gap.Command,
				RequiresApproval: gap.RequiresApproval,
				BlocksRelease:    gap.BlocksRelease,
				RelatedItems:     []string{"known_manual_gaps"},
			})
		}
		steps = append(steps, V0RecommendedStep{
			ID:           "review_operator_runbook",
			Title:        "Review the operator runbook and final acceptance pass",
			Reason:       "v0 needs a clear human operation path for start, long run, inspect, pause, resume, maintenance, rollback, and reports.",
			Command:      "arena-broker --mode v0-runbook",
			RelatedItems: []string{"known_manual_gaps"},
		})
	}
	return steps
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

func itemEvidenceContains(item V0ReadinessItem, needle string) bool {
	for _, evidence := range item.Evidence {
		if strings.Contains(evidence, needle) {
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
