package broker

import (
	"strings"
	"testing"
	"time"
)

func TestBuildV0AcceptanceBlocksOnManualValidation(t *testing.T) {
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	readiness := BuildV0Readiness(HostInspectSummary{
		ResidentCount:    3,
		ResidentsRunning: 3,
		Capacity:         HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 8, AllocatableFree: 4, Unit: "vcpu"}}},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-ok",
		},
	}, now, "test")

	out := BuildV0Acceptance(readiness, BuildV0Runbook(now), nil, now, "test")

	if out.Gate != "blocked_by_manual_validation" {
		t.Fatalf("expected manual validation blocker, got %#v", out)
	}
	if out.Summary.ManualBlocking != 4 || out.Summary.ApprovalRequired != 5 {
		t.Fatalf("expected manual blocking approval summary: %#v", out.Summary)
	}
	if !hasAcceptanceCheck(out, "host_only_maintenance_smoke", v0AcceptanceWarn) {
		t.Fatalf("expected host-only maintenance smoke warning check: %#v", out.Checks)
	}
	if !hasAcceptanceCheck(out, "operator_runbook", v0AcceptancePass) {
		t.Fatalf("expected operator runbook pass: %#v", out.Checks)
	}
	if !hasAcceptanceCheck(out, "cpu_maintenance_regression", v0AcceptancePending) ||
		!hasAcceptanceCheck(out, "disk_maintenance_regression", v0AcceptancePending) ||
		!hasAcceptanceCheck(out, "checkpoint_cleanup_apply_regression", v0AcceptancePending) ||
		!hasAcceptanceCheck(out, "final_acceptance_manual_pass", v0AcceptancePending) {
		t.Fatalf("expected split manual validation pending checks: %#v", out.Checks)
	}
	check, ok := findAcceptanceCheck(out, "cpu_maintenance_regression")
	if !ok {
		t.Fatalf("expected cpu maintenance check")
	}
	if !strings.Contains(check.EvidenceCommand, "--mode v0-acceptance-evidence") ||
		!strings.Contains(check.EvidenceCommand, "--check-id cpu_maintenance_regression") ||
		!strings.Contains(check.EvidenceCommand, "--apply") {
		t.Fatalf("expected record-evidence command template, got %#v", check.EvidenceCommand)
	}
	if check.Command != "arena-broker --mode v0-runbook" {
		t.Fatalf("expected cpu maintenance validation command to point at runbook, got %#v", check.Command)
	}
	if !strings.Contains(strings.Join(check.Evidence, "\n"), "host-plan/start/complete") {
		t.Fatalf("expected cpu maintenance evidence to mention host-only lifecycle, got %#v", check.Evidence)
	}
}

func TestBuildV0AcceptanceBlocksOnAutomaticFailure(t *testing.T) {
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	readiness := BuildV0Readiness(HostInspectSummary{
		ResidentsMissingInventory: 1,
		Capacity:                  HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "memory", AllocatableTotal: 8192, AllocatableFree: 0, Unit: "MiB"}}},
	}, now, "test")

	out := BuildV0Acceptance(readiness, BuildV0Runbook(now), nil, now, "test")

	if out.Gate != "blocked_by_automatic_failures" {
		t.Fatalf("expected automatic failure gate, got %#v", out)
	}
	if out.Summary.AutomaticFailed == 0 {
		t.Fatalf("expected automatic failures: %#v", out.Summary)
	}
	if !hasAcceptanceCheck(out, "inventory_facts", v0AcceptanceFail) {
		t.Fatalf("expected inventory failure check: %#v", out.Checks)
	}
}

func TestBuildV0AcceptanceFailsWithoutRunbook(t *testing.T) {
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	readiness := BuildV0Readiness(HostInspectSummary{
		ResidentCount:    3,
		ResidentsRunning: 3,
		Capacity:         HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "disk", AllocatableTotal: 100, AllocatableFree: 50, Unit: "GiB"}}},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-ok",
		},
	}, now, "test")

	out := BuildV0Acceptance(readiness, V0RunbookOutput{}, nil, now, "test")

	if out.Gate != "blocked_by_automatic_failures" {
		t.Fatalf("expected missing runbook to block automatically, got %#v", out)
	}
	if !hasAcceptanceCheck(out, "operator_runbook", v0AcceptanceFail) {
		t.Fatalf("expected operator runbook failure: %#v", out.Checks)
	}
}

func TestBuildV0AcceptancePassesManualChecksWithEvidence(t *testing.T) {
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	readiness := BuildV0Readiness(HostInspectSummary{
		ResidentCount:    3,
		ResidentsRunning: 3,
		Capacity:         HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "cpu", AllocatableTotal: 8, AllocatableFree: 4, Unit: "vcpu"}}},
		LatestOrchestrator: &OrchestratorInspectionDigest{
			RunID: "orchestrator-ok",
		},
	}, now, "test")

	evidence := []V0AcceptanceEvidenceRecord{
		passedAcceptanceEvidence("cpu_maintenance_regression", "2026-06-18T08:01:00Z"),
		passedAcceptanceEvidence("disk_maintenance_regression", "2026-06-18T08:02:00Z"),
		passedAcceptanceEvidence("checkpoint_cleanup_apply_regression", "2026-06-18T08:03:00Z"),
		passedAcceptanceEvidence("final_acceptance_manual_pass", "2026-06-18T08:04:00Z"),
	}
	out := BuildV0Acceptance(readiness, BuildV0Runbook(now), evidence, now, "test")

	if out.Gate != "ready_with_warnings" {
		t.Fatalf("expected manual evidence to clear blocker while automatic warnings remain, got %#v", out)
	}
	if out.Summary.ManualBlocking != 0 || out.Summary.ManualPending != 0 || out.Summary.EvidenceRecords != 4 {
		t.Fatalf("expected evidence to clear manual blockers: %#v", out.Summary)
	}
	if !hasAcceptanceCheck(out, "cpu_maintenance_regression", v0AcceptancePass) ||
		!hasAcceptanceCheck(out, "final_acceptance_manual_pass", v0AcceptancePass) {
		t.Fatalf("expected manual checks to pass with evidence: %#v", out.Checks)
	}
}

func passedAcceptanceEvidence(checkID, recordedAt string) V0AcceptanceEvidenceRecord {
	return V0AcceptanceEvidenceRecord{
		ID:         "evidence-" + checkID,
		CheckID:    checkID,
		Status:     "passed",
		RecordedAt: recordedAt,
		Operator:   "test-operator",
		Summary:    "test evidence",
		Evidence:   []string{"test evidence detail"},
	}
}

func hasAcceptanceCheck(out V0AcceptanceOutput, id, status string) bool {
	check, ok := findAcceptanceCheck(out, id)
	return ok && check.Status == status
}

func findAcceptanceCheck(out V0AcceptanceOutput, id string) (V0AcceptanceCheck, bool) {
	for _, check := range out.Checks {
		if check.ID == id {
			return check, true
		}
	}
	return V0AcceptanceCheck{}, false
}
