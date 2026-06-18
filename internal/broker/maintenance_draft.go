package broker

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/worldstate"
)

const maintenanceDraftPolicy = "draft_only_no_world_or_resource_side_effects: review manually; live mode may refresh inventory, but this command never executes resource/environment changes, creates tickets, sends notices, or updates interventions. Use an explicit maintenance notice, approved window, stop/change/start when needed, completion notice, and inventory refresh before any real change."

func (a *App) RunHostMaintenanceDraft(limit int, apply bool) (HostMaintenanceDraftOutput, error) {
	decision, err := a.RunHostDecisionAssist(limit)
	if err != nil {
		return HostMaintenanceDraftOutput{}, err
	}
	return BuildHostMaintenanceDraft(decision, time.Now().UTC(), apply), nil
}

func (a *App) RunHostMaintenanceDraftFromSnapshot(limit int, apply bool) (HostMaintenanceDraftOutput, error) {
	decision, err := a.RunHostDecisionAssistFromSnapshot(limit)
	if err != nil {
		return HostMaintenanceDraftOutput{}, err
	}
	return BuildHostMaintenanceDraft(decision, time.Now().UTC(), apply), nil
}

func BuildHostMaintenanceDraft(decision HostDecisionAssist, now time.Time, apply bool) HostMaintenanceDraftOutput {
	out := HostMaintenanceDraftOutput{
		GeneratedAt:    now.Format(time.RFC3339),
		Apply:          false,
		Policy:         maintenanceDraftPolicy,
		SourceSeverity: decision.Severity,
		SourceHeadline: decision.Headline,
	}
	for idx, action := range decision.Actions {
		draft, ok := maintenanceDraftForAction(action, decision, idx)
		if ok {
			out.Drafts = append(out.Drafts, draft)
		}
	}
	return out
}

func maintenanceDraftForAction(action HostSuggestedAction, decision HostDecisionAssist, index int) (HostMaintenanceDraft, bool) {
	kind := strings.TrimSpace(action.Kind)
	if kind == "" {
		return HostMaintenanceDraft{}, false
	}
	reason := matchingDecisionReason(kind, decision)
	draft := HostMaintenanceDraft{
		ID:                        fmt.Sprintf("draft-%02d-%s", index+1, kind),
		Kind:                      draftKind(kind),
		Priority:                  action.Priority,
		Visibility:                action.Visibility,
		Title:                     draftTitle(kind),
		Reason:                    reason,
		RecommendedOperatorAction: action.Summary,
		ManualOnly:                true,
		SourceActionKind:          kind,
		NextSteps:                 draftNextSteps(kind),
	}
	draft.ResidentIDs = residentIDsForAction(kind, decision)
	attachDraftContext(&draft, kind, decision)
	switch kind {
	case "capacity_review", "drift_review", "runtime_memory_observation":
		draft.RequiresMaintenanceWindow = true
		draft.RequiresStopStart = true
	case "ticket_review":
		draft.RequiresTicketReview = true
	case "maintenance_state_review", "intervention_followup":
		draft.RequiresMaintenanceWindow = true
	}
	return draft, true
}

func attachDraftContext(draft *HostMaintenanceDraft, actionKind string, decision HostDecisionAssist) {
	if draft == nil {
		return
	}
	switch actionKind {
	case "chat_reply":
		draft.RelatedPendingChats = filterHostFollowupsByResident(decision.TopPendingChats, draft.ResidentIDs)
	case "ticket_review":
		draft.RelatedOpenTickets = filterTicketsByResident(decision.TopOpenTickets, draft.ResidentIDs)
	case "maintenance_run_review":
		draft.RelatedMaintenanceRuns = filterMaintenanceRunsByResident(decision.RecentMaintenanceRuns, draft.ResidentIDs)
	case "maintenance_state_review", "intervention_followup":
		draft.RelatedHostInterventions = filterHostFollowupsByResident(decision.TopHostInterventions, draft.ResidentIDs)
	}
}

func draftKind(actionKind string) string {
	switch actionKind {
	case "capacity_review", "drift_review", "runtime_memory_observation":
		return "resource_or_runtime_maintenance_review"
	case "inventory_check":
		return "inventory_refresh_review"
	case "orchestrator_failure_review", "runtime_budget_review":
		return "runtime_run_review"
	case "memory_operator_review", "memory_safe_decay_review", "memory_lifecycle_review", "memory_compaction_review", "resident_memory_review_queue":
		return "memory_governance_review"
	case "ticket_review":
		return "ticket_review"
	case "maintenance_run_review":
		return "maintenance_run_review"
	case "maintenance_state_review", "intervention_followup":
		return "maintenance_intervention_followup"
	case "chat_reply":
		return "chat_followup"
	default:
		return "operator_review"
	}
}

func draftTitle(actionKind string) string {
	switch actionKind {
	case "capacity_review":
		return "Review host capacity before approving resource changes"
	case "inventory_check":
		return "Refresh inventory before making host decisions"
	case "drift_review":
		return "Review resource drift against configured baseline"
	case "runtime_memory_observation":
		return "Review host RSS versus guest memory before any restart"
	case "orchestrator_failure_review":
		return "Review latest orchestrator failure before retry"
	case "runtime_budget_review":
		return "Review quota, spark, debt, and recovery before another long run"
	case "memory_operator_review":
		return "Inspect memory items requiring operator review"
	case "memory_safe_decay_review":
		return "Review safe memory decay candidates"
	case "memory_lifecycle_review":
		return "Review memory lifecycle attention"
	case "memory_compaction_review":
		return "Review memory compaction candidates"
	case "resident_memory_review_queue":
		return "Let residents handle marked memory self-review"
	case "ticket_review":
		return "Review open resident tickets"
	case "maintenance_run_review":
		return "Review recent maintenance run records"
	case "intervention_followup":
		return "Close or advance active host interventions"
	case "maintenance_state_review":
		return "Review active maintenance intervention states"
	case "chat_reply":
		return "Review pending resident chats"
	default:
		return "Review host decision action"
	}
}

func draftNextSteps(actionKind string) []string {
	switch actionKind {
	case "capacity_review", "drift_review", "runtime_memory_observation":
		return []string{
			"Inspect host capacity, inventory, and resident facts.",
			"If a resource change is justified, draft a maintenance notice with window and expected restart.",
			"Execute stop/change/start only after explicit approval, then send completion notice and refresh inventory.",
		}
	case "inventory_check":
		return []string{
			"Refresh live inventory or cached inventory snapshot.",
			"Do not approve resource changes while resident facts are missing.",
		}
	case "orchestrator_failure_review", "runtime_budget_review":
		return []string{
			"Inspect latest run report and resident quota/spark/debt state.",
			"Do not retry automatically until the failure or budget boundary is understood.",
		}
	case "memory_operator_review", "memory_safe_decay_review", "memory_lifecycle_review", "memory_compaction_review":
		return []string{
			"Run memory maintenance tools in dry-run mode first.",
			"Apply only operator-safe actions; do not rewrite protected resident memories from host review.",
		}
	case "resident_memory_review_queue":
		return []string{
			"Leave protected memories for resident self-review.",
			"Do not turn private or protected memory content into world-facing messages.",
		}
	case "ticket_review":
		return []string{
			"Read the ticket and decide whether to ask a follow-up, defer, or plan maintenance.",
			"Resource changes must remain maintenance-window based.",
		}
	case "maintenance_run_review":
		return []string{
			"Inspect recent maintenance run records and compare them with tickets, interventions, and inventory snapshots.",
			"Complete, fail, roll back, or refresh inventory through the explicit maintenance workflow; do not mutate resources from the draft.",
		}
	case "maintenance_state_review":
		return []string{
			"Inspect each maintenance intervention state and its structured metadata.",
			"Use the explicit maintenance workflow to complete, fail, or roll back; do not mutate resources from the draft.",
			"Send resident-facing notices only through the normal maintenance/intervention channel.",
		}
	case "intervention_followup":
		return []string{
			"Check whether the intervention should be planned, started, completed, failed, or rolled back.",
			"Send resident-facing notice only through the normal maintenance/intervention channel.",
		}
	case "chat_reply":
		return []string{
			"Read pending resident chats from the world-facing context.",
			"Reply only with information Chenglin could know in-world.",
		}
	default:
		return []string{"Review manually before taking any host-side action."}
	}
}

func matchingDecisionReason(actionKind string, decision HostDecisionAssist) string {
	keywords := []string{strings.ReplaceAll(actionKind, "_", " ")}
	switch actionKind {
	case "capacity_review":
		keywords = []string{"allocatable pool", "free capacity"}
	case "inventory_check":
		keywords = []string{"missing live inventory"}
	case "drift_review":
		keywords = []string{"resource drift"}
	case "runtime_memory_observation":
		keywords = []string{"high host qemu rss"}
	case "orchestrator_failure_review":
		keywords = []string{"errored residents"}
	case "runtime_budget_review":
		keywords = []string{"budget-blocked"}
	case "memory_operator_review":
		keywords = []string{"require operator review"}
	case "memory_safe_decay_review":
		keywords = []string{"safe decay candidates"}
	case "memory_lifecycle_review":
		keywords = []string{"need lifecycle attention"}
	case "memory_compaction_review":
		keywords = []string{"duplicate memory history groups"}
	case "resident_memory_review_queue":
		keywords = []string{"resident self-review"}
	case "ticket_review":
		keywords = []string{"open tickets"}
	case "maintenance_run_review":
		keywords = []string{"maintenance run records need operator review"}
	case "intervention_followup":
		keywords = []string{"host interventions"}
	case "maintenance_state_review":
		keywords = []string{"maintenance interventions need status review"}
	case "chat_reply":
		keywords = []string{"waiting on chat replies"}
	}
	for _, reason := range decision.Reasons {
		lower := strings.ToLower(reason)
		for _, keyword := range keywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				return reason
			}
		}
	}
	return ""
}

func residentIDsForAction(actionKind string, decision HostDecisionAssist) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, focus := range decision.ResidentFocus {
		if focus.ResidentID == "" {
			continue
		}
		if focusMatchesAction(focus, actionKind) {
			if _, ok := seen[focus.ResidentID]; !ok {
				seen[focus.ResidentID] = struct{}{}
				out = append(out, focus.ResidentID)
			}
		}
	}
	return out
}

func filterHostFollowupsByResident(items []worldstate.HostFollowup, residentIDs []string) []worldstate.HostFollowup {
	if len(items) == 0 {
		return nil
	}
	if len(residentIDs) == 0 {
		return append([]worldstate.HostFollowup(nil), items...)
	}
	allowed := residentSet(residentIDs)
	out := []worldstate.HostFollowup{}
	for _, item := range items {
		if _, ok := allowed[item.Resident]; ok {
			out = append(out, item)
		}
	}
	return out
}

func filterTicketsByResident(items []worldstate.ResidentTicketSummary, residentIDs []string) []worldstate.ResidentTicketSummary {
	if len(items) == 0 {
		return nil
	}
	if len(residentIDs) == 0 {
		return append([]worldstate.ResidentTicketSummary(nil), items...)
	}
	allowed := residentSet(residentIDs)
	out := []worldstate.ResidentTicketSummary{}
	for _, item := range items {
		if _, ok := allowed[item.Resident]; ok {
			out = append(out, item)
		}
	}
	return out
}

func filterMaintenanceRunsByResident(items []MaintenanceRunRecord, residentIDs []string) []MaintenanceRunRecord {
	if len(items) == 0 {
		return nil
	}
	if len(residentIDs) == 0 {
		return append([]MaintenanceRunRecord(nil), items...)
	}
	allowed := residentSet(residentIDs)
	out := []MaintenanceRunRecord{}
	for _, item := range items {
		if _, ok := allowed[item.ResidentID]; ok {
			out = append(out, item)
		}
	}
	return out
}

func residentSet(residentIDs []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, residentID := range residentIDs {
		residentID = strings.TrimSpace(residentID)
		if residentID != "" {
			out[residentID] = struct{}{}
		}
	}
	return out
}

func focusMatchesAction(focus ResidentDecisionFocus, actionKind string) bool {
	haystack := strings.ToLower(strings.Join(append(append([]string{}, focus.Reasons...), focus.OperatorOnlyObservations...), "\n"))
	haystack += "\n" + strings.ToLower(strings.Join(focus.WorldEventCandidates, "\n"))
	switch actionKind {
	case "inventory_check":
		return strings.Contains(haystack, "missing observed inventory")
	case "drift_review":
		return strings.Contains(haystack, "resource drift")
	case "runtime_memory_observation":
		return strings.Contains(haystack, "host qemu rss")
	case "orchestrator_failure_review":
		return strings.Contains(haystack, "orchestrator error")
	case "runtime_budget_review":
		return strings.Contains(haystack, "budget-blocked")
	case "memory_operator_review":
		return strings.Contains(haystack, "require operator review")
	case "memory_safe_decay_review":
		return strings.Contains(haystack, "safe decay candidates")
	case "memory_lifecycle_review":
		return strings.Contains(haystack, "memory items need lifecycle attention")
	case "memory_compaction_review":
		return strings.Contains(haystack, "duplicate memory history groups")
	case "resident_memory_review_queue":
		return strings.Contains(haystack, "resident self-review")
	case "ticket_review":
		return strings.Contains(haystack, "open ticket")
	case "maintenance_run_review":
		return strings.Contains(haystack, "maintenance")
	case "intervention_followup":
		return strings.Contains(haystack, "host intervention")
	case "maintenance_state_review":
		return strings.Contains(haystack, "host intervention") || strings.Contains(haystack, "maintenance")
	case "chat_reply":
		return strings.Contains(haystack, "pending resident chat")
	default:
		return false
	}
}
