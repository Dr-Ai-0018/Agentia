package newborn

import (
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/memory"
	"strings"
)

type RecentAction struct {
	Round       int    `json:"round"`
	Action      string `json:"action"`
	Signature   string `json:"signature"`
	Intent      string `json:"intent,omitempty"`
	Situation   string `json:"situation,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Observation string `json:"observation,omitempty"`
	Suppressed  bool   `json:"suppressed,omitempty"`
}

type ExplorationSurface string

const (
	SurfaceIdentity   ExplorationSurface = "identity"
	SurfaceFilesystem ExplorationSurface = "filesystem"
	SurfaceResources  ExplorationSurface = "resources"
	SurfaceNetwork    ExplorationSurface = "network"
	SurfaceServices   ExplorationSurface = "services"
	SurfacePackages   ExplorationSurface = "packages"
	SurfaceWorld      ExplorationSurface = "world"
)

type SurfaceCost string

const (
	SurfaceCostLow    SurfaceCost = "low"
	SurfaceCostMedium SurfaceCost = "medium"
	SurfaceCostHigh   SurfaceCost = "high"
)

type AgentDecision struct {
	Situation      string `json:"situation"`
	NextAction     string `json:"next_action"`
	Command        string `json:"command,omitempty"`
	Message        string `json:"message,omitempty"`
	TicketTitle    string `json:"ticket_title,omitempty"`
	TicketBody     string `json:"ticket_body,omitempty"`
	TicketPriority string `json:"ticket_priority,omitempty"`
	MemoryID       string `json:"memory_id,omitempty"`
	MemoryAction   string `json:"memory_action,omitempty"`
	MemorySummary  string `json:"memory_summary,omitempty"`
	MemoryText     string `json:"memory_text,omitempty"`
	MemoryLayer    string `json:"memory_layer,omitempty"`
	MemoryReason   string `json:"memory_reason,omitempty"`
	Reason         string `json:"reason"`
}

func (d AgentDecision) CompactForHistory() string {
	parts := []string{}
	if v := truncateForModel(strings.TrimSpace(d.Situation), 160); v != "" {
		parts = append(parts, "situation="+v)
	}
	if v := truncateForModel(strings.TrimSpace(d.Reason), 160); v != "" {
		parts = append(parts, "reason="+v)
	}
	switch d.NextAction {
	case "guest_exec", "write_note":
		if v := truncateForModel(strings.TrimSpace(d.Command), 200); v != "" {
			parts = append(parts, "command="+v)
		}
	case "talk_to_chenglin":
		if v := truncateForModel(strings.TrimSpace(d.Message), 180); v != "" {
			parts = append(parts, "message="+v)
		}
	case "submit_ticket":
		if v := truncateForModel(strings.TrimSpace(d.TicketTitle), 80); v != "" {
			parts = append(parts, "ticket_title="+v)
		}
		if v := truncateForModel(strings.TrimSpace(d.TicketBody), 180); v != "" {
			parts = append(parts, "ticket_body="+v)
		}
	case "memory_review":
		if v := truncateForModel(strings.TrimSpace(d.MemoryID), 80); v != "" {
			parts = append(parts, "memory_id="+v)
		}
		if v := truncateForModel(strings.TrimSpace(d.MemoryAction), 40); v != "" {
			parts = append(parts, "memory_action="+v)
		}
	}
	return strings.Join(parts, "\n")
}

func (d AgentDecision) MemoryReviewRequest() memory.MemoryReviewRequest {
	return memory.MemoryReviewRequest{
		Action:       mapMemoryReviewAction(d.MemoryAction),
		NewSummary:   d.MemorySummary,
		NewText:      d.MemoryText,
		TargetLayer:  memory.Layer(d.MemoryLayer),
		ReasonNote:   d.MemoryReason,
		ResidentNote: d.Reason,
	}
}

func mapMemoryReviewAction(action string) memory.Action {
	switch action {
	case "keep":
		return memory.ActionRetain
	case "rewrite":
		return memory.ActionUpdate
	case "compress":
		return memory.ActionSummarize
	case "demote":
		return memory.ActionDecay
	case "delete":
		return memory.ActionDelete
	default:
		return memory.Action("")
	}
}

type BrokerUsageLog struct {
	Applied            bool                        `json:"applied"`
	Denied             bool                        `json:"denied"`
	DeniedReason       []string                    `json:"denied_reason,omitempty"`
	BeforeSpark        float64                     `json:"before_spark"`
	AfterSpark         float64                     `json:"after_spark,omitempty"`
	BeforeDebtActive   bool                        `json:"before_debt_active"`
	AfterDebtActive    bool                        `json:"after_debt_active,omitempty"`
	SparkDelta         float64                     `json:"spark_delta,omitempty"`
	Window6HUsed       int                         `json:"window_6h_used,omitempty"`
	DayUsed            int                         `json:"day_used,omitempty"`
	WeekUsed           int                         `json:"week_used,omitempty"`
	ApplyReason        string                      `json:"apply_reason,omitempty"`
	PreparedSparkCost  float64                     `json:"prepared_spark_cost"`
	PreparedStrainCost int                         `json:"prepared_strain_cost"`
	Quota              *brokerstate.QuotaSnapshot  `json:"quota,omitempty"`
	AfterStatus        *brokerstate.ResidentStatus `json:"after_status,omitempty"`
}

type RoundLog struct {
	Round        int             `json:"round"`
	RemainingSec int             `json:"remaining_sec"`
	Decision     AgentDecision   `json:"decision"`
	Observation  string          `json:"observation"`
	ResponseID   string          `json:"response_id"`
	InputTokens  int             `json:"input_tokens"`
	CachedTokens int             `json:"cached_tokens"`
	OutputTokens int             `json:"output_tokens"`
	Broker       *BrokerUsageLog `json:"broker,omitempty"`
}

type FinalReport struct {
	Resident         string          `json:"resident"`
	Model            string          `json:"model"`
	DurationSeconds  int             `json:"duration_seconds"`
	Rounds           int             `json:"rounds"`
	StartedAt        string          `json:"started_at"`
	EndedAt          string          `json:"ended_at"`
	Acceptance       string          `json:"acceptance"`
	AcceptanceBroker *BrokerUsageLog `json:"acceptance_broker,omitempty"`
	RoundLogs        []RoundLog      `json:"round_logs"`
	StoppedReason    string          `json:"stopped_reason,omitempty"`
}
