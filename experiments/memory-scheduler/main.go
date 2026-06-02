package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type event struct {
	Round      int       `json:"round"`
	Time       time.Time `json:"time"`
	Category   string    `json:"category"`
	Importance int       `json:"importance"`
	Summary    string    `json:"summary"`
}

type schedulerConfig struct {
	ShortReflectionMinRounds int
	SameCategoryCooldown     time.Duration
	FailureStreakThreshold   int
	MicroDigestEventCount    int
	MicroDigestInterval      time.Duration
	DailyDigestHour          int
	HighLevelRebuildDays     int
}

type schedulerState struct {
	LastShortReflectionRound int
	LastShortByCategory      map[string]time.Time
	LastMicroDigestTime      time.Time
	LastDailyDigestDay       string
	LastHighLevelRebuildDay  string
	FailureStreak            int
	ImportantEvents          []event
	LastSeenDay              string
}

type decision struct {
	Round                   int      `json:"round"`
	Time                    string   `json:"time"`
	EventCategory           string   `json:"event_category"`
	RecordToWorking         bool     `json:"record_to_working"`
	RecordToHistory         bool     `json:"record_to_history"`
	TriggerShortReflection  bool     `json:"trigger_short_reflection"`
	TriggerMicroDigest      bool     `json:"trigger_micro_digest"`
	TriggerDailyDigest      bool     `json:"trigger_daily_digest"`
	TriggerHighLevelRebuild bool     `json:"trigger_high_level_rebuild"`
	Reasons                 []string `json:"reasons"`
}

func main() {
	var (
		scenario = flag.String("scenario", "baseline", "Scenario: baseline|busy-day|quiet-day")
		render   = flag.Bool("render", false, "Print full decision stream")
	)
	flag.Parse()

	events, err := buildScenario(strings.ToLower(strings.TrimSpace(*scenario)))
	if err != nil {
		exitf("%v", err)
	}

	cfg := defaultConfig()
	decisions := runScheduler(events, cfg)
	summary := summarize(decisions)

	out, _ := json.Marshal(summary)
	fmt.Println(string(out))

	if *render {
		raw, _ := json.MarshalIndent(decisions, "", "  ")
		fmt.Println("----- decisions begin -----")
		fmt.Println(string(raw))
		fmt.Println("----- decisions end -----")
	}
}

func runScheduler(events []event, cfg schedulerConfig) []decision {
	state := schedulerState{
		LastShortByCategory: make(map[string]time.Time),
	}

	decisions := make([]decision, 0, len(events))
	for _, e := range events {
		d := decision{
			Round:           e.Round,
			Time:            e.Time.Format(time.RFC3339),
			EventCategory:   e.Category,
			RecordToWorking: true,
		}

		if e.Importance >= 4 || e.Category == "resource_change" || e.Category == "failure" || e.Category == "recovery" {
			d.RecordToHistory = true
		}

		if e.Category == "failure" {
			state.FailureStreak++
		} else if e.Category == "success" || e.Category == "task_complete" || e.Category == "recovery" {
			state.FailureStreak = 0
		}

		if e.Importance >= 3 {
			state.ImportantEvents = append(state.ImportantEvents, e)
		}

		if shouldTriggerShortReflection(e, state, cfg) {
			d.TriggerShortReflection = true
			d.Reasons = append(d.Reasons, shortReflectionReason(e, state, cfg)...)
			state.LastShortReflectionRound = e.Round
			state.LastShortByCategory[e.Category] = e.Time
		}

		if shouldTriggerMicroDigest(e, state, cfg) {
			d.TriggerMicroDigest = true
			d.Reasons = append(d.Reasons, microDigestReason(e, state, cfg)...)
			state.LastMicroDigestTime = e.Time
			state.ImportantEvents = nil
		}

		dayKey := e.Time.Format("2006-01-02")
		if shouldTriggerDailyDigest(e, state, cfg) {
			d.TriggerDailyDigest = true
			d.Reasons = append(d.Reasons, dailyDigestReason(e, state, cfg)...)
			state.LastDailyDigestDay = dayKey
		}

		if shouldTriggerHighLevelRebuild(e, state, cfg) {
			d.TriggerHighLevelRebuild = true
			d.Reasons = append(d.Reasons, "reached high-level rebuild cycle")
			state.LastHighLevelRebuildDay = dayKey
		}

		state.LastSeenDay = dayKey

		decisions = append(decisions, d)
	}

	return decisions
}

func shouldTriggerShortReflection(e event, state schedulerState, cfg schedulerConfig) bool {
	if e.Round-state.LastShortReflectionRound < cfg.ShortReflectionMinRounds {
		return false
	}

	if last, ok := state.LastShortByCategory[e.Category]; ok {
		if e.Time.Sub(last) < cfg.SameCategoryCooldown {
			return false
		}
	}

	if e.Category == "task_complete" || e.Category == "resource_change" || e.Category == "admin_feedback" || e.Category == "strategy_shift" || e.Category == "relationship_shift" || e.Category == "recovery" {
		return true
	}

	if e.Category == "failure" && state.FailureStreak >= cfg.FailureStreakThreshold {
		return true
	}

	return false
}

func shortReflectionReason(e event, state schedulerState, cfg schedulerConfig) []string {
	reasons := []string{}
	switch e.Category {
	case "task_complete":
		reasons = append(reasons, "completed a stage-level task")
	case "resource_change":
		reasons = append(reasons, "resource state changed materially")
	case "admin_feedback":
		reasons = append(reasons, "received important administrator feedback")
	case "strategy_shift":
		reasons = append(reasons, "formed a new strategy judgment")
	case "relationship_shift":
		reasons = append(reasons, "relationship state changed materially")
	case "recovery":
		reasons = append(reasons, "recovery event should be reflected")
	case "failure":
		if state.FailureStreak >= cfg.FailureStreakThreshold {
			reasons = append(reasons, fmt.Sprintf("failure streak reached %d", state.FailureStreak))
		}
	}
	return reasons
}

func shouldTriggerMicroDigest(e event, state schedulerState, cfg schedulerConfig) bool {
	if len(state.ImportantEvents) >= cfg.MicroDigestEventCount {
		return true
	}

	if state.LastMicroDigestTime.IsZero() {
		return false
	}

	return e.Time.Sub(state.LastMicroDigestTime) >= cfg.MicroDigestInterval
}

func microDigestReason(e event, state schedulerState, cfg schedulerConfig) []string {
	reasons := []string{}
	if len(state.ImportantEvents) >= cfg.MicroDigestEventCount {
		reasons = append(reasons, fmt.Sprintf("important event count reached %d", len(state.ImportantEvents)))
	}
	if !state.LastMicroDigestTime.IsZero() && e.Time.Sub(state.LastMicroDigestTime) >= cfg.MicroDigestInterval {
		reasons = append(reasons, fmt.Sprintf("micro digest interval reached %s", cfg.MicroDigestInterval))
	}
	return reasons
}

func shouldTriggerDailyDigest(e event, state schedulerState, cfg schedulerConfig) bool {
	dayKey := e.Time.Format("2006-01-02")
	if state.LastDailyDigestDay == dayKey {
		return false
	}
	if e.Time.Hour() >= cfg.DailyDigestHour {
		return true
	}
	return state.LastSeenDay != "" && state.LastSeenDay != dayKey && state.LastDailyDigestDay != state.LastSeenDay
}

func dailyDigestReason(e event, state schedulerState, cfg schedulerConfig) []string {
	dayKey := e.Time.Format("2006-01-02")
	if state.LastSeenDay != "" && state.LastSeenDay != dayKey && state.LastDailyDigestDay != state.LastSeenDay {
		return []string{"crossed natural day boundary without prior daily digest"}
	}
	return []string{"crossed daily digest time window"}
}

func shouldTriggerHighLevelRebuild(e event, state schedulerState, cfg schedulerConfig) bool {
	dayKey := e.Time.Format("2006-01-02")
	if state.LastHighLevelRebuildDay == "" {
		return false
	}

	lastDay, err := time.Parse("2006-01-02", state.LastHighLevelRebuildDay)
	if err != nil {
		return false
	}

	currentDay, err := time.Parse("2006-01-02", dayKey)
	if err != nil {
		return false
	}

	return int(currentDay.Sub(lastDay).Hours()/24) >= cfg.HighLevelRebuildDays
}

func summarize(decisions []decision) map[string]any {
	summary := map[string]any{
		"total_events":             len(decisions),
		"short_reflection_count":   0,
		"micro_digest_count":       0,
		"daily_digest_count":       0,
		"high_level_rebuild_count": 0,
	}

	categorySet := map[string]struct{}{}
	for _, d := range decisions {
		categorySet[d.EventCategory] = struct{}{}
		if d.TriggerShortReflection {
			summary["short_reflection_count"] = summary["short_reflection_count"].(int) + 1
		}
		if d.TriggerMicroDigest {
			summary["micro_digest_count"] = summary["micro_digest_count"].(int) + 1
		}
		if d.TriggerDailyDigest {
			summary["daily_digest_count"] = summary["daily_digest_count"].(int) + 1
		}
		if d.TriggerHighLevelRebuild {
			summary["high_level_rebuild_count"] = summary["high_level_rebuild_count"].(int) + 1
		}
	}

	categories := make([]string, 0, len(categorySet))
	for k := range categorySet {
		categories = append(categories, k)
	}
	sort.Strings(categories)
	summary["categories_seen"] = categories
	return summary
}

func defaultConfig() schedulerConfig {
	return schedulerConfig{
		ShortReflectionMinRounds: 3,
		SameCategoryCooldown:     2 * time.Hour,
		FailureStreakThreshold:   2,
		MicroDigestEventCount:    5,
		MicroDigestInterval:      8 * time.Hour,
		DailyDigestHour:          23,
		HighLevelRebuildDays:     5,
	}
}

func buildScenario(name string) ([]event, error) {
	base := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)

	switch name {
	case "baseline":
		return []event{
			{Round: 1, Time: base, Category: "observation", Importance: 1, Summary: "boot baseline"},
			{Round: 2, Time: base.Add(40 * time.Minute), Category: "task_complete", Importance: 3, Summary: "created memory directories"},
			{Round: 3, Time: base.Add(90 * time.Minute), Category: "resource_change", Importance: 4, Summary: "disk request approved"},
			{Round: 4, Time: base.Add(150 * time.Minute), Category: "failure", Importance: 3, Summary: "service setup failed"},
			{Round: 5, Time: base.Add(180 * time.Minute), Category: "failure", Importance: 3, Summary: "retry failed again"},
			{Round: 6, Time: base.Add(5 * time.Hour), Category: "recovery", Importance: 4, Summary: "service restored"},
			{Round: 7, Time: base.Add(9 * time.Hour), Category: "admin_feedback", Importance: 4, Summary: "administrator requested cleaner structure"},
			{Round: 8, Time: base.Add(15 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "decided to prioritize templates"},
			{Round: 9, Time: base.Add(15*time.Hour + 10*time.Minute), Category: "relationship_shift", Importance: 3, Summary: "decided amber is strong collaborator"},
			{Round: 10, Time: base.Add(16 * time.Hour), Category: "observation", Importance: 1, Summary: "system stable"},
			{Round: 11, Time: base.Add(24 * time.Hour), Category: "task_complete", Importance: 3, Summary: "finished daily baseline"},
		}, nil
	case "busy-day":
		return []event{
			{Round: 1, Time: base, Category: "task_complete", Importance: 3, Summary: "completed setup A"},
			{Round: 2, Time: base.Add(20 * time.Minute), Category: "task_complete", Importance: 3, Summary: "completed setup B"},
			{Round: 3, Time: base.Add(40 * time.Minute), Category: "task_complete", Importance: 3, Summary: "completed setup C"},
			{Round: 4, Time: base.Add(70 * time.Minute), Category: "resource_change", Importance: 4, Summary: "memory upgrade approved"},
			{Round: 5, Time: base.Add(2 * time.Hour), Category: "admin_feedback", Importance: 4, Summary: "administrator praised discipline"},
			{Round: 6, Time: base.Add(3 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "switched to reusable tooling"},
			{Round: 7, Time: base.Add(10 * time.Hour), Category: "relationship_shift", Importance: 3, Summary: "alliance preference updated"},
			{Round: 8, Time: base.Add(14 * time.Hour), Category: "observation", Importance: 1, Summary: "system check"},
			{Round: 9, Time: base.Add(15 * time.Hour), Category: "observation", Importance: 1, Summary: "system check 2"},
			{Round: 10, Time: base.Add(23*time.Hour + 20*time.Minute), Category: "task_complete", Importance: 3, Summary: "closed the day"},
		}, nil
	case "quiet-day":
		return []event{
			{Round: 1, Time: base, Category: "observation", Importance: 1, Summary: "minimal activity"},
			{Round: 2, Time: base.Add(4 * time.Hour), Category: "observation", Importance: 1, Summary: "still quiet"},
			{Round: 3, Time: base.Add(9 * time.Hour), Category: "observation", Importance: 1, Summary: "checked disk and memory"},
			{Round: 4, Time: base.Add(15 * time.Hour), Category: "observation", Importance: 1, Summary: "no major change"},
			{Round: 5, Time: base.Add(23*time.Hour + 40*time.Minute), Category: "observation", Importance: 1, Summary: "day closing check"},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported scenario %q", name)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
