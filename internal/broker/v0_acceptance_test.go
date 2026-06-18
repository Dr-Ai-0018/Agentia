package broker

import (
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

	out := BuildV0Acceptance(readiness, BuildV0Runbook(now), now, "test")

	if out.Gate != "blocked_by_manual_validation" {
		t.Fatalf("expected manual validation blocker, got %#v", out)
	}
	if out.Summary.ManualBlocking != 4 || out.Summary.ApprovalRequired != 4 {
		t.Fatalf("expected manual blocking approval summary: %#v", out.Summary)
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
}

func TestBuildV0AcceptanceBlocksOnAutomaticFailure(t *testing.T) {
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	readiness := BuildV0Readiness(HostInspectSummary{
		ResidentsMissingInventory: 1,
		Capacity:                  HostCapacityReport{Pools: []ResourcePoolSummary{{Resource: "memory", AllocatableTotal: 8192, AllocatableFree: 0, Unit: "MiB"}}},
	}, now, "test")

	out := BuildV0Acceptance(readiness, BuildV0Runbook(now), now, "test")

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

	out := BuildV0Acceptance(readiness, V0RunbookOutput{}, now, "test")

	if out.Gate != "blocked_by_automatic_failures" {
		t.Fatalf("expected missing runbook to block automatically, got %#v", out)
	}
	if !hasAcceptanceCheck(out, "operator_runbook", v0AcceptanceFail) {
		t.Fatalf("expected operator runbook failure: %#v", out.Checks)
	}
}

func hasAcceptanceCheck(out V0AcceptanceOutput, id, status string) bool {
	for _, check := range out.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}
