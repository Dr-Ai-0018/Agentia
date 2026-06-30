package broker

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultProbeDurationSeconds = 45
	defaultSoakDurationSeconds  = 600
	defaultBudgetSafetyFactor   = 1.35
	defaultMinimumProbeSpark    = 4.0
	defaultMinimumSoakSpark     = 12.0
	defaultWindow6HDelta        = 600000
)

type OrchestratorBudgetEstimateOutput struct {
	GeneratedFromRuns int                              `json:"generated_from_runs"`
	HistoryLimit      int                              `json:"history_limit"`
	ProbeDurationSec  int                              `json:"probe_duration_sec"`
	SoakDurationSec   int                              `json:"soak_duration_sec"`
	SafetyFactor      float64                          `json:"safety_factor"`
	Residents         []ResidentBudgetEstimate         `json:"residents"`
	Totals            BudgetEstimateTotals             `json:"totals"`
	Recommended       BudgetEstimateRecommendedCommand `json:"recommended"`
	Notes             []string                         `json:"notes,omitempty"`
}

type ResidentBudgetEstimate struct {
	Resident              string   `json:"resident"`
	Model                 string   `json:"model,omitempty"`
	Samples               int      `json:"samples"`
	Runs                  int      `json:"runs"`
	ObservedDurationSec   float64  `json:"observed_duration_sec"`
	ObservedSparkTotal    float64  `json:"observed_spark_total"`
	AvgSparkPerRound      float64  `json:"avg_spark_per_round"`
	P95SparkPerRound      float64  `json:"p95_spark_per_round"`
	MaxSparkPerRound      float64  `json:"max_spark_per_round"`
	SparkPerSecond        float64  `json:"spark_per_second"`
	ProbeSparkRecommended float64  `json:"probe_spark_recommended"`
	SoakSparkRecommended  float64  `json:"soak_spark_recommended"`
	Reasons               []string `json:"reasons,omitempty"`
}

type BudgetEstimateTotals struct {
	ProbeSparkRecommended float64 `json:"probe_spark_recommended"`
	SoakSparkRecommended  float64 `json:"soak_spark_recommended"`
	Window6HDelta         int     `json:"window_6h_delta"`
}

type BudgetEstimateRecommendedCommand struct {
	ProbeAllowanceByResident []string `json:"probe_allowance_by_resident"`
	SoakAllowanceByResident  []string `json:"soak_allowance_by_resident"`
	ProbeRun                 string   `json:"probe_run"`
	SoakRun                  string   `json:"soak_run"`
}

type orchestratorBudgetSummary struct {
	RunID    string `json:"run_id"`
	Duration string `json:"duration"`
	Runs     []struct {
		Resident string `json:"resident"`
		Report   struct {
			Resident  string `json:"resident"`
			Model     string `json:"model"`
			StartedAt string `json:"started_at"`
			EndedAt   string `json:"ended_at"`
			Rounds    int    `json:"rounds"`
			RoundLogs []struct {
				Broker *struct {
					Applied    bool    `json:"applied"`
					SparkDelta float64 `json:"spark_delta"`
				} `json:"broker"`
			} `json:"round_logs"`
		} `json:"report"`
	} `json:"runs"`
}

type residentBudgetAccumulator struct {
	resident        string
	model           string
	costs           []float64
	runs            int
	observedSeconds float64
	totalSpark      float64
}

func (a *App) RunOrchestratorBudgetEstimate(limit int) (OrchestratorBudgetEstimateOutput, error) {
	return EstimateOrchestratorBudget(a.root, limit, defaultProbeDurationSeconds, defaultSoakDurationSeconds, defaultBudgetSafetyFactor)
}

func EstimateOrchestratorBudget(root string, limit, probeSeconds, soakSeconds int, safetyFactor float64) (OrchestratorBudgetEstimateOutput, error) {
	if limit <= 0 {
		limit = 8
	}
	if probeSeconds <= 0 {
		probeSeconds = defaultProbeDurationSeconds
	}
	if soakSeconds <= 0 {
		soakSeconds = defaultSoakDurationSeconds
	}
	if safetyFactor <= 0 {
		safetyFactor = defaultBudgetSafetyFactor
	}

	runRoot := filepath.Join(root, "orchestrator-runs")
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return OrchestratorBudgetEstimateOutput{}, nil
		}
		return OrchestratorBudgetEstimateOutput{}, err
	}

	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.TrimSpace(entry.Name()) != "" {
			runIDs = append(runIDs, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runIDs)))

	accs := map[string]*residentBudgetAccumulator{}
	loaded := 0
	for _, runID := range runIDs {
		path := filepath.Join(runRoot, runID, "summary.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var summary orchestratorBudgetSummary
		if err := json.Unmarshal(raw, &summary); err != nil {
			continue
		}
		if len(summary.Runs) == 0 {
			continue
		}
		loaded++
		for _, run := range summary.Runs {
			report := run.Report
			resident := strings.TrimSpace(report.Resident)
			if resident == "" {
				resident = strings.TrimSpace(run.Resident)
			}
			if resident == "" {
				continue
			}
			acc := accs[resident]
			if acc == nil {
				acc = &residentBudgetAccumulator{resident: resident}
				accs[resident] = acc
			}
			if report.Model != "" {
				acc.model = report.Model
			}
			var runSpark float64
			var runCosts int
			for _, round := range report.RoundLogs {
				if round.Broker == nil || !round.Broker.Applied || round.Broker.SparkDelta >= 0 {
					continue
				}
				cost := -round.Broker.SparkDelta
				acc.costs = append(acc.costs, cost)
				runSpark += cost
				runCosts++
			}
			if runCosts > 0 {
				acc.runs++
				acc.totalSpark += runSpark
				if seconds := parseObservedSeconds(report.StartedAt, report.EndedAt); seconds > 0 {
					acc.observedSeconds += seconds
				}
			}
		}
		if loaded >= limit {
			break
		}
	}

	out := OrchestratorBudgetEstimateOutput{
		GeneratedFromRuns: loaded,
		HistoryLimit:      limit,
		ProbeDurationSec:  probeSeconds,
		SoakDurationSec:   soakSeconds,
		SafetyFactor:      roundFloat(safetyFactor),
		Notes: []string{
			"estimate is based on applied historical work-call spark deltas from orchestrator summary.json files",
			"probe allowance is intentionally small; soak allowance is separate and should be issued only after stop-before-debt probe verification",
			"test allowance cards are for controlled probe/ordinary soak recovery only; they must not be used as evidence for ultra_long_soak_pre_release",
		},
	}

	residents := make([]string, 0, len(accs))
	for resident := range accs {
		residents = append(residents, resident)
	}
	sort.Strings(residents)
	for _, resident := range residents {
		estimate := buildResidentBudgetEstimate(accs[resident], probeSeconds, soakSeconds, safetyFactor)
		out.Residents = append(out.Residents, estimate)
		out.Totals.ProbeSparkRecommended += estimate.ProbeSparkRecommended
		out.Totals.SoakSparkRecommended += estimate.SoakSparkRecommended
	}
	out.Totals.ProbeSparkRecommended = roundUpSpark(out.Totals.ProbeSparkRecommended)
	out.Totals.SoakSparkRecommended = roundUpSpark(out.Totals.SoakSparkRecommended)
	out.Totals.Window6HDelta = defaultWindow6HDelta

	roster := strings.Join(residents, ",")
	if roster == "" {
		roster = "jade,amber,onyx"
	}
	recommended := BudgetEstimateRecommendedCommand{
		ProbeRun: "arena-orchestrator --mode run --run-mode parallel --residents jade --duration 45s",
		SoakRun:  "arena-orchestrator --mode run --run-mode parallel --residents " + roster + " --duration 10m",
	}
	for _, estimate := range out.Residents {
		recommended.ProbeAllowanceByResident = append(recommended.ProbeAllowanceByResident, allowanceCommand(estimate.Resident, estimate.ProbeSparkRecommended, defaultWindow6HDelta, "<45s probe test reason>"))
		recommended.SoakAllowanceByResident = append(recommended.SoakAllowanceByResident, allowanceCommand(estimate.Resident, estimate.SoakSparkRecommended, defaultWindow6HDelta, "<10m soak test reason>"))
	}
	out.Recommended = recommended
	return out, nil
}

func buildResidentBudgetEstimate(acc *residentBudgetAccumulator, probeSeconds, soakSeconds int, safetyFactor float64) ResidentBudgetEstimate {
	out := ResidentBudgetEstimate{
		Resident:            acc.resident,
		Model:               acc.model,
		Samples:             len(acc.costs),
		Runs:                acc.runs,
		ObservedDurationSec: roundFloat(acc.observedSeconds),
		ObservedSparkTotal:  roundFloat(acc.totalSpark),
	}
	if len(acc.costs) == 0 {
		out.ProbeSparkRecommended = defaultMinimumProbeSpark
		out.SoakSparkRecommended = defaultMinimumSoakSpark
		out.Reasons = append(out.Reasons, "no historical applied work-call samples; using minimum defaults")
		return out
	}
	sort.Float64s(acc.costs)
	out.AvgSparkPerRound = roundFloat(acc.totalSpark / float64(len(acc.costs)))
	out.P95SparkPerRound = roundFloat(percentileNearest(acc.costs, 0.95))
	out.MaxSparkPerRound = roundFloat(acc.costs[len(acc.costs)-1])
	if acc.observedSeconds > 0 {
		out.SparkPerSecond = roundFloat(acc.totalSpark / acc.observedSeconds)
	}

	probeByRate := float64(probeSeconds) * out.SparkPerSecond * safetyFactor
	probeByRounds := out.P95SparkPerRound * 2 * safetyFactor
	soakByRate := float64(soakSeconds) * out.SparkPerSecond * safetyFactor
	soakByRounds := out.P95SparkPerRound * 4 * safetyFactor

	out.ProbeSparkRecommended = roundUpSpark(maxFloat(defaultMinimumProbeSpark, probeByRate, probeByRounds))
	out.SoakSparkRecommended = roundUpSpark(maxFloat(defaultMinimumSoakSpark, soakByRate, soakByRounds))
	out.Reasons = append(out.Reasons,
		fmt.Sprintf("probe=max(minimum %.1f, rate %.2f, p95_two_rounds %.2f)", defaultMinimumProbeSpark, probeByRate, probeByRounds),
		fmt.Sprintf("soak=max(minimum %.1f, rate %.2f, p95_four_rounds %.2f)", defaultMinimumSoakSpark, soakByRate, soakByRounds),
	)
	return out
}

func allowanceCommand(residents string, spark float64, windowDelta int, reason string) string {
	return fmt.Sprintf("arena-broker --mode test-allowance-card --resident %s --spark-amount %.1f --window-6h-delta %d --reset-window-6h-used --reason '%s' --operator <operator>", residents, spark, windowDelta, reason)
}

func parseObservedSeconds(started, ended string) float64 {
	start, err := parseTimeFlexible(started)
	if err != nil {
		return 0
	}
	end, err := parseTimeFlexible(ended)
	if err != nil || !end.After(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

func parseTimeFlexible(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func percentileNearest(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func roundUpSpark(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Ceil(v*10) / 10
}

func roundFloat(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func maxFloat(values ...float64) float64 {
	var out float64
	for i, value := range values {
		if i == 0 || value > out {
			out = value
		}
	}
	return out
}
