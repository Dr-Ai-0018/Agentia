package newborn

import (
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/memory"
	"strconv"
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
	NoteFile       string `json:"note_file,omitempty"`
	NoteText       string `json:"note_text,omitempty"`
	BackupFile     string `json:"backup_file,omitempty"`
	TicketTitle    string `json:"ticket_title,omitempty"`
	TicketBody     string `json:"ticket_body,omitempty"`
	TicketPriority string `json:"ticket_priority,omitempty"`
	MemoryID       string `json:"memory_id,omitempty"`
	MemoryAction   string `json:"memory_action,omitempty"`
	MemorySummary  string `json:"memory_summary,omitempty"`
	MemoryText     string `json:"memory_text,omitempty"`
	MemoryLayer    string `json:"memory_layer,omitempty"`
	MemoryReason   string `json:"memory_reason,omitempty"`
	SleepMinutes   int    `json:"sleep_minutes,omitempty"`
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
	case "guest_exec":
		if v := truncateForModel(strings.TrimSpace(d.Command), 200); v != "" {
			parts = append(parts, "command="+v)
		}
	case "note_append", "note_replace_with_backup", "note_summarize_or_compact":
		if v := truncateForModel(strings.TrimSpace(d.NoteFile), 80); v != "" {
			parts = append(parts, "note_file="+v)
		}
		if v := truncateForModel(strings.TrimSpace(d.NoteText), 200); v != "" {
			parts = append(parts, "note="+v)
		}
	case "note_read", "note_list", "note_restore_backup":
		if v := truncateForModel(strings.TrimSpace(d.NoteFile), 80); v != "" {
			parts = append(parts, "note_file="+v)
		}
		if v := truncateForModel(strings.TrimSpace(d.BackupFile), 80); v != "" {
			parts = append(parts, "backup_file="+v)
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
	case "sleep":
		if d.SleepMinutes > 0 {
			parts = append(parts, "sleep_minutes="+strconv.Itoa(d.SleepMinutes))
		}
	}
	return strings.Join(parts, "\n")
}

func isNoteAction(action string) bool {
	switch action {
	case "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact":
		return true
	default:
		return false
	}
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
	Applied              bool                        `json:"applied"`
	Denied               bool                        `json:"denied"`
	DeniedReason         []string                    `json:"denied_reason,omitempty"`
	ProviderCostRecorded bool                        `json:"provider_cost_recorded,omitempty"`
	CostClass            string                      `json:"cost_class,omitempty"`
	BeforeSpark          float64                     `json:"before_spark"`
	AfterSpark           float64                     `json:"after_spark,omitempty"`
	BeforeDebtActive     bool                        `json:"before_debt_active"`
	AfterDebtActive      bool                        `json:"after_debt_active,omitempty"`
	SparkDelta           float64                     `json:"spark_delta,omitempty"`
	Window6HUsed         int                         `json:"window_6h_used,omitempty"`
	DayUsed              int                         `json:"day_used,omitempty"`
	WeekUsed             int                         `json:"week_used,omitempty"`
	ApplyReason          string                      `json:"apply_reason,omitempty"`
	PreparedSparkCost    float64                     `json:"prepared_spark_cost"`
	PreparedStrainCost   int                         `json:"prepared_strain_cost"`
	Quota                *brokerstate.QuotaSnapshot  `json:"quota,omitempty"`
	AfterStatus          *brokerstate.ResidentStatus `json:"after_status,omitempty"`
}

type SummaryPaneEvidenceRef struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Rounds []int  `json:"rounds,omitempty"`
}

type SummaryPane struct {
	Text           string                   `json:"text"`
	UpdatedAt      string                   `json:"updated_at"`
	RoundsAbsorbed int                      `json:"rounds_absorbed"`
	ApproxTokens   int                      `json:"approx_tokens"`
	EvidenceRefs   []SummaryPaneEvidenceRef `json:"evidence_refs,omitempty"`
}

type CompactionTriggerReason string

const (
	CompactionTriggerPreflightMeasured CompactionTriggerReason = "preflight_measured"
	CompactionTriggerAcceptanceMicro   CompactionTriggerReason = "acceptance_microcompact"
	CompactionTriggerReactiveOverflow  CompactionTriggerReason = "reactive_overflow"
	CompactionTriggerManual            CompactionTriggerReason = "manual"
)

type CompactionOutcome string

const (
	CompactionOutcomeSummarized    CompactionOutcome = "summarized"
	CompactionOutcomeSilentTrim    CompactionOutcome = "silent_trim"
	CompactionOutcomeGuardRejected CompactionOutcome = "guard_rejected"
	CompactionOutcomeFailed        CompactionOutcome = "failed"
)

type CompactionEvent struct {
	CompactionID                   string                  `json:"compaction_id"`
	RunID                          string                  `json:"run_id,omitempty"`
	Resident                       string                  `json:"resident"`
	OccurredAt                     string                  `json:"occurred_at"`
	TriggerReason                  CompactionTriggerReason `json:"trigger_reason"`
	TriggerDetail                  string                  `json:"trigger_detail,omitempty"`
	TokensBefore                   int                     `json:"tokens_before"`
	TokensAfter                    int                     `json:"tokens_after"`
	RoundsAbsorbed                 int                     `json:"rounds_absorbed"`
	SummaryPaneTokensAfter         int                     `json:"summary_pane_tokens_after"`
	Outcome                        CompactionOutcome       `json:"outcome"`
	GuardRejectedSample            string                  `json:"guard_rejected_sample,omitempty"`
	DurationMs                     int                     `json:"duration_ms"`
	CachePrefixHitOnCompactionCall bool                    `json:"cache_prefix_hit_on_compaction_call"`
	ProviderUsage                  *BrokerUsageLog         `json:"provider_usage,omitempty"`
}

type RoundLog struct {
	Round        int             `json:"round"`
	RemainingSec int             `json:"remaining_sec"`
	Decision     AgentDecision   `json:"decision"`
	Observation  string          `json:"observation"`
	ActionError  bool            `json:"action_error,omitempty"`
	ErrorKind    string          `json:"error_kind,omitempty"`
	RawOutput    string          `json:"raw_output,omitempty"`
	ResponseID   string          `json:"response_id"`
	ParseError   string          `json:"parse_error,omitempty"`
	FallbackUsed bool            `json:"fallback_used,omitempty"`
	InputTokens  int             `json:"input_tokens"`
	CachedTokens int             `json:"cached_tokens"`
	OutputTokens int             `json:"output_tokens"`
	Broker       *BrokerUsageLog `json:"broker,omitempty"`
}

type ProgressEvent struct {
	Phase               string       `json:"phase"`
	Round               int          `json:"round,omitempty"`
	RemainingSec        int          `json:"remaining_sec,omitempty"`
	Action              string       `json:"action,omitempty"`
	ResponseID          string       `json:"response_id,omitempty"`
	InFlightStartedAt   string       `json:"in_flight_started_at,omitempty"`
	LastRoundFinishedAt string       `json:"last_round_finished_at,omitempty"`
	InputTokens         int          `json:"input_tokens,omitempty"`
	CachedTokens        int          `json:"cached_tokens,omitempty"`
	OutputTokens        int          `json:"output_tokens,omitempty"`
	TotalInputTokens    int          `json:"total_input_tokens,omitempty"`
	TotalCachedTokens   int          `json:"total_cached_tokens,omitempty"`
	TotalOutputTokens   int          `json:"total_output_tokens,omitempty"`
	SummaryPane         *SummaryPane `json:"summary_pane,omitempty"`
}

type RunOptions struct {
	ContinueOnNoop         bool
	Purpose                string
	CompactionRecentRounds int
}

type FinalReport struct {
	Resident         string            `json:"resident"`
	Model            string            `json:"model"`
	DurationSeconds  int               `json:"duration_seconds"`
	Rounds           int               `json:"rounds"`
	StartedAt        string            `json:"started_at"`
	EndedAt          string            `json:"ended_at"`
	Acceptance       string            `json:"acceptance"`
	AcceptanceBroker *BrokerUsageLog   `json:"acceptance_broker,omitempty"`
	RoundLogs        []RoundLog        `json:"round_logs"`
	StoppedReason    string            `json:"stopped_reason,omitempty"`
	SummaryPane      *SummaryPane      `json:"summary_pane,omitempty"`
	CompactionEvents []CompactionEvent `json:"compaction_events,omitempty"`
}

type PartialRunError struct {
	Report FinalReport
	Err    error
}

func (e *PartialRunError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *PartialRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
