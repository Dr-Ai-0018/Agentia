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

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
)

const defaultBaseURL = "https://api.openai.com/v1"

func main() {
	loadDotEnvIfPresent(".env")

	var (
		baseURL   = flag.String("base-url", orchestrator.EnvOrDefault("OPENAI_BASE_URL", defaultBaseURL), "OpenAI API base URL")
		residents = flag.String("residents", "jade,amber,onyx", "Comma-separated residents")
		duration  = flag.Duration("duration", 90*time.Second, "Run duration per resident")
		outDir    = flag.String("out-dir", "runs/orchestrator", "Output directory")
		verbose   = flag.Bool("verbose", false, "Print streamed text as it arrives")
		reset     = flag.Bool("reset-resident", false, "Reset resident runtime state before each run")
		mode      = flag.String("mode", "run", "Mode: run|list|status|summary|report|pause|resume|retry-failed")
		runMode   = flag.String("run-mode", "sequential", "Run mode for run: sequential|parallel")
		runID     = flag.String("run-id", "", "Run ID for status|summary|pause|resume")
		limit     = flag.Int("limit", 10, "Run list limit for list mode")
	)
	flag.Parse()

	app := broker.New(".agents")
	modeValue := strings.ToLower(strings.TrimSpace(*mode))
	apiKey := os.Getenv("OPENAI_API_KEY")
	if modeValue == "run" || modeValue == "retry-failed" {
		if apiKey == "" && noResidentAPIKeys(orchestrator.ParseResidentRoster(*residents)) {
			exitf("OPENAI_API_KEY or resident-specific *_OPENAI_API_KEY is required")
		}
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			exitf("create out dir: %v", err)
		}
	}
	service := orchestrator.New(app, &http.Client{Timeout: 5 * time.Minute}, *baseURL, apiKey)
	switch modeValue {
	case "run":
		out, err := service.Run(orchestrator.RunInput{
			Residents:     orchestrator.ParseResidentRoster(*residents),
			Duration:      *duration,
			OutDir:        *outDir,
			Verbose:       *verbose,
			ResetResident: *reset,
			Mode:          orchestrator.RunMode(strings.ToLower(strings.TrimSpace(*runMode))),
		})
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "list":
		out, err := service.ListRuns(*limit)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "status":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for status mode")
		}
		out, err := service.ReadRunStatus(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "summary":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for summary mode")
		}
		out, err := service.ReadRunSummary(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "report":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for report mode")
		}
		out, err := service.ReadInspectionReport(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "pause":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for pause mode")
		}
		out, err := service.PauseRun(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "resume":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for resume mode")
		}
		out, err := service.ResumeRun(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	case "retry-failed":
		if strings.TrimSpace(*runID) == "" {
			exitf("run-id is required for retry-failed mode")
		}
		out, err := service.RetryFailedRun(*runID)
		if err != nil {
			exitf("%v", err)
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	default:
		exitf("unknown mode: %s", *mode)
	}
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

func noResidentAPIKeys(residents []string) bool {
	for _, resident := range residents {
		if orchestrator.ResidentAPIKey(resident, "") != "" {
			return false
		}
	}
	return true
}
