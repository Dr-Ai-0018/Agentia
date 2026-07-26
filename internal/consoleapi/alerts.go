package consoleapi

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

func buildAlerts(active *orchestrator.RunStatus, budget broker.BudgetStatusOutput, inbox worldstate.HostInboxSummary) []OperatorAlert {
	out := make([]OperatorAlert, 0)
	if active != nil {
		if staleAlert := activeRunFreshnessAlert(*active, 15*time.Minute); staleAlert != nil {
			out = append(out, *staleAlert)
		}
		for _, resident := range active.Residents {
			if resident.TransientBlocked {
				out = append(out, OperatorAlert{
					Severity: "P1",
					Kind:     "resident_transient_blocked",
					Resident: resident.Resident,
					RunID:    active.RunID,
					Message:  fmt.Sprintf("%s 暂时卡住了。先观察她是否会自己恢复。", residentName(resident.Resident)),
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
				Message:  fmt.Sprintf("%s 现在不能继续行动，原因是%s。", residentName(resident.ResidentID), budgetBlockReason(resident.BlockingReason)),
			})
			continue
		}
		if resident.QuotaTightestRemainingRatio > 0 && resident.QuotaTightestRemainingRatio <= 0.2 {
			out = append(out, OperatorAlert{
				Severity: "P1",
				Kind:     "resident_quota_pressure",
				Resident: resident.ResidentID,
				Message:  fmt.Sprintf("%s 的%s精力有点紧，适合放慢一点。", residentName(resident.ResidentID), quotaLayerLabel(resident.QuotaTightestLayer)),
			})
		}
	}
	for _, message := range inbox.PendingChatMessages {
		out = append(out, OperatorAlert{
			Severity: "P2",
			Kind:     "pending_reply",
			Resident: message.Resident,
			Message:  fmt.Sprintf("%s 有条话还等程林回。", residentName(message.Resident)),
			Since:    message.CreatedAt,
		})
	}
	return out
}

func activeRunFreshnessAlert(status orchestrator.RunStatus, threshold time.Duration) *OperatorAlert {
	return activeRunFreshnessAlertAt(status, time.Now(), threshold)
}

func activeRunFreshnessAlertAt(status orchestrator.RunStatus, now time.Time, threshold time.Duration) *OperatorAlert {
	return runFreshnessAlertAt(status, now, threshold, "这次观察")
}

func historicalRunFreshnessAlertAt(status orchestrator.RunStatus, now time.Time, threshold time.Duration) *OperatorAlert {
	return runFreshnessAlertAt(status, now, threshold, "历史观察")
}

func runFreshnessAlertAt(status orchestrator.RunStatus, now time.Time, threshold time.Duration, label string) *OperatorAlert {
	if status.UpdatedAt == "" {
		return nil
	}
	updatedAt, err := time.Parse(time.RFC3339, status.UpdatedAt)
	if err != nil {
		return nil
	}
	age := now.Sub(updatedAt)
	if age < threshold {
		return nil
	}
	return &OperatorAlert{
		Severity: "P1",
		Kind:     "run_stale",
		RunID:    status.RunID,
		Message:  fmt.Sprintf("%s已经 %s 没有新动静。", label, humanDuration(age.Round(time.Second))),
		Since:    status.UpdatedAt,
	}
}

func residentName(resident string) string {
	switch strings.ToLower(strings.TrimSpace(resident)) {
	case "jade":
		return "Jade"
	case "amber":
		return "Amber"
	case "onyx":
		return "Onyx"
	default:
		if strings.TrimSpace(resident) == "" {
			return "住户"
		}
		return strings.TrimSpace(resident)
	}
}

func quotaLayerLabel(layer string) string {
	switch strings.ToLower(strings.TrimSpace(layer)) {
	case "6h":
		return "近 6h"
	case "day", "1day":
		return "今日"
	case "week", "1week":
		return "本周"
	default:
		return "额度"
	}
}

func budgetBlockReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	switch {
	case reason == "":
		return "额度暂时不够"
	case strings.Contains(reason, "6h"):
		return "近 6h 额度暂时不够"
	case strings.Contains(reason, "day"):
		return "今日额度暂时不够"
	case strings.Contains(reason, "week"):
		return "本周额度暂时不够"
	default:
		return strings.TrimSpace(reason)
	}
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		sec := int(d.Seconds())
		if sec < 1 {
			sec = 1
		}
		return fmt.Sprintf("%d 秒", sec)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
}
