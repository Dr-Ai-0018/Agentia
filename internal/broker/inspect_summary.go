package broker

import (
	"strings"

	"ai-arena/internal/worldstate"
)

func SummarizeHostInspect(out HostInspectOutput) HostInspectSummary {
	summary := HostInspectSummary{
		CollectedAt:   out.Inventory.CollectedAt,
		InventoryPath: out.Path,
		ResidentCount: len(out.ResidentFacts),
		FollowupCount: len(out.Followups),
		TopFollowups:  append([]worldstate.HostFollowup(nil), out.Followups...),
	}
	if len(summary.TopFollowups) > 5 {
		summary.TopFollowups = summary.TopFollowups[:5]
	}

	pendingChat := map[string]struct{}{}
	openTicket := map[string]struct{}{}
	openIntervention := map[string]struct{}{}
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
			ResidentID:      item.ResidentID,
			Status:          item.Status,
			DriftFields:     append([]string(nil), item.DriftFields...),
			HasPendingChat:  hasResident(pendingChat, item.ResidentID),
			HasOpenTicket:   hasResident(openTicket, item.ResidentID),
			HasIntervention: hasResident(openIntervention, item.ResidentID),
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
		risk.NeedsAttention = len(risk.DriftFields) > 0 || risk.HasPendingChat || risk.HasOpenTicket || risk.HasIntervention || strings.TrimSpace(item.Status) == ""
		risks = append(risks, risk)
	}
	summary.ResidentRisk = risks
	return summary
}

func hasResident(set map[string]struct{}, resident string) bool {
	_, ok := set[resident]
	return ok
}
