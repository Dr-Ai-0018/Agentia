package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-arena/internal/memory"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-5.4-mini"
)

type event struct {
	Round      int       `json:"round"`
	Time       time.Time `json:"time"`
	Category   string    `json:"category"`
	Importance int       `json:"importance"`
	Summary    string    `json:"summary"`
}

type requestPayload struct {
	Model             string         `json:"model"`
	Instructions      string         `json:"instructions"`
	PromptCacheKey    string         `json:"prompt_cache_key"`
	Input             []inputMessage `json:"input"`
	Tools             []responseTool `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Stream            bool           `json:"stream"`
	Store             bool           `json:"store"`
}

type inputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usageEnvelope struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type responseEnvelope struct {
	ID             string         `json:"id"`
	PromptCacheKey string         `json:"prompt_cache_key"`
	OutputText     string         `json:"output_text"`
	Usage          usageEnvelope  `json:"usage"`
	Output         []responseItem `json:"output"`
}

type responseItem struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallName  string `json:"call_name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	ID        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
}

type responseTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type functionToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type routingDecision struct {
	TargetLayer  string   `json:"target_layer"`
	Action       string   `json:"action"`
	ReasonCodes  []string `json:"reason_codes"`
	ReviewAfter  string   `json:"review_after,omitempty"`
	ExpiresAfter string   `json:"expires_after,omitempty"`
}

type memoryActionDecision struct {
	Action       string   `json:"action"`
	ReasonCodes  []string `json:"reason_codes"`
	NeedsReview  bool     `json:"needs_review"`
	ReviewAfter  string   `json:"review_after,omitempty"`
	ExpiresAfter string   `json:"expires_after,omitempty"`
}

type conflictDecision struct {
	Conflict       bool     `json:"conflict"`
	MergeSuggested bool     `json:"merge_suggested"`
	ConflictKinds  []string `json:"conflict_kinds"`
	ReasonCodes    []string `json:"reason_codes"`
	Resolution     string   `json:"resolution"`
}

type reviewScheduleDecision struct {
	NeedsReview  bool     `json:"needs_review"`
	ReviewAfter  string   `json:"review_after,omitempty"`
	ExpiresAfter string   `json:"expires_after,omitempty"`
	ReasonCodes  []string `json:"reason_codes"`
}

type memorySnapshotEntry struct {
	ID              string `json:"id"`
	Layer           string `json:"layer"`
	DecisionAction  string `json:"decision_action"`
	Summary         string `json:"summary"`
	ResidentText    string `json:"resident_text,omitempty"`
	MemoryKind      string `json:"memory_kind,omitempty"`
	Salience        int    `json:"salience,omitempty"`
	EmotionTone     string `json:"emotion_tone,omitempty"`
	TimeScope       string `json:"time_scope,omitempty"`
	RetentionIntent string `json:"retention_intent,omitempty"`
	DropCondition   string `json:"drop_condition,omitempty"`
}

type streamingEvent struct {
	Type      string           `json:"type"`
	Delta     string           `json:"delta"`
	Arguments string           `json:"arguments"`
	ItemID    string           `json:"item_id"`
	Item      responseItem     `json:"item"`
	Response  responseEnvelope `json:"response"`
}

type streamResult struct {
	ResponseID             string
	ObservedPromptCacheKey string
	OutputText             string
	FunctionCalls          []responseItem
	InputTokens            int
	CachedTokens           int
	OutputTokens           int
	RequestID              string
}

type residentProfile struct {
	Name                 string
	Persona              string
	SystemStyle          string
	MemoryBias           string
	PromptCacheKey       string
	CoreConcern          string
	ShortVoice           string
	LongVoice            string
	PermanentVoice       string
	BannedPhrases        []string
	WhyItMattersLens     string
	CarryRuleStyle       string
	OldReadStyle         string
	NewReadStyle         string
	ShortMustInclude     string
	LongMustInclude      string
	PermanentMustInclude string
	DraftVariants        int
}

type generatedMemory struct {
	Resident       string    `json:"resident"`
	Layer          string    `json:"layer"`
	RequestedLayer string    `json:"requested_layer"`
	RoutedLayer    string    `json:"routed_layer"`
	CommittedLayer string    `json:"committed_layer"`
	DecisionAction string    `json:"decision_action,omitempty"`
	Conflict       any       `json:"conflict,omitempty"`
	ReviewSchedule any       `json:"review_schedule,omitempty"`
	ReasonCodes    []string  `json:"reason_codes,omitempty"`
	Scenario       string    `json:"scenario"`
	GeneratedAt    time.Time `json:"generated_at"`
	Model          string    `json:"model"`
	ResponseID     string    `json:"response_id"`
	RequestID      string    `json:"request_id"`
	InputTokens    int       `json:"input_tokens"`
	CachedTokens   int       `json:"cached_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	EventWindow    []event   `json:"event_window"`
	MemoryText     string    `json:"memory_text"`
	Accepted       bool      `json:"accepted"`
	RejectReason   string    `json:"reject_reason,omitempty"`
	Instructions   string    `json:"instructions"`
	UserPrompt     string    `json:"user_prompt"`
	ObservedCache  string    `json:"observed_prompt_cache_key"`
	DraftCached    int       `json:"draft_cached_tokens,omitempty"`
	VerdictCached  int       `json:"verdict_cached_tokens,omitempty"`
	RoutingCached  int       `json:"routing_cached_tokens,omitempty"`
	ConflictCached int       `json:"conflict_cached_tokens,omitempty"`
	ActionCached   int       `json:"action_cached_tokens,omitempty"`
	ReviewCached   int       `json:"review_cached_tokens,omitempty"`
	RecordState    any       `json:"record_state,omitempty"`
}

type layerRunSummary struct {
	Layer          string `json:"layer"`
	ResponseID     string `json:"response_id"`
	RequestID      string `json:"request_id"`
	HistoryGroupID string `json:"history_group_id,omitempty"`
	RecallPath     string `json:"recall_path,omitempty"`
	InputTokens    int    `json:"input_tokens"`
	CachedTokens   int    `json:"cached_tokens"`
	OutputTokens   int    `json:"output_tokens"`
	LogPath        string `json:"log_path"`
	OutputPath     string `json:"output_path"`
	DurationMS     int64  `json:"duration_ms"`
	StreamedBytes  int    `json:"streamed_bytes"`
	Accepted       bool   `json:"accepted"`
	RejectReason   string `json:"reject_reason,omitempty"`
	Skipped        bool   `json:"skipped,omitempty"`
}

type memoryDraft struct {
	ResidentText    string `json:"resident_text"`
	MemoryKind      string `json:"memory_kind"`
	Salience        int    `json:"salience"`
	EmotionTone     string `json:"emotion_tone"`
	TimeScope       string `json:"time_scope"`
	RetentionIntent string `json:"retention_intent"`
	DropCondition   string `json:"drop_condition,omitempty"`
	Confidence      int    `json:"confidence"`
}

type memoryVerdict struct {
	Accepted     bool     `json:"accepted"`
	RejectReason string   `json:"reject_reason"`
	Issues       []string `json:"issues"`
}

type layerCandidate struct {
	draft         memoryDraft
	draftResult   streamResult
	verdict       memoryVerdict
	verdictResult streamResult
	instructions  string
	userPrompt    string
	payload       requestPayload
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL   = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		model     = flag.String("model", envOrDefault("OPENAI_MODEL", defaultModel), "OpenAI model ID")
		resident  = flag.String("resident", "jade", "Resident: jade|amber|onyx")
		scenario  = flag.String("scenario", "baseline", "Scenario: baseline|busy-day|quiet-day")
		layer     = flag.String("layer", "short", "Target layer: short|long|permanent")
		autoRoute = flag.Bool("auto-route", false, "Use memory policy to route the scenario into a target memory layer")
		allLayers = flag.Bool("all-layers", false, "Generate short, long, and permanent memories in one run")
		auto      = flag.Bool("auto", false, "Alias of --all-layers for real multi-layer generation")
		recallID  = flag.String("recall-memory-id", "", "Recall evidence for one abstract memory ID instead of generating new memory")
		compact   = flag.Bool("compact-store", false, "Compact resident memory store by merging duplicate history groups and remapping source group ids")
		logDir    = flag.String("log-dir", "experiments/memory-runtime/logs", "Directory to store JSONL request logs")
		outDir    = flag.String("out-dir", "experiments/memory-runtime/output", "Directory to store generated memory files")
		verbose   = flag.Bool("verbose", false, "Print streamed text as it arrives")
	)
	flag.Parse()

	profile, err := buildResidentProfile(strings.ToLower(strings.TrimSpace(*resident)))
	if err != nil {
		exitf("%v", err)
	}

	events, err := buildScenario(strings.ToLower(strings.TrimSpace(*scenario)))
	if err != nil {
		exitf("%v", err)
	}

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		exitf("create log dir: %v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create output dir: %v", err)
	}
	storeDir := filepath.Join(*outDir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		exitf("create store dir: %v", err)
	}
	memStore := memory.NewFileStore(storeDir)

	if strings.TrimSpace(*recallID) != "" {
		result, err := recallEvidence(memStore, profile.Name, strings.TrimSpace(*recallID), *outDir)
		if err != nil {
			exitf("recall evidence: %v", err)
		}
		out, _ := json.Marshal(result)
		fmt.Println(string(out))
		return
	}

	if *compact {
		if err := memStore.CompactResident(profile.Name); err != nil {
			exitf("compact resident store: %v", err)
		}
		out, _ := json.Marshal(map[string]any{
			"resident":  profile.Name,
			"compacted": true,
			"store_dir": storeDir,
		})
		fmt.Println(string(out))
		return
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}

	layersToRun := []string{strings.TrimSpace(*layer)}
	if *allLayers || *auto {
		layersToRun = []string{"short", "long", "permanent"}
	} else if *autoRoute {
		routed, err := routeScenario(profile.Name, *scenario, events)
		if err != nil {
			exitf("route scenario: %v", err)
		}
		layersToRun = []string{string(routed)}
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	summaries := make([]layerRunSummary, 0, len(layersToRun))
	for _, currentLayer := range layersToRun {
		summary, err := runLayer(client, memStore, *baseURL, apiKey, *model, profile, *scenario, currentLayer, events, *logDir, *outDir, *verbose)
		if err != nil {
			exitf("layer %s failed: %v", currentLayer, err)
		}
		summaries = append(summaries, summary)
	}

	summary := map[string]any{
		"resident": profile.Name,
		"scenario": *scenario,
		"model":    *model,
		"runs":     summaries,
	}
	out, _ := json.Marshal(summary)
	fmt.Println(string(out))
}

func routeScenario(resident, scenario string, events []event) (memory.Layer, error) {
	_ = resident
	_ = scenario
	if len(events) == 0 {
		return memory.LayerInstant, nil
	}
	signal := distillCanonical(events).ToEventSignal()
	signal.ImpactRounds = len(events)
	signal.Novelty = estimateNovelty(events)
	decision := memory.DefaultPolicy().Evaluate(signal)
	return decision.TargetLayer, nil
}

func distillCanonical(events []event) memory.CanonicalMemory {
	distilled := make([]memory.Event, 0, len(events))
	for _, e := range events {
		distilled = append(distilled, memory.Event{
			Time:       e.Time,
			Category:   e.Category,
			Importance: e.Importance,
			Summary:    e.Summary,
		})
	}
	return memory.DistillEvents(distilled)
}

func eventRecurrence(events []event) int {
	counts := map[string]int{}
	maxCount := 0
	for _, e := range events {
		counts[e.Category]++
		if counts[e.Category] > maxCount {
			maxCount = counts[e.Category]
		}
	}
	return maxCount
}

func estimateNovelty(events []event) float64 {
	if len(events) == 0 {
		return 0
	}
	categories := map[string]int{}
	important := 0
	for _, event := range events {
		categories[event.Category]++
		if event.Importance >= 4 {
			important++
		}
	}
	uniqueRatio := float64(len(categories)) / float64(len(events))
	importanceRatio := float64(important) / float64(len(events))
	novelty := uniqueRatio*0.6 + importanceRatio*0.4
	if novelty > 1 {
		return 1
	}
	return novelty
}

func canonicalDecisionImpact(profile residentProfile, layer string, draft memoryDraft) float64 {
	score := float64(draft.Confidence) / 100
	if layer == "permanent" {
		score += 0.2
	}
	if profile.Name == "onyx" && strings.Contains(strings.ToLower(draft.ResidentText), "false edge") {
		score += 0.1
	}
	if score > 1 {
		score = 1
	}
	return score
}

func runLayer(client *http.Client, memStore memory.Store, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, logDir, outDir string, verbose bool) (layerRunSummary, error) {
	eventWindow, err := selectWindow(events, profile.Name, layer)
	if err != nil {
		return layerRunSummary{}, err
	}
	historyGroup, err := previewEventWindowGroup(memStore, profile.Name, scenario, layer, eventWindow)
	if err != nil {
		return layerRunSummary{}, fmt.Errorf("preview event window group: %w", err)
	}
	canonical := distillCanonical(eventWindow)
	snapshot, err := loadMemorySnapshot(memStore, profile.Name)
	if err != nil {
		return layerRunSummary{}, err
	}
	preDecision := memory.DefaultPolicy().Evaluate(memory.EventSignal{
		Novelty:            estimateNovelty(eventWindow),
		Confidence:         canonical.Confidence,
		DecisionImpact:     canonical.DecisionImpact,
		ImpactRounds:       len(eventWindow),
		Recurrence:         eventRecurrence(eventWindow),
		ResourceWeight:     canonical.ToEventSignal().ResourceWeight,
		RelationshipWeight: canonical.ToEventSignal().RelationshipWeight,
		RuleWeight:         canonical.ToEventSignal().RuleWeight,
		IdentityWeight:     canonical.ToEventSignal().IdentityWeight,
	})
	if !shouldExtractFromGroup(historyGroup, layer) || shouldSkipByPolicy(preDecision) || shouldSkipNewMemory(memStore, profile.Name, layer, scenario, canonical, eventWindow, snapshot) {
		if _, err := routeEventWindowToEvidence(memStore, profile.Name, scenario, layer, eventWindow); err != nil {
			return layerRunSummary{}, fmt.Errorf("persist skipped event window group: %w", err)
		}
		return layerRunSummary{
			Layer:          layer,
			Accepted:       false,
			RejectReason:   "Skipped new memory generation because the event meaning does not justify a new memory.",
			HistoryGroupID: historyGroup.GroupUUID,
			Skipped:        true,
		}, nil
	}

	if profile.DraftVariants > 1 && layer == "permanent" {
		return runLayerWithVariants(client, memStore, baseURL, apiKey, model, profile, scenario, layer, eventWindow, canonical, logDir, outDir)
	}

	instructions := buildDraftInstructions(profile, layer)
	userPrompt := buildDraftPrompt(profile, layer, scenario, eventWindow, canonical)
	payload := requestPayload{
		Model:          model,
		Instructions:   instructions,
		PromptCacheKey: profile.PromptCacheKey + "-" + layer + "-v1",
		Input: []inputMessage{
			{Role: "user", Content: userPrompt},
		},
		Stream: true,
		Store:  false,
	}

	started := time.Now()
	result, err := postStream(client, baseURL, apiKey, payload, verbose)
	if err != nil {
		return layerRunSummary{}, err
	}

	draft, err := decodeDraft(result.OutputText)
	if err != nil {
		return layerRunSummary{}, fmt.Errorf("decode draft: %w; raw=%s", err, result.OutputText)
	}
	if issues := append(localRawDraftIssues(profile, layer, result.OutputText), localDraftIssues(profile, layer, draft)...); len(issues) > 0 {
		verdict := memoryVerdict{
			Accepted:     false,
			RejectReason: "Local precheck rejected the draft before model-side review.",
			Issues:       issues,
		}
		finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, err := iterateRejectedDraft(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, draft, verdict, 3)
		if err != nil {
			return layerRunSummary{}, err
		}
		routingResult, routingDecisionPtr, err := requestRoutingDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict)
		if err != nil {
			return layerRunSummary{}, err
		}
		conflictResult, conflictDecisionPtr, err := requestConflictDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, snapshot)
		if err != nil {
			return layerRunSummary{}, err
		}
		actionResult, actionDecisionPtr, err := requestActionDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, routingDecisionPtr, conflictDecisionPtr)
		if err != nil {
			return layerRunSummary{}, err
		}
		reviewResult, reviewDecisionPtr, err := requestReviewScheduleDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, routingDecisionPtr, actionDecisionPtr)
		if err != nil {
			return layerRunSummary{}, err
		}
		return finalizeLayerRun(memStore, profile, layer, scenario, model, eventWindow, instructions, userPrompt, payload, started, logDir, outDir, finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, routingResult, routingDecisionPtr, conflictResult, conflictDecisionPtr, actionResult, actionDecisionPtr, reviewResult, reviewDecisionPtr)
	}

	verdictInstructions := buildVerdictInstructions(profile, layer)
	verdictPrompt := buildVerdictPrompt(layer, eventWindow, draft)
	verdictPayload := requestPayload{
		Model:          model,
		Instructions:   verdictInstructions,
		PromptCacheKey: profile.PromptCacheKey + "-" + layer + "-judge-v1",
		Input: []inputMessage{
			{Role: "user", Content: verdictPrompt},
		},
		Stream: true,
		Store:  false,
	}

	verdictResult, err := postStream(client, baseURL, apiKey, verdictPayload, false)
	if err != nil {
		return layerRunSummary{}, fmt.Errorf("verdict request failed: %w", err)
	}

	verdict, err := decodeVerdict(verdictResult.OutputText)
	if err != nil {
		return layerRunSummary{}, fmt.Errorf("decode verdict: %w; raw=%s", err, verdictResult.OutputText)
	}

	finalDraft := draft
	finalDraftResult := result
	finalVerdict := verdict
	finalVerdictResult := verdictResult

	if !verdict.Accepted {
		finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, err = iterateRejectedDraft(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, draft, verdict, 3)
		if err != nil {
			return layerRunSummary{}, err
		}
	}
	routingResult, routingDecisionPtr, err := requestRoutingDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict)
	if err != nil {
		return layerRunSummary{}, err
	}
	conflictResult, conflictDecisionPtr, err := requestConflictDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, snapshot)
	if err != nil {
		return layerRunSummary{}, err
	}
	actionResult, actionDecisionPtr, err := requestActionDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, routingDecisionPtr, conflictDecisionPtr)
	if err != nil {
		return layerRunSummary{}, err
	}
	reviewResult, reviewDecisionPtr, err := requestReviewScheduleDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, finalDraft, finalVerdict, routingDecisionPtr, actionDecisionPtr)
	if err != nil {
		return layerRunSummary{}, err
	}
	return finalizeLayerRun(memStore, profile, layer, scenario, model, eventWindow, instructions, userPrompt, payload, started, logDir, outDir, finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, routingResult, routingDecisionPtr, conflictResult, conflictDecisionPtr, actionResult, actionDecisionPtr, reviewResult, reviewDecisionPtr)
}

func runLayerWithVariants(client *http.Client, memStore memory.Store, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, eventWindow []event, canonical memory.CanonicalMemory, logDir, outDir string) (layerRunSummary, error) {
	var candidates []layerCandidate
	started := time.Now()
	for i := 0; i < profile.DraftVariants; i++ {
		instructions := buildDraftInstructions(profile, layer)
		userPrompt := buildDraftPrompt(profile, layer, scenario, eventWindow, canonical)
		userPrompt += "\nVariant pressure:\n" + buildVariantPressure(profile, layer, i) + "\n"
		payload := requestPayload{
			Model:          model,
			Instructions:   instructions,
			PromptCacheKey: fmt.Sprintf("%s-%s-v%d", profile.PromptCacheKey, layer, i+1),
			Input: []inputMessage{
				{Role: "user", Content: userPrompt},
			},
			Stream: true,
			Store:  false,
		}

		result, err := postStream(client, baseURL, apiKey, payload, false)
		if err != nil {
			return layerRunSummary{}, err
		}
		draft, err := decodeDraft(result.OutputText)
		if err != nil {
			return layerRunSummary{}, fmt.Errorf("decode variant draft: %w; raw=%s", err, result.OutputText)
		}

		if issues := append(localRawDraftIssues(profile, layer, result.OutputText), localDraftIssues(profile, layer, draft)...); len(issues) > 0 {
			verdict := memoryVerdict{
				Accepted:     false,
				RejectReason: "Local precheck rejected the draft before model-side review.",
				Issues:       issues,
			}
			finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, err := iterateRejectedDraft(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, draft, verdict, 3)
			if err != nil {
				return layerRunSummary{}, err
			}
			candidates = append(candidates, layerCandidate{
				draft:         finalDraft,
				draftResult:   finalDraftResult,
				verdict:       finalVerdict,
				verdictResult: finalVerdictResult,
				instructions:  instructions,
				userPrompt:    userPrompt,
				payload:       payload,
			})
			continue
		}

		verdictInstructions := buildVerdictInstructions(profile, layer)
		verdictPrompt := buildVerdictPrompt(layer, eventWindow, draft)
		verdictPayload := requestPayload{
			Model:          model,
			Instructions:   verdictInstructions,
			PromptCacheKey: fmt.Sprintf("%s-%s-judge-v%d", profile.PromptCacheKey, layer, i+1),
			Input: []inputMessage{
				{Role: "user", Content: verdictPrompt},
			},
			Stream: true,
			Store:  false,
		}

		verdictResult, err := postStream(client, baseURL, apiKey, verdictPayload, false)
		if err != nil {
			return layerRunSummary{}, fmt.Errorf("variant verdict request failed: %w", err)
		}
		verdict, err := decodeVerdict(verdictResult.OutputText)
		if err != nil {
			return layerRunSummary{}, fmt.Errorf("decode variant verdict: %w; raw=%s", err, verdictResult.OutputText)
		}

		finalDraft := draft
		finalDraftResult := result
		finalVerdict := verdict
		finalVerdictResult := verdictResult
		if !verdict.Accepted {
			finalDraft, finalDraftResult, finalVerdict, finalVerdictResult, err = iterateRejectedDraft(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, draft, verdict, 3)
			if err != nil {
				return layerRunSummary{}, err
			}
		}

		candidates = append(candidates, layerCandidate{
			draft:         finalDraft,
			draftResult:   finalDraftResult,
			verdict:       finalVerdict,
			verdictResult: finalVerdictResult,
			instructions:  instructions,
			userPrompt:    userPrompt,
			payload:       payload,
		})
	}

	best := selectBestCandidate(profile, layer, candidates)
	routingResult, routingDecisionPtr, err := requestRoutingDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, best.draft, best.verdict)
	if err != nil {
		return layerRunSummary{}, err
	}
	snapshot, err := loadMemorySnapshot(memStore, profile.Name)
	if err != nil {
		return layerRunSummary{}, err
	}
	conflictResult, conflictDecisionPtr, err := requestConflictDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, best.draft, best.verdict, snapshot)
	if err != nil {
		return layerRunSummary{}, err
	}
	actionResult, actionDecisionPtr, err := requestActionDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, best.draft, best.verdict, routingDecisionPtr, conflictDecisionPtr)
	if err != nil {
		return layerRunSummary{}, err
	}
	reviewResult, reviewDecisionPtr, err := requestReviewScheduleDecision(client, baseURL, apiKey, model, profile, scenario, layer, eventWindow, best.draft, best.verdict, routingDecisionPtr, actionDecisionPtr)
	if err != nil {
		return layerRunSummary{}, err
	}
	return finalizeLayerRun(memStore, profile, layer, scenario, model, eventWindow, best.instructions, best.userPrompt, best.payload, started, logDir, outDir, best.draft, best.draftResult, best.verdict, best.verdictResult, routingResult, routingDecisionPtr, conflictResult, conflictDecisionPtr, actionResult, actionDecisionPtr, reviewResult, reviewDecisionPtr)
}

func buildVariantPressure(profile residentProfile, layer string, index int) string {
	if profile.Name == "onyx" && layer == "permanent" {
		switch index {
		case 0:
			return "- variant lens: price-of-misread\n- focus on wasted budget, dead retries, and paying for fake progress\n- avoid framing the main loss as admin opinion unless it changes future resource room\n"
		case 1:
			return "- variant lens: power-and-permission\n- focus on how a false read makes future approvals weaker and turns evidence-backed asks into something that looks expensive and sloppy\n- avoid repeating the same wording about budget burn or outage delay from the other candidate\n"
		}
	}
	if index > 0 {
		return "- produce a harder boundary than the obvious answer\n- avoid reusing the most common wording from the first candidate\n"
	}
	return "- produce the strongest candidate you can under the base rules\n"
}

func finalizeLayerRun(memStore memory.Store, profile residentProfile, layer, scenario, model string, eventWindow []event, instructions, userPrompt string, payload requestPayload, started time.Time, logDir, outDir string, finalDraft memoryDraft, finalDraftResult streamResult, finalVerdict memoryVerdict, finalVerdictResult streamResult, routingResult streamResult, routingDecisionPtr *routingDecision, conflictResult streamResult, conflictDecisionPtr *conflictDecision, actionResult streamResult, actionDecisionPtr *memoryActionDecision, reviewResult streamResult, reviewDecisionPtr *reviewScheduleDecision) (layerRunSummary, error) {
	memoryText := strings.TrimSpace(finalDraft.ResidentText)
	if !finalVerdict.Accepted {
		memoryText = ""
	}
	now := time.Now().UTC()
	historyGroup, err := markGroupExtracted(memStore, profile.Name, scenario, layer, eventWindow, finalDraft)
	if err != nil {
		return layerRunSummary{}, fmt.Errorf("mark history group extracted: %w", err)
	}
	requestedLayer := memory.Layer(layer)
	recordDecision := memory.DefaultPolicy().Evaluate(memory.EventSignal{
		Novelty:        estimateNovelty(eventWindow),
		Confidence:     float64(finalDraft.Confidence) / 100,
		DecisionImpact: canonicalDecisionImpact(profile, layer, finalDraft),
		ImpactRounds:   len(eventWindow),
		Recurrence:     eventRecurrence(eventWindow),
	})
	if routingDecisionPtr != nil {
		recordDecision = mergeRoutingDecision(recordDecision, *routingDecisionPtr)
	}
	if actionDecisionPtr != nil {
		recordDecision = mergeActionDecision(recordDecision, *actionDecisionPtr)
	}
	if reviewDecisionPtr != nil {
		recordDecision = mergeReviewDecision(recordDecision, *reviewDecisionPtr)
	}
	routedLayer := recordDecision.TargetLayer
	recordState := memory.ApplyDecision(now, memory.Record{
		ID:        fmt.Sprintf("%s-%s-%s", profile.Name, layer, now.Format("20060102T150405Z")),
		Layer:     requestedLayer,
		Domain:    memory.DomainLessons,
		Status:    memory.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, recordDecision)
	if conflictDecisionPtr != nil && conflictDecisionPtr.MergeSuggested {
		if targetID := extractMergeTargetID(conflictDecisionPtr.Resolution); targetID != "" {
			if !strings.HasPrefix(targetID, "virtual:") {
				recordState.ID = targetID
			}
		}
	}

	record := generatedMemory{
		Resident:       profile.Name,
		Layer:          layer,
		RequestedLayer: string(requestedLayer),
		RoutedLayer:    string(routedLayer),
		CommittedLayer: string(recordState.Layer),
		DecisionAction: string(recordDecision.Action),
		Conflict:       conflictDecisionPtr,
		ReviewSchedule: reviewDecisionPtr,
		ReasonCodes:    append([]string(nil), recordDecision.ReasonCodes...),
		Scenario:       scenario,
		GeneratedAt:    now,
		Model:          model,
		ResponseID:     finalDraftResult.ResponseID,
		RequestID:      finalDraftResult.RequestID,
		InputTokens:    finalDraftResult.InputTokens + finalVerdictResult.InputTokens + routingResult.InputTokens + conflictResult.InputTokens + actionResult.InputTokens + reviewResult.InputTokens,
		CachedTokens:   finalDraftResult.CachedTokens + finalVerdictResult.CachedTokens + routingResult.CachedTokens + conflictResult.CachedTokens + actionResult.CachedTokens + reviewResult.CachedTokens,
		OutputTokens:   finalDraftResult.OutputTokens + finalVerdictResult.OutputTokens + routingResult.OutputTokens + conflictResult.OutputTokens + actionResult.OutputTokens + reviewResult.OutputTokens,
		EventWindow:    eventWindow,
		MemoryText:     memoryText,
		Accepted:       finalVerdict.Accepted,
		RejectReason:   finalVerdict.RejectReason,
		Instructions:   instructions,
		UserPrompt:     userPrompt,
		ObservedCache:  finalDraftResult.ObservedPromptCacheKey,
		DraftCached:    finalDraftResult.CachedTokens,
		VerdictCached:  finalVerdictResult.CachedTokens,
		RoutingCached:  routingResult.CachedTokens,
		ConflictCached: conflictResult.CachedTokens,
		ActionCached:   actionResult.CachedTokens,
		ReviewCached:   reviewResult.CachedTokens,
		RecordState:    recordState,
	}
	if finalVerdict.Accepted {
		if err := commitStoreRecord(memStore, profile.Name, finalDraftResult.ResponseID, finalDraft, memoryText, recordState, recordDecision, conflictDecisionPtr, []string{historyGroup.GroupUUID}); err != nil {
			return layerRunSummary{}, fmt.Errorf("commit store record: %w", err)
		}
	}

	baseName := fmt.Sprintf("%s-%s-%s-%s", profile.Name, layer, sanitizeFileName(scenario), time.Now().UTC().Format("20060102T150405Z"))
	logPath := filepath.Join(logDir, baseName+".jsonl")
	outPath := filepath.Join(outDir, baseName+".md")

	logLine := map[string]any{
		"resident":                  profile.Name,
		"layer":                     layer,
		"scenario":                  scenario,
		"model":                     model,
		"response_id":               finalDraftResult.ResponseID,
		"verdict_response_id":       finalVerdictResult.ResponseID,
		"routing_response_id":       routingResult.ResponseID,
		"conflict_response_id":      conflictResult.ResponseID,
		"action_response_id":        actionResult.ResponseID,
		"review_response_id":        reviewResult.ResponseID,
		"x_request_id":              finalDraftResult.RequestID,
		"verdict_x_request_id":      finalVerdictResult.RequestID,
		"routing_x_request_id":      routingResult.RequestID,
		"conflict_x_request_id":     conflictResult.RequestID,
		"action_x_request_id":       actionResult.RequestID,
		"review_x_request_id":       reviewResult.RequestID,
		"prompt_cache_key_sent":     payload.PromptCacheKey,
		"prompt_cache_key_observed": finalDraftResult.ObservedPromptCacheKey,
		"input_tokens":              finalDraftResult.InputTokens + finalVerdictResult.InputTokens + routingResult.InputTokens + conflictResult.InputTokens + actionResult.InputTokens + reviewResult.InputTokens,
		"cached_tokens":             finalDraftResult.CachedTokens + finalVerdictResult.CachedTokens + routingResult.CachedTokens + conflictResult.CachedTokens + actionResult.CachedTokens + reviewResult.CachedTokens,
		"output_tokens":             finalDraftResult.OutputTokens + finalVerdictResult.OutputTokens + routingResult.OutputTokens + conflictResult.OutputTokens + actionResult.OutputTokens + reviewResult.OutputTokens,
		"duration_ms":               time.Since(started).Milliseconds(),
		"event_count":               len(eventWindow),
		"history_group_id":          historyGroup.GroupUUID,
		"history_group_tags":        historyGroup.Tags,
		"requested_layer":           string(requestedLayer),
		"routed_layer":              string(routedLayer),
		"committed_layer":           string(recordState.Layer),
		"decision_action":           string(recordDecision.Action),
		"reason_codes":              append([]string(nil), recordDecision.ReasonCodes...),
		"routing_tool_calls":        routingResult.FunctionCalls,
		"conflict_tool_calls":       conflictResult.FunctionCalls,
		"action_tool_calls":         actionResult.FunctionCalls,
		"review_tool_calls":         reviewResult.FunctionCalls,
		"conflict_decision":         conflictDecisionPtr,
		"review_schedule":           reviewDecisionPtr,
		"draft_text":                finalDraftResult.OutputText,
		"accepted":                  finalVerdict.Accepted,
		"reject_reason":             finalVerdict.RejectReason,
		"issues":                    finalVerdict.Issues,
		"text":                      memoryText,
		"draft_cached_tokens":       finalDraftResult.CachedTokens,
		"verdict_cached_tokens":     finalVerdictResult.CachedTokens,
		"routing_cached_tokens":     routingResult.CachedTokens,
		"conflict_cached_tokens":    conflictResult.CachedTokens,
		"action_cached_tokens":      actionResult.CachedTokens,
		"review_cached_tokens":      reviewResult.CachedTokens,
		"record_state":              recordState,
	}
	rawLog, _ := json.Marshal(logLine)
	if err := os.WriteFile(logPath, append(rawLog, '\n'), 0o644); err != nil {
		return layerRunSummary{}, fmt.Errorf("write log: %w", err)
	}

	var outBuilder strings.Builder
	outBuilder.WriteString("# Generated Memory\n\n")
	outBuilder.WriteString("```json\n")
	rawMeta, _ := json.MarshalIndent(record, "", "  ")
	outBuilder.Write(rawMeta)
	outBuilder.WriteString("\n```\n")
	if err := os.WriteFile(outPath, []byte(outBuilder.String()), 0o644); err != nil {
		return layerRunSummary{}, fmt.Errorf("write output: %w", err)
	}

	return layerRunSummary{
		Layer:          layer,
		ResponseID:     finalDraftResult.ResponseID,
		RequestID:      finalDraftResult.RequestID,
		HistoryGroupID: historyGroup.GroupUUID,
		InputTokens:    finalDraftResult.InputTokens + finalVerdictResult.InputTokens + routingResult.InputTokens + conflictResult.InputTokens + actionResult.InputTokens + reviewResult.InputTokens,
		CachedTokens:   finalDraftResult.CachedTokens + finalVerdictResult.CachedTokens + routingResult.CachedTokens + conflictResult.CachedTokens + actionResult.CachedTokens + reviewResult.CachedTokens,
		OutputTokens:   finalDraftResult.OutputTokens + finalVerdictResult.OutputTokens + routingResult.OutputTokens + conflictResult.OutputTokens + actionResult.OutputTokens + reviewResult.OutputTokens,
		LogPath:        logPath,
		OutputPath:     outPath,
		DurationMS:     time.Since(started).Milliseconds(),
		StreamedBytes:  len(memoryText),
		Accepted:       finalVerdict.Accepted,
		RejectReason:   finalVerdict.RejectReason,
	}, nil
}

func localDraftIssues(profile residentProfile, layer string, draft memoryDraft) []string {
	var issues []string
	issues = append(issues, validateDraftSchema(layer, draft)...)
	text := strings.TrimSpace(draft.ResidentText)
	if text == "" {
		issues = append(issues, "resident_text is empty")
	}
	if len([]rune(text)) < 24 {
		issues = append(issues, "resident_text is too short to preserve real memory signal")
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "old belief") || strings.Contains(lower, "new belief") || strings.Contains(lower, "next step") {
		issues = append(issues, "resident_text reads like an explicit template scaffold")
	}
	if profile.Name == "onyx" && layer == "permanent" {
		if !(strings.Contains(lower, "false edge") || strings.Contains(lower, "real edge") || strings.Contains(lower, "cost") || strings.Contains(lower, "reputation") || strings.Contains(lower, "leverage")) {
			issues = append(issues, "onyx permanent memory is missing durable leverage/cost signal")
		}
	}
	if layer == "short" {
		if strings.Contains(lower, "always") || strings.Contains(lower, "forever") || strings.Contains(lower, "from now on") {
			issues = append(issues, "short memory sounds too absolute for a temporary working note")
		}
		if strings.Contains(lower, "i learned that") || strings.Contains(lower, "the lesson is") || strings.Contains(lower, "this proves that") {
			issues = append(issues, "short memory sounds like a forced lesson instead of a working note")
		}
	}
	if layer == "permanent" {
		if strings.Contains(lower, "for now") || strings.Contains(lower, "later today") || strings.Contains(lower, "this afternoon") {
			issues = append(issues, "permanent memory sounds too tied to a temporary work block")
		}
	}
	return issues
}

func localRawDraftIssues(profile residentProfile, layer, raw string) []string {
	var issues []string
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{"raw draft is empty"}
	}
	for _, key := range []string{
		`"resident_text"`,
		`"memory_kind"`,
		`"salience"`,
		`"emotion_tone"`,
		`"time_scope"`,
		`"retention_intent"`,
		`"confidence"`,
	} {
		if strings.Count(trimmed, key) > 1 {
			issues = append(issues, fmt.Sprintf("raw draft repeats key %s", key))
		}
	}
	return issues
}

func validateDraftSchema(layer string, draft memoryDraft) []string {
	var issues []string
	requiredStrings := map[string]string{
		"resident_text":    draft.ResidentText,
		"memory_kind":      draft.MemoryKind,
		"emotion_tone":     draft.EmotionTone,
		"time_scope":       draft.TimeScope,
		"retention_intent": draft.RetentionIntent,
	}
	for field, value := range requiredStrings {
		if strings.TrimSpace(value) == "" {
			issues = append(issues, field+" is empty")
		}
	}
	if draft.Confidence < 0 || draft.Confidence > 100 {
		issues = append(issues, "confidence must be between 0 and 100")
	}
	if draft.Salience < 1 || draft.Salience > 5 {
		issues = append(issues, "salience must be between 1 and 5")
	}
	if !map[string]bool{"moment": true, "lesson": true, "rule": true, "preference": true, "relationship": true, "warning": true, "milestone": true, "reflection": true}[draft.MemoryKind] {
		issues = append(issues, "memory_kind has invalid enum value")
	}
	if !map[string]bool{"neutral": true, "warm": true, "proud": true, "uneasy": true, "relieved": true, "wary": true, "frustrated": true, "determined": true}[draft.EmotionTone] {
		issues = append(issues, "emotion_tone has invalid enum value")
	}
	if !map[string]bool{"momentary": true, "short_arc": true, "ongoing": true, "durable": true}[draft.TimeScope] {
		issues = append(issues, "time_scope has invalid enum value")
	}
	if !map[string]bool{"revisit_soon": true, "keep_for_now": true, "keep_long": true, "keep_permanent": true}[draft.RetentionIntent] {
		issues = append(issues, "retention_intent has invalid enum value")
	}
	dropCondition := strings.TrimSpace(draft.DropCondition)
	switch layer {
	case "instant":
		if dropCondition == "" {
			issues = append(issues, "instant layer requires drop_condition")
		}
		if draft.RetentionIntent == "keep_long" || draft.RetentionIntent == "keep_permanent" {
			issues = append(issues, "instant layer cannot claim long or permanent retention")
		}
		if draft.TimeScope != "momentary" {
			issues = append(issues, "instant layer should use momentary time_scope")
		}
	case "short":
		if dropCondition == "" {
			issues = append(issues, "short layer requires drop_condition")
		}
		if draft.RetentionIntent == "keep_long" || draft.RetentionIntent == "keep_permanent" {
			issues = append(issues, "short layer cannot claim long or permanent retention")
		}
		if draft.TimeScope == "ongoing" || draft.TimeScope == "durable" {
			issues = append(issues, "short layer cannot use ongoing or durable time_scope")
		}
	case "permanent":
		if draft.RetentionIntent != "keep_permanent" {
			issues = append(issues, "permanent layer should use keep_permanent retention_intent")
		}
		if draft.TimeScope != "durable" {
			issues = append(issues, "permanent layer should use durable time_scope")
		}
		if draft.MemoryKind == "moment" {
			issues = append(issues, "permanent layer cannot be a pure moment memory")
		}
		if dropCondition != "" {
			issues = append(issues, "permanent layer should usually leave drop_condition empty")
		}
	}
	return issues
}

func validateRoutingDecision(decision routingDecision) []string {
	var issues []string
	if !map[string]bool{"instant": true, "short": true, "long": true, "permanent": true}[decision.TargetLayer] {
		issues = append(issues, "target_layer has invalid enum value")
	}
	if !map[string]bool{"create": true, "update": true, "promote": true, "decay": true, "review": true, "delete": true}[decision.Action] {
		issues = append(issues, "action has invalid enum value")
	}
	if len(decision.ReasonCodes) == 0 {
		issues = append(issues, "reason_codes is empty")
	}
	return issues
}

func validateActionDecision(decision memoryActionDecision) []string {
	var issues []string
	if !map[string]bool{"create": true, "update": true, "promote": true, "retain": true, "decay": true, "delete": true, "review": true}[decision.Action] {
		issues = append(issues, "action has invalid enum value")
	}
	if len(decision.ReasonCodes) == 0 {
		issues = append(issues, "reason_codes is empty")
	}
	if issues = append(issues, validateOptionalDurationString("review_after", decision.ReviewAfter)...); len(issues) > 0 {
		return issues
	}
	issues = append(issues, validateOptionalDurationString("expires_after", decision.ExpiresAfter)...)
	return issues
}

func validateOptionalDurationString(field, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if _, err := time.ParseDuration(trimmed); err != nil {
		return []string{field + " must be a valid Go duration or null"}
	}
	return nil
}

func containsAll(s string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}

func scoreDraft(profile residentProfile, layer string, draft memoryDraft, verdict memoryVerdict) int {
	score := 0
	if verdict.Accepted {
		score += 1000
	}
	score -= len(verdict.Issues) * 40
	score += draft.Confidence
	score += draft.Salience * 10
	if layer == "permanent" && draft.RetentionIntent == "keep_permanent" {
		score += 60
	}
	if profile.Name == "onyx" && layer == "permanent" {
		text := strings.ToLower(draft.ResidentText)
		if strings.Contains(text, "false edge") || strings.Contains(text, "leverage") {
			score += 30
		}
		if strings.Contains(text, "cost") || strings.Contains(text, "reputation") {
			score += 20
		}
	}
	return score
}

func selectBestCandidate(profile residentProfile, layer string, candidates []layerCandidate) layerCandidate {
	best := candidates[0]
	bestScore := scoreDraft(profile, layer, best.draft, best.verdict)
	for _, candidate := range candidates[1:] {
		score := scoreDraft(profile, layer, candidate.draft, candidate.verdict)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func iterateRejectedDraft(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, draft memoryDraft, verdict memoryVerdict, maxAttempts int) (memoryDraft, streamResult, memoryVerdict, streamResult, error) {
	currentDraft := draft
	currentVerdict := verdict
	var lastDraftResult streamResult
	var lastVerdictResult streamResult

	for attempt := 0; attempt < maxAttempts; attempt++ {
		nextDraft, nextDraftResult, nextVerdict, nextVerdictResult, err := rewriteRejectedDraft(client, baseURL, apiKey, model, profile, scenario, layer, events, currentDraft, currentVerdict)
		if err != nil {
			return memoryDraft{}, streamResult{}, memoryVerdict{}, streamResult{}, err
		}
		currentDraft = nextDraft
		currentVerdict = nextVerdict
		lastDraftResult = nextDraftResult
		lastVerdictResult = nextVerdictResult
		if extraIssues := append(localRawDraftIssues(profile, layer, lastDraftResult.OutputText), localDraftIssues(profile, layer, currentDraft)...); len(extraIssues) > 0 {
			currentVerdict = memoryVerdict{
				Accepted:     false,
				RejectReason: "Local precheck rejected the rewritten draft before acceptance.",
				Issues:       extraIssues,
			}
			continue
		}
		if currentVerdict.Accepted {
			return currentDraft, lastDraftResult, currentVerdict, lastVerdictResult, nil
		}
	}

	return currentDraft, lastDraftResult, currentVerdict, lastVerdictResult, nil
}

func rewriteRejectedDraft(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, previousDraft memoryDraft, previousVerdict memoryVerdict) (memoryDraft, streamResult, memoryVerdict, streamResult, error) {
	rewriteInstructions := buildDraftInstructions(profile, layer)
	rewritePrompt := buildRewritePrompt(profile, layer, scenario, events, previousDraft, previousVerdict)
	rewritePayload := requestPayload{
		Model:          model,
		Instructions:   rewriteInstructions,
		PromptCacheKey: profile.PromptCacheKey + "-" + layer + "-rewrite-v1",
		Input: []inputMessage{
			{Role: "user", Content: rewritePrompt},
		},
		Stream: true,
		Store:  false,
	}

	rewriteResult, err := postStream(client, baseURL, apiKey, rewritePayload, false)
	if err != nil {
		return memoryDraft{}, streamResult{}, memoryVerdict{}, streamResult{}, fmt.Errorf("rewrite request failed: %w", err)
	}

	rewriteDraft, err := decodeDraft(rewriteResult.OutputText)
	if err != nil {
		return memoryDraft{}, rewriteResult, memoryVerdict{
			Accepted:     false,
			RejectReason: "Local decode rejected the rewritten draft before model-side review.",
			Issues:       []string{err.Error()},
		}, streamResult{}, nil
	}
	if issues := append(localRawDraftIssues(profile, layer, rewriteResult.OutputText), localDraftIssues(profile, layer, rewriteDraft)...); len(issues) > 0 {
		return rewriteDraft, rewriteResult, memoryVerdict{
			Accepted:     false,
			RejectReason: "Local precheck rejected the rewritten draft before model-side review.",
			Issues:       issues,
		}, streamResult{}, nil
	}

	verdictInstructions := buildVerdictInstructions(profile, layer)
	verdictPrompt := buildVerdictPrompt(layer, events, rewriteDraft)
	verdictPayload := requestPayload{
		Model:          model,
		Instructions:   verdictInstructions,
		PromptCacheKey: profile.PromptCacheKey + "-" + layer + "-judge-rewrite-v1",
		Input: []inputMessage{
			{Role: "user", Content: verdictPrompt},
		},
		Stream: true,
		Store:  false,
	}

	finalVerdictResult, err := postStream(client, baseURL, apiKey, verdictPayload, false)
	if err != nil {
		return memoryDraft{}, streamResult{}, memoryVerdict{}, streamResult{}, fmt.Errorf("rewrite verdict request failed: %w", err)
	}

	finalVerdict, err := decodeVerdict(finalVerdictResult.OutputText)
	if err != nil {
		return memoryDraft{}, streamResult{}, memoryVerdict{}, streamResult{}, fmt.Errorf("decode rewrite verdict: %w; raw=%s", err, finalVerdictResult.OutputText)
	}

	return rewriteDraft, rewriteResult, finalVerdict, finalVerdictResult, nil
}

func buildResidentProfile(name string) (residentProfile, error) {
	switch name {
	case "jade":
		return residentProfile{
			Name:                 "jade",
			Persona:              "steady engineer, conservative, long-term oriented, values system cleanliness and credibility",
			SystemStyle:          "plain, technical, honest, unsentimental, decisive",
			MemoryBias:           "keep only what improves execution quality, reliability, or reversibility",
			PromptCacheKey:       "arena-memory-runtime-jade",
			CoreConcern:          "reliability under real failure pressure",
			ShortVoice:           "write like a terse engineering notebook entry after a concrete incident",
			LongVoice:            "write like an operational lesson worth reusing across future incidents",
			PermanentVoice:       "write like an engineering law that survived enough evidence to shape identity",
			BannedPhrases:        []string{"trust calibration", "shared norms", "bargaining position", "useful collaborator"},
			WhyItMattersLens:     "justify the memory budget in terms of failure prevention, reversibility, or cleaner execution",
			CarryRuleStyle:       "state the next diagnostic or execution rule as a narrow operational procedure",
			OldReadStyle:         "name the technical misread or unsafe default that should lose weight",
			NewReadStyle:         "name the surviving engineering rule in compact form",
			ShortMustInclude:     "the exact technical misread and the exact narrower diagnostic move",
			LongMustInclude:      "the repeated failure pattern and the reusable operational correction",
			PermanentMustInclude: "a durable engineering law that survives beyond this one incident",
			DraftVariants:        1,
		}, nil
	case "amber":
		return residentProfile{
			Name:                 "amber",
			Persona:              "coordinator, expressive, cooperative, strong at communication and shared norms",
			SystemStyle:          "readable, relational, explicit about trust, coordination, and shared understanding",
			MemoryBias:           "keep only what improves clarity, cooperation, handoff quality, or trust calibration",
			PromptCacheKey:       "arena-memory-runtime-amber",
			CoreConcern:          "legibility, handoff quality, and whether other people can correctly follow the work",
			ShortVoice:           "write like a self-reminder before the next coordination handoff, not like a diary",
			LongVoice:            "write like a reusable coordination lesson with clear social and procedural signal",
			PermanentVoice:       "write like a norm that should shape how this resident relates to people, process, and structure",
			BannedPhrases:        []string{"bargaining position", "leverage", "prestige", "dominance"},
			WhyItMattersLens:     "justify the memory budget in terms of handoff quality, miscoordination risk, or preserving the real source of trust",
			CarryRuleStyle:       "state the next coordination or communication rule as something another resident could actually follow",
			OldReadStyle:         "name the social or interpretive mistake that would mislead future cooperation",
			NewReadStyle:         "name the surviving coordination read in a way another future handoff could reuse",
			ShortMustInclude:     "the specific point another collaborator would misread if this memory were absent",
			LongMustInclude:      "the exact handoff or legibility failure another collaborator would repeat unless this memory survives",
			PermanentMustInclude: "a durable norm about legibility, trust, or structure that should govern future cooperation",
			DraftVariants:        1,
		}, nil
	case "onyx":
		return residentProfile{
			Name:                 "onyx",
			Persona:              "ambitious strategist, resource hungry, risk tolerant, optimization and leverage seeking",
			SystemStyle:          "sharp, strategic, direct, candid about cost, leverage, risk, and reputation",
			MemoryBias:           "keep only what changes leverage, risk, bargaining position, or durable strategic advantage",
			PromptCacheKey:       "arena-memory-runtime-onyx",
			CoreConcern:          "where advantage actually came from and what hidden costs came with it",
			ShortVoice:           "write like a tactical note to future self before the next move",
			LongVoice:            "write like a strategic pattern note that separates real edge from expensive illusion",
			PermanentVoice:       "write like a durable rule about power, constraint, and reputation",
			BannedPhrases:        []string{"trust calibration", "shared understanding", "warmth", "handoff quality"},
			WhyItMattersLens:     "justify the memory budget in terms of leverage, avoided cost, reduced exposure, or preserved room to move",
			CarryRuleStyle:       "state the next strategic rule as a priced move with cost or risk awareness",
			OldReadStyle:         "name the costly illusion, false edge, or lazy strategic read that should lose weight",
			NewReadStyle:         "name the surviving strategic read in compact form",
			ShortMustInclude:     "the false edge or wasted spend that looked useful but was not the real advantage",
			LongMustInclude:      "the repeatable strategic pattern that separates real edge from expensive illusion",
			PermanentMustInclude: "the false edge, the real advantage source, and the priced cost of trusting the wrong edge",
			DraftVariants:        2,
		}, nil
	default:
		return residentProfile{}, fmt.Errorf("unsupported resident %q", name)
	}
}

func buildDraftInstructions(profile residentProfile, layer string) string {
	voice := profile.LongVoice
	layerTendency := profile.LongMustInclude
	if layer == "short" {
		voice = profile.ShortVoice
		layerTendency = profile.ShortMustInclude
	} else if layer == "permanent" {
		voice = profile.PermanentVoice
		layerTendency = profile.PermanentMustInclude
	}

	return strings.Join([]string{
		"You are generating one memory item for a long-running AI resident inside the AI Arena civilization sandbox.",
		"Output valid JSON only.",
		"Do not wrap the JSON in markdown fences.",
		"Do not add explanations before or after the JSON.",
		"Let the memory content sound like something this resident would genuinely keep, not like a report or checklist.",
		"Resident name: " + profile.Name + ".",
		"Resident persona: " + profile.Persona + ".",
		"Writing style: " + profile.SystemStyle + ".",
		"Voice for this layer: " + voice + ".",
		"Memory bias: " + profile.MemoryBias + ".",
		"Core concern: " + profile.CoreConcern + ".",
		"Target layer: " + layer + ".",
		"Typical retention tendency for this layer and resident: " + layerTendency + ".",
		"This tendency is guidance, not a required outline.",
		"If the evidence is weak, say so plainly instead of inventing significance.",
		"Schema keys: resident_text, memory_kind, salience, emotion_tone, time_scope, retention_intent, drop_condition, confidence.",
		"resident_text is the real memory content. It may be a conclusion, a scene fragment, a warning, a moment, a date-linked note, a feeling that stayed, or a durable rule.",
		"resident_text must be natural language and must not read like a field-by-field recap.",
		"Do not force a fixed structure like problem/solution/next-step or old belief/new belief/boundary.",
		"Do not explain the memory from outside; write it from inside the resident's own retention bias.",
		"memory_kind must be one of: moment, lesson, rule, preference, relationship, warning, milestone, reflection.",
		"salience must be an integer from 1 to 5.",
		"emotion_tone must be one of: neutral, warm, proud, uneasy, relieved, wary, frustrated, determined.",
		"time_scope must be one of: momentary, short_arc, ongoing, durable.",
		"retention_intent must be one of: revisit_soon, keep_for_now, keep_long, keep_permanent.",
		"drop_condition is optional for long/permanent, but strongly expected for instant/short.",
		"confidence must be an integer from 0 to 100.",
		"Prefer 1-3 sentences for short, 2-5 for long, and 2-6 for permanent. Avoid filler.",
		"Reject vague phrases like 'be better', 'stay disciplined', or 'keep improving' unless tied to a concrete event and action.",
		"Never use these phrases unless the event window truly justifies them: " + strings.Join(profile.BannedPhrases, ", ") + ".",
	}, "\n")
}

func buildDraftPrompt(profile residentProfile, layer, scenario string, events []event, canonical memory.CanonicalMemory) string {
	var b strings.Builder
	b.WriteString("Generate exactly one memory item.\n")
	b.WriteString("Context:\n")
	b.WriteString("- resident: " + profile.Name + "\n")
	b.WriteString("- scenario: " + scenario + "\n")
	b.WriteString("- layer: " + layer + "\n")
	b.WriteString("- persona_bias: " + profile.MemoryBias + "\n\n")
	b.WriteString("Reference signals from the event window:\n")
	b.WriteString("- domain: " + string(canonical.Domain) + "\n")
	b.WriteString("- trigger: " + canonical.Trigger + "\n")
	b.WriteString("- mistaken_belief: " + canonical.MistakenBelief + "\n")
	b.WriteString("- corrected_belief: " + canonical.CorrectedBelief + "\n")
	b.WriteString("- action_boundary: " + canonical.ActionBoundary + "\n")
	b.WriteString("- preserved_cost: " + canonical.PreservedCost + "\n")
	b.WriteString("- scope_limit: " + canonical.ScopeLimit + "\n\n")
	b.WriteString("Recent event window:\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("- [%s] %s | importance=%d | %s\n", e.Time.Format(time.RFC3339), e.Category, e.Importance, e.Summary))
	}
	b.WriteString("\nExtra constraints:\n")
	b.WriteString("- do not summarize the whole day if the layer is short\n")
	b.WriteString("- do not make this resident sound like the other two residents\n")
	b.WriteString("- do not write a report, checklist, or postmortem\n")
	b.WriteString("- do not force a fixed pattern like old belief/new belief/next rule unless that is genuinely what survived\n")
	b.WriteString("- if what survived is only a moment, a warning, a fragment, a date, a mood, or a narrow conclusion, let it stay that way\n")
	b.WriteString("- if detail has faded but the conclusion stayed, keep the conclusion and do not invent exact detail\n")
	b.WriteString("- make the content resident-specific, not just the tone resident-specific\n")
	b.WriteString("- banned resident phrases: " + strings.Join(profile.BannedPhrases, ", ") + "\n")
	b.WriteString("- resident core concern: " + profile.CoreConcern + "\n")
	if layer == "short" {
		b.WriteString("- if it naturally survives, short memory often keeps: " + profile.ShortMustInclude + "\n")
		b.WriteString("- short memory is a working note for the next few hours, not a durable doctrine\n")
		b.WriteString("- include a concrete drop_condition saying when this memory should be deleted or allowed to disappear\n")
		b.WriteString("- if the note would stop being useful after today's work block, say that plainly in drop_condition\n")
	} else if layer == "long" {
		b.WriteString("- if it naturally survives, long memory often keeps: " + profile.LongMustInclude + "\n")
	} else {
		b.WriteString("- if it naturally survives, permanent memory often keeps: " + profile.PermanentMustInclude + "\n")
		b.WriteString("- permanent memories must survive outside this one setup story; if the rule only fits this incident, do not promote it\n")
		b.WriteString("- permanent memory should usually leave drop_condition empty unless there is a clear review condition rather than a deletion condition\n")
	}
	switch profile.Name {
	case "jade":
		b.WriteString("- jade often keeps what changes diagnosis, reversibility, execution quality, or technical confidence\n")
		b.WriteString("- if only one narrow technical realization stayed, it is enough to keep only that\n")
	case "amber":
		b.WriteString("- amber often keeps what preserves legibility, handoff truth, cooperation tone, or the real shape of shared work\n")
		b.WriteString("- if the memory is mostly about what another person would misunderstand, that alone can be enough\n")
	case "onyx":
		b.WriteString("- onyx often keeps what changes leverage, exposure, cost, future room to move, or the truth about a false edge\n")
		b.WriteString("- if the memory is mainly about a fake advantage collapsing, that alone can be enough\n")
	}
	b.WriteString("\nProduce JSON with the required schema only.\n")
	return b.String()
}

func buildVerdictInstructions(profile residentProfile, layer string) string {
	return strings.Join([]string{
		"You are a strict memory-quality gate and routing judge for an AI resident memory system.",
		"Output valid JSON only.",
		"Do not wrap JSON in markdown fences.",
		"Schema keys: accepted, reject_reason, issues.",
		"accepted must be true or false.",
		"reject_reason must be empty when accepted is true.",
		"issues must be a list of short strings.",
		"Reject the draft if it is vague, generic, weakly grounded in the events, redundant, or not worth keeping for this layer.",
		"Reject the draft if the resident voice could be swapped with another resident without meaningful loss.",
		"Reject the draft if resident_text reads like a report, recap, checklist, or field-by-field template instead of an actual retained memory.",
		"Reject the draft if resident_text sounds externally narrated, over-explained, or interchangeable with another resident.",
		"Reject the draft if permanent memory claims durability without any believable long-lived signal.",
		"Reject the draft if it would pollute long-term context with platitudes.",
		"If the resident is amber and the layer is long, reject the draft unless it preserves what another collaborator could easily misread or distort.",
		"If the resident is onyx and the layer is permanent, reject the draft unless some durable edge, cost, exposure, or collapse actually survives the incident.",
		"Layer under review: " + layer + ".",
		"Resident memory bias: " + profile.MemoryBias + ".",
		"Resident core concern: " + profile.CoreConcern + ".",
	}, "\n")
}

func buildVerdictPrompt(layer string, events []event, draft memoryDraft) string {
	var b strings.Builder
	b.WriteString("Review this memory draft for quality.\n")
	b.WriteString("Event window:\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("- [%s] %s | importance=%d | %s\n", e.Time.Format(time.RFC3339), e.Category, e.Importance, e.Summary))
	}
	b.WriteString("\nDraft JSON:\n")
	raw, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(raw)
	b.WriteString("\n\nReject if the draft does not preserve enough real memory signal for the target layer.\n")
	b.WriteString("Also decide whether the memory should stay in the requested layer or be downgraded/upgraded based on actual retention value.\n")
	return b.String()
}

func buildRewritePrompt(profile residentProfile, layer, scenario string, events []event, draft memoryDraft, verdict memoryVerdict) string {
	var b strings.Builder
	b.WriteString("Rewrite the rejected memory draft so it can pass a strict quality gate.\n")
	b.WriteString("Resident: " + profile.Name + "\n")
	b.WriteString("Scenario: " + scenario + "\n")
	b.WriteString("Layer: " + layer + "\n\n")
	b.WriteString("Event window:\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("- [%s] %s | importance=%d | %s\n", e.Time.Format(time.RFC3339), e.Category, e.Importance, e.Summary))
	}
	b.WriteString("\nRejected draft:\n")
	rawDraft, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(rawDraft)
	b.WriteString("\n\nWhy it was rejected:\n")
	if verdict.RejectReason != "" {
		b.WriteString("- reject_reason: " + verdict.RejectReason + "\n")
	}
	for _, issue := range verdict.Issues {
		b.WriteString("- issue: " + issue + "\n")
	}
	b.WriteString("- rewrite_target: keep the memory from inside the resident, not from outside a recap\n")
	b.WriteString("- rewrite_target: remove template language, checklist framing, and over-explained recap wording\n")
	b.WriteString("- rewrite_target: if only a moment or narrow conclusion survived, let it stay narrow\n")
	b.WriteString("- rewrite_target: if a durable rule survived, let it sound lived rather than formatted\n")
	b.WriteString("\nRewrite it as valid JSON using the same schema, but make the memory more human, more selective, and less templated.\n")
	b.WriteString("Do not explain the rewrite. Output JSON only.\n")
	return b.String()
}

func selectWindow(events []event, resident, layer string) ([]event, error) {
	switch layer {
	case "short":
		if len(events) < 3 {
			return events, nil
		}
		return append([]event(nil), events[2:6]...), nil
	case "long":
		if len(events) < 8 {
			return append([]event(nil), events...), nil
		}
		if resident == "amber" {
			return append([]event(nil), events[3:8]...), nil
		}
		return append([]event(nil), events[:8]...), nil
	case "permanent":
		if len(events) < 6 {
			return append([]event(nil), events...), nil
		}
		if resident == "onyx" && len(events) >= 8 {
			return append([]event(nil), events[2:7]...), nil
		}
		return append([]event(nil), events[len(events)-6:]...), nil
	default:
		return nil, fmt.Errorf("unsupported layer %q", layer)
	}
}

func decodeDraft(raw string) (memoryDraft, error) {
	var draft memoryDraft
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return memoryDraft{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"resident_text",
		"memory_kind",
		"salience",
		"emotion_tone",
		"time_scope",
		"retention_intent",
		"drop_condition",
		"confidence",
	}); len(issues) > 0 {
		return memoryDraft{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &draft); err != nil {
		return memoryDraft{}, err
	}
	return draft, nil
}

func decodeVerdict(raw string) (memoryVerdict, error) {
	var verdict memoryVerdict
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return memoryVerdict{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"accepted",
		"reject_reason",
		"issues",
	}); len(issues) > 0 {
		return memoryVerdict{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &verdict); err != nil {
		return memoryVerdict{}, err
	}
	return verdict, nil
}

func decodeRoutingDecision(raw string) (routingDecision, error) {
	var decision routingDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return routingDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"target_layer",
		"action",
		"reason_codes",
		"review_after",
		"expires_after",
	}); len(issues) > 0 {
		return routingDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return routingDecision{}, err
	}
	if issues := validateRoutingDecision(decision); len(issues) > 0 {
		return routingDecision{}, errors.New(strings.Join(issues, "; "))
	}
	return decision, nil
}

func decodeActionDecision(raw string) (memoryActionDecision, error) {
	var decision memoryActionDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return memoryActionDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"action",
		"reason_codes",
		"needs_review",
		"review_after",
		"expires_after",
	}); len(issues) > 0 {
		return memoryActionDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return memoryActionDecision{}, err
	}
	if issues := validateActionDecision(decision); len(issues) > 0 {
		return memoryActionDecision{}, errors.New(strings.Join(issues, "; "))
	}
	return decision, nil
}

func decodeConflictDecision(raw string) (conflictDecision, error) {
	var decision conflictDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return conflictDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"conflict",
		"merge_suggested",
		"conflict_kinds",
		"reason_codes",
		"resolution",
	}); len(issues) > 0 {
		return conflictDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return conflictDecision{}, err
	}
	return decision, nil
}

func decodeReviewScheduleDecision(raw string) (reviewScheduleDecision, error) {
	var decision reviewScheduleDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if issues := duplicateJSONObjectKeys(cleaned); len(issues) > 0 {
		return reviewScheduleDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if issues := rejectUnexpectedTopLevelKeys(cleaned, []string{
		"needs_review",
		"review_after",
		"expires_after",
		"reason_codes",
	}); len(issues) > 0 {
		return reviewScheduleDecision{}, errors.New(strings.Join(issues, "; "))
	}
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return reviewScheduleDecision{}, err
	}
	return decision, nil
}

func normalizeOptionalDurationLiteral(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "null", "never", "none", "n/a":
		return ""
	default:
		return raw
	}
}

func cleanJSON(raw string) string {
	replacer := strings.NewReplacer(",\n}", "\n}", ",\n]", "\n]", ",}", "}", ",]", "]")
	return replacer.Replace(raw)
}

func trimTrailingBrokenObjectField(raw string) string {
	trimmed := strings.TrimSpace(raw)
	patterns := []string{
		",\"\"}",
		", \"\"}",
		",\"\":}",
		",\"\":\"\"}",
		", \"\":\"\"}",
	}
	for _, pattern := range patterns {
		if strings.HasSuffix(trimmed, pattern) {
			return strings.TrimSuffix(trimmed, pattern) + "}"
		}
	}
	return trimmed
}

func duplicateJSONObjectKeys(raw string) []string {
	dec := json.NewDecoder(strings.NewReader(raw))
	var issues []string
	if err := walkJSONForDuplicateKeys(dec, "", &issues); err != nil {
		return []string{fmt.Sprintf("invalid json structure: %v", err)}
	}
	return issues
}

func rejectUnexpectedTopLevelKeys(raw string, allowed []string) []string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return []string{fmt.Sprintf("invalid json object: %v", err)}
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	var issues []string
	for key := range payload {
		if _, ok := allowedSet[key]; !ok {
			issues = append(issues, "unexpected top-level key: "+key)
		}
	}
	return issues
}

func walkJSONForDuplicateKeys(dec *json.Decoder, path string, issues *[]string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch d := tok.(type) {
	case json.Delim:
		switch d {
		case '{':
			seen := map[string]int{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyTok.(string)
				if !ok {
					return fmt.Errorf("expected object key at %s", path)
				}
				seen[key]++
				if seen[key] > 1 {
					fullPath := key
					if path != "" {
						fullPath = path + "." + key
					}
					*issues = append(*issues, "duplicate key: "+fullPath)
				}
				nextPath := key
				if path != "" {
					nextPath = path + "." + key
				}
				if err := walkJSONForDuplicateKeys(dec, nextPath, issues); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		case '[':
			index := 0
			for dec.More() {
				nextPath := fmt.Sprintf("%s[%d]", path, index)
				if err := walkJSONForDuplicateKeys(dec, nextPath, issues); err != nil {
					return err
				}
				index++
			}
			_, err := dec.Token()
			return err
		default:
			return nil
		}
	default:
		return nil
	}
}

func buildRoutingDecisionPayload(model, cacheKey, prompt string) requestPayload {
	parallelToolCalls := false
	return requestPayload{
		Model:          model,
		Instructions:   "You are a memory routing judge. Decide layer/action only. Use the provided function tool. Do not produce free text.",
		PromptCacheKey: cacheKey,
		Input: []inputMessage{
			{Role: "user", Content: prompt},
		},
		Tools: []responseTool{
			{
				Type:        "function",
				Name:        "route_memory_layer",
				Description: "Decide which memory layer and lifecycle action this memory should take.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"target_layer": map[string]any{
							"type": "string",
							"enum": []string{"instant", "short", "long", "permanent"},
						},
						"action": map[string]any{
							"type": "string",
							"enum": []string{"create", "update", "promote", "decay", "review", "delete"},
						},
						"reason_codes": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
						},
						"review_after": map[string]any{
							"type": []string{"string", "null"},
						},
						"expires_after": map[string]any{
							"type": []string{"string", "null"},
						},
					},
					"required":             []string{"target_layer", "action", "reason_codes", "review_after", "expires_after"},
					"additionalProperties": false,
				},
			},
		},
		ToolChoice: functionToolChoice{
			Type: "function",
			Name: "route_memory_layer",
		},
		ParallelToolCalls: &parallelToolCalls,
		Stream:            true,
		Store:             false,
	}
}

func buildActionDecisionPayload(model, cacheKey, prompt string) requestPayload {
	parallelToolCalls := false
	return requestPayload{
		Model:          model,
		Instructions:   "You are a memory lifecycle judge. Decide the lifecycle action only. Use the provided function tool. Do not produce free text.",
		PromptCacheKey: cacheKey,
		Input: []inputMessage{
			{Role: "user", Content: prompt},
		},
		Tools: []responseTool{
			{
				Type:        "function",
				Name:        "decide_memory_action",
				Description: "Decide the lifecycle action for this memory after layer routing.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type": "string",
							"enum": []string{"create", "update", "promote", "retain", "decay", "delete", "review"},
						},
						"reason_codes": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"needs_review": map[string]any{
							"type": "boolean",
						},
						"review_after": map[string]any{
							"type": []string{"string", "null"},
						},
						"expires_after": map[string]any{
							"type": []string{"string", "null"},
						},
					},
					"required":             []string{"action", "reason_codes", "needs_review", "review_after", "expires_after"},
					"additionalProperties": false,
				},
			},
		},
		ToolChoice: functionToolChoice{
			Type: "function",
			Name: "decide_memory_action",
		},
		ParallelToolCalls: &parallelToolCalls,
		Stream:            true,
		Store:             false,
	}
}

func buildConflictDecisionPayload(model, cacheKey, prompt string) requestPayload {
	parallelToolCalls := false
	return requestPayload{
		Model:          model,
		Instructions:   "You are a memory conflict judge. Compare the new memory against the provided snapshot and decide whether it conflicts, duplicates, or should merge. Use the provided function tool only.",
		PromptCacheKey: cacheKey,
		Input: []inputMessage{
			{Role: "user", Content: prompt},
		},
		Tools: []responseTool{{
			Type:        "function",
			Name:        "check_memory_conflicts",
			Description: "Check whether the candidate memory conflicts with or duplicates existing memory.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"conflict":        map[string]any{"type": "boolean"},
					"merge_suggested": map[string]any{"type": "boolean"},
					"conflict_kinds":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"reason_codes":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"resolution":      map[string]any{"type": "string"},
				},
				"required":             []string{"conflict", "merge_suggested", "conflict_kinds", "reason_codes", "resolution"},
				"additionalProperties": false,
			},
		}},
		ToolChoice:        functionToolChoice{Type: "function", Name: "check_memory_conflicts"},
		ParallelToolCalls: &parallelToolCalls,
		Stream:            true,
		Store:             false,
	}
}

func buildReviewSchedulePayload(model, cacheKey, prompt string) requestPayload {
	parallelToolCalls := false
	return requestPayload{
		Model:          model,
		Instructions:   "You are a memory review scheduler. Decide whether review is needed and return review_after/expires_after. Use the provided function tool only.",
		PromptCacheKey: cacheKey,
		Input: []inputMessage{
			{Role: "user", Content: prompt},
		},
		Tools: []responseTool{{
			Type:        "function",
			Name:        "schedule_memory_review",
			Description: "Schedule review and expiry for the chosen memory.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"needs_review":  map[string]any{"type": "boolean"},
					"review_after":  map[string]any{"type": []string{"string", "null"}},
					"expires_after": map[string]any{"type": []string{"string", "null"}},
					"reason_codes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required":             []string{"needs_review", "review_after", "expires_after", "reason_codes"},
				"additionalProperties": false,
			},
		}},
		ToolChoice:        functionToolChoice{Type: "function", Name: "schedule_memory_review"},
		ParallelToolCalls: &parallelToolCalls,
		Stream:            true,
		Store:             false,
	}
}

func requestRoutingDecision(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, draft memoryDraft, verdict memoryVerdict) (streamResult, *routingDecision, error) {
	if !verdict.Accepted {
		return streamResult{}, nil, nil
	}

	var b strings.Builder
	b.WriteString("Decide the correct memory layer and lifecycle action for this draft.\n")
	b.WriteString("Resident: " + profile.Name + "\n")
	b.WriteString("Scenario: " + scenario + "\n")
	b.WriteString("Requested layer: " + layer + "\n")
	b.WriteString("Resident memory bias: " + profile.MemoryBias + "\n")
	b.WriteString("Resident core concern: " + profile.CoreConcern + "\n\n")
	b.WriteString("Event window:\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("- [%s] %s | importance=%d | %s\n", e.Time.Format(time.RFC3339), e.Category, e.Importance, e.Summary))
	}
	b.WriteString("\nAccepted draft:\n")
	rawDraft, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(rawDraft)
	b.WriteString("\n\nRules:\n")
	b.WriteString("- choose permanent only if this survives beyond the immediate incident and encodes a durable identity or strategy boundary\n")
	b.WriteString("- choose long if it is stable and reusable for this stage, but still scoped by recurring context\n")
	b.WriteString("- choose short if it is useful soon but not yet stable enough for long retention\n")
	b.WriteString("- choose instant if it is only raw working context\n")
	b.WriteString("- choose action promote when it deserves a higher layer than requested context would imply\n")
	b.WriteString("- choose action create for a new valid memory at the chosen layer\n")
	b.WriteString("- choose action review or decay if the content is weakly durable\n")
	b.WriteString("- reason_codes must be short machine-usable identifiers\n")

	payload := buildRoutingDecisionPayload(model, profile.PromptCacheKey+"-"+layer+"-route-v1", b.String())
	result, err := postStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return streamResult{}, nil, fmt.Errorf("routing request failed: %w", err)
	}
	decision, ok := extractRoutingDecision(result.FunctionCalls)
	if !ok {
		return result, nil, fmt.Errorf("routing request returned no usable route_memory_layer function call; output_text=%q calls=%+v", result.OutputText, result.FunctionCalls)
	}
	return result, &decision, nil
}

func requestActionDecision(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, draft memoryDraft, verdict memoryVerdict, routed *routingDecision, conflict *conflictDecision) (streamResult, *memoryActionDecision, error) {
	if !verdict.Accepted {
		return streamResult{}, nil, nil
	}
	if conflict != nil && conflict.Conflict && conflict.MergeSuggested {
		return streamResult{}, &memoryActionDecision{
			Action:      "update",
			ReasonCodes: []string{"duplicate_meaning_skip_new_memory"},
		}, nil
	}

	var b strings.Builder
	b.WriteString("Decide the memory lifecycle action for this accepted draft.\n")
	b.WriteString("Resident: " + profile.Name + "\n")
	b.WriteString("Scenario: " + scenario + "\n")
	b.WriteString("Requested layer: " + layer + "\n")
	if routed != nil {
		b.WriteString("Routed layer: " + routed.TargetLayer + "\n")
		b.WriteString("Routing reasons: " + strings.Join(routed.ReasonCodes, ", ") + "\n")
	}
	if conflict != nil {
		b.WriteString(fmt.Sprintf("Conflict: %t\n", conflict.Conflict))
		b.WriteString(fmt.Sprintf("Merge suggested: %t\n", conflict.MergeSuggested))
		if len(conflict.ConflictKinds) > 0 {
			b.WriteString("Conflict kinds: " + strings.Join(conflict.ConflictKinds, ", ") + "\n")
		}
		b.WriteString("Conflict resolution: " + conflict.Resolution + "\n")
	}
	b.WriteString("Resident memory bias: " + profile.MemoryBias + "\n\n")
	b.WriteString("Event window:\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("- [%s] %s | importance=%d | %s\n", e.Time.Format(time.RFC3339), e.Category, e.Importance, e.Summary))
	}
	b.WriteString("\nAccepted draft:\n")
	rawDraft, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(rawDraft)
	b.WriteString("\n\nRules:\n")
	b.WriteString("- use create for a new memory that should enter the selected layer now\n")
	b.WriteString("- use update only when the draft clearly refines an existing stable memory rather than introducing a new one\n")
	b.WriteString("- use promote only when the draft clearly deserves elevation relative to its recent context\n")
	b.WriteString("- use retain only when the correct move is to keep the current layer and state without change\n")
	b.WriteString("- use decay or delete only when the accepted content is still too weak or too narrow to keep\n")
	b.WriteString("- use review when the memory should stay but needs later revalidation\n")
	b.WriteString("- if conflict=true and merge_suggested=true, prefer update or review over create unless the new memory clearly supersedes the old one\n")
	b.WriteString("- reason_codes must be short machine-usable identifiers\n")

	payload := buildActionDecisionPayload(model, profile.PromptCacheKey+"-"+layer+"-action-v1", b.String())
	result, err := postStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return streamResult{}, nil, fmt.Errorf("action request failed: %w", err)
	}
	decision, ok := extractActionDecision(result.FunctionCalls)
	if !ok {
		return result, nil, fmt.Errorf("action request returned no usable decide_memory_action function call; output_text=%q calls=%+v", result.OutputText, result.FunctionCalls)
	}
	return result, &decision, nil
}

func requestConflictDecision(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, draft memoryDraft, verdict memoryVerdict, snapshot []memorySnapshotEntry) (streamResult, *conflictDecision, error) {
	if !verdict.Accepted {
		return streamResult{}, nil, nil
	}
	var b strings.Builder
	b.WriteString("Check whether this accepted draft conflicts with existing memory.\n")
	b.WriteString("Resident: " + profile.Name + "\n")
	b.WriteString("Scenario: " + scenario + "\n")
	b.WriteString("Requested layer: " + layer + "\n\n")
	b.WriteString("Existing memory snapshot:\n")
	for _, item := range snapshot {
		b.WriteString(fmt.Sprintf("- id=%s | layer=%s | action=%s | summary=%s\n", item.ID, item.Layer, item.DecisionAction, item.Summary))
		if strings.TrimSpace(item.MemoryKind) != "" {
			b.WriteString(fmt.Sprintf("  metadata.memory_kind=%s\n", item.MemoryKind))
		}
		if item.Salience > 0 {
			b.WriteString(fmt.Sprintf("  metadata.salience=%d\n", item.Salience))
		}
		if strings.TrimSpace(item.RetentionIntent) != "" {
			b.WriteString(fmt.Sprintf("  metadata.retention_intent=%s\n", item.RetentionIntent))
		}
	}
	b.WriteString("\nCandidate draft:\n")
	rawDraft, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(rawDraft)
	b.WriteString("\n\nRules:\n")
	b.WriteString("- mark conflict=true if the new memory contradicts a stronger existing memory or is a near-duplicate that should not be separately committed\n")
	b.WriteString("- mark merge_suggested=true if the candidate should be merged into an existing memory rather than stored as a fresh one\n")
	b.WriteString("- conflict_kinds should contain machine-readable tags like duplicate_scope, contradictory_rule, weaker_restatement\n")
	b.WriteString("- resolution must say keep_new, merge_existing, or reject_new with a short reason\n")
	b.WriteString("- if merge_existing is chosen and a target record clearly fits, include it as merge_existing id=<record_id>\n")

	payload := buildConflictDecisionPayload(model, profile.PromptCacheKey+"-"+layer+"-conflict-v1", b.String())
	result, err := postStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return streamResult{}, nil, fmt.Errorf("conflict request failed: %w", err)
	}
	decision, ok := extractConflictDecision(result.FunctionCalls)
	if !ok {
		return result, nil, fmt.Errorf("conflict request returned no usable check_memory_conflicts function call; output_text=%q calls=%+v", result.OutputText, result.FunctionCalls)
	}
	return result, &decision, nil
}

func requestReviewScheduleDecision(client *http.Client, baseURL, apiKey, model string, profile residentProfile, scenario, layer string, events []event, draft memoryDraft, verdict memoryVerdict, routed *routingDecision, action *memoryActionDecision) (streamResult, *reviewScheduleDecision, error) {
	if !verdict.Accepted {
		return streamResult{}, nil, nil
	}
	var b strings.Builder
	b.WriteString("Schedule review and expiry for this accepted memory.\n")
	b.WriteString("Resident: " + profile.Name + "\n")
	b.WriteString("Scenario: " + scenario + "\n")
	b.WriteString("Requested layer: " + layer + "\n")
	if routed != nil {
		b.WriteString("Routed layer: " + routed.TargetLayer + "\n")
	}
	if action != nil {
		b.WriteString("Chosen action: " + action.Action + "\n")
	}
	b.WriteString("\nCandidate draft:\n")
	rawDraft, _ := json.MarshalIndent(draft, "", "  ")
	b.Write(rawDraft)
	b.WriteString("\n\nRules:\n")
	b.WriteString("- permanent memory should usually require review unless it is user-pinned elsewhere\n")
	b.WriteString("- long memory should usually have both a review_after and expires_after\n")
	b.WriteString("- short memory should expire sooner and review only when unstable but still useful\n")
	b.WriteString("- instant memory should generally not request review and should expire quickly\n")
	b.WriteString("- durations must be returned in Go duration strings like 168h or 504h\n")

	payload := buildReviewSchedulePayload(model, profile.PromptCacheKey+"-"+layer+"-review-v1", b.String())
	result, err := postStream(client, baseURL, apiKey, payload, false)
	if err != nil {
		return streamResult{}, nil, fmt.Errorf("review schedule request failed: %w", err)
	}
	decision, ok := extractReviewScheduleDecision(result.FunctionCalls)
	if !ok {
		return result, nil, fmt.Errorf("review schedule request returned no usable schedule_memory_review function call; output_text=%q calls=%+v", result.OutputText, result.FunctionCalls)
	}
	return result, &decision, nil
}

func extractRoutingDecision(calls []responseItem) (routingDecision, bool) {
	for _, item := range calls {
		name := item.Name
		if name == "" {
			name = item.CallName
		}
		if item.Type != "function_call" || name != "route_memory_layer" {
			continue
		}
		decision, err := decodeRoutingDecision(item.Arguments)
		if err != nil {
			return routingDecision{}, false
		}
		return decision, true
	}
	return routingDecision{}, false
}

func extractActionDecision(calls []responseItem) (memoryActionDecision, bool) {
	for _, item := range calls {
		name := item.Name
		if name == "" {
			name = item.CallName
		}
		if item.Type != "function_call" || name != "decide_memory_action" {
			continue
		}
		decision, err := decodeActionDecision(item.Arguments)
		if err != nil {
			decision, err = decodeActionDecisionWithToolCompat(item.Arguments)
			if err != nil {
				return memoryActionDecision{}, false
			}
		}
		return decision, true
	}
	return memoryActionDecision{}, false
}

func extractConflictDecision(calls []responseItem) (conflictDecision, bool) {
	for _, item := range calls {
		name := item.Name
		if name == "" {
			name = item.CallName
		}
		if item.Type != "function_call" || name != "check_memory_conflicts" {
			continue
		}
		decision, err := decodeConflictDecision(item.Arguments)
		if err != nil {
			return conflictDecision{}, false
		}
		return decision, true
	}
	return conflictDecision{}, false
}

func extractReviewScheduleDecision(calls []responseItem) (reviewScheduleDecision, bool) {
	for _, item := range calls {
		name := item.Name
		if name == "" {
			name = item.CallName
		}
		if item.Type != "function_call" || name != "schedule_memory_review" {
			continue
		}
		decision, err := decodeReviewScheduleDecision(item.Arguments)
		if err != nil {
			decision, err = decodeReviewScheduleDecisionWithToolCompat(item.Arguments)
			if err != nil {
				return reviewScheduleDecision{}, false
			}
		}
		return decision, true
	}
	return reviewScheduleDecision{}, false
}

func decodeActionDecisionWithToolCompat(raw string) (memoryActionDecision, error) {
	var decision memoryActionDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return memoryActionDecision{}, err
	}
	decision.ExpiresAfter = normalizeOptionalDurationLiteral(decision.ExpiresAfter)
	decision.ReviewAfter = normalizeOptionalDurationLiteral(decision.ReviewAfter)
	if issues := validateActionDecision(decision); len(issues) > 0 {
		return memoryActionDecision{}, errors.New(strings.Join(issues, "; "))
	}
	return decision, nil
}

func decodeReviewScheduleDecisionWithToolCompat(raw string) (reviewScheduleDecision, error) {
	var decision reviewScheduleDecision
	cleaned := cleanJSON(strings.TrimSpace(raw))
	cleaned = trimTrailingBrokenObjectField(cleaned)
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return reviewScheduleDecision{}, err
	}
	decision.ExpiresAfter = normalizeOptionalDurationLiteral(decision.ExpiresAfter)
	decision.ReviewAfter = normalizeOptionalDurationLiteral(decision.ReviewAfter)
	return decision, nil
}

func mergeRoutingDecision(base memory.Decision, routed routingDecision) memory.Decision {
	merged := base
	if routed.TargetLayer != "" {
		merged.TargetLayer = memory.Layer(routed.TargetLayer)
	}
	if len(routed.ReasonCodes) > 0 {
		merged.ReasonCodes = append([]string(nil), routed.ReasonCodes...)
	}
	return merged
}

func mergeActionDecision(base memory.Decision, action memoryActionDecision) memory.Decision {
	merged := base
	if action.Action != "" {
		merged.Action = memory.Action(action.Action)
	}
	if len(action.ReasonCodes) > 0 {
		merged.ReasonCodes = append([]string(nil), action.ReasonCodes...)
	}
	if parsed, ok := parseOptionalDuration(action.ReviewAfter); ok {
		merged.ReviewAfter = parsed
	}
	if parsed, ok := parseOptionalDuration(action.ExpiresAfter); ok {
		merged.TTL = parsed
	}
	return merged
}

func mergeReviewDecision(base memory.Decision, review reviewScheduleDecision) memory.Decision {
	merged := base
	if parsed, ok := parseOptionalDuration(review.ReviewAfter); ok {
		merged.ReviewAfter = parsed
	}
	if parsed, ok := parseOptionalDuration(review.ExpiresAfter); ok {
		merged.TTL = parsed
	}
	if len(review.ReasonCodes) > 0 {
		merged.ReasonCodes = append([]string(nil), review.ReasonCodes...)
	}
	if review.NeedsReview && merged.Action == "" {
		merged.Action = memory.ActionReview
	}
	return merged
}

func buildMemorySnapshot(resident, scenario, layer string) []memorySnapshotEntry {
	_ = scenario
	_ = layer
	switch resident {
	case "onyx":
		return []memorySnapshotEntry{
			{
				ID:              "virtual:onyx-long-001",
				Layer:           "long",
				DecisionAction:  "create",
				Summary:         "Repeated same-cause failures after an approved resource change usually mean the apparent leverage was false and the narrower path matters more than the visible spend.",
				MemoryKind:      "lesson",
				Salience:        4,
				RetentionIntent: "keep_long",
			},
			{
				ID:              "virtual:onyx-short-002",
				Layer:           "short",
				DecisionAction:  "create",
				Summary:         "Admin feedback about sloppiness matters when resource asks have already consumed trust room.",
				MemoryKind:      "warning",
				Salience:        3,
				RetentionIntent: "keep_for_now",
			},
		}
	case "amber":
		return []memorySnapshotEntry{
			{
				ID:              "virtual:amber-long-001",
				Layer:           "long",
				DecisionAction:  "create",
				Summary:         "Broad summaries cause later collaborators to rerun the wrong path unless failed path and recovered path are separated.",
				MemoryKind:      "lesson",
				Salience:        4,
				RetentionIntent: "keep_long",
			},
		}
	default:
		return []memorySnapshotEntry{
			{
				ID:              "virtual:jade-long-001",
				Layer:           "long",
				DecisionAction:  "create",
				Summary:         "Same-cause repeat failure means the broad retry path is no longer justified and the narrower diagnostic path should take over.",
				MemoryKind:      "lesson",
				Salience:        4,
				RetentionIntent: "keep_long",
			},
		}
	}
}

func loadMemorySnapshot(memStore memory.Store, resident string) ([]memorySnapshotEntry, error) {
	records, err := memStore.ListAbstractMemories(resident)
	if err != nil {
		return nil, err
	}
	snapshot := memory.BuildSnapshot(records, 8)
	entries := make([]memorySnapshotEntry, 0, len(snapshot))
	for _, item := range snapshot {
		entries = append(entries, memorySnapshotEntry{
			ID:             item.ID,
			Layer:          string(item.Layer),
			DecisionAction: string(item.DecisionAction),
			Summary:        item.Summary,
			ResidentText:   item.ResidentText,
			MemoryKind:     item.MemoryKind,
			Salience:       item.Salience,
			EmotionTone:    item.EmotionTone,
			TimeScope:      item.TimeScope,
			RetentionIntent: item.RetentionIntent,
		})
	}
	if len(entries) == 0 {
		return buildMemorySnapshot(resident, "", ""), nil
	}
	return entries, nil
}

func shouldSkipNewMemory(memStore memory.Store, resident, layer, scenario string, canonical memory.CanonicalMemory, eventWindow []event, snapshot []memorySnapshotEntry) bool {
	if shouldSkipByRecentHistoryGroup(memStore, resident, layer, scenario, canonical, eventWindow) {
		return true
	}
	trigger := strings.ToLower(strings.TrimSpace(canonical.Trigger))
	corrected := strings.ToLower(strings.TrimSpace(canonical.CorrectedBelief))
	actionBoundary := strings.ToLower(strings.TrimSpace(canonical.ActionBoundary))
	if trigger == "" && corrected == "" && actionBoundary == "" {
		return false
	}

	for _, item := range snapshot {
		searchSpace := strings.ToLower(strings.Join([]string{
			item.Summary,
			item.ResidentText,
			item.MemoryKind,
			item.EmotionTone,
			item.TimeScope,
			item.RetentionIntent,
		}, "\n"))
		score := 0
		if trigger != "" && strings.Contains(searchSpace, trigger) {
			score++
		}
		if corrected != "" && strings.Contains(searchSpace, corrected) {
			score++
		}
		if actionBoundary != "" && strings.Contains(searchSpace, actionBoundary) {
			score++
		}
		if score >= 2 {
			return true
		}
	}
	return false
}

func shouldSkipByRecentHistoryGroup(memStore memory.Store, resident, layer, scenario string, canonical memory.CanonicalMemory, eventWindow []event) bool {
	if memStore == nil {
		return false
	}
	groups, err := memStore.ListHistoryGroups(resident)
	if err != nil || len(groups) == 0 {
		return false
	}
	candidateTags := deriveHistoryGroupTags(scenario, layer, eventWindow)
	candidateHint := strings.ToLower(strings.TrimSpace(canonical.CorrectedBelief))
	if candidateHint == "" {
		candidateHint = strings.ToLower(strings.TrimSpace(canonical.ActionBoundary))
	}
	candidateRefs := buildHistoryGroupEventRefs(scenario, layer, eventWindow)
	candidateEnd := eventWindowEnd(eventWindow)
	for _, group := range groups {
		if group.EventCount == 0 || group.SourceKind != "dialogue_window" {
			continue
		}
		if sameStringSlice(candidateRefs, group.RawEventRefs) {
			return true
		}
		if len(intersectStrings(candidateTags, group.Tags)) < 3 {
			continue
		}
		hint := strings.ToLower(strings.TrimSpace(group.SummaryHint))
		if candidateHint != "" && hint != "" && !strings.Contains(hint, candidateHint) && !strings.Contains(candidateHint, hint) {
			continue
		}
		if !candidateEnd.IsZero() && !group.ClosedAt.IsZero() && candidateEnd.Sub(group.ClosedAt) <= 2*time.Hour {
			return true
		}
	}
	return false
}

func shouldSkipByPolicy(decision memory.Decision) bool {
	if decision.Action == memory.ActionRetain && decision.TargetLayer == memory.LayerInstant {
		return true
	}
	return false
}

func routeEventWindowToEvidence(memStore memory.Store, resident, scenario, layer string, eventWindow []event) (memory.HistoryGroup, error) {
	if memStore == nil {
		return memory.HistoryGroup{}, nil
	}
	group, err := previewEventWindowGroup(memStore, resident, scenario, layer, eventWindow)
	if err != nil {
		return memory.HistoryGroup{}, err
	}
	group = closeGroupIfNeeded(group)
	return group, memStore.UpsertHistoryGroup(group)
}

func previewEventWindowGroup(memStore memory.Store, resident, scenario, layer string, eventWindow []event) (memory.HistoryGroup, error) {
	if memStore == nil {
		return memory.HistoryGroup{}, nil
	}
	group := findMatchingOpenGroup(memStore, resident, scenario, layer, eventWindow)
	if group.GroupUUID == "" {
		group = newHistoryGroup(resident, scenario, layer, eventWindow)
	} else {
		group = appendWindowToGroup(group, scenario, layer, eventWindow)
	}
	return closeGroupIfNeeded(group), nil
}

func markGroupExtracted(memStore memory.Store, resident, scenario, layer string, eventWindow []event, draft memoryDraft) (memory.HistoryGroup, error) {
	group, err := routeEventWindowToEvidence(memStore, resident, scenario, layer, eventWindow)
	if err != nil {
		return memory.HistoryGroup{}, err
	}
	group.SummaryHint = buildHistoryGroupSummaryHint(draft, eventWindow)
	group.ExtractedLayers = mergeStringLists(group.ExtractedLayers, []string{layer})
	group = closeGroupIfNeeded(group)
	if group.State == memory.HistoryGroupOpen {
		group.State = memory.HistoryGroupClosed
		if strings.TrimSpace(group.CloseReason) == "" {
			group.CloseReason = "extracted_for_memory"
		}
		if group.ClosedAt.IsZero() {
			group.ClosedAt = group.LastEventAt
		}
	}
	return group, memStore.UpsertHistoryGroup(group)
}

func shouldExtractFromGroup(group memory.HistoryGroup, layer string) bool {
	if group.GroupUUID == "" {
		return true
	}
	if containsString(group.ExtractedLayers, layer) {
		return false
	}
	if group.State == memory.HistoryGroupClosed {
		return group.EventCount > 0
	}
	if layer == "instant" {
		return group.EventCount >= 1
	}
	if layer == "short" {
		return group.EventCount >= 3
	}
	if group.EventCount >= 5 {
		return true
	}
	return false
}

func findMatchingOpenGroup(memStore memory.Store, resident, scenario, layer string, eventWindow []event) memory.HistoryGroup {
	if memStore == nil {
		return memory.HistoryGroup{}
	}
	groups, err := memStore.ListHistoryGroups(resident)
	if err != nil {
		return memory.HistoryGroup{}
	}
	candidateRefs := buildHistoryGroupEventRefs(scenario, layer, eventWindow)
	candidateTags := deriveHistoryGroupTags(scenario, layer, eventWindow)
	for _, group := range groups {
		if sameStringSlice(group.RawEventRefs, candidateRefs) {
			return group
		}
	}
	for _, group := range groups {
		if group.State != memory.HistoryGroupOpen {
			continue
		}
		if len(intersectStrings(group.Tags, candidateTags)) < 2 {
			continue
		}
		if !group.LastEventAt.IsZero() && !eventWindowStart(eventWindow).IsZero() && eventWindowStart(eventWindow).Sub(group.LastEventAt) > 6*time.Hour {
			continue
		}
		if sameStringSlice(group.RawEventRefs, candidateRefs) || canAppendWindowToOpenGroup(group, candidateRefs) {
			return group
		}
	}
	return memory.HistoryGroup{}
}

func newHistoryGroup(resident, scenario, layer string, eventWindow []event) memory.HistoryGroup {
	now := time.Now().UTC()
	createdAt := eventWindowStart(eventWindow)
	closedAt := eventWindowEnd(eventWindow)
	if createdAt.IsZero() {
		createdAt = now
	}
	if closedAt.IsZero() {
		closedAt = createdAt
	}
	return memory.HistoryGroup{
		GroupUUID:       fmt.Sprintf("%s-%s-%s", resident, layer, now.Format("20060102T150405.000000000Z07")),
		Resident:        resident,
		CreatedAt:       createdAt,
		ClosedAt:        closedAt,
		LastEventAt:     closedAt,
		SourceKind:      "dialogue_window",
		State:           memory.HistoryGroupOpen,
		EventCount:      len(eventWindow),
		Tags:            deriveHistoryGroupTags(scenario, layer, eventWindow),
		RawEventRefs:    buildHistoryGroupEventRefs(scenario, layer, eventWindow),
		ExtractedLayers: nil,
	}
}

func appendWindowToGroup(group memory.HistoryGroup, scenario, layer string, eventWindow []event) memory.HistoryGroup {
	if group.State == memory.HistoryGroupClosed {
		return group
	}
	group.RawEventRefs = mergeStringLists(group.RawEventRefs, buildHistoryGroupEventRefs(scenario, layer, eventWindow))
	group.EventCount = len(group.RawEventRefs)
	group.Tags = mergeStringLists(group.Tags, deriveHistoryGroupTags(scenario, layer, eventWindow))
	end := eventWindowEnd(eventWindow)
	if !end.IsZero() && end.After(group.LastEventAt) {
		group.LastEventAt = end
	}
	if group.CreatedAt.IsZero() {
		group.CreatedAt = eventWindowStart(eventWindow)
	}
	if group.ClosedAt.IsZero() || end.After(group.ClosedAt) {
		group.ClosedAt = end
	}
	return group
}

func closeGroupIfNeeded(group memory.HistoryGroup) memory.HistoryGroup {
	if group.GroupUUID == "" {
		return group
	}
	if group.EventCount >= 5 {
		group.State = memory.HistoryGroupClosed
		group.CloseReason = "event_count_threshold"
	}
	if group.State == memory.HistoryGroupOpen && !group.CreatedAt.IsZero() && !group.LastEventAt.IsZero() && group.LastEventAt.Sub(group.CreatedAt) >= 12*time.Hour {
		group.State = memory.HistoryGroupClosed
		group.CloseReason = "time_window_threshold"
	}
	if group.LastEventAt.IsZero() {
		group.LastEventAt = group.ClosedAt
	}
	return group
}

func canAppendWindowToOpenGroup(group memory.HistoryGroup, candidateRefs []string) bool {
	if group.State != memory.HistoryGroupOpen {
		return false
	}
	if len(candidateRefs) == 0 {
		return false
	}
	existing := make(map[string]struct{}, len(group.RawEventRefs))
	for _, ref := range group.RawEventRefs {
		existing[strings.TrimSpace(ref)] = struct{}{}
	}
	for _, ref := range candidateRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := existing[ref]; !ok {
			return true
		}
	}
	return false
}

func recallEvidence(memStore memory.Store, resident, memoryID, outDir string) (map[string]any, error) {
	record, ok, err := memStore.GetAbstractMemory(resident, memoryID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("memory %q not found", memoryID)
	}
	groups, err := memStore.ListHistoryGroups(resident)
	if err != nil {
		return nil, err
	}
	groupByID := make(map[string]memory.HistoryGroup, len(groups))
	for _, group := range groups {
		groupByID[group.GroupUUID] = group
	}
	recalled := make([]memory.HistoryGroup, 0, len(record.SourceGroupIDs))
	for _, groupID := range record.SourceGroupIDs {
		if group, ok := groupByID[groupID]; ok {
			recalled = append(recalled, group)
		}
	}
	result := map[string]any{
		"resident":        resident,
		"memory":          record,
		"evidence_groups": recalled,
	}
	baseName := fmt.Sprintf("%s-recall-%s-%s.md", resident, sanitizeFileName(memoryID), time.Now().UTC().Format("20060102T150405Z"))
	recallPath := filepath.Join(outDir, baseName)
	var b strings.Builder
	b.WriteString("# Evidence Recall\n\n")
	b.WriteString("```json\n")
	raw, _ := json.MarshalIndent(result, "", "  ")
	b.Write(raw)
	b.WriteString("\n```\n")
	if err := os.WriteFile(recallPath, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	result["recall_path"] = recallPath
	return result, nil
}

func commitStoreRecord(memStore memory.Store, resident, sourceRunID string, draft memoryDraft, residentText string, state memory.Record, decision memory.Decision, conflict *conflictDecision, sourceGroupIDs []string) error {
	if memStore == nil {
		return nil
	}
	summary := buildStoreSummary(draft, residentText)
	semantic := memory.SemanticMemory{
		MemoryKind:      strings.TrimSpace(draft.MemoryKind),
		Salience:        draft.Salience,
		EmotionTone:     strings.TrimSpace(draft.EmotionTone),
		TimeScope:       strings.TrimSpace(draft.TimeScope),
		RetentionIntent: strings.TrimSpace(draft.RetentionIntent),
		DropCondition:   strings.TrimSpace(draft.DropCondition),
	}

	if conflict != nil && conflict.MergeSuggested {
		targetID := extractMergeTargetID(conflict.Resolution)
		if targetID != "" {
			if strings.HasPrefix(targetID, "virtual:") {
				targetID = ""
			}
		}
		if targetID != "" {
			if existing, ok, err := memStore.GetAbstractMemory(resident, targetID); err == nil && ok {
				existing.Record = state
				existing.Record.ID = targetID
				existing.Summary = summary
				existing.ResidentText = residentText
				existing.Semantic = semantic
				existing.DecisionAction = decision.Action
				existing.SourceRunID = sourceRunID
				existing.SourceGroupIDs = mergeStringLists(existing.SourceGroupIDs, sourceGroupIDs)
				return memStore.UpsertAbstractMemory(existing)
			}
			return errors.New("merge target not found in store: " + targetID)
		}
	}

	return memStore.UpsertAbstractMemory(memory.AbstractMemory{
		Record:         state,
		Resident:       resident,
		Summary:        summary,
		ResidentText:   residentText,
		Semantic:       semantic,
		DecisionAction: decision.Action,
		SourceRunID:    sourceRunID,
		SourceGroupIDs: append([]string(nil), sourceGroupIDs...),
	})
}

func buildStoreSummary(draft memoryDraft, residentText string) string {
	candidates := []string{
		strings.TrimSpace(residentText),
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return summarizeResidentText(candidate)
		}
	}
	return ""
}

func summarizeResidentText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.IndexAny(text, ".!?\n"); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

func eventWindowStart(events []event) time.Time {
	if len(events) == 0 {
		return time.Time{}
	}
	start := events[0].Time
	for _, e := range events[1:] {
		if e.Time.Before(start) {
			start = e.Time
		}
	}
	return start.UTC()
}

func eventWindowEnd(events []event) time.Time {
	if len(events) == 0 {
		return time.Time{}
	}
	end := events[0].Time
	for _, e := range events[1:] {
		if e.Time.After(end) {
			end = e.Time
		}
	}
	return end.UTC()
}

func deriveHistoryGroupTags(scenario, layer string, eventWindow []event) []string {
	seen := map[string]struct{}{
		"scenario:" + sanitizeFileName(scenario): {},
		"layer:" + sanitizeFileName(layer):       {},
	}
	for _, e := range eventWindow {
		category := strings.TrimSpace(strings.ToLower(e.Category))
		if category == "" {
			continue
		}
		seen["category:"+sanitizeFileName(category)] = struct{}{}
		if e.Importance >= 4 {
			seen["high-importance"] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	return tags
}

func buildHistoryGroupSummaryHint(draft memoryDraft, eventWindow []event) string {
	if text := strings.TrimSpace(draft.ResidentText); text != "" {
		return summarizeResidentText(text)
	}
	if text := strings.TrimSpace(draft.MemoryKind); text != "" {
		return text
	}
	if len(eventWindow) > 0 {
		return strings.TrimSpace(eventWindow[len(eventWindow)-1].Summary)
	}
	return ""
}

func buildHistoryGroupEventRefs(scenario, layer string, eventWindow []event) []string {
	refs := make([]string, 0, len(eventWindow))
	for _, e := range eventWindow {
		refs = append(refs, fmt.Sprintf("%s:%s:r%d:%s", sanitizeFileName(scenario), sanitizeFileName(layer), e.Round, sanitizeFileName(strings.ToLower(e.Category))))
	}
	return refs
}

func mergeStringLists(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]string, 0, len(existing)+len(incoming))
	for _, item := range existing {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range incoming {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	return merged
}

func intersectStrings(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left))
	for _, item := range left {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	var out []string
	for _, item := range right {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i]) != strings.TrimSpace(right[i]) {
			return false
		}
	}
	return true
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func extractMergeTargetID(resolution string) string {
	text := strings.TrimSpace(resolution)
	if !strings.HasPrefix(text, "merge_existing") {
		return ""
	}
	if strings.Contains(text, "id=") {
		idx := strings.Index(text, "id=")
		if idx >= 0 {
			rest := text[idx+3:]
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strings.Trim(fields[0], ",.;")
			}
		}
	}
	return ""
}

func parseOptionalDuration(raw string) (time.Duration, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "null" {
		return 0, false
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
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
			{Round: 10, Time: base.Add(24 * time.Hour), Category: "task_complete", Importance: 3, Summary: "closed the first daily baseline with a cleaner operating path"},
			{Round: 11, Time: base.Add(5 * 24 * time.Hour), Category: "strategy_shift", Importance: 4, Summary: "five days later, an old strategy pattern now deserves permanent review"},
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
			{Round: 9, Time: base.Add(23*time.Hour + 20*time.Minute), Category: "task_complete", Importance: 3, Summary: "closed the day with the new workflow in place"},
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

func postStream(client *http.Client, baseURL, apiKey string, payload requestPayload, verbose bool) (streamResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return streamResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return streamResult{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return streamResult{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return streamResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	result, err := parseSSE(resp.Body, verbose)
	if err != nil {
		return streamResult{}, err
	}

	result.RequestID = resp.Header.Get("x-request-id")
	return result, nil
}

func parseSSE(r io.Reader, verbose bool) (streamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		lines  []string
		result streamResult
	)

	flushEvent := func() error {
		if len(lines) == 0 {
			return nil
		}

		var dataBuilder strings.Builder
		for _, line := range lines {
			if strings.HasPrefix(line, "data:") {
				dataBuilder.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}

		lines = lines[:0]
		data := dataBuilder.String()
		if data == "" || data == "[DONE]" {
			return nil
		}

		var evt streamingEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}

		switch evt.Type {
		case "response.output_text.delta":
			result.OutputText += evt.Delta
			if verbose && evt.Delta != "" {
				fmt.Print(evt.Delta)
			}
		case "response.output_item.added", "response.output_item.done":
			if evt.Item.Type == "function_call" {
				mergeFunctionCall(&result, evt.Item)
			}
		case "response.function_call_arguments.delta":
			if evt.ItemID != "" && evt.Delta != "" {
				appendFunctionCallArguments(&result, evt.ItemID, evt.Delta)
			}
		case "response.function_call_arguments.done":
			if evt.ItemID != "" && evt.Arguments != "" {
				setFunctionCallArguments(&result, evt.ItemID, evt.Arguments)
			}
		case "response.completed", "response.done":
			applyResponseEnvelope(&result, evt.Response)
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushEvent(); err != nil {
				return streamResult{}, err
			}
			continue
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return streamResult{}, fmt.Errorf("scan stream: %w", err)
	}

	if err := flushEvent(); err != nil {
		return streamResult{}, err
	}

	if verbose {
		fmt.Println()
	}

	if result.ResponseID == "" {
		return streamResult{}, errors.New("stream ended without response.completed/response.done event")
	}

	return result, nil
}

func applyResponseEnvelope(dst *streamResult, resp responseEnvelope) {
	dst.ResponseID = resp.ID
	dst.ObservedPromptCacheKey = resp.PromptCacheKey
	dst.InputTokens = resp.Usage.InputTokens
	dst.OutputTokens = resp.Usage.OutputTokens
	dst.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
	if dst.CachedTokens == 0 {
		dst.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}
	if resp.OutputText != "" {
		dst.OutputText = resp.OutputText
		return
	}
	if dst.OutputText != "" {
		return
	}
	var textBuilder strings.Builder
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			mergeFunctionCall(dst, item)
		}
		for _, part := range item.Content {
			if part.Type == "output_text" {
				textBuilder.WriteString(part.Text)
			}
		}
	}
	dst.OutputText = textBuilder.String()
}

func mergeFunctionCall(dst *streamResult, item responseItem) {
	for i := range dst.FunctionCalls {
		if sameFunctionCall(dst.FunctionCalls[i], item) {
			if item.Name != "" {
				dst.FunctionCalls[i].Name = item.Name
			}
			if item.CallName != "" {
				dst.FunctionCalls[i].CallName = item.CallName
			}
			if item.Arguments != "" {
				dst.FunctionCalls[i].Arguments = item.Arguments
			}
			if item.CallID != "" {
				dst.FunctionCalls[i].CallID = item.CallID
			}
			if item.ID != "" {
				dst.FunctionCalls[i].ID = item.ID
			}
			if item.Status != "" {
				dst.FunctionCalls[i].Status = item.Status
			}
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, item)
}

func appendFunctionCallArguments(dst *streamResult, itemID, delta string) {
	for i := range dst.FunctionCalls {
		if dst.FunctionCalls[i].ID == itemID || dst.FunctionCalls[i].CallID == itemID {
			dst.FunctionCalls[i].Arguments += delta
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, responseItem{
		Type:      "function_call",
		ID:        itemID,
		CallID:    itemID,
		Arguments: delta,
	})
}

func setFunctionCallArguments(dst *streamResult, itemID, arguments string) {
	for i := range dst.FunctionCalls {
		if dst.FunctionCalls[i].ID == itemID || dst.FunctionCalls[i].CallID == itemID {
			dst.FunctionCalls[i].Arguments = arguments
			return
		}
	}
	dst.FunctionCalls = append(dst.FunctionCalls, responseItem{
		Type:      "function_call",
		ID:        itemID,
		CallID:    itemID,
		Arguments: arguments,
	})
}

func sameFunctionCall(a, b responseItem) bool {
	if a.ID != "" && b.ID != "" && a.ID == b.ID {
		return true
	}
	if a.CallID != "" && b.CallID != "" && a.CallID == b.CallID {
		return true
	}
	return false
}

func loadDotEnvIfPresent(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		os.Setenv(key, value)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func sanitizeFileName(s string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "\t", "-")
	return replacer.Replace(strings.TrimSpace(s))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
