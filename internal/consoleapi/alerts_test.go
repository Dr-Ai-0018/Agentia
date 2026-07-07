package consoleapi

import (
	"strings"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

func TestBuildAlertsUsesObservationLanguage(t *testing.T) {
	stale := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	alerts := buildAlerts(
		&orchestrator.RunStatus{
			RunID:     "orchestrator-test",
			UpdatedAt: stale,
			Residents: []orchestrator.ResidentRunStatus{{
				Resident:         "jade",
				TransientBlocked: true,
				UpdatedAt:        stale,
			}},
		},
		broker.BudgetStatusOutput{
			Residents: []broker.ResidentBudgetStatus{
				{
					ResidentID:                  "onyx",
					WorkAllowedNow:              true,
					QuotaTightestLayer:          "week",
					QuotaTightestRemainingRatio: 0.18,
				},
				{
					ResidentID:     "amber",
					WorkAllowedNow: false,
					BlockingReason: "6h quota exhausted",
				},
			},
		},
		worldstate.HostInboxSummary{
			PendingChatMessages: []worldstate.ThreadMessage{{
				Message: worldstate.Message{
					Resident:  "amber",
					CreatedAt: stale,
				},
			}},
		},
	)

	joined := strings.ToLower(joinAlertMessages(alerts))
	for _, banned := range []string{
		"transient blocked",
		"budget blocked",
		"quota is tight",
		"pending reply",
		"run status has not updated",
		"6h quota exhausted",
	} {
		if strings.Contains(joined, banned) {
			t.Fatalf("alert messages should not expose backend phrase %q in %q", banned, joined)
		}
	}
	for _, required := range []string{"暂时卡住", "近 6h", "本周", "等程林回", "没有新动静"} {
		if !strings.Contains(joined, strings.ToLower(required)) {
			t.Fatalf("expected observation phrase %q in %q", required, joined)
		}
	}
}

func joinAlertMessages(alerts []OperatorAlert) string {
	messages := make([]string, 0, len(alerts))
	for _, alert := range alerts {
		messages = append(messages, alert.Message)
	}
	return strings.Join(messages, "\n")
}
