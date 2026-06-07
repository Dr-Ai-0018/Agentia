package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"ai-arena/internal/openai"
	"ai-arena/internal/runtime/newborn"
)

const defaultBaseURL = "https://api.openai.com/v1"

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL  = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		resident = flag.String("resident", "jade", "Resident: jade|amber|onyx")
		duration = flag.Duration("duration", 5*time.Minute, "Exploration duration")
		outDir   = flag.String("out-dir", "experiments/multi-agent-baseline/output", "Output directory")
		verbose  = flag.Bool("verbose", false, "Print streamed text as it arrives")
		reset    = flag.Bool("reset-resident", true, "Reset resident runtime state to current baseline before the run")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}

	profile, err := newborn.BuildProfile(strings.ToLower(strings.TrimSpace(*resident)))
	if err != nil {
		exitf("%v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create out dir: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	if err := openai.ProbeResponses(client, *baseURL, apiKey, newborn.BuildDecisionProbePayload(profile)); err != nil {
		exitf("openai health probe failed: %v", err)
	}
	runner := newborn.NewRunner(client, *baseURL, apiKey)
	report, err := runner.Run(profile, *duration, *outDir, *verbose, *reset)
	if err != nil {
		exitf("%v", err)
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(raw))
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
