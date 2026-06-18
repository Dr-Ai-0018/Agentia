package broker

import (
	"sort"
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
		TopOpenTickets: append([]worldstate.ResidentTicketSummary(nil),
			out.Inbox.OpenTickets...),
	}
	if len(summary.TopFollowups) > 5 {
		summary.TopFollowups = summary.TopFollowups[:5]
	}
	if len(summary.TopOpenTickets) > 5 {
		summary.TopOpenTickets = summary.TopOpenTickets[:5]
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
	summary.RecentMaintenanceRuns = append([]MaintenanceRunRecord(nil), out.RecentMaintenanceRuns...)

	pendingChat := map[string]struct{}{}
	openTicket := map[string]struct{}{}
	openIntervention := map[string]struct{}{}
	maintenanceInterventions := MaintenanceInterventionSummary{}
	memoryAttention := map[string]int{}
	memoryDuplicateGroups := map[string]int{}
	memoryMaintenanceByResident := map[string]ResidentMemoryMaintenance{}
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
		memoryMaintenanceByResident[item.ResidentID] = item
		if item.LifecycleAttention > 0 {
			memoryAttention[item.ResidentID] = item.LifecycleAttention
		}
		summary.MemoryOperatorDecayCandidates += item.OperatorDecayCandidates
		summary.MemoryResidentReviewQueue += item.ResidentReviewQueue
		summary.MemoryOperatorReviewRequired += item.OperatorReviewRequired
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
	for resident, item := range memoryMaintenanceByResident {
		if item.OperatorDecayCandidates > 0 || item.ResidentReviewQueue > 0 || item.OperatorReviewRequired > 0 {
			maintenanceResidents[resident] = struct{}{}
		}
	}
	summary.MemoryMaintenanceResidents = len(maintenanceResidents)
	for _, item := range out.Followups {
		switch strings.TrimSpace(item.Kind) {
		case "chat_reply":
			if item.Resident != "" {
				pendingChat[item.Resident] = struct{}{}
			}
			if len(summary.TopPendingChats) < 5 {
				summary.TopPendingChats = append(summary.TopPendingChats, item)
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
			addMaintenanceInterventionSummary(&maintenanceInterventions, item)
			if len(summary.TopHostInterventions) < 5 {
				summary.TopHostInterventions = append(summary.TopHostInterventions, item)
			}
		}
	}
	if maintenanceInterventions.Total > 0 {
		summary.MaintenanceInterventions = &maintenanceInterventions
	}
	summary.PendingChatResidents = len(pendingChat)
	summary.OpenTicketResidents = len(openTicket)

	risks := make([]ResidentInspectRisk, 0, len(out.ResidentFacts))
	for _, item := range out.ResidentFacts {
		risk := ResidentInspectRisk{
			ResidentID:               item.ResidentID,
			Status:                   item.Status,
			DriftFields:              append([]string(nil), item.DriftFields...),
			HostRSSHighGuestUsageLow: item.HostRSSHighGuestUsageLow,
			HostQEMURSSMiB:           item.HostQEMURSSMiB,
			IncusMemoryCurrentMiB:    item.IncusMemoryCurrentMiB,
			GuestMemAvailableMiB:     item.GuestMemAvailableMiB,
			GuestBuffCacheMiB:        item.GuestBuffCacheMiB,
			GuestTopMemoryProcess:    item.GuestTopMemoryProcess,
			LiveMetricsError:         item.LiveMetricsError,
			HasPendingChat:           hasResident(pendingChat, item.ResidentID),
			HasOpenTicket:            hasResident(openTicket, item.ResidentID),
			HasIntervention:          hasResident(openIntervention, item.ResidentID),
			MemoryAttention:          memoryAttention[item.ResidentID],
			MemoryOperatorDecayCandidates: memoryMaintenanceCount(memoryMaintenanceByResident, item.ResidentID, func(item ResidentMemoryMaintenance) int {
				return item.OperatorDecayCandidates
			}),
			MemoryResidentReviewQueue: memoryMaintenanceCount(memoryMaintenanceByResident, item.ResidentID, func(item ResidentMemoryMaintenance) int {
				return item.ResidentReviewQueue
			}),
			MemoryOperatorReviewRequired: memoryMaintenanceCount(memoryMaintenanceByResident, item.ResidentID, func(item ResidentMemoryMaintenance) int {
				return item.OperatorReviewRequired
			}),
			MemoryDuplicateHistoryGroups: memoryDuplicateGroups[item.ResidentID],
		}
		if item.HostRSSHighGuestUsageLow {
			summary.RuntimeMemoryObservationResidents++
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
			risk.MemoryOperatorDecayCandidates > 0 ||
			risk.MemoryResidentReviewQueue > 0 ||
			risk.MemoryOperatorReviewRequired > 0 ||
			risk.MemoryDuplicateHistoryGroups > 0 ||
			risk.HostRSSHighGuestUsageLow ||
			strings.TrimSpace(risk.LiveMetricsError) != "" ||
			risk.OrchestratorBudgetBlocked ||
			strings.TrimSpace(risk.OrchestratorError) != "" ||
			strings.EqualFold(strings.TrimSpace(risk.OrchestratorStatus), "error") ||
			strings.TrimSpace(item.Status) == ""
		risks = append(risks, risk)
	}
	sortResidentInspectRisks(risks)
	summary.ResidentRisk = risks
	return summary
}

func hasResident(set map[string]struct{}, resident string) bool {
	_, ok := set[resident]
	return ok
}

func addMaintenanceInterventionSummary(summary *MaintenanceInterventionSummary, item worldstate.HostFollowup) {
	if summary == nil || strings.TrimSpace(item.Kind) != "host_intervention" {
		return
	}
	if strings.TrimSpace(item.Title) == "" && len(item.Maintenance) == 0 {
		return
	}
	if !strings.Contains(strings.ToLower(item.Title), "maintenance") && len(item.Maintenance) == 0 {
		return
	}
	summary.Total++
	state := strings.TrimSpace(item.Maintenance["maintenance_state"])
	if state == "" {
		state = strings.TrimSpace(item.Status)
	}
	switch strings.ToLower(state) {
	case "planned", "open":
		summary.Planned++
	case "in_progress":
		summary.InProgress++
	case "completed":
		summary.Completed++
	case "failed":
		summary.Failed++
	case "rolled_back":
		summary.RolledBack++
	default:
		summary.Unknown++
	}
}

func memoryMaintenanceCount(items map[string]ResidentMemoryMaintenance, resident string, value func(ResidentMemoryMaintenance) int) int {
	item, ok := items[resident]
	if !ok {
		return 0
	}
	return value(item)
}

func sortResidentInspectRisks(items []ResidentInspectRisk) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if inspectRiskRank(left) != inspectRiskRank(right) {
			return inspectRiskRank(left) > inspectRiskRank(right)
		}
		return left.ResidentID < right.ResidentID
	})
}

func inspectRiskRank(item ResidentInspectRisk) int {
	switch {
	case strings.TrimSpace(item.Status) == "":
		return 4
	case len(item.DriftFields) > 0 || item.HasIntervention:
		return 3
	case item.HasPendingChat || item.HasOpenTicket:
		return 2
	case item.MemoryAttention > 0 ||
		item.MemoryOperatorDecayCandidates > 0 ||
		item.MemoryResidentReviewQueue > 0 ||
		item.MemoryOperatorReviewRequired > 0 ||
		item.MemoryDuplicateHistoryGroups > 0 ||
		item.HostRSSHighGuestUsageLow ||
		strings.TrimSpace(item.LiveMetricsError) != "" ||
		item.OrchestratorBudgetBlocked ||
		strings.TrimSpace(item.OrchestratorError) != "":
		return 1
	default:
		return 0
	}
}
