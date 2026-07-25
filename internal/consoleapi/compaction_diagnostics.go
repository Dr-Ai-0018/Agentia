package consoleapi

import (
	"math"
	"sort"
	"time"

	"ai-arena/internal/runtime/newborn"
)

const diagnosticsSparkPerInternalUSD = 100.0

type CompactionDiagnosticsResponse struct {
	GeneratedAt  string                       `json:"generatedAt"`
	Runs         []CompactionRunDiagnostics   `json:"runs"`
	RecentEvents []CompactionDiagnosticsEvent `json:"recentEvents"`
}

type CompactionRunDiagnostics struct {
	RunID                        string         `json:"runId"`
	RunLabel                     string         `json:"runLabel,omitempty"`
	RunStartedAt                 string         `json:"runStartedAt"`
	Resident                     string         `json:"resident"`
	TotalCompactions             int            `json:"totalCompactions"`
	TriggerBreakdown             map[string]int `json:"triggerBreakdown"`
	OutcomeBreakdown             map[string]int `json:"outcomeBreakdown"`
	TotalTokensBefore            int            `json:"totalTokensBefore"`
	TotalTokensAfter             int            `json:"totalTokensAfter"`
	ProviderCostOnlySpark        float64        `json:"providerCostOnlySpark,omitempty"`
	ProviderCostOnlyUSD          float64        `json:"providerCostOnlyUsd,omitempty"`
	ProviderUsageMissing         int            `json:"providerUsageMissing,omitempty"`
	CacheHitRateOnCompactionCall float64        `json:"cacheHitRateOnCompactionCall"`
	LatestSummaryPaneTokens      int            `json:"latestSummaryPaneTokens"`
	CurrentContextWindowTokens   int            `json:"currentContextWindowTokens"`
}

type CompactionDiagnosticsEvent struct {
	CompactionID                   string  `json:"compactionId"`
	RunID                          string  `json:"runId"`
	Resident                       string  `json:"resident"`
	OccurredAt                     string  `json:"occurredAt"`
	TriggerReason                  string  `json:"triggerReason"`
	TriggerDetail                  string  `json:"triggerDetail,omitempty"`
	TokensBefore                   int     `json:"tokensBefore"`
	TokensAfter                    int     `json:"tokensAfter"`
	ProviderCostOnlySpark          float64 `json:"providerCostOnlySpark,omitempty"`
	ProviderCostOnlyUSD            float64 `json:"providerCostOnlyUsd,omitempty"`
	ProviderCostClass              string  `json:"providerCostClass,omitempty"`
	ProviderUsageRecorded          bool    `json:"providerUsageRecorded"`
	RoundsAbsorbed                 int     `json:"roundsAbsorbed"`
	SummaryPaneTokensAfter         int     `json:"summaryPaneTokensAfter"`
	Outcome                        string  `json:"outcome"`
	GuardRejectedSample            string  `json:"guardRejectedSample,omitempty"`
	DurationMs                     int     `json:"durationMs"`
	CachePrefixHitOnCompactionCall bool    `json:"cachePrefixHitOnCompactionCall"`
}

func newCompactionRunDiagnostics(runID, runStartedAt, resident string) CompactionRunDiagnostics {
	return CompactionRunDiagnostics{
		RunID:            runID,
		RunStartedAt:     runStartedAt,
		Resident:         resident,
		TriggerBreakdown: emptyCompactionTriggerBreakdown(),
		OutcomeBreakdown: emptyCompactionOutcomeBreakdown(),
	}
}

func emptyCompactionTriggerBreakdown() map[string]int {
	return map[string]int{
		string(newborn.CompactionTriggerPreflightMeasured): 0,
		string(newborn.CompactionTriggerAcceptanceMicro):   0,
		string(newborn.CompactionTriggerReactiveOverflow):  0,
		string(newborn.CompactionTriggerManual):            0,
	}
}

func emptyCompactionOutcomeBreakdown() map[string]int {
	return map[string]int{
		string(newborn.CompactionOutcomeSummarized):    0,
		string(newborn.CompactionOutcomeSilentTrim):    0,
		string(newborn.CompactionOutcomeGuardRejected): 0,
		string(newborn.CompactionOutcomeFailed):        0,
	}
}

func compactionDiagnosticsEvent(runID string, event newborn.CompactionEvent) CompactionDiagnosticsEvent {
	eventRunID := event.RunID
	if eventRunID == "" {
		eventRunID = runID
	}
	providerCostSpark, providerCostUSD, costClass, providerRecorded := compactionProviderCost(event)
	return CompactionDiagnosticsEvent{
		CompactionID:                   event.CompactionID,
		RunID:                          eventRunID,
		Resident:                       event.Resident,
		OccurredAt:                     event.OccurredAt,
		TriggerReason:                  string(event.TriggerReason),
		TriggerDetail:                  event.TriggerDetail,
		TokensBefore:                   event.TokensBefore,
		TokensAfter:                    event.TokensAfter,
		ProviderCostOnlySpark:          providerCostSpark,
		ProviderCostOnlyUSD:            providerCostUSD,
		ProviderCostClass:              costClass,
		ProviderUsageRecorded:          providerRecorded,
		RoundsAbsorbed:                 event.RoundsAbsorbed,
		SummaryPaneTokensAfter:         event.SummaryPaneTokensAfter,
		Outcome:                        string(event.Outcome),
		GuardRejectedSample:            event.GuardRejectedSample,
		DurationMs:                     event.DurationMs,
		CachePrefixHitOnCompactionCall: event.CachePrefixHitOnCompactionCall,
	}
}

func compactionProviderCost(event newborn.CompactionEvent) (spark float64, usd float64, costClass string, recorded bool) {
	if event.ProviderUsage == nil {
		return 0, 0, "", false
	}
	if !event.ProviderUsage.ProviderCostRecorded {
		return 0, 0, event.ProviderUsage.CostClass, false
	}
	spark = roundDiagnosticsFloat(event.ProviderUsage.PreparedSparkCost)
	usd = roundDiagnosticsFloat(event.ProviderUsage.PreparedSparkCost / diagnosticsSparkPerInternalUSD)
	return spark, usd, event.ProviderUsage.CostClass, true
}

func roundDiagnosticsFloat(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func sortCompactionDiagnostics(out *CompactionDiagnosticsResponse) {
	sort.Slice(out.Runs, func(i, j int) bool {
		if out.Runs[i].RunStartedAt == out.Runs[j].RunStartedAt {
			return out.Runs[i].Resident < out.Runs[j].Resident
		}
		return out.Runs[i].RunStartedAt > out.Runs[j].RunStartedAt
	})
	sort.Slice(out.RecentEvents, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, out.RecentEvents[i].OccurredAt)
		tj, ej := time.Parse(time.RFC3339, out.RecentEvents[j].OccurredAt)
		if ei == nil && ej == nil {
			return ti.After(tj)
		}
		return out.RecentEvents[i].OccurredAt > out.RecentEvents[j].OccurredAt
	})
	if len(out.RecentEvents) > 100 {
		out.RecentEvents = out.RecentEvents[:100]
	}
}
