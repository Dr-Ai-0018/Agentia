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
	"sync"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/runtime/newborn"
)

const defaultBaseURL = "https://api.openai.com/v1"

type residentRun struct {
	Resident string                     `json:"resident"`
	Status   *brokerstate.ResidentStatus `json:"status,omitempty"`
	Report   *newborn.FinalReport       `json:"report,omitempty"`
	Error    string                     `json:"error,omitempty"`
}

type parallelSummary struct {
	StartedAt  string                          `json:"started_at"`
	EndedAt    string                          `json:"ended_at"`
	Duration   string                          `json:"duration"`
	Runs       []residentRun                   `json:"runs"`
	Assessment newborn.ParallelRunSummary      `json:"assessment"`
}

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL   = flag.String("base-url", envOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		residents = flag.String("residents", "jade,amber,onyx", "Comma-separated residents")
		duration  = flag.Duration("duration", 90*time.Second, "Run duration per resident")
		outDir    = flag.String("out-dir", "experiments/multi-agent-parallel/output", "Output directory")
		verbose   = flag.Bool("verbose", false, "Print streamed text as it arrives")
		reset     = flag.Bool("reset-resident", false, "Reset resident runtime state before each run")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create out dir: %v", err)
	}

	started := time.Now().UTC()
	names := parseResidents(*residents)
	results := make([]residentRun, len(names))
	app := broker.New(".agents")
	var wg sync.WaitGroup

	for i, name := range names {
		i := i
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			run := residentRun{Resident: name}
			status, err := app.RunStatus(name)
			if err == nil {
				run.Status = &status
			}
			profile, err := newborn.BuildProfile(name)
			if err != nil {
				run.Error = err.Error()
				results[i] = run
				return
			}
			runner := newborn.NewRunner(&http.Client{Timeout: 5 * time.Minute}, *baseURL, apiKey)
			report, err := runner.Run(profile, *duration, *outDir, *verbose, *reset)
			if err != nil {
				run.Error = err.Error()
				results[i] = run
				return
			}
			run.Report = &report
			results[i] = run
		}()
	}

	wg.Wait()
	summary := parallelSummary{
		StartedAt: started.Format(time.RFC3339),
		EndedAt:   time.Now().UTC().Format(time.RFC3339),
		Duration:  time.Since(started).String(),
		Runs:      results,
	}
	reports := make([]newborn.FinalReport, 0, len(results))
	for _, item := range results {
		if item.Report != nil {
			reports = append(reports, *item.Report)
		}
	}
	summary.Assessment = newborn.SummarizeParallelReports(reports)
	raw, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(raw))
}

func parseResidents(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
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
