package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-5.4-mini"
)

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

type streamResult struct {
	ResponseID             string
	ObservedPromptCacheKey string
	OutputText             string
	InputTokens            int
	CachedTokens           int
	OutputTokens           int
	RequestID              string
}

type turnLog struct {
	Turn                   int    `json:"turn"`
	Model                  string `json:"model"`
	ResponseID             string `json:"response_id"`
	RequestID              string `json:"x_request_id"`
	PromptCacheKeySent     string `json:"prompt_cache_key_sent"`
	PromptCacheKeyObserved string `json:"prompt_cache_key_observed"`
	InstructionsHash       string `json:"instructions_hash"`
	InputPrefixHash        string `json:"input_prefix_hash"`
	InputMessageCount      int    `json:"input_message_count"`
	InputTokens            int    `json:"input_tokens"`
	CachedTokens           int    `json:"cached_tokens"`
	OutputTokens           int    `json:"output_tokens"`
	DurationMS             int64  `json:"duration_ms"`
	Text                   string `json:"text"`
}

type runSummary struct {
	Model                      string   `json:"model"`
	Turns                      int      `json:"turns"`
	CacheHitTurns              int      `json:"cache_hit_turns"`
	CacheMissTurns             int      `json:"cache_miss_turns"`
	TotalInputTokens           int      `json:"total_input_tokens"`
	TotalCachedTokens          int      `json:"total_cached_tokens"`
	TotalOutputTokens          int      `json:"total_output_tokens"`
	AverageDurationMS          int64    `json:"average_duration_ms"`
	ObservedPromptCacheKeys    []string `json:"observed_prompt_cache_keys"`
	InstructionsHashStable     bool     `json:"instructions_hash_stable"`
	InputPrefixHashStableTurns int      `json:"input_prefix_hash_stable_turns"`
	LogPath                    string   `json:"log_path"`
	SummaryPath                string   `json:"summary_path"`
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

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		model   = flag.String("model", envOrDefault("OPENAI_MODEL", defaultModel), "Single OpenAI model ID")
		models  = flag.String("models", envOrDefault("OPENAI_MODELS", ""), "Comma-separated OpenAI model IDs")
		turns   = flag.Int("turns", envOrDefaultInt("OPENAI_CACHE_TURNS", 5), "Number of replay turns")
		verbose = flag.Bool("verbose", false, "Print streamed text as it arrives")
		outDir  = flag.String("out-dir", "experiments/openai-cache/output", "Directory to store run logs and summaries")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	modelList := resolveModels(*model, *models)
	if len(modelList) == 0 {
		exitf("no models resolved")
	}

	summaries := make([]runSummary, 0, len(modelList))
	for _, modelID := range modelList {
		summary, err := runExperiment(client, *baseURL, apiKey, modelID, *turns, *verbose, *outDir)
		if err != nil {
			exitf("model %s failed: %v", modelID, err)
		}
		summaries = append(summaries, summary)
	}
	if len(summaries) > 1 {
		indexPath, err := writeRunIndex(*outDir, summaries)
		if err != nil {
			exitf("write run index: %v", err)
		}
		fmt.Printf("run_index=%s\n", indexPath)
	}
}

func runExperiment(client *http.Client, baseURL, apiKey, model string, turns int, verbose bool, outDir string) (runSummary, error) {
	instructions := makeInstructions()
	promptCacheKey := fmt.Sprintf("arena-openai-cache-%s-v1", model)
	history := make([]message, 0, turns*2)
	runID := fmt.Sprintf("%s-%s", sanitizeFileName(model), time.Now().UTC().Format("20060102T150405Z"))
	runDir := filepath.Join(outDir, runID)
	logDir := filepath.Join(runDir, "logs")

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return runSummary{}, fmt.Errorf("create run log dir: %w", err)
	}

	logPath := filepath.Join(logDir, "turns.jsonl")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return runSummary{}, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	fmt.Printf("== model: %s ==\n", model)
	fmt.Printf("log_file=%s\n", logPath)

	turnLogs := make([]turnLog, 0, turns)
	observedCacheKeys := make([]string, 0, turns)
	seenCacheKeys := make(map[string]struct{}, turns)
	var (
		totalInputTokens    int
		totalCachedTokens   int
		totalOutputTokens   int
		totalDurationMS     int64
		cacheHitTurns       int
		stablePrefixMatches int
		firstInstructions   string
	)

	for turn := 1; turn <= turns; turn++ {
		history = append(history, message{
			Role:    "user",
			Content: makeHeavyUserTurn(turn),
		})

		payload := requestPayload{
			Model:          model,
			Instructions:   instructions,
			PromptCacheKey: promptCacheKey,
			Input:          append([]message(nil), history...),
			Stream:         true,
			Store:          false,
		}

		started := time.Now()
		result, err := postStream(client, baseURL, apiKey, payload, verbose)
		if err != nil {
			return runSummary{}, fmt.Errorf("turn %d: %w", turn, err)
		}

		duration := time.Since(started).Milliseconds()
		prefixHash := hashPrefix(instructions, payload.Input)
		instructionsHash := sha256Hex(instructions)

		logLine := turnLog{
			Turn:                   turn,
			Model:                  model,
			ResponseID:             result.ResponseID,
			RequestID:              result.RequestID,
			PromptCacheKeySent:     promptCacheKey,
			PromptCacheKeyObserved: result.ObservedPromptCacheKey,
			InstructionsHash:       instructionsHash,
			InputPrefixHash:        prefixHash,
			InputMessageCount:      len(payload.Input),
			InputTokens:            result.InputTokens,
			CachedTokens:           result.CachedTokens,
			OutputTokens:           result.OutputTokens,
			DurationMS:             duration,
			Text:                   result.OutputText,
		}

		out, _ := json.Marshal(logLine)
		fmt.Println(string(out))
		if _, err := logFile.Write(append(out, '\n')); err != nil {
			return runSummary{}, fmt.Errorf("write log file: %w", err)
		}
		turnLogs = append(turnLogs, logLine)

		totalInputTokens += result.InputTokens
		totalCachedTokens += result.CachedTokens
		totalOutputTokens += result.OutputTokens
		totalDurationMS += duration
		if result.CachedTokens > 0 {
			cacheHitTurns++
		}
		if firstInstructions == "" {
			firstInstructions = instructionsHash
		}
		if firstInstructions == instructionsHash {
			stablePrefixMatches++
		}
		if key := strings.TrimSpace(result.ObservedPromptCacheKey); key != "" {
			if _, ok := seenCacheKeys[key]; !ok {
				seenCacheKeys[key] = struct{}{}
				observedCacheKeys = append(observedCacheKeys, key)
			}
		}

		history = append(history, message{
			Role:    "assistant",
			Content: result.OutputText,
		})
	}

	summary := runSummary{
		Model:                      model,
		Turns:                      turns,
		CacheHitTurns:              cacheHitTurns,
		CacheMissTurns:             turns - cacheHitTurns,
		TotalInputTokens:           totalInputTokens,
		TotalCachedTokens:          totalCachedTokens,
		TotalOutputTokens:          totalOutputTokens,
		AverageDurationMS:          safeAverage(totalDurationMS, turns),
		ObservedPromptCacheKeys:    observedCacheKeys,
		InstructionsHashStable:     stablePrefixMatches == turns,
		InputPrefixHashStableTurns: countStablePrefixTransitions(turnLogs),
		LogPath:                    logPath,
		SummaryPath:                filepath.Join(runDir, "summary.json"),
	}
	summaryRaw, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summary.SummaryPath, summaryRaw, 0o644); err != nil {
		return runSummary{}, fmt.Errorf("write summary: %w", err)
	}
	fmt.Printf("summary_file=%s\n", summary.SummaryPath)
	fmt.Println(string(summaryRaw))

	return summary, nil
}

func countStablePrefixTransitions(turns []turnLog) int {
	if len(turns) == 0 {
		return 0
	}
	stable := 1
	last := turns[0].InstructionsHash
	for _, turn := range turns[1:] {
		if turn.InstructionsHash == last {
			stable++
		}
		last = turn.InstructionsHash
	}
	return stable
}

func safeAverage(total int64, count int) int64 {
	if count <= 0 {
		return 0
	}
	return total / int64(count)
}

func writeRunIndex(outDir string, summaries []runSummary) (string, error) {
	indexPath := filepath.Join(outDir, "latest-index.json")
	raw, err := json.MarshalIndent(map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"runs":         summaries,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(indexPath, raw, 0o644); err != nil {
		return "", err
	}
	return indexPath, nil
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

func makeInstructions() string {
	const prefixLine = "Arena cache probe prefix: keep this prefix byte-stable across turns so prompt caching can match the leading token window. "

	return strings.Repeat(prefixLine, 320) +
		"\nScenario marker: arena-openai-cache-v1.\n" +
		"You are running a cache experiment. Reply in one short sentence only."
}

func makeHeavyUserTurn(turn int) string {
	parts := make([]string, 0, 57)
	parts = append(parts, fmt.Sprintf("Heavy turn %d.", turn))

	for i := 1; i <= 55; i++ {
		parts = append(parts,
			fmt.Sprintf("Turn %d payload block %d: keep this dialogue body byte-stable inside the repeated transcript except for the turn marker and the closing instruction.", turn, i),
		)
	}

	parts = append(parts, fmt.Sprintf("Closing instruction for turn %d: answer with the exact digit %d only.", turn, turn))
	return strings.Join(parts, "\n")
}

func hashPrefix(instructions string, input []message) string {
	prefix := struct {
		Instructions string    `json:"instructions"`
		Input        []message `json:"input"`
	}{
		Instructions: instructions,
		Input:        input,
	}

	raw, _ := json.Marshal(prefix)
	return sha256Hex(string(raw))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func resolveModels(singleModel, multiModels string) []string {
	if strings.TrimSpace(multiModels) == "" {
		if strings.TrimSpace(singleModel) == "" {
			return nil
		}
		return []string{strings.TrimSpace(singleModel)}
	}

	raw := strings.Split(multiModels, ",")
	models := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		model := strings.TrimSpace(item)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	return models
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

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		os.Setenv(key, value)
	}
}

func sanitizeFileName(s string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "\t", "-")
	return replacer.Replace(strings.TrimSpace(s))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
