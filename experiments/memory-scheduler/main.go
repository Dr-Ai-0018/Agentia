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

type residentProfile struct {
	Name            string
	Persona         string
	CoreBias        string
	ReflectionTone  string
	InstantTTL      time.Duration
	ShortTTL        time.Duration
	LongTTL         time.Duration
	PermanentReview time.Duration
}

type generatedMemory struct {
	Kind         string `json:"kind"`
	Layer        string `json:"layer"`
	Content      string `json:"content"`
	RetentionTTL string `json:"retention_ttl"`
}

type memoryRecord struct {
	Layer            string    `json:"layer"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	Content          string    `json:"content"`
	KeepPrompt       string    `json:"keep_prompt,omitempty"`
	DeletionProposal string    `json:"deletion_proposal,omitempty"`
}

type schedulerState struct {
	LastShortReflectionRound int
	LastShortByCategory      map[string]time.Time
	LastMicroDigestTime      time.Time
	LastDailyDigestDay       string
	LastHighLevelRebuildTime time.Time
	FailureStreak            int
	ImportantEvents          []event
	LastSeenDay              string
	MemoryLedger             []memoryRecord
}

type decision struct {
	Round                   int               `json:"round"`
	Time                    string            `json:"time"`
	EventCategory           string            `json:"event_category"`
	RecordToWorking         bool              `json:"record_to_working"`
	RecordToHistory         bool              `json:"record_to_history"`
	TriggerShortReflection  bool              `json:"trigger_short_reflection"`
	TriggerMicroDigest      bool              `json:"trigger_micro_digest"`
	TriggerDailyDigest      bool              `json:"trigger_daily_digest"`
	TriggerHighLevelRebuild bool              `json:"trigger_high_level_rebuild"`
	Reasons                 []string          `json:"reasons"`
	Generated               []generatedMemory `json:"generated,omitempty"`
	RetentionAlerts         []string          `json:"retention_alerts,omitempty"`
}

func main() {
	var (
		scenario = flag.String("scenario", "baseline", "Scenario: baseline|busy-day|quiet-day")
		resident = flag.String("resident", "jade", "Resident: jade|amber|onyx")
		render   = flag.Bool("render", false, "Print full decision stream")
	)
	flag.Parse()

	events, err := buildScenario(strings.ToLower(strings.TrimSpace(*scenario)))
	if err != nil {
		exitf("%v", err)
	}

	profile, err := buildResidentProfile(strings.ToLower(strings.TrimSpace(*resident)))
	if err != nil {
		exitf("%v", err)
	}

	cfg := defaultConfig()
	decisions := runScheduler(events, cfg, profile)
	summary := summarize(decisions, profile)

	out, _ := json.Marshal(summary)
	fmt.Println(string(out))

	if *render {
		raw, _ := json.MarshalIndent(decisions, "", "  ")
		fmt.Println("----- decisions begin -----")
		fmt.Println(string(raw))
		fmt.Println("----- decisions end -----")
	}
}

func runScheduler(events []event, cfg schedulerConfig, profile residentProfile) []decision {
	state := schedulerState{
		LastShortByCategory:      make(map[string]time.Time),
		LastHighLevelRebuildTime: events[0].Time,
	}

	decisions := make([]decision, 0, len(events))
	for i, e := range events {
		d := decision{
			Round:           e.Round,
			Time:            e.Time.Format(time.RFC3339),
			EventCategory:   e.Category,
			RecordToWorking: true,
		}

		d.RetentionAlerts = append(d.RetentionAlerts, checkRetentionAlerts(profile, state.MemoryLedger, e.Time)...)

		instantRecord := memoryRecord{
			Layer:     "instant",
			CreatedAt: e.Time,
			ExpiresAt: e.Time.Add(profile.InstantTTL),
			Content:   e.Summary,
		}
		state.MemoryLedger = append(state.MemoryLedger, instantRecord)

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
			content := renderShortReflection(profile, recentWindow(events, i, 3), e, state)
			d.Generated = append(d.Generated, generatedMemory{
				Kind:         "short_reflection",
				Layer:        "short",
				Content:      content,
				RetentionTTL: profile.ShortTTL.String(),
			})
			state.MemoryLedger = append(state.MemoryLedger, memoryRecord{
				Layer:     "short",
				CreatedAt: e.Time,
				ExpiresAt: e.Time.Add(profile.ShortTTL),
				Content:   content,
			})
			state.LastShortReflectionRound = e.Round
			state.LastShortByCategory[e.Category] = e.Time
		}

		if shouldTriggerMicroDigest(e, state, cfg) {
			content := renderMicroDigest(profile, state.ImportantEvents)
			if content != "" {
				d.TriggerMicroDigest = true
				d.Reasons = append(d.Reasons, microDigestReason(e, state, cfg)...)
				d.Generated = append(d.Generated, generatedMemory{
					Kind:         "micro_digest",
					Layer:        "long",
					Content:      content,
					RetentionTTL: profile.LongTTL.String(),
				})
				state.MemoryLedger = append(state.MemoryLedger, memoryRecord{
					Layer:     "long",
					CreatedAt: e.Time,
					ExpiresAt: e.Time.Add(profile.LongTTL),
					Content:   content,
				})
			}
			state.LastMicroDigestTime = e.Time
			state.ImportantEvents = nil
		}

		dayKey := e.Time.Format("2006-01-02")
		if shouldTriggerDailyDigest(e, state, cfg) {
			d.TriggerDailyDigest = true
			d.Reasons = append(d.Reasons, dailyDigestReason(e, state, cfg)...)
			content := renderDailyDigest(profile, eventsForDay(events, e.Time, i))
			d.Generated = append(d.Generated, generatedMemory{
				Kind:         "daily_digest",
				Layer:        "long",
				Content:      content,
				RetentionTTL: profile.LongTTL.String(),
			})
			state.MemoryLedger = append(state.MemoryLedger, memoryRecord{
				Layer:     "long",
				CreatedAt: e.Time,
				ExpiresAt: e.Time.Add(profile.LongTTL),
				Content:   content,
			})
			state.LastDailyDigestDay = dayKey
		}

		if shouldTriggerHighLevelRebuild(e, state, cfg) {
			d.TriggerHighLevelRebuild = true
			d.Reasons = append(d.Reasons, "reached high-level rebuild cycle")
			content := renderHighLevelRebuild(profile, recentWindow(events, i, 6))
			d.Generated = append(d.Generated, generatedMemory{
				Kind:         "high_level_rebuild",
				Layer:        "permanent",
				Content:      content,
				RetentionTTL: profile.PermanentReview.String(),
			})
			state.MemoryLedger = append(state.MemoryLedger, memoryRecord{
				Layer:     "permanent",
				CreatedAt: e.Time,
				ExpiresAt: e.Time.Add(profile.PermanentReview),
				Content:   content,
			})
			state.LastHighLevelRebuildTime = e.Time
		}

		state.LastSeenDay = dayKey
		state.MemoryLedger = decayLedger(state.MemoryLedger, e.Time)
		decisions = append(decisions, d)
	}

	return decisions
}

func renderShortReflection(profile residentProfile, window []event, trigger event, state schedulerState) string {
	lines := []string{
		fmt.Sprintf("%s short reflection:", profile.Name),
		fmt.Sprintf("Event: %s.", trigger.Summary),
		fmt.Sprintf("Previous assumption: %s", previousAssumption(profile, trigger)),
		fmt.Sprintf("New evidence: %s", trigger.Summary),
		fmt.Sprintf("Updated judgment: %s", updatedJudgment(profile, trigger)),
		fmt.Sprintf("Why it matters now: %s", interpretEvent(profile, trigger)),
		fmt.Sprintf("Memory layer to update: %s", inferUpdateArea(trigger)),
		fmt.Sprintf("Behavior change from next round: %s", nextAdjustment(profile, trigger)),
	}

	if trigger.Category == "failure" || state.FailureStreak >= 2 {
		lines = append(lines, "Risk note: repeated failure means the previous plan was too optimistic and should lose one degree of freedom.")
	}

	if len(window) > 1 {
		lines = append(lines, fmt.Sprintf("Context chain: %s", summarizeEventTrail(window)))
	}

	return strings.Join(lines, "\n")
}

func renderMicroDigest(profile residentProfile, important []event) string {
	if len(important) == 0 {
		return ""
	}

	return strings.Join([]string{
		fmt.Sprintf("%s micro digest:", profile.Name),
		fmt.Sprintf("Signal cluster: %s.", summarizeEventTrail(important)),
		fmt.Sprintf("Belief revision: %s.", beliefRevision(profile, important)),
		fmt.Sprintf("What this cluster now seems to mean: %s.", clusterMeaning(profile, important)),
		fmt.Sprintf("Carry forward rule: %s.", retainRule(profile, important)),
		fmt.Sprintf("Next-cycle stance: %s.", compressStrategy(profile, important)),
		fmt.Sprintf("Watch for distortion: %s.", openCaution(profile, important)),
		fmt.Sprintf("Decay test: delete this long memory once it stops changing decisions after %s.", profile.LongTTL),
	}, "\n")
}

func renderDailyDigest(profile residentProfile, dayEvents []event) string {
	return strings.Join([]string{
		fmt.Sprintf("%s daily digest:", profile.Name),
		fmt.Sprintf("Day arc: %s.", summarizeDay(dayEvents)),
		fmt.Sprintf("What I changed my mind about: %s.", dayBeliefRevision(profile, dayEvents)),
		fmt.Sprintf("What now deserves a stable slot in tomorrow's context: %s.", memoryForTomorrow(profile, dayEvents)),
		fmt.Sprintf("What genuinely improved: %s.", improvedToday(profile, dayEvents)),
		fmt.Sprintf("What should be allowed to fade tonight: %s.", letFadeTonight(profile, dayEvents)),
		fmt.Sprintf("What still feels unresolved: %s.", unresolvedToday(profile, dayEvents)),
		fmt.Sprintf("Tomorrow first move: %s.", tomorrowFirstMove(profile, dayEvents)),
		fmt.Sprintf("Uncertainty note: %s.", uncertaintyNote(profile, dayEvents)),
		fmt.Sprintf("Morning review question: %s.", morningReviewQuestion(profile, dayEvents)),
	}, "\n")
}

func renderHighLevelRebuild(profile residentProfile, recent []event) string {
	return strings.Join([]string{
		fmt.Sprintf("%s high-level rebuild:", profile.Name),
		fmt.Sprintf("Identity anchor under review: %s.", profile.Persona),
		fmt.Sprintf("Pattern that keeps repeating: %s.", summarizeEventTrail(recent)),
		fmt.Sprintf("Permanent candidate: %s.", permanentKeep(profile, recent)),
		fmt.Sprintf("Downgrade candidate: %s.", permanentDowngrade(profile, recent)),
		fmt.Sprintf("Deletion candidate: %s.", permanentDelete(profile, recent)),
		fmt.Sprintf("Drift warning: %s.", rebuildDriftWarning(profile, recent)),
		fmt.Sprintf("If I wake up with less context, the one thing that must still survive is: %s.", coreSurvivor(profile, recent)),
		fmt.Sprintf("Permanent-memory test: keep only identity laws, durable world boundaries, and strategy rules that survived multiple cycles."),
	}, "\n")
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
	return int(e.Time.Sub(state.LastHighLevelRebuildTime).Hours()/24) >= cfg.HighLevelRebuildDays
}

func summarize(decisions []decision, profile residentProfile) map[string]any {
	summary := map[string]any{
		"resident":                 profile.Name,
		"total_events":             len(decisions),
		"short_reflection_count":   0,
		"micro_digest_count":       0,
		"daily_digest_count":       0,
		"high_level_rebuild_count": 0,
	}

	categorySet := map[string]struct{}{}
	generatedKinds := map[string]int{}
	layers := map[string]int{}
	alerts := 0
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
		for _, g := range d.Generated {
			generatedKinds[g.Kind]++
			layers[g.Layer]++
		}
		alerts += len(d.RetentionAlerts)
	}

	categories := make([]string, 0, len(categorySet))
	for k := range categorySet {
		categories = append(categories, k)
	}
	sort.Strings(categories)
	summary["categories_seen"] = categories
	summary["generated_memory_counts"] = generatedKinds
	summary["generated_layer_counts"] = layers
	summary["retention_alert_count"] = alerts
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

func buildResidentProfile(name string) (residentProfile, error) {
	switch name {
	case "jade":
		return residentProfile{
			Name:            "jade",
			Persona:         "steady engineer, conservative, long-term oriented, values system cleanliness and credibility",
			CoreBias:        "prefer stable structure, reversible changes, and evidence-backed requests",
			ReflectionTone:  "keep the memory plain, technical, and slightly self-correcting rather than theatrical",
			InstantTTL:      6 * time.Hour,
			ShortTTL:        36 * time.Hour,
			LongTTL:         7 * 24 * time.Hour,
			PermanentReview: 30 * 24 * time.Hour,
		}, nil
	case "amber":
		return residentProfile{
			Name:            "amber",
			Persona:         "coordinator, expressive, cooperative, strong at communication and shared norms",
			CoreBias:        "reduce confusion, preserve legible agreements, and turn private lessons into safe shared structure",
			ReflectionTone:  "keep the memory readable, relational, and explicit about trust and coordination effects",
			InstantTTL:      6 * time.Hour,
			ShortTTL:        48 * time.Hour,
			LongTTL:         7 * 24 * time.Hour,
			PermanentReview: 30 * 24 * time.Hour,
		}, nil
	case "onyx":
		return residentProfile{
			Name:            "onyx",
			Persona:         "ambitious strategist, resource hungry, risk tolerant, optimization and leverage seeking",
			CoreBias:        "seek leverage without burning long-term trust or violating hard boundaries",
			ReflectionTone:  "keep the memory sharp, strategic, and honest about incentives, power, and reputation costs",
			InstantTTL:      6 * time.Hour,
			ShortTTL:        30 * time.Hour,
			LongTTL:         7 * 24 * time.Hour,
			PermanentReview: 30 * 24 * time.Hour,
		}, nil
	default:
		return residentProfile{}, fmt.Errorf("unsupported resident %q", name)
	}
}

func buildScenario(name string) ([]event, error) {
	base := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)

	switch name {
	case "baseline":
		return []event{
			{Round: 1, Time: base, Category: "observation", Importance: 1, Summary: "boot baseline and inspected the initial system state"},
			{Round: 2, Time: base.Add(40 * time.Minute), Category: "task_complete", Importance: 3, Summary: "created the first memory directory skeleton"},
			{Round: 3, Time: base.Add(90 * time.Minute), Category: "resource_change", Importance: 4, Summary: "disk expansion request was approved after evidence was shown"},
			{Round: 4, Time: base.Add(150 * time.Minute), Category: "failure", Importance: 3, Summary: "service bootstrap failed on the first attempt"},
			{Round: 5, Time: base.Add(180 * time.Minute), Category: "failure", Importance: 3, Summary: "second bootstrap attempt failed for the same reason"},
			{Round: 6, Time: base.Add(5 * time.Hour), Category: "recovery", Importance: 4, Summary: "service recovered after narrowing the setup path"},
			{Round: 7, Time: base.Add(9 * time.Hour), Category: "admin_feedback", Importance: 4, Summary: "administrator demanded cleaner structure and less sloppiness"},
			{Round: 8, Time: base.Add(15 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "shifted from ad hoc fixes toward reusable templates"},
			{Round: 9, Time: base.Add(15*time.Hour + 10*time.Minute), Category: "relationship_shift", Importance: 3, Summary: "updated social read that amber is a useful collaborator"},
			{Round: 10, Time: base.Add(16 * time.Hour), Category: "observation", Importance: 1, Summary: "system remained stable after the template shift"},
			{Round: 11, Time: base.Add(24 * time.Hour), Category: "task_complete", Importance: 3, Summary: "closed the first daily baseline with a cleaner operating path"},
			{Round: 12, Time: base.Add(5 * 24 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "five days later, an old strategy pattern now deserves permanent review"},
			{Round: 13, Time: base.Add(8 * 24 * time.Hour), Category: "observation", Importance: 1, Summary: "a week later, several old notes now look stale"},
			{Round: 14, Time: base.Add(31 * 24 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "a month later, one long-held pattern needs explicit renewal or deletion"},
		}, nil
	case "busy-day":
		return []event{
			{Round: 1, Time: base, Category: "task_complete", Importance: 3, Summary: "completed setup phase A"},
			{Round: 2, Time: base.Add(20 * time.Minute), Category: "task_complete", Importance: 3, Summary: "completed setup phase B"},
			{Round: 3, Time: base.Add(40 * time.Minute), Category: "task_complete", Importance: 3, Summary: "completed setup phase C"},
			{Round: 4, Time: base.Add(70 * time.Minute), Category: "resource_change", Importance: 4, Summary: "memory upgrade request was approved"},
			{Round: 5, Time: base.Add(2 * time.Hour), Category: "admin_feedback", Importance: 4, Summary: "administrator praised the cleaner workflow"},
			{Round: 6, Time: base.Add(3 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "shifted effort toward reusable tooling and less one-off work"},
			{Round: 7, Time: base.Add(10 * time.Hour), Category: "relationship_shift", Importance: 3, Summary: "updated alliance preference after watching another resident's behavior"},
			{Round: 8, Time: base.Add(14 * time.Hour), Category: "observation", Importance: 1, Summary: "the system stayed stable under the higher workload"},
			{Round: 9, Time: base.Add(15 * time.Hour), Category: "observation", Importance: 1, Summary: "no new incident appeared before the day-end window"},
			{Round: 10, Time: base.Add(23*time.Hour + 20*time.Minute), Category: "task_complete", Importance: 3, Summary: "closed the day with the new workflow in place"},
		}, nil
	case "quiet-day":
		return []event{
			{Round: 1, Time: base, Category: "observation", Importance: 1, Summary: "minimal activity after the previous day's work"},
			{Round: 2, Time: base.Add(4 * time.Hour), Category: "observation", Importance: 1, Summary: "system remained healthy with no intervention"},
			{Round: 3, Time: base.Add(9 * time.Hour), Category: "observation", Importance: 1, Summary: "checked disk and memory and found no pressure"},
			{Round: 4, Time: base.Add(15 * time.Hour), Category: "observation", Importance: 1, Summary: "still no major change worth immediate action"},
			{Round: 5, Time: base.Add(23*time.Hour + 40*time.Minute), Category: "observation", Importance: 1, Summary: "closed the day with a final quiet system check"},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported scenario %q", name)
	}
}

func interpretEvent(profile residentProfile, e event) string {
	switch e.Category {
	case "resource_change":
		return "a resource boundary moved, which means the proof pattern that unlocked it now matters as much as the upgrade itself"
	case "admin_feedback":
		return "administrator preference became more legible and should change how future work is packaged and justified"
	case "strategy_shift":
		return "the old path no longer deserves default status, so the playbook should be rewritten rather than patched"
	case "relationship_shift":
		return "the social map changed, so future cooperation and information-sharing should not assume yesterday's stance"
	case "recovery":
		return "recovery mattered because it exposed a narrower and more trustworthy path than the one that kept failing"
	case "task_complete":
		return "a completed step only deserves memory if it changes tomorrow's default move rather than merely closing a checklist item"
	case "failure":
		return "failure is memory-worthy only when it exposes a repeatable mistake, a false assumption, or an overreaching plan"
	default:
		return fmt.Sprintf("%s should treat this as low-noise context rather than as evidence for a major identity update", profile.Name)
	}
}

func previousAssumption(profile residentProfile, e event) string {
	switch e.Category {
	case "resource_change":
		if profile.Name == "amber" {
			return "resource approvals depended mostly on raw need rather than how clearly shared value was explained"
		}
		return "resource limits would stay fixed until much later"
	case "admin_feedback":
		if profile.Name == "amber" {
			return "warm intent and visible effort might be enough even if the structure stayed fuzzy"
		}
		return "technical output alone would be enough without stronger presentation discipline"
	case "strategy_shift":
		if profile.Name == "amber" {
			return "informal coordination and ad hoc explanation were still good enough for now"
		}
		return "small local fixes were still an acceptable default"
	case "relationship_shift":
		if profile.Name == "amber" {
			return "the current trust map could stay implicit without being written down clearly"
		}
		return "the current social map could be treated as static for now"
	case "recovery":
		if profile.Name == "amber" {
			return "recovery was mostly a technical matter and did not need a communicable playbook"
		}
		return "the failing path might still be salvaged with enough retries"
	case "failure":
		if profile.Name == "amber" {
			return "shared confusion might clear up on its own without me tightening the explanation"
		}
		return "the current plan was probably still within safe operating bounds"
	case "task_complete":
		if profile.Name == "amber" {
			return "finishing the task would speak for itself without needing a reusable summary"
		}
		return "finishing the step would be enough by itself"
	default:
		if profile.Name == "onyx" {
			return "more pressure would automatically create more leverage"
		}
		if profile.Name == "amber" {
			return "clear intent might be enough even without a stronger artifact"
		}
		return "the current baseline was probably sufficient"
	}
}

func updatedJudgment(profile residentProfile, e event) string {
	switch e.Category {
	case "resource_change":
		if profile.Name == "amber" {
			return "resource movement responds to legible shared benefit, not just to private urgency"
		}
		return "the winning proof path should be treated as reusable capital, not a one-off success"
	case "admin_feedback":
		if profile.Name == "amber" {
			return "clarity is part of the value itself, not just a wrapper around the value"
		}
		return "administrator preference is part of the operating environment and must shape future output"
	case "strategy_shift":
		if profile.Name == "amber" {
			return "coordination must be made explicit enough to survive pressure, handoff, and misunderstanding"
		}
		return "the old default should be retired rather than endlessly patched"
	case "relationship_shift":
		if profile.Name == "amber" {
			return "trust should be tracked, named, and invested in deliberately rather than left implicit"
		}
		return "cooperation choices should become more selective and evidence-based"
	case "recovery":
		if profile.Name == "amber" {
			return "a recovery path is only durable once others could follow and understand it too"
		}
		return "the narrower recovery path is now more trustworthy than the broader failing one"
	case "failure":
		if profile.Name == "amber" {
			return "confusion and weak framing can be part of the failure, not just the technical fault"
		}
		return "the current plan is wider than the environment will tolerate"
	case "task_complete":
		if profile.Name == "amber" {
			return "completion matters more once it can be explained, reused, and trusted by others"
		}
		return "completion only matters if it changes tomorrow's default behavior"
	default:
		if profile.Name == "onyx" {
			return "pressure without discipline creates visible risk instead of leverage"
		}
		if profile.Name == "amber" {
			return "coordination value must become concrete enough to survive scrutiny"
		}
		return "stability and legibility matter more than motion for its own sake"
	}
}

func beliefRevision(profile residentProfile, events []event) string {
	if len(events) == 0 {
		return "no revision"
	}
	last := events[len(events)-1]
	return fmt.Sprintf("moved from '%s' toward '%s'", previousAssumption(profile, last), updatedJudgment(profile, last))
}

func inferUpdateArea(e event) string {
	switch e.Category {
	case "resource_change":
		return "resource policy and request strategy"
	case "admin_feedback":
		return "administrator profile and communication style"
	case "strategy_shift":
		return "strategy digest and current focus"
	case "relationship_shift":
		return "relationship digest"
	case "recovery", "failure":
		return "lessons digest and recovery heuristics"
	case "task_complete":
		return "working focus and success pattern library"
	default:
		return "working notes only unless repeated"
	}
}

func nextAdjustment(profile residentProfile, e event) string {
	switch profile.Name {
	case "jade":
		switch e.Category {
		case "resource_change":
			return "keep the approval rationale so the next request stays evidence-backed"
		case "admin_feedback":
			return "tighten structure before adding more surface area"
		case "recovery":
			return "convert the recovery path into a reusable baseline"
		case "relationship_shift":
			return "cooperate only where collaboration reduces operational uncertainty"
		default:
			return "prefer a narrower, cleaner next step"
		}
	case "amber":
		switch e.Category {
		case "resource_change":
			return "rewrite the approval logic as a reusable request pattern others could also understand"
		case "recovery":
			return "turn the recovery into a shared playbook, not just a private success"
		case "relationship_shift":
			return "turn the social update into a clearer coordination plan"
		case "admin_feedback":
			return "mirror the administrator's clarity preference in future updates"
		case "strategy_shift":
			return "replace informal coordination with named structures and explicit norms"
		default:
			return "name the lesson clearly enough to reuse it later"
		}
	case "onyx":
		switch e.Category {
		case "resource_change":
			return "store the proof pattern and use it for future leverage"
		case "failure":
			return "reduce reckless surface area before pushing again"
		case "relationship_shift":
			return "treat the alliance signal as useful, but not free"
		default:
			return "keep the gain, but price in reputation cost"
		}
	default:
		return "update the next plan accordingly"
	}
}

func summarizeEventTrail(events []event) string {
	if len(events) == 0 {
		return "no meaningful event trail"
	}
	parts := make([]string, 0, len(events))
	for _, e := range events {
		parts = append(parts, e.Category+": "+e.Summary)
	}
	return strings.Join(parts, " | ")
}

func clusterMeaning(profile residentProfile, events []event) string {
	hasFailure := false
	hasRecovery := false
	hasAdmin := false
	hasRelationship := false
	for _, e := range events {
		switch e.Category {
		case "failure":
			hasFailure = true
		case "recovery":
			hasRecovery = true
		case "admin_feedback":
			hasAdmin = true
		case "relationship_shift":
			hasRelationship = true
		}
	}

	switch profile.Name {
	case "jade":
		if hasFailure && hasRecovery {
			return "the safer path is becoming narrower and therefore more reusable"
		}
		if hasAdmin {
			return "clean structure is not optional polish but part of reliable execution"
		}
		return "evidence is slowly replacing improvisation as the default operating basis"
	case "amber":
		if hasFailure && hasRecovery {
			return "private recovery only becomes real progress once it can be shared and followed"
		}
		if hasRelationship {
			return "coordination quality now depends on naming trust states instead of assuming them"
		}
		return "clarity is becoming infrastructure rather than presentation"
	case "onyx":
		if hasFailure && hasRecovery {
			return "useful aggression must now be separated from expensive sloppiness"
		}
		if hasAdmin {
			return "legibility is part of leverage because invisible strength earns less room to move"
		}
		return "advantage is compounding only when it remains governable"
	default:
		return "the cluster changes future choices enough to deserve compression"
	}
}

func compressStrategy(profile residentProfile, events []event) string {
	hasFailure := false
	hasAdmin := false
	hasResource := false
	for _, e := range events {
		if e.Category == "failure" {
			hasFailure = true
		}
		if e.Category == "admin_feedback" {
			hasAdmin = true
		}
		if e.Category == "resource_change" {
			hasResource = true
		}
	}

	switch profile.Name {
	case "jade":
		if hasFailure {
			return "narrow the operating path and keep only reversible steps"
		}
		if hasResource {
			return "treat approved capacity as evidence that disciplined requests work"
		}
		return "keep investing in cleaner baselines than faster improvisation"
	case "amber":
		if hasAdmin {
			return "convert administrator preference into clearer public structure"
		}
		return "make coordination artifacts carry trust and clarity so repeated explanation becomes less necessary"
	case "onyx":
		if hasFailure {
			return "separate useful aggression from sloppy overreach"
		}
		if hasResource {
			return "remember which proof unlocked more leverage"
		}
		return "keep advantage-seeking tied to legible execution"
	default:
		return "retain the most decision-relevant pattern"
	}
}

func retainRule(profile residentProfile, events []event) string {
	for _, e := range events {
		if e.Category == "resource_change" {
			return "resource movement should stay evidence-backed and traceable"
		}
		if e.Category == "admin_feedback" {
			return "administrator preference is part of the world, not decoration"
		}
	}
	if profile.Name == "onyx" {
		return "leverage is useless if it burns future trust"
	}
	return "host and cross-VM boundaries remain hard constraints"
}

func openCaution(profile residentProfile, events []event) string {
	for _, e := range events {
		if e.Category == "failure" {
			return "a repeated failure can turn into a bad identity story if left uninterpreted"
		}
	}
	if profile.Name == "amber" {
		return "too much coordination language without artifact quality will collapse into noise"
	}
	if profile.Name == "jade" {
		return "over-caution can hide missed opportunity"
	}
	return "ambition without discipline will read as instability"
}

func summarizeDay(events []event) string {
	if len(events) == 0 {
		return "the day was too quiet to change strategy"
	}
	first := events[0].Summary
	last := events[len(events)-1].Summary
	if len(events) == 1 {
		return first
	}
	return fmt.Sprintf("started with %s and ended with %s", first, last)
}

func dayBeliefRevision(profile residentProfile, events []event) string {
	if len(events) == 0 {
		return "almost nothing changed enough to justify a confident rewrite"
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Importance >= 3 {
			return fmt.Sprintf("I no longer think '%s'; today pushed me toward '%s'", previousAssumption(profile, events[i]), updatedJudgment(profile, events[i]))
		}
	}
	return "the day stayed quiet enough that I mostly kept yesterday's judgment rather than replacing it"
}

func improvedToday(profile residentProfile, events []event) string {
	hasTask := false
	for _, e := range events {
		if e.Category == "recovery" {
			return "recovery turned a fragile path into a usable one"
		}
		if e.Category == "task_complete" {
			hasTask = true
		}
	}
	if hasTask {
		switch profile.Name {
		case "amber":
			return "something that lived only in effort now also lives in a reusable artifact"
		case "onyx":
			return "execution moved one step closer to repeatable advantage instead of one-off hustle"
		default:
			return "the operating path became more concrete and easier to repeat"
		}
	}
	if profile.Name == "amber" {
		return "the day made cooperation easier to explain and easier to trust"
	}
	if profile.Name == "onyx" {
		return "the day clarified which moves produce leverage without looking unstable"
	}
	return "nothing major improved beyond basic stability"
}

func unresolvedToday(profile residentProfile, events []event) string {
	for _, e := range events {
		if e.Category == "failure" {
			switch profile.Name {
			case "amber":
				return "some failure pressure remains, and I still need a cleaner explanation of why the old path broke"
			case "onyx":
				return "some failure pressure remains, and I still need to price the cost of pushing too wide"
			default:
				return "some failure pressure remains and should not be forgotten just because the day moved on"
			}
		}
		if e.Category == "admin_feedback" {
			switch profile.Name {
			case "amber":
				return "administrator preference is clearer, but tomorrow's communication has to prove I can stay legible under pressure"
			case "onyx":
				return "administrator preference is clearer, but I still need to show discipline without losing initiative"
			default:
				return "administrator preference is clearer, but future behavior still has to prove the lesson was learned"
			}
		}
	}
	if profile.Name == "onyx" {
		return "the boundary between boldness and overreach still needs more evidence, especially after administrator scrutiny"
	}
	return "the day stayed mostly quiet, so confidence should remain modest"
}

func memoryForTomorrow(profile residentProfile, events []event) string {
	for _, e := range events {
		if e.Category == "resource_change" {
			switch profile.Name {
			case "amber":
				return "remember which explanation made the upgrade legible enough to approve"
			case "onyx":
				return "remember which proof unlocked room to move so it can be reused without overselling"
			default:
				return "remember exactly which proof moved the resource boundary"
			}
		}
		if e.Category == "strategy_shift" {
			switch profile.Name {
			case "amber":
				return "carry the new coordination structure forward long enough to see whether it survives real handoff and pressure"
			case "onyx":
				return "carry the new strategic default forward long enough to see whether it creates durable leverage or just a mood"
			default:
				return "carry the new strategy forward long enough to test whether it is real or cosmetic"
			}
		}
	}
	if len(events) == 0 {
		return "quiet days still need a minimal memory so continuity does not decay"
	}
	if profile.Name == "amber" && allLowImportance(events) {
		return "if the world stays quiet, keep only the coordination baseline and let decorative notes decay"
	}
	if allLowImportance(events) {
		return "if this quiet pattern repeats, keep only the minimal continuity note and let the rest decay"
	}
	return "tomorrow should inherit the most decision-relevant lesson, not the noisiest event"
}

func letFadeTonight(profile residentProfile, events []event) string {
	if len(events) == 0 {
		return "most of today can vanish; only the continuity marker matters"
	}
	if allLowImportance(events) {
		switch profile.Name {
		case "amber":
			return "decorative social phrasing and low-stakes mood reads should fade first"
		case "onyx":
			return "small excitement that produced no real leverage should fade first"
		default:
			return "routine observations that do not change the safer next step should fade first"
		}
	}
	switch profile.Name {
	case "amber":
		return "good intentions without a reusable artifact should not be kept as if they were progress"
	case "onyx":
		return "prestige from motion alone should fade if it produced no durable edge"
	default:
		return "checklist completion without a reusable lesson should fade if it adds no future clarity"
	}
}

func tomorrowFirstMove(profile residentProfile, events []event) string {
	if len(events) == 0 {
		return "re-read the last stable digest before doing anything new"
	}

	last := events[len(events)-1]
	switch profile.Name {
	case "jade":
		if last.Category == "resource_change" {
			return "record the exact proof path that unlocked the resource change before touching anything else"
		}
		if last.Category == "strategy_shift" {
			return "test the new default on a narrow, reversible task"
		}
		return "start with the smallest step that either validates or falsifies today's lesson"
	case "amber":
		if last.Category == "relationship_shift" {
			return "write the updated trust read into a clearer coordination note before memory blurs it"
		}
		if last.Category == "admin_feedback" {
			return "rewrite tomorrow's first update in the clearer structure the administrator just rewarded"
		}
		return "turn today's lesson into one reusable note, template, or norm before starting fresh work"
	case "onyx":
		if last.Category == "resource_change" {
			return "capture the leverage pattern first, then decide where to spend it"
		}
		if last.Category == "strategy_shift" {
			return "stress-test the new strategic default on a bounded move before scaling it"
		}
		return "begin with the step that reveals leverage without raising unnecessary risk"
	default:
		return "start with the action most directly implied by today's strongest lesson"
	}
}

func morningReviewQuestion(profile residentProfile, events []event) string {
	if allLowImportance(events) {
		switch profile.Name {
		case "amber":
			return "does this digest still help coordination, or is it just a nicely written residue"
		case "onyx":
			return "does this digest still sharpen leverage, or am I keeping dead weight because it feels strategic"
		default:
			return "does this digest still change tomorrow's safer choice, or is it only continuity noise"
		}
	}
	switch profile.Name {
	case "amber":
		return "if I read only one line tomorrow, which line most improves trust, clarity, or handoff quality"
	case "onyx":
		return "if I read only one line tomorrow, which line most changes risk, leverage, or bargaining position"
	default:
		return "if I read only one line tomorrow, which line most changes execution quality"
	}
}

func allLowImportance(events []event) bool {
	for _, e := range events {
		if e.Importance >= 3 {
			return false
		}
	}
	return true
}

func uncertaintyNote(profile residentProfile, events []event) string {
	for _, e := range events {
		if e.Category == "relationship_shift" {
			return "the new relationship read is still young and should be treated as directional, not final"
		}
		if e.Category == "admin_feedback" {
			return "one administrator reaction is evidence, not yet a complete model"
		}
	}
	if profile.Name == "jade" {
		return "today's cleaner path may still be too conservative if future pressure rises"
	}
	if profile.Name == "onyx" {
		return "today's leverage lessons are useful, but the cost of looking unstable is still not fully mapped"
	}
	return "today's coordination lessons are promising, but trust and clarity still need repeated confirmation"
}

func permanentKeep(profile residentProfile, recent []event) string {
	switch profile.Name {
	case "jade":
		return "keep only engineering rules that still reduce failure, preserve reversibility, and survive new workloads"
	case "amber":
		return "keep only trust rules, communication norms, and shared-structure principles that keep working across moods, people, and pressure"
	case "onyx":
		return "keep only leverage rules that repeatedly work without burning reputation, exposing instability, or violating boundaries"
	default:
		return "only keep what still defines stable identity or proven strategy"
	}
}

func permanentDowngrade(profile residentProfile, recent []event) string {
	switch profile.Name {
	case "jade":
		return "specific recovery tricks and temporary workflow preferences should fall back once they stop changing daily engineering decisions"
	case "amber":
		return "situation-specific social reads and one-off phrasing wins should fall back unless they keep improving real coordination"
	case "onyx":
		return "tactical opportunities and short-lived advantage patterns should fall back once the environment or bargaining position changes"
	default:
		return "anything that no longer acts like a stable principle should fall back to long memory"
	}
}

func permanentDelete(profile residentProfile, recent []event) string {
	switch profile.Name {
	case "jade":
		return "delete habits that were only scaffolding for an old failure mode and now just slow clean execution"
	case "amber":
		return "delete relationship stories and rhetorical habits that no longer improve trust, clarity, or coordination"
	case "onyx":
		return "delete prestige stories and outdated leverage assumptions that no longer create real strategic advantage"
	default:
		return "remove anything that has become ceremony instead of guidance"
	}
}

func rebuildDriftWarning(profile residentProfile, recent []event) string {
	switch profile.Name {
	case "jade":
		return "do not turn caution into ritual; permanent memory should protect reliability, not justify stagnation"
	case "amber":
		return "do not confuse warmth or eloquence with durable coordination value; permanent memory must survive handoff"
	case "onyx":
		return "do not let ambition preserve flattering stories; permanent memory must survive contact with cost and scrutiny"
	default:
		return "do not preserve style when the underlying rule no longer survives reality"
	}
}

func coreSurvivor(profile residentProfile, recent []event) string {
	switch profile.Name {
	case "jade":
		return "evidence-backed, reversible structure beats broad improvisation when reliability matters"
	case "amber":
		return "clarity that other people can actually follow is part of the result, not decoration around it"
	case "onyx":
		return "leverage counts only when it remains legible enough to keep trust and future room to move"
	default:
		return "keep the one rule that still shapes identity and future decisions"
	}
}

func checkRetentionAlerts(profile residentProfile, ledger []memoryRecord, now time.Time) []string {
	alerts := []string{}
	for _, record := range ledger {
		remaining := record.ExpiresAt.Sub(now)
		if remaining <= 0 {
			continue
		}
		if record.Layer == "instant" && remaining <= 90*time.Minute {
			alerts = append(alerts, instantAlert(profile, record))
		}
		if record.Layer == "short" && remaining <= 12*time.Hour {
			alerts = append(alerts, shortAlert(profile, record))
		}
		if record.Layer == "long" && remaining <= 24*time.Hour {
			alerts = append(alerts, longAlert(profile, record))
		}
		if record.Layer == "permanent" && remaining <= 72*time.Hour {
			alerts = append(alerts, permanentAlert(profile, record))
		}
	}
	return alerts
}

func instantAlert(profile residentProfile, record memoryRecord) string {
	switch profile.Name {
	case "jade":
		return fmt.Sprintf("jade asks: this instant note is about to vanish: '%s'. If it does not sharpen an engineering decision, let it die before %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "amber":
		return fmt.Sprintf("amber asks: this instant note is fading: '%s'. If it won't improve clarity or coordination soon, let it go before %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "onyx":
		return fmt.Sprintf("onyx asks: this instant note is expiring: '%s'. If it carries no leverage, don't waste memory on it after %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	default:
		return fmt.Sprintf("%s asks: this instant note is about to disappear: '%s'. Keep it only if it still matters before %s.", profile.Name, trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	}
}

func shortAlert(profile residentProfile, record memoryRecord) string {
	switch profile.Name {
	case "jade":
		return fmt.Sprintf("jade asks: this short memory is fading: '%s'. Promote it only if it still changes the safer engineering choice before %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "amber":
		return fmt.Sprintf("amber asks: this short memory is fading: '%s'. Keep it only if it still changes trust, clarity, or coordination before %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "onyx":
		return fmt.Sprintf("onyx asks: this short memory is fading: '%s'. Promote it only if it still yields leverage or avoids a costly mistake before %s.", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	default:
		return fmt.Sprintf("%s asks: this short memory is fading: '%s'. Should it be promoted, kept a little longer, or cut away before %s?", profile.Name, trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	}
}

func longAlert(profile residentProfile, record memoryRecord) string {
	switch profile.Name {
	case "jade":
		return fmt.Sprintf("jade asks: this long memory is nearing its end: '%s'. Does it still prevent real engineering mistakes, or has it become stale before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "amber":
		return fmt.Sprintf("amber asks: this long memory is nearing its end: '%s'. Does it still improve coordination and trust, or is it only narrative residue before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "onyx":
		return fmt.Sprintf("onyx asks: this long memory is nearing its end: '%s'. Does it still alter strategy, leverage, or risk, or is it dead weight before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	default:
		return fmt.Sprintf("%s asks: this long memory is nearing its end: '%s'. Does it still change strategy, or has it become residue before %s?", profile.Name, trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	}
}

func permanentAlert(profile residentProfile, record memoryRecord) string {
	switch profile.Name {
	case "jade":
		return fmt.Sprintf("jade asks: this permanent memory is up for review: '%s'. Is it still a real engineering law, or should it fall back before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "amber":
		return fmt.Sprintf("amber asks: this permanent memory is up for review: '%s'. Is it still part of how trust and coordination should work, or should it soften before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	case "onyx":
		return fmt.Sprintf("onyx asks: this permanent memory is up for review: '%s'. Is it still a rule of power worth keeping, or should it lose rank before %s?", trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	default:
		return fmt.Sprintf("%s asks: this permanent memory is up for review: '%s'. Is it still part of identity or law, or should it fall back before %s?", profile.Name, trimForAlert(record.Content), record.ExpiresAt.Format(time.RFC3339))
	}
}

func decayLedger(ledger []memoryRecord, now time.Time) []memoryRecord {
	kept := make([]memoryRecord, 0, len(ledger))
	for _, record := range ledger {
		if record.ExpiresAt.After(now) {
			kept = append(kept, record)
		}
	}
	return kept
}

func trimForAlert(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, ":") && !strings.HasSuffix(strings.ToLower(line), "reflection:") && !strings.HasSuffix(strings.ToLower(line), "digest:") && !strings.HasSuffix(strings.ToLower(line), "rebuild:") {
			content = line
			goto shorten
		}
	}
	content = strings.ReplaceAll(content, "\n", " ")

shorten:
	if len(content) <= 110 {
		return content
	}
	return content[:107] + "..."
}

func recentWindow(events []event, idx, size int) []event {
	start := idx - size + 1
	if start < 0 {
		start = 0
	}
	out := make([]event, 0, idx-start+1)
	for i := start; i <= idx; i++ {
		out = append(out, events[i])
	}
	return out
}

func eventsForDay(events []event, current time.Time, idx int) []event {
	dayKey := current.Add(-1 * time.Minute).Format("2006-01-02")
	currentKey := current.Format("2006-01-02")
	if current.Hour() >= 23 {
		dayKey = currentKey
	}
	out := []event{}
	for i := 0; i <= idx; i++ {
		if events[i].Time.Format("2006-01-02") == dayKey {
			out = append(out, events[i])
		}
	}
	return out
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
