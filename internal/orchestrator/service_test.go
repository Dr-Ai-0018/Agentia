package orchestrator

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtime/newborn"
)

type fakeRunner struct {
	report newborn.FinalReport
	err    error
	calls  []newborn.ResidentProfile
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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

func TestServiceRunParallelKeepsAllResidentStatuses(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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

func TestServiceRunRejectsUnknownResident(t *testing.T) {
	app := broker.New(t.TempDir())
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	runner := &fakeRunner{report: newborn.FinalReport{Rounds: 1}}
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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
}

func TestRetryFailedRun(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "key")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")

	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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

	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
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

type RunnerFunc func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error)

func (fn RunnerFunc) Run(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
	return fn(profile, duration, outDir, verbose, resetResident)
}
