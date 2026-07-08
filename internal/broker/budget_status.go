package broker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-arena/internal/brokerstate"
)

type BudgetStatusOutput struct {
	ResidentCount int                    `json:"resident_count"`
	Residents     []ResidentBudgetStatus `json:"residents"`
	Totals        BudgetStatusTotals     `json:"totals"`
}

type ResidentBudgetStatus struct {
	ResidentID                  string        `json:"resident_id"`
	SparkBalance                float64       `json:"spark_balance"`
	WorkAllowedNow              bool          `json:"work_allowed_now"`
	BlockingReason              string        `json:"blocking_reason,omitempty"`
	Window6HUsed                int           `json:"window_6h_used"`
	RollingWindow6HUsed         int           `json:"rolling_6h_used,omitempty"`
	EffectiveWindow6HCap        int           `json:"effective_window_6h_cap"`
	EffectiveWindow6HRemaining  int           `json:"effective_window_6h_remaining"`
	DayUsed                     int           `json:"day_used"`
	RollingDayUsed              int           `json:"rolling_day_used,omitempty"`
	RollingDayRemaining         int           `json:"rolling_day_remaining,omitempty"`
	EffectiveDayCap             int           `json:"effective_day_cap"`
	EffectiveDayRemaining       int           `json:"effective_day_remaining"`
	WeekUsed                    int           `json:"week_used"`
	RollingWeekUsed             int           `json:"rolling_week_used,omitempty"`
	RollingWeekRemaining        int           `json:"rolling_week_remaining,omitempty"`
	EffectiveWeekCap            int           `json:"effective_week_cap"`
	EffectiveWeekRemaining      int           `json:"effective_week_remaining"`
	QuotaTightestLayer          string        `json:"quota_tightest_layer,omitempty"`
	QuotaTightestRemainingRatio float64       `json:"quota_tightest_remaining_ratio,omitempty"`
	Pressure                    string        `json:"pressure,omitempty"`
	RecoverySuggested           bool          `json:"recovery_suggested,omitempty"`
	RecoveryUrgency             string        `json:"recovery_urgency,omitempty"`
	NextRecoveryAt              string        `json:"next_recovery_at,omitempty"`
	Fatigue                     FatigueStatus `json:"fatigue"`
	Sleep                       SleepStatus   `json:"sleep"`
}

type FatigueStatus struct {
	Level int    `json:"level"`
	Mood  string `json:"mood"`
}

type SleepStatus struct {
	Depth     string  `json:"depth"`
	DebtHours float64 `json:"debt_hours"`
}

type BudgetStatusTotals struct {
	SparkBalance               float64 `json:"spark_balance"`
	WorkAllowedCount           int     `json:"work_allowed_count"`
	BlockedCount               int     `json:"blocked_count"`
	EffectiveWindow6HRemaining int     `json:"effective_window_6h_remaining"`
	RollingDayRemaining        int     `json:"rolling_day_remaining,omitempty"`
	EffectiveDayRemaining      int     `json:"effective_day_remaining"`
	RollingWeekRemaining       int     `json:"rolling_week_remaining,omitempty"`
	EffectiveWeekRemaining     int     `json:"effective_week_remaining"`
}

type OrchestratorBudgetReportOutput struct {
	RunID         string                             `json:"run_id"`
	ResidentCount int                                `json:"resident_count"`
	Residents     []ResidentOrchestratorBudgetReport `json:"residents"`
	Totals        OrchestratorBudgetReportTotals     `json:"totals"`
	CacheHealth   string                             `json:"cache_health"`
	Warnings      []string                           `json:"warnings,omitempty"`
	SparkPerUSD   float64                            `json:"spark_per_usd"`
	InternalUSD   float64                            `json:"internal_usd"`
	Source        string                             `json:"source"`
}

type ResidentOrchestratorBudgetReport struct {
	ResidentID         string  `json:"resident_id"`
	Model              string  `json:"model,omitempty"`
	Rounds             int     `json:"rounds"`
	ChargedCalls       int     `json:"charged_calls"`
	InputTokens        int     `json:"input_tokens"`
	CachedTokens       int     `json:"cached_tokens"`
	CacheHitRatio      float64 `json:"cache_hit_ratio"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	SpentSpark         float64 `json:"spent_spark"`
	InternalUSD        float64 `json:"internal_usd"`
	AllowanceSpark     float64 `json:"allowance_spark,omitempty"`
	AllowanceUsedRatio float64 `json:"allowance_used_ratio,omitempty"`
}

type OrchestratorBudgetReportTotals struct {
	ChargedCalls   int     `json:"charged_calls"`
	InputTokens    int     `json:"input_tokens"`
	CachedTokens   int     `json:"cached_tokens"`
	CacheHitRatio  float64 `json:"cache_hit_ratio"`
	OutputTokens   int     `json:"output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	SpentSpark     float64 `json:"spent_spark"`
	AllowanceSpark float64 `json:"allowance_spark,omitempty"`
}

type orchestratorBudgetReportSummary struct {
	RunID string `json:"run_id"`
	Runs  []struct {
		Resident string `json:"resident"`
		Report   struct {
			Resident         string `json:"resident"`
			Model            string `json:"model"`
			Rounds           int    `json:"rounds"`
			AcceptanceBroker *struct {
				Applied    bool    `json:"applied"`
				SparkDelta float64 `json:"spark_delta"`
			} `json:"acceptance_broker"`
			RoundLogs []struct {
				InputTokens  int `json:"input_tokens"`
				CachedTokens int `json:"cached_tokens"`
				OutputTokens int `json:"output_tokens"`
				Broker       *struct {
					Applied    bool    `json:"applied"`
					SparkDelta float64 `json:"spark_delta"`
				} `json:"broker"`
			} `json:"round_logs"`
		} `json:"report"`
	} `json:"runs"`
}

const sparkPerInternalUSD = 100.0

func (a *App) RunBudgetStatus(residentIDs []string) (BudgetStatusOutput, error) {
	residentIDs = normalizeResidentIDs(residentIDs, a.cfg.Residents)
	out := BudgetStatusOutput{ResidentCount: len(residentIDs)}
	for _, residentID := range residentIDs {
		status, err := a.RunStatus(residentID)
		if err != nil {
			return BudgetStatusOutput{}, err
		}
		quota := brokerstate.BuildQuotaSnapshot(status)
		row := ResidentBudgetStatus{
			ResidentID:                  residentID,
			SparkBalance:                roundFloat(status.SparkBalance),
			WorkAllowedNow:              quota.WorkAllowedNow,
			BlockingReason:              quota.BlockingReason,
			Window6HUsed:                quota.Window6HUsed,
			RollingWindow6HUsed:         quota.RollingWindow6HUsed,
			EffectiveWindow6HCap:        quota.EffectiveWindow6HCap,
			EffectiveWindow6HRemaining:  quota.EffectiveWindow6HRemaining,
			DayUsed:                     quota.DayUsed,
			RollingDayUsed:              quota.RollingDayUsed,
			RollingDayRemaining:         quota.RollingDayRemaining,
			EffectiveDayCap:             quota.EffectiveDayCap,
			EffectiveDayRemaining:       quota.EffectiveDayRemaining,
			WeekUsed:                    quota.WeekUsed,
			RollingWeekUsed:             quota.RollingWeekUsed,
			RollingWeekRemaining:        quota.RollingWeekRemaining,
			EffectiveWeekCap:            quota.EffectiveWeekCap,
			EffectiveWeekRemaining:      quota.EffectiveWeekRemaining,
			QuotaTightestLayer:          status.Physiology.QuotaTightestLayer,
			QuotaTightestRemainingRatio: roundFloat(status.Physiology.QuotaTightestRatio),
			Pressure:                    status.Physiology.Pressure,
			RecoverySuggested:           status.Physiology.RecoverySuggested,
			RecoveryUrgency:             status.Physiology.RecoveryUrgency,
			NextRecoveryAt:              status.NextRecoveryAt,
			Fatigue:                     buildFatigueStatus(status),
			Sleep:                       buildSleepStatus(status),
		}
		out.Residents = append(out.Residents, row)
		out.Totals.SparkBalance += row.SparkBalance
		out.Totals.EffectiveWindow6HRemaining += row.EffectiveWindow6HRemaining
		out.Totals.RollingDayRemaining += row.RollingDayRemaining
		out.Totals.EffectiveDayRemaining += row.EffectiveDayRemaining
		out.Totals.RollingWeekRemaining += row.RollingWeekRemaining
		out.Totals.EffectiveWeekRemaining += row.EffectiveWeekRemaining
		if row.WorkAllowedNow {
			out.Totals.WorkAllowedCount++
		} else {
			out.Totals.BlockedCount++
		}
	}
	out.Totals.SparkBalance = roundFloat(out.Totals.SparkBalance)
	return out, nil
}

func buildFatigueStatus(status brokerstate.ResidentStatus) FatigueStatus {
	level := 0
	cap := brokerstate.DefaultRuntimeConfig().FatigueCap
	if cap > 0 && status.Fatigue > 0 {
		level = int(float64(status.Fatigue) / float64(cap) * 100)
		if level > 100 {
			level = 100
		}
	}
	return FatigueStatus{
		Level: level,
		Mood:  fatigueMood(level),
	}
}

func fatigueMood(level int) string {
	switch {
	case level >= 80:
		return "exhausted"
	case level >= 60:
		return "quite_tired"
	case level >= 40:
		return "some_tiredness"
	case level >= 20:
		return "warming_up"
	default:
		return "fresh"
	}
}

func buildSleepStatus(status brokerstate.ResidentStatus) SleepStatus {
	depth := string(status.Sleep.Depth)
	if depth == "" {
		depth = "awake"
	}
	return SleepStatus{
		Depth:     depth,
		DebtHours: status.Sleep.DebtHours,
	}
}

func (a *App) RunOrchestratorBudgetReport(runID string, allowances map[string]float64) (OrchestratorBudgetReportOutput, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return OrchestratorBudgetReportOutput{}, fmt.Errorf("run id is required")
	}
	path := filepath.Join(a.root, "orchestrator-runs", runID, "summary.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return OrchestratorBudgetReportOutput{}, err
	}
	var summary orchestratorBudgetReportSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return OrchestratorBudgetReportOutput{}, err
	}
	if summary.RunID == "" {
		summary.RunID = runID
	}
	out := OrchestratorBudgetReportOutput{
		RunID:       summary.RunID,
		SparkPerUSD: sparkPerInternalUSD,
		Source:      path,
	}
	for _, run := range summary.Runs {
		report := run.Report
		residentID := strings.TrimSpace(report.Resident)
		if residentID == "" {
			residentID = strings.TrimSpace(run.Resident)
		}
		if residentID == "" {
			continue
		}
		row := ResidentOrchestratorBudgetReport{
			ResidentID: residentID,
			Model:      report.Model,
			Rounds:     report.Rounds,
		}
		if report.AcceptanceBroker != nil && report.AcceptanceBroker.Applied && report.AcceptanceBroker.SparkDelta < 0 {
			row.ChargedCalls++
			row.SpentSpark += -report.AcceptanceBroker.SparkDelta
		}
		for _, round := range report.RoundLogs {
			row.InputTokens += round.InputTokens
			row.CachedTokens += round.CachedTokens
			row.OutputTokens += round.OutputTokens
			if round.Broker != nil && round.Broker.Applied && round.Broker.SparkDelta < 0 {
				row.ChargedCalls++
				row.SpentSpark += -round.Broker.SparkDelta
			}
		}
		row.TotalTokens = row.InputTokens + row.OutputTokens
		row.CacheHitRatio = ratio(row.CachedTokens, row.InputTokens)
		row.SpentSpark = roundFloat(row.SpentSpark)
		row.InternalUSD = roundFloat(row.SpentSpark / sparkPerInternalUSD)
		if allowance := allowances[residentID]; allowance > 0 {
			row.AllowanceSpark = roundFloat(allowance)
			row.AllowanceUsedRatio = roundFloat(row.SpentSpark / allowance)
		}
		out.Residents = append(out.Residents, row)
		out.Totals.ChargedCalls += row.ChargedCalls
		out.Totals.InputTokens += row.InputTokens
		out.Totals.CachedTokens += row.CachedTokens
		out.Totals.OutputTokens += row.OutputTokens
		out.Totals.TotalTokens += row.TotalTokens
		out.Totals.SpentSpark += row.SpentSpark
		out.Totals.AllowanceSpark += row.AllowanceSpark
	}
	sort.Slice(out.Residents, func(i, j int) bool {
		return out.Residents[i].ResidentID < out.Residents[j].ResidentID
	})
	out.ResidentCount = len(out.Residents)
	out.Totals.SpentSpark = roundFloat(out.Totals.SpentSpark)
	out.Totals.AllowanceSpark = roundFloat(out.Totals.AllowanceSpark)
	out.Totals.CacheHitRatio = ratio(out.Totals.CachedTokens, out.Totals.InputTokens)
	out.InternalUSD = roundFloat(out.Totals.SpentSpark / sparkPerInternalUSD)
	out.CacheHealth = cacheHealth(out.Totals.CacheHitRatio)
	if out.CacheHealth == "poor" {
		out.Warnings = append(out.Warnings, "prompt cache hit ratio is below 50%; do not run another long soak until stable prompt prefix reuse is verified")
	}
	return out, nil
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return roundFloat(float64(numerator) / float64(denominator))
}

func cacheHealth(hitRatio float64) string {
	switch {
	case hitRatio >= 0.75:
		return "good"
	case hitRatio >= 0.50:
		return "watch"
	default:
		return "poor"
	}
}

func normalizeResidentIDs(residentIDs []string, bindings []ResidentBinding) []string {
	seen := map[string]bool{}
	var out []string
	for _, residentID := range residentIDs {
		residentID = strings.TrimSpace(residentID)
		if residentID == "" || seen[residentID] {
			continue
		}
		seen[residentID] = true
		out = append(out, residentID)
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, binding := range bindings {
		if binding.ResidentID == "" || seen[binding.ResidentID] {
			continue
		}
		seen[binding.ResidentID] = true
		out = append(out, binding.ResidentID)
	}
	sort.Strings(out)
	return out
}
