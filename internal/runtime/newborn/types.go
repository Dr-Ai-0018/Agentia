package newborn

import "ai-arena/internal/brokerstate"

type AgentDecision struct {
	Situation  string `json:"situation"`
	NextAction string `json:"next_action"`
	Command    string `json:"command,omitempty"`
	Message    string `json:"message,omitempty"`
	Reason     string `json:"reason"`
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
