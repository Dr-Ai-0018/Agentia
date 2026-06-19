package broker

import "time"

func (a *App) RunV0Runbook(now time.Time) V0RunbookOutput {
	return BuildV0Runbook(now)
}

func BuildV0Runbook(now time.Time) V0RunbookOutput {
	return V0RunbookOutput{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Scope:       "operator_only_v0",
		Policy: []string{
			"Runbook output is operator-only and must not be pasted into resident world context as omniscient knowledge.",
			"World-internal Chenglin may only act on information learned through world-facing channels.",
			"Resource changes are maintenance-window operations: notify, stop/change/start when needed, complete or rollback, then refresh inventory.",
			"Readiness and runbook commands are descriptive; approval-required steps must not be executed automatically.",
			"Memory governance commands must preserve private/operator boundaries and avoid rewriting protected resident memory.",
		},
		Sections: []V0RunbookSection{
			v0PreflightRunbookSection(),
			v0OrchestratorRunbookSection(),
			v0HostDecisionRunbookSection(),
			v0MaintenanceRunbookSection(),
			v0BaselineRecoveryRunbookSection(),
			v0MemoryRunbookSection(),
			v0FinalAcceptanceRunbookSection(),
		},
	}
}

func v0PreflightRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "preflight",
		Title:   "Preflight and Current State",
		Purpose: "Establish a read-only view before any real resident, VM, or worldstate action.",
		Steps: []V0RunbookStep{
			{
				ID:      "readiness_cached",
				Title:   "Read cached v0 readiness and weighted completion",
				Command: "go run ./cmd/arena-broker --mode v0-readiness-cached --limit 5",
				Notes: []string{
					"Use cached mode for lightweight inspection.",
					"Check status, weighted_percent, release_gate, recommended_steps, and manual_validation_gaps.",
				},
			},
			{
				ID:      "host_inspect_cached",
				Title:   "Read cached host inspect summary",
				Command: "go run ./cmd/arena-broker --mode host-inspect-summary-cached --limit 5",
				Notes: []string{
					"Use this before live inventory refresh when the goal is quick operator review.",
					"Confirm pending chats, interventions, maintenance state, latest orchestrator, and memory governance counts.",
				},
			},
			{
				ID:      "memory_maintenance_summary",
				Title:   "Read memory governance summary",
				Command: "go run ./cmd/arena-broker --mode memory-maintenance",
				Notes: []string{
					"Dry-run only.",
					"Do not expose operator-only memory summaries to residents as world facts.",
				},
			},
		},
	}
}

func v0OrchestratorRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "orchestrator",
		Title:   "Long Run, Pause, Resume, Report",
		Purpose: "Run real multi-resident soak tests while keeping pause/resume and report inspection explicit.",
		Steps: []V0RunbookStep{
			{
				ID:               "long_parallel_soak",
				Title:            "Run approved multi-resident parallel soak",
				Command:          "go run ./cmd/arena-orchestrator --mode run --run-mode parallel --residents jade,amber,onyx --duration 10m --out-dir runs/orchestrator",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Requires resident API keys or OPENAI_API_KEY.",
					"This writes orchestrator run registry and lets residents act in their world loop.",
				},
			},
			{
				ID:      "list_runs",
				Title:   "List recent orchestrator runs",
				Command: "go run ./cmd/arena-orchestrator --mode list --limit 10",
			},
			{
				ID:      "run_status",
				Title:   "Inspect a run status",
				Command: "go run ./cmd/arena-orchestrator --mode status --run-id <run-id>",
			},
			{
				ID:            "pause_run",
				Title:         "Pause an active run at resident boundary",
				Command:       "go run ./cmd/arena-orchestrator --mode pause --run-id <run-id>",
				WritesRuntime: true,
				Notes: []string{
					"Pause does not promise to interrupt the active resident mid-step.",
					"It is expected to take effect at resident boundaries.",
				},
			},
			{
				ID:            "resume_run",
				Title:         "Resume a paused run",
				Command:       "go run ./cmd/arena-orchestrator --mode resume --run-id <run-id>",
				WritesRuntime: true,
			},
			{
				ID:      "run_summary",
				Title:   "Read run summary",
				Command: "go run ./cmd/arena-orchestrator --mode summary --run-id <run-id>",
			},
			{
				ID:      "inspection_report",
				Title:   "Read inspection report",
				Command: "go run ./cmd/arena-orchestrator --mode report --run-id <run-id>",
			},
			{
				ID:               "retry_failed",
				Title:            "Retry failed residents as a new lineage",
				Command:          "go run ./cmd/arena-orchestrator --mode retry-failed --run-id <run-id>",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Use after reviewing the original error and keeping the failed run record.",
					"Retry must not overwrite the original run output.",
				},
			},
		},
	}
}

func v0HostDecisionRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "host_decision",
		Title:   "Host Inspect and Decision Assist",
		Purpose: "Keep operator-only observations separate from world-facing followups.",
		Steps: []V0RunbookStep{
			{
				ID:      "decision_assist_cached",
				Title:   "Read host decision assist",
				Command: "go run ./cmd/arena-broker --mode host-decision-assist-cached --limit 5",
				Notes: []string{
					"Review actions and visibility before replying to residents.",
					"Pending chat previews are not a license to leak operator-only context.",
				},
			},
			{
				ID:      "maintenance_draft",
				Title:   "Generate side-effect-free maintenance drafts",
				Command: "go run ./cmd/arena-broker --mode host-maintenance-draft-cached --limit 5",
				Notes: []string{
					"Draft output is manual-only and does not create tickets, notices, or VM changes.",
				},
			},
			{
				ID:               "world_reply",
				Title:            "Reply to a resident through world-facing channel",
				Command:          "go run ./cmd/arena-broker --mode reply --resident <resident> --message-id <message-id> --body '<world-facing reply>'",
				RequiresApproval: true,
				WritesWorldState: true,
				Notes: []string{
					"Reply as world-internal Chenglin only with information available through world-facing context.",
				},
			},
		},
	}
}

func v0MaintenanceRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "maintenance",
		Title:   "Resource Maintenance and Rollback",
		Purpose: "Perform resource changes as explicit maintenance windows, not casual hot changes.",
		Steps: []V0RunbookStep{
			{
				ID:               "host_plan_maintenance",
				Title:            "Plan host-initiated maintenance without a resident ticket",
				Command:          "go run ./cmd/arena-broker --mode host-plan-maintenance --resident <resident> --resource <cpu|memory|disk> --amount <amount> --window '<window>' --operator <operator> --body '<maintenance notice>' --create-checkpoint",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Use when the host needs maintenance independent of a resident-submitted ticket.",
					"Creates a host intervention and maintenance run record; it does not perform VM stop/change/start by itself.",
				},
			},
			{
				ID:               "host_start_maintenance",
				Title:            "Mark host-initiated maintenance start",
				Command:          "go run ./cmd/arena-broker --mode host-start-maintenance --intervention-id <host-intervention-id> --resident <resident> --resource <cpu|memory|disk> --amount <amount> --operator <operator> --checkpoint-name <checkpoint-name> --body '<start note>'",
				RequiresApproval: true,
				WritesWorldState: true,
			},
			{
				ID:               "manual_stop_change_start",
				Title:            "Manual VM stop/change/start maintenance window",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Execute with host Incus/KVM tooling during the announced maintenance window.",
					"Do not treat KVM CPU/memory/disk changes as casual online hot changes.",
					"After boot, refresh inventory before completing the maintenance record.",
				},
			},
			{
				ID:               "host_complete_maintenance",
				Title:            "Complete host-initiated maintenance and notify resident",
				Command:          "go run ./cmd/arena-broker --mode host-complete-maintenance --intervention-id <host-intervention-id> --resident <resident> --resource <cpu|memory|disk> --amount <amount> --operator <operator> --checkpoint-name <checkpoint-name> --body '<completion note>'",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Updates the host intervention to completed and refreshes inventory.",
				},
			},
			{
				ID:               "host_fail_or_rollback",
				Title:            "Fail or rollback host-initiated maintenance",
				Command:          "go run ./cmd/arena-broker --mode host-rollback-maintenance --intervention-id <host-intervention-id> --resident <resident> --resource <cpu|memory|disk> --amount <amount> --operator <operator> --checkpoint-name <checkpoint-name> --body '<rollback note>'",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Use host-fail-maintenance if no rollback happened.",
					"Use host-rollback-maintenance only after an actual rollback or documented rollback procedure.",
				},
			},
			{
				ID:               "plan_ticket_maintenance",
				Title:            "Plan approved ticket maintenance",
				Command:          "go run ./cmd/arena-broker --mode ticket-plan-maintenance --resident <resident> --message-id <ticket-id> --window '<window>' --operator <operator> --create-checkpoint",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Creates a maintenance notice and may create a host checkpoint.",
					"Use only after reviewing the resident request and resource capacity.",
				},
			},
			{
				ID:      "checkpoint_cleanup_dry_run",
				Title:   "Dry-run host checkpoint cleanup",
				Command: "go run ./cmd/arena-broker --mode checkpoint-cleanup --resident <resident> --keep 2",
				Notes: []string{
					"Dry-run only when --apply is absent.",
					"Review protected baseline/self snapshots before any cleanup apply.",
				},
			},
			{
				ID:               "checkpoint_cleanup_apply",
				Title:            "Apply host checkpoint cleanup after review",
				Command:          "go run ./cmd/arena-broker --mode checkpoint-cleanup --resident <resident> --keep 2 --apply --operator <operator>",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Do not run without explicit approval.",
					"Only host checkpoint snapshots should be deleted.",
				},
			},
			{
				ID:            "refresh_inventory",
				Title:         "Refresh inventory after maintenance",
				Command:       "go run ./cmd/arena-broker --mode inventory",
				WritesRuntime: true,
			},
			{
				ID:               "complete_maintenance",
				Title:            "Complete ticket-based maintenance and notify resident",
				Command:          "go run ./cmd/arena-broker --mode ticket-complete-maintenance --resident <resident> --message-id <ticket-id> --operator <operator> --checkpoint-name <checkpoint-name> --body '<completion note>' --close-ticket",
				RequiresApproval: true,
				WritesWorldState: true,
			},
			{
				ID:               "fail_or_rollback",
				Title:            "Fail or rollback ticket-based maintenance if validation fails",
				Command:          "go run ./cmd/arena-broker --mode ticket-rollback-maintenance --resident <resident> --message-id <ticket-id> --operator <operator> --body '<rollback note>'",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Use fail when the maintenance could not complete.",
					"Use rollback only after an actual rollback or a clearly documented rollback plan has been followed.",
				},
			},
		},
	}
}

func v0MemoryRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "memory",
		Title:   "Memory Governance",
		Purpose: "Maintain memory without violating resident privacy or protected-memory boundaries.",
		Steps: []V0RunbookStep{
			{
				ID:      "memory_summary",
				Title:   "Inspect memory maintenance summary",
				Command: "go run ./cmd/arena-broker --mode memory-maintenance",
			},
			{
				ID:      "safe_lifecycle_dry_run",
				Title:   "Dry-run safe lifecycle candidates",
				Command: "go run ./cmd/arena-broker --mode memory-lifecycle-safe-apply --resident <resident>",
				Notes: []string{
					"Dry-run only without --apply.",
					"Review summaries before changing memory state.",
				},
			},
			{
				ID:               "safe_lifecycle_apply",
				Title:            "Apply reviewed safe lifecycle candidates",
				Command:          "go run ./cmd/arena-broker --mode memory-lifecycle-safe-apply --resident <resident> --apply",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Only applies candidates already categorized as safe decay.",
					"Do not rewrite or delete protected resident memory.",
				},
			},
			{
				ID:               "protected_memory_mark",
				Title:            "Mark protected resident memory for resident self-review",
				Command:          "go run ./cmd/arena-broker --mode memory-review --resident <resident> --memory-id <memory-id> --memory-action mark --reason '<reason>' --apply",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Host may mark protected memory for resident self-review.",
					"Host must not rewrite/delete/demote protected resident memory.",
				},
			},
		},
	}
}

func v0BaselineRecoveryRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "baseline_recovery",
		Title:   "Baseline, Checkpoint, Restore",
		Purpose: "Handle snapshot visibility, baseline recovery, and resident self-restore without blurring host and resident authority.",
		Steps: []V0RunbookStep{
			{
				ID:      "checkpoint_list",
				Title:   "List resident checkpoints before any cleanup or restore",
				Command: "go run ./cmd/arena-broker --mode checkpoint-list --resident <resident>",
				Notes: []string{
					"Read-only.",
					"Confirm baseline, host checkpoints, resident self snapshots, and unknown snapshots before any destructive action.",
				},
			},
			{
				ID:               "create_host_checkpoint",
				Title:            "Create a host checkpoint before risky maintenance or recovery",
				Command:          "go run ./cmd/arena-broker --mode checkpoint-create --resident <resident> --operator <operator>",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Creates checkpoint-<resident>-<timestamp>.",
					"Use before risky host maintenance or recovery work when rollback evidence matters.",
				},
			},
			{
				ID:               "resident_self_restore",
				Title:            "Resident-initiated self snapshot restore",
				Command:          "go run ./cmd/arena-broker --mode self-restore --resident <resident> --snapshot-name <self-snapshot-name> --reason '<resident-visible reason>'",
				RequiresApproval: true,
				WritesWorldState: true,
				WritesRuntime:    true,
				Notes: []string{
					"Use only when the restore is within resident-visible self-service authority.",
					"Do not use operator-only checkpoint knowledge as world-internal Chenglin context.",
				},
			},
			{
				ID:               "operator_baseline_restore",
				Title:            "Operator baseline or host checkpoint restore",
				Command:          "incus snapshot restore <instance> <clean-base-or-host-checkpoint>",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"Operator-only host action; announce maintenance or outage through world-facing channels when residents are affected.",
					"Prefer a fresh host checkpoint and inventory snapshot before restore.",
					"After restore, run inventory refresh and write a maintenance completion or rollback note if a resident-facing process exists.",
				},
			},
			{
				ID:               "broker_state_reset",
				Title:            "Reset broker-side resident state only when intentionally reseeding",
				Command:          "go run ./cmd/arena-broker --mode reset --resident <resident>",
				RequiresApproval: true,
				WritesRuntime:    true,
				Notes: []string{
					"This resets broker-side state, not the VM by itself.",
					"Do not use as routine recovery for a living resident without a clear operator decision.",
				},
			},
			{
				ID:      "post_restore_inventory",
				Title:   "Refresh host inventory after restore or reset",
				Command: "go run ./cmd/arena-broker --mode inventory",
				Notes: []string{
					"Required after any host restore or maintenance rollback that can change VM facts.",
				},
				WritesRuntime: true,
			},
		},
	}
}

func v0FinalAcceptanceRunbookSection() V0RunbookSection {
	return V0RunbookSection{
		ID:      "final_acceptance",
		Title:   "Final Acceptance",
		Purpose: "Declare v0 only after tests, readiness, and manual validation gaps are explicitly handled.",
		Steps: []V0RunbookStep{
			{
				ID:      "go_test_broker",
				Title:   "Run broker-focused tests",
				Command: "env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go test ./internal/broker ./cmd/arena-broker",
			},
			{
				ID:      "go_test_all",
				Title:   "Run full test suite",
				Command: "env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go test ./...",
			},
			{
				ID:      "final_readiness",
				Title:   "Run final readiness panel",
				Command: "go run ./cmd/arena-broker --mode v0-readiness-cached --limit 5",
				Notes: []string{
					"Check release_gate.",
					"Do not declare v0 complete if required failures or unapproved manual blockers remain.",
				},
			},
		},
	}
}
