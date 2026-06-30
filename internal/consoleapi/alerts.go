package consoleapi

import (
	"fmt"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

func buildAlerts(active *orchestrator.RunStatus, budget broker.BudgetStatusOutput, inbox worldstate.HostInboxSummary) []OperatorAlert {
	out := make([]OperatorAlert, 0)
	if active != nil {
		if staleAlert := runFreshnessAlert(*active, 3*time.Minute); staleAlert != nil {
			out = append(out, *staleAlert)
		}
		for _, resident := range active.Residents {
			if resident.TransientBlocked {
				out = append(out, OperatorAlert{
					Severity: "P1",
					Kind:     "resident_transient_blocked",
					Resident: resident.Resident,
					RunID:    active.RunID,
					Message:  fmt.Sprintf("%s is transient blocked", resident.Resident),
					Since:    resident.UpdatedAt,
				})
			}
		}
	}
	for _, resident := range budget.Residents {
		if !resident.WorkAllowedNow {
			out = append(out, OperatorAlert{
				Severity: "P0",
				Kind:     "resident_budget_blocked",
				Resident: resident.ResidentID,
				Message:  fmt.Sprintf("%s is budget blocked: %s", resident.ResidentID, resident.BlockingReason),
			})
			continue
		}
		if resident.QuotaTightestRemainingRatio > 0 && resident.QuotaTightestRemainingRatio <= 0.2 {
			out = append(out, OperatorAlert{
				Severity: "P1",
				Kind:     "resident_quota_pressure",
				Resident: resident.ResidentID,
				Message:  fmt.Sprintf("%s %s quota is tight", resident.ResidentID, resident.QuotaTightestLayer),
			})
		}
	}
	for _, message := range inbox.PendingChatMessages {
		out = append(out, OperatorAlert{
			Severity: "P2",
			Kind:     "pending_reply",
			Resident: message.Resident,
			Message:  fmt.Sprintf("pending reply from %s", message.Resident),
			Since:    message.CreatedAt,
		})
	}
	return out
}

func runFreshnessAlert(status orchestrator.RunStatus, threshold time.Duration) *OperatorAlert {
	if status.UpdatedAt == "" {
		return nil
	}
	updatedAt, err := time.Parse(time.RFC3339, status.UpdatedAt)
	if err != nil {
		return nil
	}
	if time.Since(updatedAt) < threshold {
		return nil
	}
	return &OperatorAlert{
		Severity: "P1",
		Kind:     "run_stale",
		RunID:    status.RunID,
		Message:  fmt.Sprintf("run status has not updated for %s", time.Since(updatedAt).Round(time.Second)),
		Since:    status.UpdatedAt,
	}
}
