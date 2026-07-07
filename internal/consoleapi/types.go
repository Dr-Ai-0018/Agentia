package consoleapi

import (
	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

type DashboardSummary struct {
	GeneratedAt string                      `json:"generated_at"`
	ActiveRun   *orchestrator.RunStatus     `json:"active_run,omitempty"`
	LatestRun   *orchestrator.RunRecord     `json:"latest_run,omitempty"`
	Runs        []orchestrator.RunRecord    `json:"runs"`
	Budget      broker.BudgetStatusOutput   `json:"budget"`
	Inbox       worldstate.HostInboxSummary `json:"inbox"`
	Followups   []worldstate.HostFollowup   `json:"followups"`
	Inspect     *broker.HostInspectSummary  `json:"inspect,omitempty"`
	Acceptance  *broker.V0AcceptanceOutput  `json:"acceptance,omitempty"`
	Alerts      []OperatorAlert             `json:"alerts"`
}

type PreflightResponse struct {
	GeneratedAt string           `json:"generated_at"`
	Overall     string           `json:"overall"`
	Summary     string           `json:"summary"`
	Checks      []PreflightCheck `json:"checks"`
}

type PreflightCheck struct {
	ID       string         `json:"id"`
	Section  string         `json:"section"`
	Label    string         `json:"label"`
	Status   string         `json:"status"`
	Required bool           `json:"required"`
	Detail   string         `json:"detail"`
	Data     map[string]any `json:"data,omitempty"`
}

type OperatorAlert struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Resident string `json:"resident,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	Message  string `json:"message"`
	Since    string `json:"since,omitempty"`
}

type ReplyRequest struct {
	MessageID   string `json:"message_id"`
	Body        string `json:"body"`
	BoundaryAck bool   `json:"boundary_ack"`
}

type TicketReplyRequest struct {
	TicketID    string `json:"ticket_id"`
	Body        string `json:"body"`
	Close       bool   `json:"close"`
	BoundaryAck bool   `json:"boundary_ack"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
