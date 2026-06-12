package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtime/newborn"
)

type RunMode string

const (
	RunModeSequential RunMode = "sequential"
	RunModeParallel   RunMode = "parallel"
)

type Runner interface {
	Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error)
}

type RunnerFactory func(client *http.Client, baseURL, apiKey string) Runner

type ResidentRun struct {
	Resident string                `json:"resident"`
	Status   string                `json:"status"`
	Report   *newborn.FinalReport  `json:"report,omitempty"`
	Error    string                `json:"error,omitempty"`
}

type RunContract struct {
	RunID         string        `json:"run_id"`
	Mode          RunMode       `json:"mode"`
	Residents     []string      `json:"residents"`
	Duration      time.Duration `json:"duration"`
	OutDir        string        `json:"out_dir"`
	ResetResident bool          `json:"reset_resident"`
	Verbose       bool          `json:"verbose"`
}

type RunSummary struct {
	RunID      string                     `json:"run_id"`
	Contract   RunContract                `json:"contract"`
	StartedAt  string                     `json:"started_at"`
	EndedAt    string                     `json:"ended_at"`
	Duration   string                     `json:"duration"`
	Mode       RunMode                    `json:"mode"`
	Residents  []string                   `json:"residents"`
	Runs       []ResidentRun              `json:"runs"`
	Assessment newborn.ParallelRunSummary `json:"assessment"`
}

type RunInput struct {
	Residents     []string
	Duration      time.Duration
	OutDir        string
	Verbose       bool
	ResetResident bool
	Mode          RunMode
}

type Service struct {
	app           *broker.App
	baseURL       string
	apiKey        string
	client        *http.Client
	runnerFactory RunnerFactory
	stateRoot      string
}

func New(app *broker.App, client *http.Client, baseURL, apiKey string) *Service {
	return &Service{
		app:     app,
		client:  client,
		baseURL: strings.TrimSpace(baseURL),
		apiKey:  strings.TrimSpace(apiKey),
		stateRoot: ".agents/orchestrator-runs",
		runnerFactory: func(client *http.Client, baseURL, apiKey string) Runner {
			return newborn.NewRunner(client, baseURL, apiKey)
		},
	}
}

func ParseResidentRoster(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func EnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (s *Service) Run(input RunInput) (RunSummary, error) {
	if len(input.Residents) == 0 {
		return RunSummary{}, fmt.Errorf("at least one resident is required")
	}
	if input.Duration <= 0 {
		return RunSummary{}, fmt.Errorf("duration must be positive")
	}
	if strings.TrimSpace(input.OutDir) == "" {
		return RunSummary{}, fmt.Errorf("out dir is required")
	}
	if input.Mode == "" {
		input.Mode = RunModeSequential
	}
	started := time.Now().UTC()
	contract := RunContract{
		RunID:         fmt.Sprintf("orchestrator-%s", started.Format("20060102T150405Z")),
		Mode:          input.Mode,
		Residents:     append([]string(nil), input.Residents...),
		Duration:      input.Duration,
		OutDir:        input.OutDir,
		ResetResident: input.ResetResident,
		Verbose:       input.Verbose,
	}
	runs := make([]ResidentRun, len(input.Residents))
	switch input.Mode {
	case RunModeSequential:
		for i, resident := range input.Residents {
			runs[i] = s.runResident(resident, input)
		}
	case RunModeParallel:
		var wg sync.WaitGroup
		for i, resident := range input.Residents {
			i := i
			resident := resident
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = s.runResident(resident, input)
			}()
		}
		wg.Wait()
	default:
		return RunSummary{}, fmt.Errorf("unsupported run mode %q", input.Mode)
	}

	reports := make([]newborn.FinalReport, 0, len(runs))
	for _, item := range runs {
		if item.Report != nil {
			reports = append(reports, *item.Report)
		}
	}
	summary := RunSummary{
		RunID:      contract.RunID,
		Contract:   contract,
		StartedAt:  started.Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
		Duration:   time.Since(started).String(),
		Mode:       input.Mode,
		Residents:  append([]string(nil), input.Residents...),
		Runs:       runs,
		Assessment: newborn.SummarizeParallelReports(reports),
	}
	if err := s.writeSummary(summary); err != nil {
		return RunSummary{}, err
	}
	return summary, nil
}

func (s *Service) runResident(resident string, input RunInput) ResidentRun {
	run := ResidentRun{Resident: resident}
	if _, ok := s.app.Binding(resident); !ok {
		run.Status = "error"
		run.Error = fmt.Sprintf("unknown resident binding: %s", resident)
		return run
	}
	profile, err := newborn.BuildProfile(resident)
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
		return run
	}
	runner := s.runnerFactory(s.client, s.baseURL, s.apiKey)
	report, err := runner.Run(profile, input.Duration, input.OutDir, input.Verbose, input.ResetResident)
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
		return run
	}
	run.Status = "ok"
	run.Report = &report
	return run
}

func (s *Service) writeSummary(summary RunSummary) error {
	runDir := filepath.Join(s.stateRoot, summary.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("mkdir orchestrator run dir: %w", err)
	}
	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orchestrator summary: %w", err)
	}
	return atomicWriteFile(filepath.Join(runDir, "summary.json"), raw, 0o644)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
