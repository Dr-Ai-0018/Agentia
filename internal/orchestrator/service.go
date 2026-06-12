package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
	Resident string               `json:"resident"`
	Status   string               `json:"status"`
	Report   *newborn.FinalReport `json:"report,omitempty"`
	Error    string               `json:"error,omitempty"`
}

type ResidentRunStatus struct {
	Resident  string `json:"resident"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	Error     string `json:"error,omitempty"`
}

type RunContract struct {
	RunID         string        `json:"run_id"`
	RetryOf       string        `json:"retry_of,omitempty"`
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

type InspectionResidentReport struct {
	Resident      string `json:"resident"`
	Status        string `json:"status"`
	Rounds        int    `json:"rounds,omitempty"`
	StoppedReason string `json:"stopped_reason,omitempty"`
	Error         string `json:"error,omitempty"`
}

type InspectionReport struct {
	RunID             string                     `json:"run_id"`
	RetryOf           string                     `json:"retry_of,omitempty"`
	Mode              RunMode                    `json:"mode"`
	ResidentsPlanned  []string                   `json:"residents_planned"`
	ResidentsFinished int                        `json:"residents_finished"`
	ResidentsErrored  int                        `json:"residents_errored"`
	StartedAt         string                     `json:"started_at"`
	EndedAt           string                     `json:"ended_at"`
	Duration          string                     `json:"duration"`
	Assessment        newborn.ParallelRunSummary `json:"assessment"`
	Residents         []InspectionResidentReport `json:"residents"`
}

type RunStatus struct {
	RunID      string              `json:"run_id"`
	Status     string              `json:"status"`
	Mode       RunMode             `json:"mode"`
	Residents  []ResidentRunStatus `json:"residents"`
	StartedAt  string              `json:"started_at"`
	UpdatedAt  string              `json:"updated_at"`
	FinishedAt string              `json:"finished_at,omitempty"`
}

type RunRecord struct {
	RunID      string   `json:"run_id"`
	Status     string   `json:"status"`
	Mode       RunMode  `json:"mode"`
	Residents  []string `json:"residents"`
	StartedAt  string   `json:"started_at"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
	FinishedAt string   `json:"finished_at,omitempty"`
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
	stateRoot     string
}

func New(app *broker.App, client *http.Client, baseURL, apiKey string) *Service {
	return &Service{
		app:       app,
		client:    client,
		baseURL:   strings.TrimSpace(baseURL),
		apiKey:    strings.TrimSpace(apiKey),
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
	return s.runWithRetryOf(input, "")
}

func (s *Service) runWithRetryOf(input RunInput, retryOf string) (RunSummary, error) {
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
		RunID:         fmt.Sprintf("orchestrator-%s", started.Format("20060102T150405.000000000Z")),
		RetryOf:       strings.TrimSpace(retryOf),
		Mode:          input.Mode,
		Residents:     append([]string(nil), input.Residents...),
		Duration:      input.Duration,
		OutDir:        input.OutDir,
		ResetResident: input.ResetResident,
		Verbose:       input.Verbose,
	}
	runStatus := RunStatus{
		RunID:     contract.RunID,
		Status:    "running",
		Mode:      input.Mode,
		StartedAt: started.Format(time.RFC3339),
		UpdatedAt: started.Format(time.RFC3339),
		Residents: make([]ResidentRunStatus, 0, len(input.Residents)),
	}
	for _, resident := range input.Residents {
		runStatus.Residents = append(runStatus.Residents, ResidentRunStatus{
			Resident:  resident,
			Status:    "pending",
			UpdatedAt: started.Format(time.RFC3339),
		})
	}
	if err := s.writeStatus(runStatus); err != nil {
		return RunSummary{}, err
	}
	runs := make([]ResidentRun, len(input.Residents))
	switch input.Mode {
	case RunModeSequential:
		for i, resident := range input.Residents {
			if err := s.waitIfPaused(contract.RunID, &runStatus); err != nil {
				return RunSummary{}, err
			}
			runs[i] = s.runResident(resident, input, &runStatus)
		}
	case RunModeParallel:
		var wg sync.WaitGroup
		for i, resident := range input.Residents {
			i := i
			resident := resident
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = s.runResident(resident, input, &runStatus)
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
	runStatus.Status = "finished"
	runStatus.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	runStatus.FinishedAt = runStatus.UpdatedAt
	if hasRunErrors(runs) {
		runStatus.Status = "finished_with_errors"
	}
	if err := s.writeStatus(runStatus); err != nil {
		return RunSummary{}, err
	}
	if err := s.writeSummary(summary); err != nil {
		return RunSummary{}, err
	}
	return summary, nil
}

func (s *Service) RetryFailedRun(runID string) (RunSummary, error) {
	summary, err := s.ReadRunSummary(runID)
	if err != nil {
		return RunSummary{}, err
	}
	failed := make([]string, 0, len(summary.Runs))
	for _, item := range summary.Runs {
		if item.Status == "error" {
			failed = append(failed, item.Resident)
		}
	}
	if len(failed) == 0 {
		return RunSummary{}, fmt.Errorf("run %s has no failed residents to retry", runID)
	}
	return s.runWithRetryOf(RunInput{
		Residents:     failed,
		Duration:      summary.Contract.Duration,
		OutDir:        summary.Contract.OutDir,
		Verbose:       summary.Contract.Verbose,
		ResetResident: summary.Contract.ResetResident,
		Mode:          summary.Contract.Mode,
	}, runID)
}

func (s *Service) PauseRun(runID string) (RunStatus, error) {
	status, err := s.ReadRunStatus(runID)
	if err != nil {
		return RunStatus{}, err
	}
	if status.Status == "finished" || status.Status == "finished_with_errors" {
		return RunStatus{}, fmt.Errorf("run %s is already finished", runID)
	}
	status.Status = "paused"
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.writeStatus(status); err != nil {
		return RunStatus{}, err
	}
	return status, nil
}

func (s *Service) ResumeRun(runID string) (RunStatus, error) {
	status, err := s.ReadRunStatus(runID)
	if err != nil {
		return RunStatus{}, err
	}
	if status.Status != "paused" {
		return RunStatus{}, fmt.Errorf("run %s is not paused", runID)
	}
	status.Status = "running"
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.writeStatus(status); err != nil {
		return RunStatus{}, err
	}
	return status, nil
}

func (s *Service) runResident(resident string, input RunInput, runStatus *RunStatus) ResidentRun {
	run := ResidentRun{Resident: resident}
	s.updateResidentStatus(runStatus, resident, "running", "")
	if _, ok := s.app.Binding(resident); !ok {
		run.Status = "error"
		run.Error = fmt.Sprintf("unknown resident binding: %s", resident)
		s.updateResidentStatus(runStatus, resident, "error", run.Error)
		return run
	}
	profile, err := newborn.BuildProfile(resident)
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
		s.updateResidentStatus(runStatus, resident, "error", run.Error)
		return run
	}
	runner := s.runnerFactory(s.client, s.baseURL, s.apiKey)
	report, err := runner.Run(profile, input.Duration, input.OutDir, input.Verbose, input.ResetResident)
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
		s.updateResidentStatus(runStatus, resident, "error", run.Error)
		return run
	}
	run.Status = "ok"
	run.Report = &report
	s.updateResidentStatus(runStatus, resident, "finished", "")
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
	if err := atomicWriteFile(filepath.Join(runDir, "summary.json"), raw, 0o644); err != nil {
		return err
	}
	reportRaw, err := json.MarshalIndent(BuildInspectionReport(summary), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inspection report: %w", err)
	}
	return atomicWriteFile(filepath.Join(runDir, "inspection-report.json"), reportRaw, 0o644)
}

func (s *Service) writeStatus(status RunStatus) error {
	runDir := filepath.Join(s.stateRoot, status.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("mkdir orchestrator status dir: %w", err)
	}
	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orchestrator status: %w", err)
	}
	if err := atomicWriteFile(filepath.Join(runDir, "run-status.json"), raw, 0o644); err != nil {
		return err
	}
	for _, resident := range status.Residents {
		residentRaw, err := json.MarshalIndent(resident, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal resident orchestrator status: %w", err)
		}
		if err := atomicWriteFile(filepath.Join(runDir, fmt.Sprintf("resident-%s.json", resident.Resident)), residentRaw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) updateResidentStatus(runStatus *RunStatus, resident, state, errText string) {
	if runStatus == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	latestStatus := runStatus.Status
	if current, err := s.ReadRunStatus(runStatus.RunID); err == nil && strings.TrimSpace(current.Status) != "" {
		latestStatus = current.Status
	}
	for i := range runStatus.Residents {
		if runStatus.Residents[i].Resident != resident {
			continue
		}
		runStatus.Residents[i].Status = state
		runStatus.Residents[i].Error = errText
		runStatus.Residents[i].UpdatedAt = now
		runStatus.UpdatedAt = now
		runStatus.Status = latestStatus
		_ = s.writeStatus(*runStatus)
		return
	}
}

func hasRunErrors(runs []ResidentRun) bool {
	for _, item := range runs {
		if item.Status == "error" {
			return true
		}
	}
	return false
}

func (s *Service) waitIfPaused(runID string, current *RunStatus) error {
	for {
		status, err := s.ReadRunStatus(runID)
		if err != nil {
			return err
		}
		if status.Status != "paused" {
			if current != nil {
				*current = status
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Service) ReadRunStatus(runID string) (RunStatus, error) {
	path := filepath.Join(s.stateRoot, strings.TrimSpace(runID), "run-status.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunStatus{}, err
	}
	var out RunStatus
	if err := json.Unmarshal(raw, &out); err != nil {
		return RunStatus{}, err
	}
	return out, nil
}

func (s *Service) ReadRunSummary(runID string) (RunSummary, error) {
	path := filepath.Join(s.stateRoot, strings.TrimSpace(runID), "summary.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunSummary{}, err
	}
	var out RunSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		return RunSummary{}, err
	}
	return out, nil
}

func (s *Service) ReadInspectionReport(runID string) (InspectionReport, error) {
	path := filepath.Join(s.stateRoot, strings.TrimSpace(runID), "inspection-report.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return InspectionReport{}, err
	}
	var out InspectionReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return InspectionReport{}, err
	}
	return out, nil
}

func (s *Service) ListRuns(limit int) ([]RunRecord, error) {
	entries, err := os.ReadDir(s.stateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]RunRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		status, err := s.ReadRunStatus(entry.Name())
		if err != nil {
			continue
		}
		record := RunRecord{
			RunID:      status.RunID,
			Status:     status.Status,
			Mode:       status.Mode,
			StartedAt:  status.StartedAt,
			UpdatedAt:  status.UpdatedAt,
			FinishedAt: status.FinishedAt,
			Residents:  make([]string, 0, len(status.Residents)),
		}
		for _, resident := range status.Residents {
			record.Residents = append(record.Residents, resident.Resident)
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt > out[j].StartedAt
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
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
