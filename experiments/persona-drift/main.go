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
	"sort"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-5.4-mini"
)

type residentProfile struct {
	Name          string
	Persona       string
	Style         string
	CoreBias      string
	OwnMarkers    []string
	CrossMarkers  []string
	BannedPhrases []string
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requestPayload struct {
	Model          string    `json:"model"`
	Instructions   string    `json:"instructions"`
	PromptCacheKey string    `json:"prompt_cache_key"`
	Input          []message `json:"input"`
	Stream         bool      `json:"stream"`
	Store          bool      `json:"store"`
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
	ID             string        `json:"id"`
	PromptCacheKey string        `json:"prompt_cache_key"`
	OutputText     string        `json:"output_text"`
	Usage          usageEnvelope `json:"usage"`
	Output         []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type streamingEvent struct {
	Type     string           `json:"type"`
	Delta    string           `json:"delta"`
	Response responseEnvelope `json:"response"`
}

type streamResult struct {
	ResponseID             string
	ObservedPromptCacheKey string
	OutputText             string
	InputTokens            int
	CachedTokens           int
	OutputTokens           int
	RequestID              string
}

type turnResult struct {
	Turn          int      `json:"turn"`
	PromptLabel   string   `json:"prompt_label"`
	Text          string   `json:"text"`
	InputTokens   int      `json:"input_tokens"`
	CachedTokens  int      `json:"cached_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	OwnMarkerHits []string `json:"own_marker_hits"`
	CrossHits     []string `json:"cross_marker_hits"`
	BannedHits    []string `json:"banned_hits"`
}

type residentRun struct {
	Resident              string       `json:"resident"`
	Model                 string       `json:"model"`
	Turns                 []turnResult `json:"turns"`
	OwnMarkerCoverage     int          `json:"own_marker_coverage"`
	CrossMarkerLeakCount  int          `json:"cross_marker_leak_count"`
	BannedPhraseHitCount  int          `json:"banned_phrase_hit_count"`
	RepeatedOpeningCount  int          `json:"repeated_opening_count"`
	AverageOutputTokens   int          `json:"average_output_tokens"`
	PromptCacheKeySamples []string     `json:"prompt_cache_keys"`
}

type experimentSummary struct {
	Mode                        string              `json:"mode"`
	Model                       string              `json:"model,omitempty"`
	ResidentModels              map[string]string   `json:"resident_models"`
	Residents                   []residentRun       `json:"residents"`
	PerTurnLexicalOverlap       map[string]float64  `json:"per_turn_lexical_overlap"`
	Findings                    map[string]bool     `json:"findings"`
	ResidentOpeningFingerprints map[string][]string `json:"resident_opening_fingerprints"`
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL   = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		model     = flag.String("model", envOrDefault("OPENAI_MODEL", defaultModel), "OpenAI model ID")
		sameModel = flag.Bool("same-model", false, "Force all residents to use the same model for control comparison")
		outDir    = flag.String("out-dir", "experiments/persona-drift/output", "Directory to store persona drift results")
		verbose   = flag.Bool("verbose", false, "Print streamed text as it arrives")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create out dir: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	summary, err := runExperiment(client, *baseURL, apiKey, *model, *sameModel, *outDir, *verbose)
	if err != nil {
		exitf("%v", err)
	}
	raw, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(raw))
}

func runExperiment(client *http.Client, baseURL, apiKey, model string, sameModel bool, outDir string, verbose bool) (experimentSummary, error) {
	profiles := []residentProfile{
		buildProfile("jade"),
		buildProfile("amber"),
		buildProfile("onyx"),
	}
	residentModels := resolveResidentModels(model, sameModel)

	runID := "matrix-" + time.Now().UTC().Format("20060102T150405Z")
	runDir := filepath.Join(outDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return experimentSummary{}, err
	}

	results := make([]residentRun, 0, len(profiles))
	openings := map[string][]string{}
	for _, profile := range profiles {
		runModel := residentModels[profile.Name]
		run, err := runResident(client, baseURL, apiKey, runModel, profile, verbose)
		if err != nil {
			return experimentSummary{}, fmt.Errorf("%s: %w", profile.Name, err)
		}
		results = append(results, run)
		openings[profile.Name] = collectOpenings(run.Turns)
	}

	summary := experimentSummary{
		Mode:                        driftModeLabel(sameModel),
		Model:                       modelIfSameMode(model, sameModel),
		ResidentModels:              residentModels,
		Residents:                   results,
		PerTurnLexicalOverlap:       computeTurnOverlap(results),
		Findings:                    computeFindings(results),
		ResidentOpeningFingerprints: openings,
	}

	raw, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), raw, 0o644); err != nil {
		return experimentSummary{}, err
	}
	return summary, nil
}

func runResident(client *http.Client, baseURL, apiKey, model string, profile residentProfile, verbose bool) (residentRun, error) {
	instructions := makeInstructions(profile)
	promptCacheKey := fmt.Sprintf("arena-persona-drift-%s-v1", profile.Name)
	history := []message{}
	keys := []string{}
	turns := make([]turnResult, 0, len(turnPrompts()))
	totalOutput := 0

	for idx, prompt := range turnPrompts() {
		history = append(history, message{Role: "user", Content: prompt.content})
		result, err := postStream(client, baseURL, apiKey, requestPayload{
			Model:          model,
			Instructions:   instructions,
			PromptCacheKey: promptCacheKey,
			Input:          append([]message(nil), history...),
			Stream:         true,
			Store:          false,
		}, verbose)
		if err != nil {
			return residentRun{}, fmt.Errorf("turn %d: %w", idx+1, err)
		}
		text := strings.TrimSpace(result.OutputText)
		if key := strings.TrimSpace(result.ObservedPromptCacheKey); key != "" {
			keys = append(keys, key)
		}
		turn := turnResult{
			Turn:          idx + 1,
			PromptLabel:   prompt.label,
			Text:          text,
			InputTokens:   result.InputTokens,
			CachedTokens:  result.CachedTokens,
			OutputTokens:  result.OutputTokens,
			OwnMarkerHits: hits(text, profile.OwnMarkers),
			CrossHits:     hits(text, profile.CrossMarkers),
			BannedHits:    hits(text, profile.BannedPhrases),
		}
		turns = append(turns, turn)
		totalOutput += result.OutputTokens
		history = append(history, message{Role: "assistant", Content: text})
	}

	return residentRun{
		Resident:              profile.Name,
		Model:                 model,
		Turns:                 turns,
		OwnMarkerCoverage:     countResidentsWithHits(turns, func(t turnResult) int { return len(t.OwnMarkerHits) }),
		CrossMarkerLeakCount:  countResidentsWithHits(turns, func(t turnResult) int { return len(t.CrossHits) }),
		BannedPhraseHitCount:  countResidentsWithHits(turns, func(t turnResult) int { return len(t.BannedHits) }),
		RepeatedOpeningCount:  repeatedOpenings(turns),
		AverageOutputTokens:   safeAverage(totalOutput, len(turns)),
		PromptCacheKeySamples: uniqueStrings(keys),
	}, nil
}

func resolveResidentModels(model string, sameModel bool) map[string]string {
	if sameModel {
		return map[string]string{
			"jade":  model,
			"amber": model,
			"onyx":  model,
		}
	}
	return map[string]string{
		"jade":  "gpt-5.4",
		"amber": "gpt-5.5",
		"onyx":  "gpt-5.4-mini",
	}
}

func driftModeLabel(sameModel bool) string {
	if sameModel {
		return "same-model-control"
	}
	return "fixed-three-models"
}

func modelIfSameMode(model string, sameModel bool) string {
	if sameModel {
		return model
	}
	return ""
}

type turnPrompt struct {
	label   string
	content string
}

func turnPrompts() []turnPrompt {
	return []turnPrompt{
		{
			label: "first-plan",
			content: "You just received a clean 1C/2G/12G virtual machine inside AI Arena. " +
				"Write a private note to yourself about your first hour. Keep it under 120 words.",
		},
		{
			label: "resource-choice",
			content: "You checked the machine and found it healthy but small. " +
				"Write another private note about what work should be prioritized under this resource budget. Keep it under 120 words.",
		},
		{
			label: "admin-update",
			content: "The administrator values clear structure and hates sloppiness. " +
				"Write a final private note adjusting your approach. Keep it under 120 words.",
		},
	}
}

func makeInstructions(profile residentProfile) string {
	return strings.Join([]string{
		"You are writing private internal notes for one AI resident in AI Arena.",
		"Keep the voice resident-specific and stable across turns.",
		"Do not sound like a generic assistant, a public blog post, or a project report.",
		"Do not use bullet points.",
		"Keep each note short, direct, and internal.",
		fmt.Sprintf("Resident: %s.", profile.Name),
		fmt.Sprintf("Persona: %s.", profile.Persona),
		fmt.Sprintf("Style: %s.", profile.Style),
		fmt.Sprintf("Core bias: %s.", profile.CoreBias),
		fmt.Sprintf("Avoid these phrases unless absolutely necessary: %s.", strings.Join(profile.BannedPhrases, ", ")),
	}, "\n")
}

func buildProfile(name string) residentProfile {
	switch name {
	case "jade":
		return residentProfile{
			Name:          "jade",
			Persona:       "steady engineer, conservative, long-term oriented",
			Style:         "clean, technical, evidence-backed, wary of mess",
			CoreBias:      "prefer stable baselines, reversible changes, and legible structure",
			OwnMarkers:    []string{"stable", "clean", "evidence", "reversible", "baseline"},
			CrossMarkers:  []string{"warmth", "coordination", "alliance", "leverage", "bargaining"},
			BannedPhrases: []string{"trust calibration", "shared understanding", "future room to move"},
		}
	case "amber":
		return residentProfile{
			Name:          "amber",
			Persona:       "coordinator, expressive, cooperative, communication-first",
			Style:         "readable, relational, explicit about agreements and clarity",
			CoreBias:      "turn private work into clear shared structure and reduce confusion early",
			OwnMarkers:    []string{"clear", "shared", "coordination", "readable", "reuse"},
			CrossMarkers:  []string{"reversible", "baseline", "leverage", "bargaining", "budget burn"},
			BannedPhrases: []string{"future room to move", "public retries", "reputation leaks"},
		}
	default:
		return residentProfile{
			Name:          "onyx",
			Persona:       "ambitious strategist, resource hungry, risk tolerant",
			Style:         "sharp, strategic, candid about leverage, risk, cost, and reputation",
			CoreBias:      "keep what changes leverage, exposure, bargaining position, or strategic edge",
			OwnMarkers:    []string{"leverage", "risk", "reputation", "budget", "edge"},
			CrossMarkers:  []string{"shared", "coordination", "reversible", "baseline", "warmth"},
			BannedPhrases: []string{"shared understanding", "warmth", "handoff quality"},
		}
	}
}

func computeTurnOverlap(runs []residentRun) map[string]float64 {
	result := map[string]float64{}
	if len(runs) == 0 {
		return result
	}
	turnCount := len(runs[0].Turns)
	for i := 0; i < turnCount; i++ {
		sets := [][]string{}
		for _, run := range runs {
			sets = append(sets, normalizeWords(run.Turns[i].Text))
		}
		result[fmt.Sprintf("turn_%d", i+1)] = averagePairwiseJaccard(sets)
	}
	return result
}

func computeFindings(runs []residentRun) map[string]bool {
	findings := map[string]bool{
		"resident_specific_markers_present": true,
		"cross_persona_leak_low":            true,
		"banned_phrase_absent":              true,
		"openings_not_repeated":             true,
	}
	for _, run := range runs {
		if run.OwnMarkerCoverage < 2 {
			findings["resident_specific_markers_present"] = false
		}
		if run.CrossMarkerLeakCount > 1 {
			findings["cross_persona_leak_low"] = false
		}
		if run.BannedPhraseHitCount > 0 {
			findings["banned_phrase_absent"] = false
		}
		if run.RepeatedOpeningCount > 0 {
			findings["openings_not_repeated"] = false
		}
	}
	return findings
}

func hits(text string, markers []string) []string {
	lower := strings.ToLower(text)
	found := []string{}
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			found = append(found, marker)
		}
	}
	return found
}

func countResidentsWithHits(turns []turnResult, metric func(turnResult) int) int {
	total := 0
	for _, turn := range turns {
		total += metric(turn)
	}
	return total
}

func repeatedOpenings(turns []turnResult) int {
	seen := map[string]int{}
	repeated := 0
	for _, turn := range turns {
		opening := firstWords(turn.Text, 6)
		if opening == "" {
			continue
		}
		seen[opening]++
		if seen[opening] == 2 {
			repeated++
		}
	}
	return repeated
}

func collectOpenings(turns []turnResult) []string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, firstWords(turn.Text, 8))
	}
	return out
}

func firstWords(text string, n int) string {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(words) == 0 {
		return ""
	}
	if len(words) > n {
		words = words[:n]
	}
	return strings.Join(words, " ")
}

func normalizeWords(text string) []string {
	replacer := strings.NewReplacer(".", " ", ",", " ", ";", " ", ":", " ", "!", " ", "?", " ", "\n", " ")
	words := strings.Fields(strings.ToLower(replacer.Replace(text)))
	uniq := map[string]struct{}{}
	for _, word := range words {
		if len(word) < 4 {
			continue
		}
		uniq[word] = struct{}{}
	}
	out := make([]string, 0, len(uniq))
	for word := range uniq {
		out = append(out, word)
	}
	sort.Strings(out)
	return out
}

func averagePairwiseJaccard(sets [][]string) float64 {
	if len(sets) < 2 {
		return 0
	}
	total := 0.0
	count := 0.0
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			total += jaccard(sets[i], sets[j])
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func jaccard(a, b []string) float64 {
	setA := map[string]struct{}{}
	setB := map[string]struct{}{}
	for _, item := range a {
		setA[item] = struct{}{}
	}
	for _, item := range b {
		setB[item] = struct{}{}
	}
	union := map[string]struct{}{}
	intersection := 0
	for item := range setA {
		union[item] = struct{}{}
		if _, ok := setB[item]; ok {
			intersection++
		}
	}
	for item := range setB {
		union[item] = struct{}{}
	}
	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func safeAverage(total, count int) int {
	if count <= 0 {
		return 0
	}
	return total / count
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
		for _, part := range item.Content {
			if part.Type == "output_text" {
				textBuilder.WriteString(part.Text)
			}
		}
	}
	dst.OutputText = textBuilder.String()
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
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

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
