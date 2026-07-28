package orchestrator

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/memory"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtime/newborn"
)

type fakeRunner struct {
	report newborn.FinalReport
	err    error
	calls  []newborn.ResidentProfile
}

type cancellationRunner struct {
	ctx context.Context
}

func (r *cancellationRunner) SetRunContext(ctx context.Context) {
	r.ctx = ctx
}

func (r *cancellationRunner) Run(profile newborn.ResidentProfile, _ time.Duration, _ string, _ bool, _ bool) (newborn.FinalReport, error) {
	<-r.ctx.Done()
	return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, StoppedReason: "aborted_by_host"}, nil
}

func (f *fakeRunner) Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
	f.calls = append(f.calls, profile)
	if f.err != nil {
		return newborn.FinalReport{}, f.err
	}
	report := f.report
	report.Resident = profile.Name
	report.Model = profile.Model
	return report, nil
}

func TestParseResidentRosterDeduplicatesAndNormalizes(t *testing.T) {
	got := ParseResidentRoster(" jade, amber,JADE,,onyx ")
	want := []string{"jade", "amber", "onyx"}
	if len(got) != len(want) {
		t.Fatalf("unexpected roster length: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected roster[%d]: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestServiceRunSequential(t *testing.T) {
	app := broker.New(t.TempDir())
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 2}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade", "amber"},
		Duration:  30 * time.Second,
		OutDir:    t.TempDir(),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if len(out.Runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(out.Runs))
	}
	if out.Runs[0].Status != "ok" || out.Runs[1].Status != "ok" {
		t.Fatalf("unexpected run statuses: %#v", out.Runs)
	}
	if out.Assessment.UsefulRuns != 2 {
		t.Fatalf("expected useful run summary, got %#v", out.Assessment)
	}
	if out.RunID == "" || out.Contract.RunID == "" {
		t.Fatalf("expected run id in summary: %#v", out)
	}
}

func TestNewRunReconcilesDeadOwnerAndWritesOperationalEvents(t *testing.T) {
	root := t.TempDir()
	service := New(broker.New(root), &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	stale := RunStatus{
		RunID:     "stale-run",
		Status:    "running",
		StartedAt: "2026-07-27T05:25:30Z",
		UpdatedAt: "2026-07-27T05:26:00Z",
		OwnerPID:  99_999_999,
		Residents: []ResidentRunStatus{{Resident: "jade", Status: "running"}},
	}
	if err := service.writeStatus(stale); err != nil {
		t.Fatalf("seed stale status: %v", err)
	}
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner {
		return &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	}
	out, err := service.Run(RunInput{Residents: []string{"jade"}, Duration: time.Minute, OutDir: filepath.Join(root, "out")})
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	reconciled, err := service.ReadRunStatus("stale-run")
	if err != nil {
		t.Fatalf("read stale status: %v", err)
	}
	if reconciled.Status != "interrupted" || reconciled.Residents[0].Status != "interrupted" {
		t.Fatalf("stale run not reconciled: %#v", reconciled)
	}
	for _, runID := range []string{"stale-run", out.RunID} {
		raw, err := os.ReadFile(filepath.Join(service.stateRoot, runID, "events.jsonl"))
		if err != nil || len(raw) == 0 {
			t.Fatalf("operational event log missing for %s: bytes=%d err=%v", runID, len(raw), err)
		}
	}
}

func TestServiceRunParallelKeepsAllResidentStatuses(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			time.Sleep(10 * time.Millisecond)
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 1}, nil
		})
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade", "amber", "onyx"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeParallel,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	status, err := service.ReadRunStatus(out.RunID)
	if err != nil {
		t.Fatalf("read run status: %v", err)
	}
	if len(status.Residents) != 3 {
		t.Fatalf("expected three resident statuses, got %#v", status.Residents)
	}
	for _, item := range status.Residents {
		if item.Status != "finished" {
			t.Fatalf("expected finished resident status, got %#v", status.Residents)
		}
	}
}

func TestServiceRunContextFinalizesHostCancellation(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner {
		return &cancellationRunner{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan RunSummary, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := service.RunContext(ctx, RunInput{
			Residents: []string{"jade"},
			Duration:  time.Hour,
			OutDir:    filepath.Join(root, "out"),
			Mode:      RunModeParallel,
		})
		if err != nil {
			errCh <- err
			return
		}
		done <- out
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	var summary RunSummary
	select {
	case err := <-errCh:
		t.Fatalf("cancelled run returned error: %v", err)
	case summary = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled run did not finalize")
	}
	status, err := service.ReadRunStatus(summary.RunID)
	if err != nil {
		t.Fatalf("read cancelled status: %v", err)
	}
	if status.Status != "aborted_by_host" || status.FinishedAt == "" {
		t.Fatalf("unexpected cancelled status: %#v", status)
	}
	if len(status.Residents) != 1 || status.Residents[0].Status != "aborted" {
		t.Fatalf("resident cancellation not finalized: %#v", status.Residents)
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Report == nil {
		t.Fatalf("partial report missing after cancellation: %#v", summary.Runs)
	}
}

func TestServiceSequentialCancellationMarksUnstartedResidentsAborted(t *testing.T) {
	root := t.TempDir()
	service := New(broker.New(root), &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner { return &cancellationRunner{} }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan RunSummary, 1)
	go func() {
		out, _ := service.RunContext(ctx, RunInput{
			Residents: []string{"jade", "amber", "onyx"}, Duration: time.Hour, OutDir: filepath.Join(root, "out"), Mode: RunModeSequential,
		})
		done <- out
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	var summary RunSummary
	select {
	case summary = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sequential cancellation did not finalize")
	}
	if len(summary.Runs) != 3 {
		t.Fatalf("missing sequential summary entries: %#v", summary.Runs)
	}
	for _, run := range summary.Runs {
		if run.Resident == "" || run.Status != "aborted" {
			t.Fatalf("resident was not explicitly aborted: %#v", summary.Runs)
		}
	}
	status, err := service.ReadRunStatus(summary.RunID)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	for _, resident := range status.Residents {
		if resident.Status != "aborted" {
			t.Fatalf("durable status was not finalized: %#v", status.Residents)
		}
	}
}

func TestNewRunReconcilesStaleResidentsOutsideReplacementRoster(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	now := time.Now().UTC().Add(-time.Hour)
	for _, resident := range []string{"jade", "amber"} {
		if _, err := app.RunReset(resident, now); err != nil {
			t.Fatalf("reset %s: %v", resident, err)
		}
		if _, err := app.RunSleepStart(resident, 30, now, "stale run"); err != nil {
			t.Fatalf("start sleep %s: %v", resident, err)
		}
		store := memory.NewFileStore(filepath.Join(root, "memory"))
		if err := store.UpsertHistoryGroup(memory.HistoryGroup{
			GroupUUID: "old-" + resident, Resident: resident, CreatedAt: now, LastEventAt: now,
			SourceKind: "newborn_runtime_rounds", State: memory.HistoryGroupOpen, Tags: []string{"run:old-run"},
		}); err != nil {
			t.Fatalf("seed history %s: %v", resident, err)
		}
	}
	if err := service.writeStatus(RunStatus{
		RunID: "old-run", Status: "running", StartedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), OwnerPID: 99_999_999,
		Residents: []ResidentRunStatus{{Resident: "jade", Status: "running"}, {Resident: "amber", Status: "running"}},
	}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner {
		return &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	}
	if _, err := service.Run(RunInput{Residents: []string{"onyx"}, Duration: time.Minute, OutDir: filepath.Join(root, "out")}); err != nil {
		t.Fatalf("replacement run: %v", err)
	}
	for _, resident := range []string{"jade", "amber"} {
		status, err := app.RunStatus(resident)
		if err != nil {
			t.Fatalf("status %s: %v", resident, err)
		}
		if status.Sleep.Active {
			t.Fatalf("stale sleep remained active for %s: %#v", resident, status.Sleep)
		}
		groups, err := memory.NewFileStore(filepath.Join(root, "memory")).ListHistoryGroups(resident)
		if err != nil || len(groups) != 1 || groups[0].State != memory.HistoryGroupClosed {
			t.Fatalf("stale history not closed for %s: groups=%#v err=%v", resident, groups, err)
		}
	}
}

func TestPIDIdentityMismatchDoesNotBlockNewRun(t *testing.T) {
	root := t.TempDir()
	service := New(broker.New(root), &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	if err := service.writeStatus(RunStatus{
		RunID: "pid-reused", Status: "running", StartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339), UpdatedAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
		OwnerPID: os.Getpid(), OwnerBootID: "different-boot", OwnerStartTicks: 1,
		Residents: []ResidentRunStatus{{Resident: "jade", Status: "running"}},
	}); err != nil {
		t.Fatalf("seed reused pid: %v", err)
	}
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner {
		return &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	}
	if _, err := service.Run(RunInput{Residents: []string{"jade"}, Duration: time.Minute, OutDir: filepath.Join(root, "out")}); err != nil {
		t.Fatalf("identity mismatch falsely blocked run: %v", err)
	}
	status, err := service.ReadRunStatus("pid-reused")
	if err != nil || status.Status != "interrupted" {
		t.Fatalf("reused pid status not reconciled: %#v err=%v", status, err)
	}
}

func TestCurrentProcessIdentityStillBlocksOwnership(t *testing.T) {
	bootID, startTicks, executable, err := readProcessIdentity(os.Getpid())
	if err != nil {
		t.Fatalf("read current identity: %v", err)
	}
	if !processOwnsRun(RunStatus{OwnerPID: os.Getpid(), OwnerBootID: bootID, OwnerStartTicks: startTicks, OwnerExecutable: executable}) {
		t.Fatal("matching live owner identity was not recognized")
	}
}

func TestServiceRequiresConfiguredCompactionProbeForFormalObjective(t *testing.T) {
	service := New(broker.New(t.TempDir()), &http.Client{}, "http://example.invalid", "key")
	_, err := service.Run(RunInput{
		Residents:                []string{"jade"},
		Duration:                 time.Minute,
		OutDir:                   t.TempDir(),
		RequiredCompactionCycles: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "compaction-probe-every-rounds") {
		t.Fatalf("expected objective-aware preflight rejection, got %v", err)
	}
}

func TestServiceFailsEvidenceWhenCompactionObjectiveIsUnmet(t *testing.T) {
	root := t.TempDir()
	service := New(broker.New(root), &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(_ *http.Client, _, _, _ string) Runner {
		return &fakeRunner{report: newborn.FinalReport{
			Rounds: 25,
			CompactionEvents: []newborn.CompactionEvent{{
				TriggerReason: newborn.CompactionTriggerScheduledProbe,
				Outcome:       newborn.CompactionOutcomeSummarized,
			}},
		}}
	}
	out, err := service.Run(RunInput{
		Residents:                  []string{"jade"},
		Duration:                   time.Minute,
		OutDir:                     filepath.Join(root, "out"),
		CompactionProbeEveryRounds: 20,
		RequiredCompactionCycles:   2,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Runs[0].Status != "error" || !strings.Contains(out.Runs[0].Error, "compaction objective unmet") {
		t.Fatalf("formal compaction shortfall did not fail evidence: %#v", out.Runs[0])
	}
}

func TestServiceWritesResidentProgressStatus(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	progressWritten := make(chan struct{}, 1)
	release := make(chan struct{})
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return &progressRunner{
			onProgress: func(event newborn.ProgressEvent) {
				if event.Phase == "model_stream" {
					progressWritten <- struct{}{}
					<-release
				}
			},
		}
	}

	done := make(chan RunSummary, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := service.Run(RunInput{
			Residents: []string{"jade"},
			Duration:  30 * time.Second,
			OutDir:    filepath.Join(root, "out"),
			Mode:      RunModeSequential,
		})
		if err != nil {
			errCh <- err
			return
		}
		done <- out
	}()

	select {
	case <-progressWritten:
	case err := <-errCh:
		t.Fatalf("run returned early error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for progress status")
	}
	runs, err := service.ListRuns(1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v %#v", err, runs)
	}
	status, err := service.ReadRunStatus(runs[0].RunID)
	if err != nil {
		t.Fatalf("read run status: %v", err)
	}
	got := status.Residents[0]
	if got.CurrentPhase != "model_stream" || got.CurrentRound != 2 || got.RemainingSec != 25 {
		t.Fatalf("unexpected progress status: %#v", got)
	}
	if got.LastAction != "guest_exec" || got.LastResponseID != "resp_test" || got.InFlightStartedAt == "" {
		t.Fatalf("expected live progress details, got %#v", got)
	}
	if got.TotalInputTokens != 100 || got.TotalCachedTokens != 64 || got.TotalOutputTokens != 7 {
		t.Fatalf("unexpected token progress: %#v", got)
	}
	if got.SummaryPane == nil || got.SummaryPane.Text != "我前面确认了机器状态。" || got.SummaryPane.EvidenceRefs[0].Ref != "round-1" {
		t.Fatalf("expected summary pane to be exposed on resident progress, got %#v", got.SummaryPane)
	}

	close(release)
	select {
	case err := <-errCh:
		t.Fatalf("run returned error: %v", err)
	case out := <-done:
		if len(out.Runs) != 1 || out.Runs[0].Status != "ok" {
			t.Fatalf("unexpected run output: %#v", out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for run to finish")
	}
	finalStatus, err := service.ReadRunStatus(runs[0].RunID)
	if err != nil {
		t.Fatalf("read final run status: %v", err)
	}
	final := finalStatus.Residents[0]
	if final.Status != "finished" || final.CurrentPhase != "finished" || final.InFlightStartedAt != "" {
		t.Fatalf("expected final status to clear in-flight state, got %#v", final)
	}
	if final.LastRoundFinishedAt == "" || final.TotalInputTokens != 180 {
		t.Fatalf("expected final progress totals to remain visible, got %#v", final)
	}
}

func TestResidentProgressAcceptsMeasuredZeroCacheTokens(t *testing.T) {
	root := t.TempDir()
	service := New(broker.New(root), &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	status := RunStatus{
		RunID:  "zero-cache-transition",
		Status: "running",
		Residents: []ResidentRunStatus{{
			Resident:     "jade",
			Status:       "running",
			CachedTokens: 64,
		}},
	}
	service.updateResidentProgress(&status, nil, "jade", newborn.ProgressEvent{
		Phase:             "model_stream_done",
		InputTokens:       100,
		CachedTokens:      0,
		OutputTokens:      7,
		TokenUsagePresent: true,
	})
	if status.Residents[0].CachedTokens != 0 {
		t.Fatalf("measured zero cache value remained stale: %#v", status.Residents[0])
	}
}

func TestServiceUsesResidentSpecificAPIKeys(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	root := t.TempDir()
	t.Setenv("JADE_OPENAI_API_KEY", "jade-key")
	t.Setenv("AMBER_OPENAI_API_KEY", "amber-key")
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "fallback-key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	seen := map[string]string{}
	service.endpointRunnerFactory = func(client *http.Client, endpoints []openai.Endpoint, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			if len(endpoints) > 0 {
				seen[profile.Name] = endpoints[0].APIKey
			}
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 1}, nil
		})
	}

	if _, err := service.Run(RunInput{
		Residents: []string{"jade", "amber", "onyx"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	}); err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if seen["jade"] != "jade-key" || seen["amber"] != "amber-key" || seen["onyx"] != "fallback-key" {
		t.Fatalf("unexpected resident api keys: %#v", seen)
	}
}

func TestResidentEndpointsSupportsPerResidentPrimaryAndBackups(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	t.Setenv("OPENAI_BASE_URL_2", "http://global-backup.invalid")
	t.Setenv("OPENAI_API_KEY_2", "global-backup-key")
	t.Setenv("JADE_OPENAI_BASE_URL", "http://jade-primary.invalid")
	t.Setenv("JADE_OPENAI_API_KEY", "jade-primary-key")
	t.Setenv("JADE_OPENAI_BASE_URL_2", "http://jade-backup1.invalid")
	t.Setenv("JADE_OPENAI_API_KEY_2", "jade-backup1-key")
	t.Setenv("JADE_OPENAI_CHANNEL_COMMENT_2", "zz1-Pro")
	t.Setenv("JADE_OPENAI_BASE_URL_3", "http://jade-backup2.invalid")
	t.Setenv("JADE_OPENAI_API_KEY_3", "jade-backup2-key")

	jade := ResidentEndpoints("jade", "http://fallback-primary.invalid", "fallback-primary-key")
	if len(jade) != 5 {
		t.Fatalf("expected jade personal chain plus global fallbacks, got %#v", jade)
	}
	if jade[0].Name != "primary" || jade[0].BaseURL != "http://jade-primary.invalid" || jade[0].APIKey != "jade-primary-key" {
		t.Fatalf("unexpected jade primary: %#v", jade[0])
	}
	if jade[1].Name != "backup_1" || jade[1].BaseURL != "http://jade-backup1.invalid" || jade[1].APIKey != "jade-backup1-key" {
		t.Fatalf("unexpected jade backup 1: %#v", jade[1])
	}
	if jade[1].Comment != "zz1-Pro" {
		t.Fatalf("unexpected jade backup 1 comment: %#v", jade[1])
	}
	if jade[2].Name != "backup_2" || jade[2].BaseURL != "http://jade-backup2.invalid" || jade[2].APIKey != "jade-backup2-key" {
		t.Fatalf("unexpected jade backup 2: %#v", jade[2])
	}
	if jade[3].Name != "global_fallback_primary" || jade[3].BaseURL != "http://fallback-primary.invalid" || jade[3].APIKey != "fallback-primary-key" {
		t.Fatalf("unexpected jade global fallback primary: %#v", jade[3])
	}
	if jade[4].Name != "global_fallback_backup_1" || jade[4].BaseURL != "http://global-backup.invalid" || jade[4].APIKey != "global-backup-key" {
		t.Fatalf("unexpected jade global fallback backup: %#v", jade[4])
	}

	onyx := ResidentEndpoints("onyx", "http://fallback-primary.invalid", "fallback-primary-key")
	if len(onyx) != 2 {
		t.Fatalf("expected onyx fallback primary plus global backup, got %#v", onyx)
	}
	if onyx[0].Name != "primary" || onyx[0].BaseURL != "http://fallback-primary.invalid" || onyx[0].APIKey != "fallback-primary-key" {
		t.Fatalf("unexpected onyx primary: %#v", onyx[0])
	}
	if onyx[1].Name != "backup_1" || onyx[1].BaseURL != "http://global-backup.invalid" || onyx[1].APIKey != "global-backup-key" {
		t.Fatalf("unexpected onyx backup: %#v", onyx[1])
	}
}

func TestResidentEndpointsPreferPersonalChainBeforeGlobalFallback(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://global-primary.invalid")
	t.Setenv("OPENAI_API_KEY", "global-primary-key")
	t.Setenv("OPENAI_BASE_URL_2", "http://global-backup.invalid")
	t.Setenv("OPENAI_API_KEY_2", "global-backup-key")
	t.Setenv("JADE_OPENAI_API_KEY_2", "jade-backup-key")
	t.Setenv("JADE_OPENAI_CHANNEL_COMMENT_2", "zz1-Pro")

	endpoints := ResidentEndpoints("jade", "http://fallback-primary.invalid", "fallback-primary-key")
	if len(endpoints) != 3 {
		t.Fatalf("expected personal backup plus global fallbacks, got %#v", endpoints)
	}
	if endpoints[0].Name != "backup_1" || endpoints[0].APIKey != "jade-backup-key" || endpoints[0].BaseURL != "http://fallback-primary.invalid" {
		t.Fatalf("expected personal backup first, got %#v", endpoints)
	}
	if endpoints[0].Comment != "zz1-Pro" {
		t.Fatalf("expected personal channel comment, got %#v", endpoints[0])
	}
	if endpoints[1].Name != "global_fallback_primary" || endpoints[1].APIKey != "fallback-primary-key" || endpoints[1].BaseURL != "http://fallback-primary.invalid" {
		t.Fatalf("expected global primary after personal chain, got %#v", endpoints[1])
	}
	if endpoints[2].Name != "global_fallback_backup_1" || endpoints[2].APIKey != "global-backup-key" || endpoints[2].BaseURL != "http://global-backup.invalid" {
		t.Fatalf("expected global backup last, got %#v", endpoints[2])
	}
}

func TestResidentEndpointsUseGlobalPrimaryWhenNoPersonalKeyExists(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://global-primary.invalid")
	t.Setenv("OPENAI_API_KEY", "global-primary-key")

	endpoints := ResidentEndpoints("newcomer", "http://fallback-primary.invalid", "fallback-primary-key")
	if len(endpoints) != 1 {
		t.Fatalf("expected one global endpoint, got %#v", endpoints)
	}
	if endpoints[0].Name != "primary" || endpoints[0].APIKey != "fallback-primary-key" || endpoints[0].BaseURL != "http://fallback-primary.invalid" {
		t.Fatalf("expected global primary for newcomer, got %#v", endpoints[0])
	}
}

func TestResidentEndpointsBackupKeyReusesResidentPrimaryBaseURL(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	t.Setenv("JADE_OPENAI_BASE_URL", "http://jade-primary.invalid")
	t.Setenv("JADE_OPENAI_API_KEY", "jade-primary-key")
	t.Setenv("JADE_OPENAI_API_KEY_2", "jade-backup-key")

	endpoints := ResidentEndpoints("jade", "http://fallback-primary.invalid", "fallback-primary-key")
	if len(endpoints) != 3 {
		t.Fatalf("expected jade primary and backup, got %#v", endpoints)
	}
	if endpoints[1].BaseURL != "http://jade-primary.invalid" || endpoints[1].APIKey != "jade-backup-key" {
		t.Fatalf("expected backup key to reuse resident primary base URL, got %#v", endpoints[1])
	}
}

func TestServicePassesResidentEndpointListToRunner(t *testing.T) {
	clearOpenAIEndpointEnv(t)
	root := t.TempDir()
	t.Setenv("JADE_OPENAI_BASE_URL", "http://jade-primary.invalid")
	t.Setenv("JADE_OPENAI_API_KEY", "jade-primary-key")
	t.Setenv("JADE_OPENAI_BASE_URL_2", "http://jade-backup.invalid")
	t.Setenv("JADE_OPENAI_API_KEY_2", "jade-backup-key")
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://fallback.invalid", "fallback-key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	var seen []openai.Endpoint
	service.endpointRunnerFactory = func(client *http.Client, endpoints []openai.Endpoint, resident string) Runner {
		seen = append([]openai.Endpoint(nil), endpoints...)
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 1}, nil
		})
	}

	if _, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	}); err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("expected two jade endpoints, got %#v", seen)
	}
	if seen[0].BaseURL != "http://jade-primary.invalid" || seen[1].BaseURL != "http://jade-backup.invalid" {
		t.Fatalf("unexpected endpoint order: %#v", seen)
	}
}

func clearOpenAIEndpointEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_CHANNEL_COMMENT",
		"OPENAI_BASE_URL_2",
		"OPENAI_API_KEY_2",
		"OPENAI_CHANNEL_COMMENT_2",
		"OPENAI_BASE_URL_3",
		"OPENAI_API_KEY_3",
		"OPENAI_CHANNEL_COMMENT_3",
	}
	for _, resident := range []string{"JADE", "AMBER", "ONYX"} {
		keys = append(keys,
			resident+"_OPENAI_BASE_URL",
			resident+"_OPENAI_API_KEY",
			resident+"_OPENAI_CHANNEL_COMMENT",
			resident+"_OPENAI_BASE_URL_2",
			resident+"_OPENAI_API_KEY_2",
			resident+"_OPENAI_CHANNEL_COMMENT_2",
			resident+"_OPENAI_BASE_URL_3",
			resident+"_OPENAI_API_KEY_3",
			resident+"_OPENAI_CHANNEL_COMMENT_3",
		)
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestServiceRunRejectsUnknownResident(t *testing.T) {
	app := broker.New(t.TempDir())
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade", "nobody"},
		Duration:  30 * time.Second,
		OutDir:    t.TempDir(),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if out.Runs[1].Status != "error" {
		t.Fatalf("expected unknown resident error, got %#v", out.Runs[1])
	}
}

func TestServiceRunWritesSummaryFile(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	path := filepath.Join(root, "orchestrator-runs", out.RunID, "summary.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected summary file at %s: %v", path, err)
	}
	statusPath := filepath.Join(root, "orchestrator-runs", out.RunID, "run-status.json")
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("expected run status file at %s: %v", statusPath, err)
	}
	residentPath := filepath.Join(root, "orchestrator-runs", out.RunID, "resident-jade.json")
	if _, err := os.Stat(residentPath); err != nil {
		t.Fatalf("expected resident status file at %s: %v", residentPath, err)
	}
}

func TestServiceReadRunStatusAndSummary(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	status, err := service.ReadRunStatus(out.RunID)
	if err != nil {
		t.Fatalf("read run status: %v", err)
	}
	if status.RunID != out.RunID || status.Status == "" {
		t.Fatalf("unexpected run status: %#v", status)
	}
	summary, err := service.ReadRunSummary(out.RunID)
	if err != nil {
		t.Fatalf("read run summary: %v", err)
	}
	if summary.RunID != out.RunID || len(summary.Runs) != 1 {
		t.Fatalf("unexpected run summary: %#v", summary)
	}
	report, err := service.ReadInspectionReport(out.RunID)
	if err != nil {
		t.Fatalf("read inspection report: %v", err)
	}
	if report.RunID != out.RunID || len(report.Residents) != 1 {
		t.Fatalf("unexpected inspection report: %#v", report)
	}
}

func TestServiceListRuns(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	for _, resident := range []string{"jade", "amber"} {
		if _, err := service.Run(RunInput{
			Residents: []string{resident},
			Duration:  30 * time.Second,
			OutDir:    filepath.Join(root, "out"),
			Mode:      RunModeSequential,
		}); err != nil {
			t.Fatalf("run orchestrator for %s: %v", resident, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	items, err := service.ListRuns(10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 runs, got %#v", items)
	}
	if items[0].RunID == "" || len(items[0].Residents) != 1 {
		t.Fatalf("unexpected run record: %#v", items[0])
	}
}

func TestServiceRunIDsDoNotCollideWithinSameSecond(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return runner
	}

	first, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := service.Run(RunInput{
		Residents: []string{"amber"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first.RunID == second.RunID {
		t.Fatalf("expected distinct run ids, got %q", first.RunID)
	}
}

func TestPauseAndResumeRunStatus(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			if calls.Add(1) == 1 {
				started <- struct{}{}
				<-release
			}
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 1}, nil
		})
	}

	done := make(chan RunSummary, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := service.Run(RunInput{
			Residents: []string{"jade", "amber"},
			Duration:  30 * time.Second,
			OutDir:    filepath.Join(root, "out"),
			Mode:      RunModeSequential,
		})
		if err != nil {
			errCh <- err
			return
		}
		done <- out
	}()

	<-started
	runs, err := service.ListRuns(1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v %#v", err, runs)
	}
	runID := runs[0].RunID

	if _, err := service.PauseRun(runID); err != nil {
		t.Fatalf("pause run: %v", err)
	}
	status, err := service.ReadRunStatus(runID)
	if err != nil {
		t.Fatalf("read paused status: %v", err)
	}
	if status.Status != "paused" {
		t.Fatalf("expected paused status, got %#v", status)
	}
	if status.PausedAt == "" {
		t.Fatalf("expected paused_at in status: %#v", status)
	}
	if !hasRunEvent(status.Events, "paused") {
		t.Fatalf("expected paused event, got %#v", status.Events)
	}

	close(release)
	time.Sleep(150 * time.Millisecond)
	if _, err := service.ResumeRun(runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("run returned error: %v", err)
	case out := <-done:
		if len(out.Runs) != 2 {
			t.Fatalf("unexpected run output: %#v", out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for resumed run to finish")
	}
	finalStatus, err := service.ReadRunStatus(runID)
	if err != nil {
		t.Fatalf("read final status: %v", err)
	}
	if finalStatus.PausedAt == "" || finalStatus.ResumedAt == "" {
		t.Fatalf("expected pause/resume timestamps in final status: %#v", finalStatus)
	}
	for _, eventType := range []string{"started", "paused", "resumed", "finished"} {
		if !hasRunEvent(finalStatus.Events, eventType) {
			t.Fatalf("expected %s event in final status, got %#v", eventType, finalStatus.Events)
		}
	}
}

func TestRetryFailedRun(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")

	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			if profile.Name == "amber" {
				return newborn.FinalReport{}, os.ErrInvalid
			}
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 1}, nil
		})
	}

	first, err := service.Run(RunInput{
		Residents: []string{"jade", "amber"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.RunID == "" {
		t.Fatalf("expected first run id")
	}

	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 2}, nil
		})
	}

	retried, err := service.RetryFailedRun(first.RunID)
	if err != nil {
		t.Fatalf("retry failed run: %v", err)
	}
	if retried.Contract.RetryOf != first.RunID {
		t.Fatalf("expected retry_of %q, got %#v", first.RunID, retried.Contract)
	}
	retriedStatus, err := service.ReadRunStatus(retried.RunID)
	if err != nil {
		t.Fatalf("read retried status: %v", err)
	}
	if len(retriedStatus.Events) == 0 || retriedStatus.Events[0].RetryOf != first.RunID {
		t.Fatalf("expected retry lineage in status events, got %#v", retriedStatus.Events)
	}
	if len(retried.Runs) != 1 || retried.Runs[0].Resident != "amber" || retried.Runs[0].Status != "ok" {
		t.Fatalf("unexpected retried runs: %#v", retried.Runs)
	}
	items, err := service.ListRuns(10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	foundRetry := false
	for _, item := range items {
		if item.RunID == retried.RunID && item.RetryOf == first.RunID {
			foundRetry = true
			break
		}
	}
	if !foundRetry {
		t.Fatalf("expected retry lineage in list output, got %#v", items)
	}
}

func TestServiceClassifiesRetryableUpstreamErrorsAsTransientBlocked(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			return newborn.FinalReport{}, &openai.APIError{
				StatusCode: http.StatusTooManyRequests,
				Body:       `{"error":{"message":"Too many pending requests"}}`,
				Retryable:  true,
			}
		})
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if out.Runs[0].Status != "transient_blocked" {
		t.Fatalf("expected transient_blocked run, got %#v", out.Runs[0])
	}
	status, err := service.ReadRunStatus(out.RunID)
	if err != nil {
		t.Fatalf("read run status: %v", err)
	}
	if status.Status != "finished_with_transient_blocks" || !status.Residents[0].TransientBlocked {
		t.Fatalf("unexpected transient status: %#v", status)
	}
	report, err := service.ReadInspectionReport(out.RunID)
	if err != nil {
		t.Fatalf("read inspection report: %v", err)
	}
	if report.ResidentsErrored != 0 || report.TransientBlocked != 1 || !report.Residents[0].TransientBlocked {
		t.Fatalf("unexpected inspection report: %#v", report)
	}
}

func TestServiceClassifiesInsufficientAccountBalanceAsTransientBlocked(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			return newborn.FinalReport{}, &openai.APIError{
				StatusCode: http.StatusForbidden,
				Body:       `{"error":{"message":"Insufficient account balance"}}`,
			}
		})
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if out.Runs[0].Status != "transient_blocked" {
		t.Fatalf("expected transient_blocked run, got %#v", out.Runs[0])
	}
	report, err := service.ReadInspectionReport(out.RunID)
	if err != nil {
		t.Fatalf("read inspection report: %v", err)
	}
	if report.ResidentsErrored != 0 || report.TransientBlocked != 1 {
		t.Fatalf("unexpected inspection report: %#v", report)
	}
}

func TestServiceKeepsPartialReportForTransientBlockedRun(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			report := newborn.FinalReport{
				Resident:      profile.Name,
				Model:         profile.Model,
				Rounds:        3,
				StoppedReason: "upstream_request_failed: round_4",
			}
			return report, &newborn.PartialRunError{
				Report: report,
				Err: &openai.APIError{
					StatusCode: http.StatusTooManyRequests,
					Body:       "too many pending requests",
					Retryable:  true,
				},
			}
		})
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if out.Runs[0].Status != "transient_blocked" || out.Runs[0].Report == nil || out.Runs[0].Report.Rounds != 3 {
		t.Fatalf("expected transient run with partial report, got %#v", out.Runs[0])
	}
	if out.Assessment.UsefulRuns != 1 || out.Assessment.BudgetBlockedRuns != 0 {
		t.Fatalf("expected partial report to count as useful, got %#v", out.Assessment)
	}
	report, err := service.ReadInspectionReport(out.RunID)
	if err != nil {
		t.Fatalf("read inspection report: %v", err)
	}
	if report.TransientBlocked != 1 || report.UsefulRuns != 1 || report.Residents[0].Rounds != 3 {
		t.Fatalf("unexpected inspection report: %#v", report)
	}
}

func TestRetryFailedRunIncludesTransientBlockedResidents(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")

	shouldFail := true
	service.runnerFactory = func(client *http.Client, baseURL, apiKey, resident string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			if shouldFail {
				return newborn.FinalReport{}, &openai.APIError{
					StatusCode: http.StatusTooManyRequests,
					Body:       "too many pending requests",
					Retryable:  true,
				}
			}
			return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 2}, nil
		})
	}

	first, err := service.Run(RunInput{
		Residents: []string{"jade"},
		Duration:  30 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Runs[0].Status != "transient_blocked" {
		t.Fatalf("expected transient block, got %#v", first.Runs)
	}

	shouldFail = false
	retried, err := service.RetryFailedRun(first.RunID)
	if err != nil {
		t.Fatalf("retry transient blocked run: %v", err)
	}
	if retried.Contract.RetryOf != first.RunID || len(retried.Runs) != 1 || retried.Runs[0].Status != "ok" {
		t.Fatalf("unexpected retry output: %#v", retried)
	}
}

func hasRunEvent(events []RunEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

type RunnerFunc func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error)

func (fn RunnerFunc) Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
	return fn(profile, duration, outDir, verbose, resetResident)
}

type progressRunner struct {
	progress   func(newborn.ProgressEvent)
	onProgress func(newborn.ProgressEvent)
}

func (r *progressRunner) SetProgressSink(fn func(newborn.ProgressEvent)) {
	r.progress = fn
}

func (r *progressRunner) emit(event newborn.ProgressEvent) {
	if r.progress != nil {
		r.progress(event)
	}
	if r.onProgress != nil {
		r.onProgress(event)
	}
}

func (r *progressRunner) Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
	r.emit(newborn.ProgressEvent{
		Phase:             "model_stream",
		Round:             2,
		RemainingSec:      25,
		Action:            "guest_exec",
		ResponseID:        "resp_test",
		InFlightStartedAt: "2026-06-21T12:00:00Z",
		TotalInputTokens:  100,
		TotalCachedTokens: 64,
		TotalOutputTokens: 7,
		SummaryPane: &newborn.SummaryPane{
			Text:           "我前面确认了机器状态。",
			UpdatedAt:      "2026-07-13T12:00:00Z",
			RoundsAbsorbed: 1,
			ApproxTokens:   16,
			EvidenceRefs: []newborn.SummaryPaneEvidenceRef{{
				Kind:   "round",
				Ref:    "round-1",
				Rounds: []int{1},
			}},
		},
	})
	r.emit(newborn.ProgressEvent{
		Phase:               "round_finished",
		Round:               2,
		RemainingSec:        25,
		Action:              "guest_exec",
		ResponseID:          "resp_test",
		LastRoundFinishedAt: "2026-06-21T12:00:02Z",
		InputTokens:         80,
		CachedTokens:        72,
		OutputTokens:        5,
		TotalInputTokens:    180,
		TotalCachedTokens:   136,
		TotalOutputTokens:   12,
	})
	return newborn.FinalReport{Resident: profile.Name, Model: profile.Model, Rounds: 2}, nil
}
