package broker

import (
	"strings"
	"testing"
	"time"
)

func TestBuildV0RunbookIncludesOperatorOnlyPolicy(t *testing.T) {
	out := BuildV0Runbook(time.Date(2026, 6, 18, 7, 10, 0, 0, time.UTC))

	if out.Scope != "operator_only_v0" {
		t.Fatalf("unexpected scope: %#v", out)
	}
	if len(out.Sections) < 7 {
		t.Fatalf("expected core runbook sections: %#v", out.Sections)
	}
	if !policyContains(out.Policy, "operator-only") {
		t.Fatalf("expected operator-only policy: %#v", out.Policy)
	}
	if !policyContains(out.Policy, "maintenance-window") {
		t.Fatalf("expected maintenance-window policy: %#v", out.Policy)
	}
}

func TestBuildV0RunbookMarksRiskySteps(t *testing.T) {
	out := BuildV0Runbook(time.Date(2026, 6, 18, 7, 10, 0, 0, time.UTC))

	assertStepFlags(t, out, "long_parallel_soak", true, false, true)
	assertStepFlags(t, out, "host_plan_maintenance", true, true, true)
	assertStepFlags(t, out, "host_start_maintenance", true, true, false)
	assertStepFlags(t, out, "host_complete_maintenance", true, true, true)
	assertStepFlags(t, out, "host_fail_or_rollback", true, true, true)
	assertStepFlags(t, out, "checkpoint_cleanup_apply", true, false, true)
	assertStepFlags(t, out, "checkpoint_list", false, false, false)
	assertStepFlags(t, out, "create_host_checkpoint", true, false, true)
	assertStepFlags(t, out, "resident_self_restore", true, true, true)
	assertStepFlags(t, out, "operator_baseline_restore", true, false, true)
	assertStepFlags(t, out, "broker_state_reset", true, false, true)
	assertStepFlags(t, out, "world_reply", true, true, false)
	assertStepFlags(t, out, "safe_lifecycle_apply", true, false, true)
	assertStepFlags(t, out, "readiness_cached", false, false, false)
}

func TestBuildV0RunbookIncludesBaselineRecoveryBoundary(t *testing.T) {
	out := BuildV0Runbook(time.Date(2026, 6, 18, 7, 10, 0, 0, time.UTC))

	selfRestore, ok := findRunbookStep(out, "resident_self_restore")
	if !ok {
		t.Fatalf("expected resident self restore step: %#v", out.Sections)
	}
	if !strings.Contains(selfRestore.Command, "self-restore") || !stepNotesContain(selfRestore, "resident-visible") {
		t.Fatalf("expected resident-visible self restore boundary, got %#v", selfRestore)
	}
	operatorRestore, ok := findRunbookStep(out, "operator_baseline_restore")
	if !ok {
		t.Fatalf("expected operator baseline restore step: %#v", out.Sections)
	}
	if !strings.Contains(operatorRestore.Command, "incus snapshot restore") || !stepNotesContain(operatorRestore, "Operator-only") {
		t.Fatalf("expected operator-only baseline restore boundary, got %#v", operatorRestore)
	}
}

func TestBuildV0RunbookPrefersHostOnlyMaintenancePath(t *testing.T) {
	out := BuildV0Runbook(time.Date(2026, 6, 18, 7, 10, 0, 0, time.UTC))

	step, ok := findRunbookStep(out, "host_plan_maintenance")
	if !ok {
		t.Fatalf("expected host-only maintenance plan step: %#v", out.Sections)
	}
	if !strings.Contains(step.Command, "host-plan-maintenance") || strings.Contains(step.Command, "ticket-plan-maintenance") {
		t.Fatalf("expected host-only maintenance command, got %#v", step)
	}
	if !stepNotesContain(step, "independent of a resident-submitted ticket") {
		t.Fatalf("expected host-only maintenance note, got %#v", step.Notes)
	}
}

func TestBuildV0RunbookIncludesFinalAcceptance(t *testing.T) {
	out := BuildV0Runbook(time.Date(2026, 6, 18, 7, 10, 0, 0, time.UTC))

	if _, ok := findRunbookStep(out, "go_test_all"); !ok {
		t.Fatalf("expected full test step: %#v", out.Sections)
	}
	step, ok := findRunbookStep(out, "final_readiness")
	if !ok {
		t.Fatalf("expected final readiness step: %#v", out.Sections)
	}
	if !strings.Contains(step.Command, "v0-readiness-cached") {
		t.Fatalf("expected final readiness command, got %#v", step)
	}
}

func policyContains(policy []string, needle string) bool {
	for _, item := range policy {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}

func stepNotesContain(step V0RunbookStep, needle string) bool {
	for _, note := range step.Notes {
		if strings.Contains(note, needle) {
			return true
		}
	}
	return false
}

func assertStepFlags(t *testing.T, out V0RunbookOutput, id string, approval, worldWrite, runtimeWrite bool) {
	t.Helper()
	step, ok := findRunbookStep(out, id)
	if !ok {
		t.Fatalf("expected runbook step %s", id)
	}
	if step.RequiresApproval != approval || step.WritesWorldState != worldWrite || step.WritesRuntime != runtimeWrite {
		t.Fatalf("unexpected flags for %s: %#v", id, step)
	}
}

func findRunbookStep(out V0RunbookOutput, id string) (V0RunbookStep, bool) {
	for _, section := range out.Sections {
		for _, step := range section.Steps {
			if step.ID == id {
				return step, true
			}
		}
	}
	return V0RunbookStep{}, false
}
