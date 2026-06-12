package orchestrator

import (
	"net/http"
	"os"
	"path/filepath"
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
}
