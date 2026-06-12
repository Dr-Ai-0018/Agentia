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
		mode      = flag.String("mode", "sequential", "Mode: sequential|parallel")
	)
	flag.Parse()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		exitf("OPENAI_API_KEY is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitf("create out dir: %v", err)
	}

	app := broker.New(".agents")
	service := orchestrator.New(app, &http.Client{Timeout: 5 * time.Minute}, *baseURL, apiKey)
	out, err := service.Run(orchestrator.RunInput{
		Residents:     orchestrator.ParseResidentRoster(*residents),
		Duration:      *duration,
		OutDir:        *outDir,
		Verbose:       *verbose,
		ResetResident: *reset,
		Mode:          orchestrator.RunMode(strings.ToLower(strings.TrimSpace(*mode))),
	})
	if err != nil {
		exitf("%v", err)
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(raw))
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
