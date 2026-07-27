package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/openai"
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

type progressAwareRunner interface {
	SetProgressSink(func(newborn.ProgressEvent))
}

type optionAwareRunner interface {
	SetRunOptions(newborn.RunOptions)
}

type contextAwareRunner interface {
	SetRunContext(context.Context)
}

type errorRunner struct {
	err error
}

func (r errorRunner) Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
	return newborn.FinalReport{}, r.err
}

type RunnerFactory func(client *http.Client, baseURL, apiKey, resident string) Runner
type EndpointRunnerFactory func(client *http.Client, endpoints []openai.Endpoint, resident string) Runner

type ResidentRun struct {
	Resident string               `json:"resident"`
	Status   string               `json:"status"`
	Report   *newborn.FinalReport `json:"report,omitempty"`
	Error    string               `json:"error,omitempty"`
}

type ResidentRunStatus struct {
	Resident            string               `json:"resident"`
	Status              string               `json:"status"`
	UpdatedAt           string               `json:"updated_at"`
	Error               string               `json:"error,omitempty"`
	TransientBlocked    bool                 `json:"transient_blocked,omitempty"`
	CurrentPhase        string               `json:"current_phase,omitempty"`
	CurrentRound        int                  `json:"current_round,omitempty"`
	RemainingSec        int                  `json:"remaining_sec,omitempty"`
	LastAction          string               `json:"last_action,omitempty"`
	LastResponseID      string               `json:"last_response_id,omitempty"`
	InFlightStartedAt   string               `json:"in_flight_started_at,omitempty"`
	LastRoundFinishedAt string               `json:"last_round_finished_at,omitempty"`
	InputTokens         int                  `json:"input_tokens,omitempty"`
	CachedTokens        int                  `json:"cached_tokens,omitempty"`
	OutputTokens        int                  `json:"output_tokens,omitempty"`
	TotalInputTokens    int                  `json:"total_input_tokens,omitempty"`
	TotalCachedTokens   int                  `json:"total_cached_tokens,omitempty"`
	TotalOutputTokens   int                  `json:"total_output_tokens,omitempty"`
	SummaryPane         *newborn.SummaryPane `json:"summary_pane,omitempty"`
}

type RunEvent struct {
	Type      string `json:"type"`
	At        string `json:"at"`
	Resident  string `json:"resident,omitempty"`
	Message   string `json:"message,omitempty"`
	RetryOf   string `json:"retry_of,omitempty"`
	RetryRun  string `json:"retry_run,omitempty"`
	FromState string `json:"from_state,omitempty"`
	ToState   string `json:"to_state,omitempty"`
}

type RunContract struct {
	RunID                      string        `json:"run_id"`
	RetryOf                    string        `json:"retry_of,omitempty"`
	Mode                       RunMode       `json:"mode"`
	Residents                  []string      `json:"residents"`
	Duration                   time.Duration `json:"duration"`
	OutDir                     string        `json:"out_dir"`
	ResetResident              bool          `json:"reset_resident"`
	Verbose                    bool          `json:"verbose"`
	ContinueOnNoop             bool          `json:"continue_on_noop,omitempty"`
	Purpose                    string        `json:"purpose,omitempty"`
	CompactionRecentRounds     int           `json:"compaction_recent_rounds,omitempty"`
	CompactionProbeEveryRounds int           `json:"compaction_probe_every_rounds,omitempty"`
	RequiredCompactionCycles   int           `json:"required_compaction_cycles,omitempty"`
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
	Resident         string `json:"resident"`
	Status           string `json:"status"`
	Rounds           int    `json:"rounds,omitempty"`
	StoppedReason    string `json:"stopped_reason,omitempty"`
	BudgetBlocked    bool   `json:"budget_blocked,omitempty"`
	CompletedUseful  bool   `json:"completed_useful,omitempty"`
	TransientBlocked bool   `json:"transient_blocked,omitempty"`
	Error            string `json:"error,omitempty"`
}

type InspectionReport struct {
	RunID             string                     `json:"run_id"`
	RetryOf           string                     `json:"retry_of,omitempty"`
	Mode              RunMode                    `json:"mode"`
	ResidentsPlanned  []string                   `json:"residents_planned"`
	ResidentsFinished int                        `json:"residents_finished"`
	ResidentsErrored  int                        `json:"residents_errored"`
	TransientBlocked  int                        `json:"transient_blocked"`
	UsefulRuns        int                        `json:"useful_runs"`
	BudgetBlockedRuns int                        `json:"budget_blocked_runs"`
	StartedAt         string                     `json:"started_at"`
	EndedAt           string                     `json:"ended_at"`
	Duration          string                     `json:"duration"`
	Assessment        newborn.ParallelRunSummary `json:"assessment"`
	Residents         []InspectionResidentReport `json:"residents"`
}

type RunStatus struct {
	RunID                 string              `json:"run_id"`
	Status                string              `json:"status"`
	Mode                  RunMode             `json:"mode"`
	Residents             []ResidentRunStatus `json:"residents"`
	StartedAt             string              `json:"started_at"`
	UpdatedAt             string              `json:"updated_at"`
	PausedAt              string              `json:"paused_at,omitempty"`
	ResumedAt             string              `json:"resumed_at,omitempty"`
	FinishedAt            string              `json:"finished_at,omitempty"`
	TargetDurationSeconds int                 `json:"target_duration_seconds,omitempty"`
	Events                []RunEvent          `json:"events,omitempty"`
	OwnerPID              int                 `json:"owner_pid,omitempty"`
}

type RunRecord struct {
	RunID                 string   `json:"run_id"`
	RetryOf               string   `json:"retry_of,omitempty"`
	Status                string   `json:"status"`
	Mode                  RunMode  `json:"mode"`
	Residents             []string `json:"residents"`
	StartedAt             string   `json:"started_at"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
	PausedAt              string   `json:"paused_at,omitempty"`
	ResumedAt             string   `json:"resumed_at,omitempty"`
	FinishedAt            string   `json:"finished_at,omitempty"`
	TargetDurationSeconds int      `json:"target_duration_seconds,omitempty"`
}

type RunInput struct {
	Residents                  []string
	Duration                   time.Duration
	OutDir                     string
	Verbose                    bool
	ResetResident              bool
	Mode                       RunMode
	ContinueOnNoop             bool
	Purpose                    string
	CompactionRecentRounds     int
	CompactionProbeEveryRounds int
	RequiredCompactionCycles   int
	Context                    context.Context
}

type Service struct {
	app                   *broker.App
	baseURL               string
	apiKey                string
	client                *http.Client
	runnerFactory         RunnerFactory
	endpointRunnerFactory EndpointRunnerFactory
	stateRoot             string
}

func New(app *broker.App, client *http.Client, baseURL, apiKey string) *Service {
	return &Service{
		app:       app,
		client:    client,
		baseURL:   strings.TrimSpace(baseURL),
		apiKey:    strings.TrimSpace(apiKey),
		stateRoot: ".agents/orchestrator-runs",
		endpointRunnerFactory: func(client *http.Client, endpoints []openai.Endpoint, resident string) Runner {
			if len(usableEndpoints(endpoints)) == 0 {
				return errorRunner{err: fmt.Errorf("missing api key for resident: %s", strings.TrimSpace(resident))}
			}
			return newborn.NewRunnerWithEndpoints(client, endpoints)
		},
	}
}

func (s *Service) SetStateRootForTest(root string) {
	s.stateRoot = strings.TrimSpace(root)
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

func ResidentAPIKey(resident, fallback string) string {
	resident = strings.ToUpper(strings.TrimSpace(resident))
	resident = strings.NewReplacer("-", "_", " ", "_").Replace(resident)
	if resident != "" {
		if value := strings.TrimSpace(os.Getenv(resident + "_OPENAI_API_KEY")); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fallback)
}

func ResidentEndpoints(resident, fallbackBaseURL, fallbackAPIKey string) []openai.Endpoint {
	normalized := normalizeResidentEnvPrefix(resident)
	fallbackBaseURL = strings.TrimSpace(fallbackBaseURL)
	fallbackAPIKey = strings.TrimSpace(fallbackAPIKey)
	personal := make([]openai.Endpoint, 0, 3)
	personalPrimaryBaseURL := fallbackBaseURL
	for i := 1; i <= 3; i++ {
		baseURL := ""
		apiKey := ""
		comment := ""
		if normalized != "" {
			baseURL = strings.TrimSpace(os.Getenv(fmt.Sprintf("%s_OPENAI_BASE_URL_%d", normalized, i)))
			apiKey = strings.TrimSpace(os.Getenv(fmt.Sprintf("%s_OPENAI_API_KEY_%d", normalized, i)))
			comment = strings.TrimSpace(os.Getenv(fmt.Sprintf("%s_OPENAI_CHANNEL_COMMENT_%d", normalized, i)))
		}
		if i == 1 {
			if baseURL == "" && normalized != "" {
				baseURL = strings.TrimSpace(os.Getenv(normalized + "_OPENAI_BASE_URL"))
			}
			if apiKey == "" && normalized != "" {
				apiKey = strings.TrimSpace(os.Getenv(normalized + "_OPENAI_API_KEY"))
			}
			if comment == "" && normalized != "" {
				comment = strings.TrimSpace(os.Getenv(normalized + "_OPENAI_CHANNEL_COMMENT"))
			}
			if baseURL == "" && apiKey != "" {
				baseURL = personalPrimaryBaseURL
			}
			if baseURL != "" {
				personalPrimaryBaseURL = baseURL
			}
		} else if baseURL == "" && apiKey != "" {
			baseURL = personalPrimaryBaseURL
		}
		if baseURL == "" || apiKey == "" {
			continue
		}
		name := "primary"
		if i > 1 {
			name = fmt.Sprintf("backup_%d", i-1)
		}
		personal = append(personal, openai.Endpoint{
			Name:    name,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Comment: comment,
		})
	}
	global := globalEndpoints(fallbackBaseURL, fallbackAPIKey)
	if len(personal) > 0 {
		return append(personal, renameEndpoints(global, "global_fallback")...)
	}
	return global
}

func normalizeResidentEnvPrefix(resident string) string {
	resident = strings.ToUpper(strings.TrimSpace(resident))
	return strings.NewReplacer("-", "_", " ", "_").Replace(resident)
}

func globalEndpoints(fallbackBaseURL, fallbackAPIKey string) []openai.Endpoint {
	fallbackBaseURL = strings.TrimSpace(fallbackBaseURL)
	fallbackAPIKey = strings.TrimSpace(fallbackAPIKey)
	out := make([]openai.Endpoint, 0, 3)
	primaryBaseURL := fallbackBaseURL
	for i := 1; i <= 3; i++ {
		baseURL := strings.TrimSpace(os.Getenv(fmt.Sprintf("OPENAI_BASE_URL_%d", i)))
		apiKey := strings.TrimSpace(os.Getenv(fmt.Sprintf("OPENAI_API_KEY_%d", i)))
		comment := strings.TrimSpace(os.Getenv(fmt.Sprintf("OPENAI_CHANNEL_COMMENT_%d", i)))
		if i == 1 {
			if baseURL == "" {
				baseURL = fallbackBaseURL
			}
			if apiKey == "" {
				apiKey = fallbackAPIKey
			}
			if comment == "" {
				comment = strings.TrimSpace(os.Getenv("OPENAI_CHANNEL_COMMENT"))
			}
			if baseURL != "" {
				primaryBaseURL = baseURL
			}
		} else if baseURL == "" && apiKey != "" {
			baseURL = primaryBaseURL
		}
		if baseURL == "" || apiKey == "" {
			continue
		}
		name := "primary"
		if i > 1 {
			name = fmt.Sprintf("backup_%d", i-1)
		}
		out = append(out, openai.Endpoint{
			Name:    name,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Comment: comment,
		})
	}
	return out
}

func renameEndpoints(endpoints []openai.Endpoint, prefix string) []openai.Endpoint {
	out := make([]openai.Endpoint, 0, len(endpoints))
	for i, endpoint := range endpoints {
		name := strings.TrimSpace(endpoint.Name)
		if name == "" {
			name = fmt.Sprintf("endpoint_%d", i+1)
		}
		endpoint.Name = prefix + "_" + name
		out = append(out, endpoint)
	}
	return out
}

func usableEndpoints(endpoints []openai.Endpoint) []openai.Endpoint {
	out := make([]openai.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint.BaseURL) == "" || strings.TrimSpace(endpoint.APIKey) == "" {
			continue
		}
		out = append(out, endpoint)
	}
	return out
}

func (s *Service) Run(input RunInput) (RunSummary, error) {
	if input.Context == nil {
		input.Context = context.Background()
	}
	return s.runWithRetryOf(input, "")
}

func (s *Service) RunContext(ctx context.Context, input RunInput) (RunSummary, error) {
	input.Context = ctx
	return s.Run(input)
}

func (s *Service) runWithRetryOf(input RunInput, retryOf string) (RunSummary, error) {
	if input.Context == nil {
		input.Context = context.Background()
	}
	if len(input.Residents) == 0 {
		return RunSummary{}, fmt.Errorf("at least one resident is required")
	}
	if input.Duration <= 0 {
		return RunSummary{}, fmt.Errorf("duration must be positive")
	}
	if strings.TrimSpace(input.OutDir) == "" {
		return RunSummary{}, fmt.Errorf("out dir is required")
	}
	if input.RequiredCompactionCycles < 0 {
		return RunSummary{}, fmt.Errorf("required compaction cycles must not be negative")
	}
	if input.RequiredCompactionCycles > 0 && input.CompactionProbeEveryRounds <= 1 {
		return RunSummary{}, fmt.Errorf("required compaction cycles need compaction-probe-every-rounds > 1")
	}
	if input.Mode == "" {
		input.Mode = RunModeSequential
	}
	if err := s.reconcileInterruptedRuns(time.Now().UTC()); err != nil {
		return RunSummary{}, err
	}
	started := time.Now().UTC()
	contract := RunContract{
		RunID:                      fmt.Sprintf("orchestrator-%s", started.Format("20060102T150405.000000000Z")),
		RetryOf:                    strings.TrimSpace(retryOf),
		Mode:                       input.Mode,
		Residents:                  append([]string(nil), input.Residents...),
		Duration:                   input.Duration,
		OutDir:                     input.OutDir,
		ResetResident:              input.ResetResident,
		Verbose:                    input.Verbose,
		ContinueOnNoop:             input.ContinueOnNoop,
		Purpose:                    strings.TrimSpace(input.Purpose),
		CompactionRecentRounds:     input.CompactionRecentRounds,
		CompactionProbeEveryRounds: input.CompactionProbeEveryRounds,
		RequiredCompactionCycles:   input.RequiredCompactionCycles,
	}
	runStatus := RunStatus{
		RunID:                 contract.RunID,
		Status:                "running",
		Mode:                  input.Mode,
		StartedAt:             started.Format(time.RFC3339),
		UpdatedAt:             started.Format(time.RFC3339),
		TargetDurationSeconds: int(input.Duration.Seconds()),
		OwnerPID:              os.Getpid(),
		Residents:             make([]ResidentRunStatus, 0, len(input.Residents)),
		Events: []RunEvent{{
			Type:    "started",
			At:      started.Format(time.RFC3339),
			Message: "orchestrator run started",
			RetryOf: contract.RetryOf,
		}},
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
	if err := s.appendRunEvent(runStatus.RunID, runStatus.Events[0]); err != nil {
		return RunSummary{}, err
	}
	runs := make([]ResidentRun, len(input.Residents))
	var statusMu sync.Mutex
	switch input.Mode {
	case RunModeSequential:
		for i, resident := range input.Residents {
			if input.Context.Err() != nil {
				break
			}
			if err := s.waitIfPaused(input.Context, contract.RunID, &runStatus); err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}
				return RunSummary{}, err
			}
			runs[i] = s.runResident(resident, input, &runStatus, &statusMu)
		}
	case RunModeParallel:
		var wg sync.WaitGroup
		for i, resident := range input.Residents {
			i := i
			resident := resident
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = s.runResident(resident, input, &runStatus, &statusMu)
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
	if input.Context.Err() != nil {
		runStatus.Status = "aborted_by_host"
	} else if hasRunErrors(runs) {
		runStatus.Status = "finished_with_errors"
	} else if hasTransientBlocks(runs) {
		runStatus.Status = "finished_with_transient_blocks"
	}
	if current, err := s.ReadRunStatus(contract.RunID); err == nil {
		runStatus.PausedAt = current.PausedAt
		runStatus.ResumedAt = current.ResumedAt
		runStatus.Events = append([]RunEvent(nil), current.Events...)
	}
	eventType := "finished"
	eventMessage := "orchestrator run finished"
	if runStatus.Status == "aborted_by_host" {
		eventType = "aborted"
		eventMessage = "orchestrator run aborted by host signal"
	}
	runStatus.Events = append(runStatus.Events, RunEvent{
		Type:      eventType,
		At:        runStatus.UpdatedAt,
		Message:   eventMessage,
		FromState: "running",
		ToState:   runStatus.Status,
	})
	if err := s.appendRunEvent(runStatus.RunID, runStatus.Events[len(runStatus.Events)-1]); err != nil {
		return RunSummary{}, err
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
		if item.Status == "error" || item.Status == "transient_blocked" {
			failed = append(failed, item.Resident)
		}
	}
	if len(failed) == 0 {
		return RunSummary{}, fmt.Errorf("run %s has no failed residents to retry", runID)
	}
	return s.runWithRetryOf(RunInput{
		Residents:                  failed,
		Duration:                   summary.Contract.Duration,
		OutDir:                     summary.Contract.OutDir,
		Verbose:                    summary.Contract.Verbose,
		ResetResident:              summary.Contract.ResetResident,
		Mode:                       summary.Contract.Mode,
		ContinueOnNoop:             summary.Contract.ContinueOnNoop,
		Purpose:                    summary.Contract.Purpose,
		CompactionRecentRounds:     summary.Contract.CompactionRecentRounds,
		CompactionProbeEveryRounds: summary.Contract.CompactionProbeEveryRounds,
		RequiredCompactionCycles:   summary.Contract.RequiredCompactionCycles,
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
	if status.Status == "paused" {
		return status, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	previous := status.Status
	status.Status = "paused"
	status.UpdatedAt = now
	status.PausedAt = now
	status.Events = append(status.Events, RunEvent{
		Type:      "paused",
		At:        now,
		Message:   "orchestrator run paused; sequential mode observes this between residents",
		FromState: previous,
		ToState:   "paused",
	})
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
	now := time.Now().UTC().Format(time.RFC3339)
	status.Status = "running"
	status.UpdatedAt = now
	status.ResumedAt = now
	status.Events = append(status.Events, RunEvent{
		Type:      "resumed",
		At:        now,
		Message:   "orchestrator run resumed",
		FromState: "paused",
		ToState:   "running",
	})
	if err := s.writeStatus(status); err != nil {
		return RunStatus{}, err
	}
	return status, nil
}

func (s *Service) runResident(resident string, input RunInput, runStatus *RunStatus, statusMu *sync.Mutex) ResidentRun {
	run := ResidentRun{Resident: resident}
	s.updateResidentStatus(runStatus, statusMu, resident, "running", "")
	if _, ok := s.app.Binding(resident); !ok {
		run.Status = "error"
		run.Error = fmt.Sprintf("unknown resident binding: %s", resident)
		s.updateResidentStatus(runStatus, statusMu, resident, "error", run.Error)
		return run
	}
	profile, err := newborn.BuildProfile(resident)
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
		s.updateResidentStatus(runStatus, statusMu, resident, "error", run.Error)
		return run
	}
	endpoints := ResidentEndpoints(profile.Name, s.baseURL, s.apiKey)
	var runner Runner
	if s.runnerFactory != nil {
		apiKey := ResidentAPIKey(profile.Name, s.apiKey)
		runner = s.runnerFactory(s.client, s.baseURL, apiKey, profile.Name)
	} else {
		runner = s.endpointRunnerFactory(s.client, endpoints, profile.Name)
	}
	if aware, ok := runner.(progressAwareRunner); ok {
		aware.SetProgressSink(func(event newborn.ProgressEvent) {
			s.updateResidentProgress(runStatus, statusMu, profile.Name, event)
		})
	}
	if aware, ok := runner.(optionAwareRunner); ok {
		aware.SetRunOptions(newborn.RunOptions{
			ContinueOnNoop:             input.ContinueOnNoop,
			Purpose:                    input.Purpose,
			CompactionRecentRounds:     input.CompactionRecentRounds,
			CompactionProbeEveryRounds: input.CompactionProbeEveryRounds,
		})
	}
	if aware, ok := runner.(contextAwareRunner); ok {
		aware.SetRunContext(input.Context)
	}
	report, err := runner.Run(profile, input.Duration, input.OutDir, input.Verbose, input.ResetResident)
	if err != nil {
		var partial *newborn.PartialRunError
		if errors.As(err, &partial) && partial.Report.Resident != "" {
			report = partial.Report
			run.Report = &report
		}
		if isTransientUpstreamError(err) {
			run.Status = "transient_blocked"
			run.Error = err.Error()
			s.updateResidentStatus(runStatus, statusMu, resident, "transient_blocked", run.Error)
			return run
		}
		run.Status = "error"
		run.Error = err.Error()
		s.updateResidentStatus(runStatus, statusMu, resident, "error", run.Error)
		return run
	}
	if strings.HasPrefix(report.StoppedReason, "aborted_by_host") {
		run.Status = "aborted"
		run.Report = &report
		s.updateResidentStatus(runStatus, statusMu, resident, "aborted", "host cancellation")
		return run
	}
	if input.RequiredCompactionCycles > 0 {
		completed := successfulRuntimeCompactions(report.CompactionEvents)
		if completed < input.RequiredCompactionCycles {
			run.Status = "error"
			run.Report = &report
			run.Error = fmt.Sprintf("compaction objective unmet: completed=%d required=%d", completed, input.RequiredCompactionCycles)
			s.updateResidentStatus(runStatus, statusMu, resident, "error", run.Error)
			return run
		}
	}
	run.Status = "ok"
	run.Report = &report
	s.updateResidentStatus(runStatus, statusMu, resident, "finished", "")
	return run
}

func successfulRuntimeCompactions(events []newborn.CompactionEvent) int {
	count := 0
	for _, event := range events {
		if event.TriggerReason == newborn.CompactionTriggerAcceptanceMicro {
			continue
		}
		if event.Outcome == newborn.CompactionOutcomeSummarized {
			count++
		}
	}
	return count
}

func (s *Service) updateResidentProgress(runStatus *RunStatus, statusMu *sync.Mutex, resident string, event newborn.ProgressEvent) {
	if runStatus == nil {
		return
	}
	if statusMu != nil {
		statusMu.Lock()
		defer statusMu.Unlock()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	latestStatus := runStatus.Status
	if current, err := s.ReadRunStatus(runStatus.RunID); err == nil && strings.TrimSpace(current.Status) != "" {
		latestStatus = current.Status
		runStatus.PausedAt = current.PausedAt
		runStatus.ResumedAt = current.ResumedAt
		runStatus.Events = append([]RunEvent(nil), current.Events...)
	}
	for i := range runStatus.Residents {
		if runStatus.Residents[i].Resident != resident {
			continue
		}
		item := &runStatus.Residents[i]
		item.CurrentPhase = strings.TrimSpace(event.Phase)
		if event.Round > 0 {
			item.CurrentRound = event.Round
		}
		if event.RemainingSec > 0 {
			item.RemainingSec = event.RemainingSec
		}
		if strings.TrimSpace(event.Action) != "" {
			item.LastAction = strings.TrimSpace(event.Action)
		}
		if strings.TrimSpace(event.ResponseID) != "" {
			item.LastResponseID = strings.TrimSpace(event.ResponseID)
		}
		if strings.TrimSpace(event.InFlightStartedAt) != "" {
			item.InFlightStartedAt = strings.TrimSpace(event.InFlightStartedAt)
		}
		if strings.TrimSpace(event.LastRoundFinishedAt) != "" {
			item.LastRoundFinishedAt = strings.TrimSpace(event.LastRoundFinishedAt)
			item.InFlightStartedAt = ""
		}
		if event.TokenUsagePresent || event.InputTokens > 0 {
			item.InputTokens = event.InputTokens
		}
		if event.TokenUsagePresent || event.CachedTokens > 0 {
			item.CachedTokens = event.CachedTokens
		}
		if event.TokenUsagePresent || event.OutputTokens > 0 {
			item.OutputTokens = event.OutputTokens
		}
		if event.TokenTotalsPresent || event.TotalInputTokens > 0 {
			item.TotalInputTokens = event.TotalInputTokens
		}
		if event.TokenTotalsPresent || event.TotalCachedTokens > 0 {
			item.TotalCachedTokens = event.TotalCachedTokens
		}
		if event.TokenTotalsPresent || event.TotalOutputTokens > 0 {
			item.TotalOutputTokens = event.TotalOutputTokens
		}
		if event.SummaryPane != nil {
			item.SummaryPane = event.SummaryPane
		}
		item.UpdatedAt = now
		runStatus.UpdatedAt = now
		runStatus.Status = latestStatus
		_ = s.writeStatus(*runStatus)
		return
	}
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

func (s *Service) updateResidentStatus(runStatus *RunStatus, statusMu *sync.Mutex, resident, state, errText string) {
	if runStatus == nil {
		return
	}
	if statusMu != nil {
		statusMu.Lock()
		defer statusMu.Unlock()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	latestStatus := runStatus.Status
	if current, err := s.ReadRunStatus(runStatus.RunID); err == nil && strings.TrimSpace(current.Status) != "" {
		latestStatus = current.Status
		runStatus.PausedAt = current.PausedAt
		runStatus.ResumedAt = current.ResumedAt
		runStatus.Events = append([]RunEvent(nil), current.Events...)
	}
	for i := range runStatus.Residents {
		if runStatus.Residents[i].Resident != resident {
			continue
		}
		previousResidentStatus := runStatus.Residents[i].Status
		runStatus.Residents[i].Status = state
		runStatus.Residents[i].Error = errText
		runStatus.Residents[i].TransientBlocked = state == "transient_blocked"
		switch state {
		case "finished", "error", "transient_blocked", "aborted":
			runStatus.Residents[i].CurrentPhase = state
			runStatus.Residents[i].InFlightStartedAt = ""
		}
		runStatus.Residents[i].UpdatedAt = now
		runStatus.UpdatedAt = now
		runStatus.Status = latestStatus
		runStatus.Events = append(runStatus.Events, RunEvent{
			Type:      "resident_status",
			At:        now,
			Resident:  resident,
			Message:   errText,
			FromState: previousResidentStatus,
			ToState:   state,
		})
		_ = s.appendRunEvent(runStatus.RunID, runStatus.Events[len(runStatus.Events)-1])
		_ = s.writeStatus(*runStatus)
		return
	}
}

func (s *Service) appendRunEvent(runID string, event RunEvent) error {
	runDir := filepath.Join(s.stateRoot, strings.TrimSpace(runID))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(runDir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Service) reconcileInterruptedRuns(now time.Time) error {
	runs, err := s.ListRuns(0)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status != "running" && run.Status != "paused" {
			continue
		}
		status, err := s.ReadRunStatus(run.RunID)
		if err != nil {
			continue
		}
		if status.OwnerPID > 0 && processAlive(status.OwnerPID) {
			return fmt.Errorf("run %s is still owned by live process %d", status.RunID, status.OwnerPID)
		}
		previous := status.Status
		status.Status = "interrupted"
		status.UpdatedAt = now.Format(time.RFC3339)
		status.FinishedAt = status.UpdatedAt
		for i := range status.Residents {
			if status.Residents[i].Status == "running" || status.Residents[i].Status == "pending" {
				status.Residents[i].Status = "interrupted"
				status.Residents[i].CurrentPhase = "interrupted"
				status.Residents[i].UpdatedAt = status.UpdatedAt
			}
		}
		event := RunEvent{Type: "reconciled_interruption", At: status.UpdatedAt, Message: "stale run reconciled before a new launch", FromState: previous, ToState: status.Status}
		status.Events = append(status.Events, event)
		if err := s.appendRunEvent(status.RunID, event); err != nil {
			return err
		}
		if err := s.writeStatus(status); err != nil {
			return err
		}
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func hasRunErrors(runs []ResidentRun) bool {
	for _, item := range runs {
		if item.Status == "error" {
			return true
		}
	}
	return false
}

func hasTransientBlocks(runs []ResidentRun) bool {
	for _, item := range runs {
		if item.Status == "transient_blocked" {
			return true
		}
	}
	return false
}

func isTransientUpstreamError(err error) bool {
	if openai.IsRetryableError(err) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "insufficient account balance")
}

func (s *Service) waitIfPaused(ctx context.Context, runID string, current *RunStatus) error {
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
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
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
			RunID:                 status.RunID,
			Status:                status.Status,
			Mode:                  status.Mode,
			StartedAt:             status.StartedAt,
			UpdatedAt:             status.UpdatedAt,
			PausedAt:              status.PausedAt,
			ResumedAt:             status.ResumedAt,
			FinishedAt:            status.FinishedAt,
			TargetDurationSeconds: status.TargetDurationSeconds,
			Residents:             make([]string, 0, len(status.Residents)),
		}
		if summary, err := s.ReadRunSummary(entry.Name()); err == nil {
			record.RetryOf = summary.Contract.RetryOf
			if record.TargetDurationSeconds == 0 {
				record.TargetDurationSeconds = int(summary.Contract.Duration.Seconds())
			}
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
