package broker

import (
	"os"
	"path/filepath"
	"strings"
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

func TestV0AcceptanceEvidenceCommandTemplateUsesKnownCheckIDs(t *testing.T) {
	cmd := v0AcceptanceEvidenceCommandTemplate("disk_maintenance_regression")
	if cmd == "" {
		t.Fatalf("expected template for known check id")
	}
	if want := "--check-id disk_maintenance_regression"; !containsAll(cmd, "--mode v0-acceptance-evidence", want, "--apply") {
		t.Fatalf("unexpected command template: %q", cmd)
	}
	if got := v0AcceptanceEvidenceCommandTemplate("typo_manual_check"); got != "" {
		t.Fatalf("expected no template for unknown check id, got %q", got)
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

func TestRunV0AcceptanceEvidenceList(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	if _, err := app.RunV0AcceptanceEvidence(V0AcceptanceEvidenceInput{
		CheckID:  "cpu_maintenance_regression",
		Status:   "passed",
		Operator: "tester",
		Apply:    true,
	}, testEvidenceTime()); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	out, err := app.RunV0AcceptanceEvidenceList(8, testEvidenceTime())
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if out.Count != 1 || len(out.Records) != 1 {
		t.Fatalf("expected one listed evidence record: %#v", out)
	}
	if out.Records[0].CheckID != "cpu_maintenance_regression" {
		t.Fatalf("unexpected evidence record: %#v", out.Records[0])
	}
}

func TestRunV0AcceptanceEvidenceTemplateIsReadOnly(t *testing.T) {
	root := t.TempDir()
	acceptance := V0AcceptanceOutput{
		Source: "test",
		Checks: []V0AcceptanceCheck{
			evidenceTemplateCheck("longer_orchestrator_soak", v0AcceptanceWarn, false, false),
			evidenceTemplateCheck("cpu_maintenance_regression", v0AcceptancePending, true, true),
			evidenceTemplateCheck("disk_maintenance_regression", v0AcceptancePending, true, true),
			evidenceTemplateCheck("checkpoint_cleanup_apply_regression", v0AcceptancePending, true, true),
			evidenceTemplateCheck("final_acceptance_manual_pass", v0AcceptancePending, true, true),
			{ID: "operator_runbook", Category: "automatic", Status: v0AcceptancePass},
		},
	}

	out := BuildV0AcceptanceEvidenceTemplate(acceptance, "", testEvidenceTime())
	if out.Count != 5 || len(out.Templates) != 5 {
		t.Fatalf("expected five manual evidence templates, got %#v", out)
	}
	cpu, ok := findEvidenceTemplate(out, "cpu_maintenance_regression")
	if !ok {
		t.Fatalf("expected cpu maintenance template: %#v", out.Templates)
	}
	if !cpu.BlocksRelease || !cpu.RequiresApproval || cpu.Status != v0AcceptancePending {
		t.Fatalf("expected blocking approval template, got %#v", cpu)
	}
	if !containsAll(cpu.EvidenceCommand, "--mode v0-acceptance-evidence", "--check-id cpu_maintenance_regression", "--apply") {
		t.Fatalf("unexpected evidence command: %q", cpu.EvidenceCommand)
	}
	if !containsAll(strings.Join(cpu.Notes, "\n"), "operator approval", "release blocker") {
		t.Fatalf("expected approval and blocker notes: %#v", cpu.Notes)
	}
	records, err := LoadRecentV0AcceptanceEvidence(root, 8)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("template command must not write evidence, got %#v", records)
	}
}

func TestBuildV0AcceptanceEvidenceTemplateFiltersCheckID(t *testing.T) {
	acceptance := V0AcceptanceOutput{
		Source: "test",
		Checks: []V0AcceptanceCheck{
			evidenceTemplateCheck("cpu_maintenance_regression", v0AcceptancePending, true, true),
			evidenceTemplateCheck("disk_maintenance_regression", v0AcceptancePending, true, true),
		},
	}

	out := BuildV0AcceptanceEvidenceTemplate(acceptance, "disk_maintenance_regression", testEvidenceTime())
	if out.Count != 1 || len(out.Templates) != 1 {
		t.Fatalf("expected one filtered template, got %#v", out)
	}
	if out.Templates[0].CheckID != "disk_maintenance_regression" {
		t.Fatalf("unexpected filtered template: %#v", out.Templates[0])
	}
}

func evidenceTemplateCheck(checkID, status string, required, blocks bool) V0AcceptanceCheck {
	return V0AcceptanceCheck{
		ID:               checkID,
		Title:            "template " + checkID,
		Category:         "manual",
		Status:           status,
		Required:         required,
		BlocksRelease:    blocks,
		RequiresApproval: true,
		Evidence:         []string{"validation reason"},
		Command:          "arena-broker --mode validation",
		EvidenceCommand:  v0AcceptanceEvidenceCommandTemplate(checkID),
	}
}

func testEvidenceTime() time.Time {
	return time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
}

func findEvidenceTemplate(out V0AcceptanceEvidenceTemplateOutput, checkID string) (V0AcceptanceEvidenceTemplate, bool) {
	for _, item := range out.Templates {
		if item.CheckID == checkID {
			return item, true
		}
	}
	return V0AcceptanceEvidenceTemplate{}, false
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
