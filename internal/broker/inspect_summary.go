package broker

import (
	"strings"

	"ai-arena/internal/worldstate"
)

func SummarizeHostInspect(out HostInspectOutput) HostInspectSummary {
	summary := HostInspectSummary{
		CollectedAt:   out.Inventory.CollectedAt,
		InventoryPath: out.Path,
		Capacity:      out.Capacity,
		ResidentCount: len(out.ResidentFacts),
		FollowupCount: len(out.Followups),
		TopFollowups:  append([]worldstate.HostFollowup(nil), out.Followups...),
	}
	if len(summary.TopFollowups) > 5 {
		summary.TopFollowups = summary.TopFollowups[:5]
	}
	if out.LatestOrchestrator != nil {
		latest := *out.LatestOrchestrator
		latest.Residents = append([]OrchestratorResidentInspectionDigest(nil), out.LatestOrchestrator.Residents...)
		latest.ResidentsPlanned = append([]string(nil), out.LatestOrchestrator.ResidentsPlanned...)
		summary.LatestOrchestrator = &latest
		summary.LatestRunNeedsAttention = latest.ResidentsErrored > 0 || latest.BudgetBlockedRuns > 0
	}
	for _, item := range out.RecentOrchestrators {
		copyItem := item
		copyItem.Residents = append([]OrchestratorResidentInspectionDigest(nil), item.Residents...)
		copyItem.ResidentsPlanned = append([]string(nil), item.ResidentsPlanned...)
		summary.RecentOrchestrators = append(summary.RecentOrchestrators, copyItem)
		if copyItem.ResidentsErrored > 0 || copyItem.BudgetBlockedRuns > 0 {
			summary.RecentRunsNeedingAttention++
		}
	}

	pendingChat := map[string]struct{}{}
	openTicket := map[string]struct{}{}
	openIntervention := map[string]struct{}{}
	memoryAttention := map[string]int{}
	memoryDuplicateGroups := map[string]int{}
	memoryRecommendation := map[string]ResidentMemoryMaintenance{}
	orchestratorByResident := map[string]OrchestratorResidentInspectionDigest{}
	if out.LatestOrchestrator != nil {
		for _, item := range out.LatestOrchestrator.Residents {
			if strings.TrimSpace(item.Resident) != "" {
				orchestratorByResident[item.Resident] = item
			}
		}
	}
	for _, item := range out.Memory {
		if item.NeedsAttention <= 0 {
			continue
		}
		memoryAttention[item.Resident] = item.NeedsAttention
	}
	for _, item := range out.MemoryMaintenance {
		if item.LifecycleAttention > 0 {
			memoryAttention[item.ResidentID] = item.LifecycleAttention
		}
		if item.DuplicateHistoryGroups > 0 {
			memoryDuplicateGroups[item.ResidentID] = item.DuplicateHistoryGroups
			summary.MemoryDuplicateHistoryGroups += item.DuplicateHistoryGroups
		}
		if item.RecommendedAction != "" || item.Summary != "" {
			memoryRecommendation[item.ResidentID] = item
		}
	}
	for _, count := range memoryAttention {
		summary.MemoryItemsAttention += count
	}
	summary.MemoryResidentsAttention = len(memoryAttention)
	maintenanceResidents := map[string]struct{}{}
	for resident := range memoryAttention {
		maintenanceResidents[resident] = struct{}{}
	}
	for resident := range memoryDuplicateGroups {
		maintenanceResidents[resident] = struct{}{}
	}
	summary.MemoryMaintenanceResidents = len(maintenanceResidents)
	for _, item := range out.Followups {
		switch strings.TrimSpace(item.Kind) {
		case "chat_reply":
			if item.Resident != "" {
				pendingChat[item.Resident] = struct{}{}
			}
		case "ticket_reply":
			summary.OpenTicketCount++
			if item.Resident != "" {
				openTicket[item.Resident] = struct{}{}
			}
		case "host_intervention":
			summary.InterventionCount++
			if item.Resident != "" {
				openIntervention[item.Resident] = struct{}{}
			}
		}
	}
	summary.PendingChatResidents = len(pendingChat)
	summary.OpenTicketResidents = len(openTicket)

	risks := make([]ResidentInspectRisk, 0, len(out.ResidentFacts))
	for _, item := range out.ResidentFacts {
		risk := ResidentInspectRisk{
			ResidentID:                   item.ResidentID,
			Status:                       item.Status,
			DriftFields:                  append([]string(nil), item.DriftFields...),
			HasPendingChat:               hasResident(pendingChat, item.ResidentID),
			HasOpenTicket:                hasResident(openTicket, item.ResidentID),
			HasIntervention:              hasResident(openIntervention, item.ResidentID),
			MemoryAttention:              memoryAttention[item.ResidentID],
			MemoryDuplicateHistoryGroups: memoryDuplicateGroups[item.ResidentID],
		}
		if recommendation, ok := memoryRecommendation[item.ResidentID]; ok {
			risk.MemoryRecommendedAction = recommendation.RecommendedAction
			risk.MemorySummary = recommendation.Summary
		}
		if run, ok := orchestratorByResident[item.ResidentID]; ok {
			risk.OrchestratorStatus = run.Status
			risk.OrchestratorStoppedReason = run.StoppedReason
			risk.OrchestratorBudgetBlocked = run.BudgetBlocked
			risk.OrchestratorError = run.Error
		}
		if strings.EqualFold(strings.TrimSpace(item.Status), "running") {
			summary.ResidentsRunning++
		}
		if len(item.DriftFields) > 0 {
			summary.ResidentsWithDrift++
		}
		if strings.TrimSpace(item.Status) == "" {
			summary.ResidentsMissingInventory++
		}
		risk.NeedsAttention = len(risk.DriftFields) > 0 ||
			risk.HasPendingChat ||
			risk.HasOpenTicket ||
			risk.HasIntervention ||
			risk.MemoryAttention > 0 ||
			risk.MemoryDuplicateHistoryGroups > 0 ||
			risk.OrchestratorBudgetBlocked ||
			strings.TrimSpace(risk.OrchestratorError) != "" ||
			strings.EqualFold(strings.TrimSpace(risk.OrchestratorStatus), "error") ||
			strings.TrimSpace(item.Status) == ""
		risks = append(risks, risk)
	}
	summary.ResidentRisk = risks
	return summary
}

func hasResident(set map[string]struct{}, resident string) bool {
	_, ok := set[resident]
	return ok
}
