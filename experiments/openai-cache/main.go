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
		logDir  = flag.String("log-dir", "experiments/openai-cache/logs", "Directory to store JSONL run logs")
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

	for _, modelID := range modelList {
		if err := runExperiment(client, *baseURL, apiKey, modelID, *turns, *verbose, *logDir); err != nil {
			exitf("model %s failed: %v", modelID, err)
		}
	}
}

func runExperiment(client *http.Client, baseURL, apiKey, model string, turns int, verbose bool, logDir string) error {
	instructions := makeInstructions()
	promptCacheKey := fmt.Sprintf("arena-openai-cache-%s-v1", model)
	history := make([]message, 0, turns*2)

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.jsonl", sanitizeFileName(model), time.Now().UTC().Format("20060102T150405Z")))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	fmt.Printf("== model: %s ==\n", model)
	fmt.Printf("log_file=%s\n", logPath)

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
			return fmt.Errorf("turn %d: %w", turn, err)
		}

		duration := time.Since(started).Milliseconds()
		prefixHash := hashPrefix(instructions, payload.Input)

		logLine := map[string]any{
			"turn":                      turn,
			"model":                     model,
			"response_id":               result.ResponseID,
			"x_request_id":              result.RequestID,
			"prompt_cache_key_sent":     promptCacheKey,
			"prompt_cache_key_observed": result.ObservedPromptCacheKey,
			"instructions_hash":         sha256Hex(instructions),
			"input_prefix_hash":         prefixHash,
			"input_message_count":       len(payload.Input),
			"input_tokens":              result.InputTokens,
			"cached_tokens":             result.CachedTokens,
			"output_tokens":             result.OutputTokens,
			"duration_ms":               duration,
			"text":                      result.OutputText,
		}

		out, _ := json.Marshal(logLine)
		fmt.Println(string(out))
		if _, err := logFile.Write(append(out, '\n')); err != nil {
			return fmt.Errorf("write log file: %w", err)
		}

		history = append(history, message{
			Role:    "assistant",
			Content: result.OutputText,
		})
	}

	return nil
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
