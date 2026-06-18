package broker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRecentV0AcceptanceEvidenceSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "operations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir operations: %v", err)
	}
	raw := "" +
		`{"id":"old","check_id":"cpu_maintenance_regression","status":"passed","recorded_at":"2026-06-18T08:00:00Z"}` + "\n" +
		`{"id":"new","check_id":"disk_maintenance_regression","status":"passed","recorded_at":"2026-06-18T09:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "v0-acceptance-evidence-2026-06-18.jsonl"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	records, err := LoadRecentV0AcceptanceEvidence(root, 8)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %#v", records)
	}
	if records[0].ID != "new" || records[1].ID != "old" {
		t.Fatalf("expected newest first, got %#v", records)
	}
}

func TestLatestPassingV0AcceptanceEvidenceIgnoresFailed(t *testing.T) {
	records := []V0AcceptanceEvidenceRecord{
		{ID: "failed", CheckID: "cpu_maintenance_regression", Status: "failed", RecordedAt: "2026-06-18T09:00:00Z"},
		{ID: "passed", CheckID: "cpu_maintenance_regression", Status: "passed", RecordedAt: "2026-06-18T08:00:00Z"},
	}

	latest := latestPassingV0AcceptanceEvidence(records)
	record, ok := latest["cpu_maintenance_regression"]
	if !ok {
		t.Fatalf("expected passing evidence")
	}
	if record.ID != "passed" {
		t.Fatalf("expected failed evidence ignored, got %#v", record)
	}
}

func TestRunV0AcceptanceEvidenceDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	app := New(root)

	out, err := app.RunV0AcceptanceEvidence(V0AcceptanceEvidenceInput{
		CheckID:  "cpu_maintenance_regression",
		Status:   "passed",
		Operator: "tester",
		Summary:  "dry run only",
		Apply:    false,
	}, testEvidenceTime())
	if err != nil {
		t.Fatalf("run evidence dry-run: %v", err)
	}
	if out.Apply || out.Path != "" {
		t.Fatalf("expected dry-run without path: %#v", out)
	}
	records, err := LoadRecentV0AcceptanceEvidence(root, 8)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no written evidence, got %#v", records)
	}
}

func TestRunV0AcceptanceEvidenceRejectsUnknownCheckID(t *testing.T) {
	app := New(t.TempDir())

	_, err := app.RunV0AcceptanceEvidence(V0AcceptanceEvidenceInput{
		CheckID: "typo_manual_check",
		Status:  "passed",
		Apply:   false,
	}, testEvidenceTime())
	if err == nil {
		t.Fatalf("expected unknown check id to fail")
	}
}

func TestRunV0AcceptanceEvidenceApplyWritesRecord(t *testing.T) {
	root := t.TempDir()
	app := New(root)

	out, err := app.RunV0AcceptanceEvidence(V0AcceptanceEvidenceInput{
		CheckID:  "checkpoint_cleanup_apply_regression",
		Status:   "passed",
		Operator: "tester",
		Summary:  "checkpoint cleanup apply reviewed",
		Evidence: []string{"dry-run reviewed", "apply preserved protected snapshots"},
		Command:  "arena-broker --mode checkpoint-cleanup --resident jade --keep 2 --apply",
		Apply:    true,
	}, testEvidenceTime())
	if err != nil {
		t.Fatalf("run evidence apply: %v", err)
	}
	if !out.Apply || out.Path == "" {
		t.Fatalf("expected applied evidence with path: %#v", out)
	}
	records, err := LoadRecentV0AcceptanceEvidence(root, 8)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one evidence record, got %#v", records)
	}
	if records[0].CheckID != "checkpoint_cleanup_apply_regression" || records[0].Status != "passed" {
		t.Fatalf("unexpected evidence record: %#v", records[0])
	}
}

func testEvidenceTime() time.Time {
	return time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
}
