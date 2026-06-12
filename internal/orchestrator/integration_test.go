package orchestrator

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtime/newborn"
)

func TestEndToEndControlSurfaceWithoutOpenAI(t *testing.T) {
	root := t.TempDir()
	app := broker.New(root)
	service := New(app, &http.Client{}, "http://example.invalid", "")
	service.stateRoot = filepath.Join(root, "orchestrator-runs")
	service.runnerFactory = func(client *http.Client, baseURL, apiKey string) Runner {
		return RunnerFunc(func(profile newborn.ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (newborn.FinalReport, error) {
			return newborn.FinalReport{
				Resident:        profile.Name,
				Model:           profile.Model,
				Rounds:          2,
				StartedAt:       time.Now().UTC().Add(-duration).Format(time.RFC3339),
				EndedAt:         time.Now().UTC().Format(time.RFC3339),
				StoppedReason:   "duration_reached",
				DurationSeconds: int(duration.Seconds()),
			}, nil
		})
	}

	out, err := service.Run(RunInput{
		Residents: []string{"jade", "amber"},
		Duration:  2 * time.Second,
		OutDir:    filepath.Join(root, "out"),
		Mode:      RunModeSequential,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	items, err := service.ListRuns(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one run record, got %#v", items)
	}
	status, err := service.ReadRunStatus(out.RunID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != "finished" {
		t.Fatalf("unexpected status: %#v", status)
	}
	summary, err := service.ReadRunSummary(out.RunID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Runs) != 2 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	report, err := service.ReadInspectionReport(out.RunID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.ResidentsFinished != 2 || report.ResidentsErrored != 0 {
		t.Fatalf("unexpected inspection report: %#v", report)
	}
}
