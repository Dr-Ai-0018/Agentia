package broker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLatestOrchestratorInspection(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260616T070957.673740074Z")
	newDir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260616T082449.311075354Z")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old run: %v", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "inspection-report.json"), []byte(`{"run_id":"old","residents_errored":1}`), 0o644); err != nil {
		t.Fatalf("write old report: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "inspection-report.json"), []byte(`{
		"run_id": "new",
		"mode": "parallel",
		"residents_finished": 3,
		"residents_errored": 0,
		"useful_runs": 3,
		"budget_blocked_runs": 2,
		"residents": [
			{"resident":"jade","status":"ok","rounds":2,"budget_blocked":true}
		]
	}`), 0o644); err != nil {
		t.Fatalf("write new report: %v", err)
	}

	out, err := LoadLatestOrchestratorInspection(root)
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if out == nil {
		t.Fatalf("expected latest report")
	}
	if out.RunID != "new" || out.BudgetBlockedRuns != 2 {
		t.Fatalf("unexpected latest report: %#v", out)
	}
	if len(out.Residents) != 1 || out.Residents[0].Resident != "jade" {
		t.Fatalf("unexpected latest residents: %#v", out.Residents)
	}
}

func TestLoadRecentOrchestratorInspections(t *testing.T) {
	root := t.TempDir()
	for _, runID := range []string{
		"orchestrator-20260616T070000.000000000Z",
		"orchestrator-20260616T080000.000000000Z",
		"orchestrator-20260616T090000.000000000Z",
	} {
		runDir := filepath.Join(root, "orchestrator-runs", runID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("mkdir run: %v", err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "inspection-report.json"), []byte(`{"run_id":"`+runID+`"}`), 0o644); err != nil {
			t.Fatalf("write report: %v", err)
		}
	}

	out, err := LoadRecentOrchestratorInspections(root, 2)
	if err != nil {
		t.Fatalf("load recent: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected two recent reports, got %#v", out)
	}
	if !strings.Contains(out[0].RunID, "090000") || !strings.Contains(out[1].RunID, "080000") {
		t.Fatalf("expected newest-first reports, got %#v", out)
	}
}

func TestLoadLatestOrchestratorInspectionSkipsBadNewestReport(t *testing.T) {
	root := t.TempDir()
	badDir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260616T090000.000000000Z")
	goodDir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260616T080000.000000000Z")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("mkdir bad run: %v", err)
	}
	if err := os.MkdirAll(goodDir, 0o755); err != nil {
		t.Fatalf("mkdir good run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "inspection-report.json"), []byte(`{bad json`), 0o644); err != nil {
		t.Fatalf("write bad report: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goodDir, "inspection-report.json"), []byte(`{"run_id":"good"}`), 0o644); err != nil {
		t.Fatalf("write good report: %v", err)
	}

	out, err := LoadLatestOrchestratorInspection(root)
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if out == nil || out.RunID != "good" {
		t.Fatalf("expected fallback to good report, got %#v", out)
	}
}

func TestLoadLatestOrchestratorInspectionMissingRoot(t *testing.T) {
	out, err := LoadLatestOrchestratorInspection(t.TempDir())
	if err != nil {
		t.Fatalf("missing root should not fail: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil without reports, got %#v", out)
	}
}

func TestLoadLatestOrchestratorInspectionFillsMissingRunID(t *testing.T) {
	root := t.TempDir()
	runID := "orchestrator-20260616T082449.311075354Z"
	runDir := filepath.Join(root, "orchestrator-runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "inspection-report.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	out, err := LoadLatestOrchestratorInspection(root)
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if out == nil || !strings.Contains(out.RunID, "20260616T082449") {
		t.Fatalf("expected run id fallback, got %#v", out)
	}
}
