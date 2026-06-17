package broker

import (
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/memory"
)

func TestRunMemoryCompactUsesAgentsMemoryStore(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"group-a", "group-b"} {
		if err := store.UpsertHistoryGroup(memory.HistoryGroup{
			GroupUUID:    id,
			Resident:     "amber",
			CreatedAt:    now,
			SourceKind:   "dialogue_window",
			State:        memory.HistoryGroupClosed,
			EventCount:   2,
			RawEventRefs: []string{"evt-1", "evt-2"},
		}); err != nil {
			t.Fatalf("upsert history group: %v", err)
		}
	}

	report, err := app.RunMemoryCompact("amber", false)
	if err != nil {
		t.Fatalf("run memory compact: %v", err)
	}
	if !report.Changed || report.BeforeHistoryGroups != 2 || report.AfterHistoryGroups != 1 {
		t.Fatalf("unexpected compact report: %#v", report)
	}
}

func TestRunMemoryLifecycleReportsStoredMemory(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC().Add(-2 * time.Hour)

	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-note",
			Layer:          memory.LayerLong,
			Status:         memory.StatusActive,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "stable useful note",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	report, err := app.RunMemoryLifecycle("amber", false)
	if err != nil {
		t.Fatalf("run memory lifecycle: %v", err)
	}
	if report.Total != 1 || len(report.Items) != 1 || report.Items[0].ID != "amber-note" {
		t.Fatalf("unexpected lifecycle report: %#v", report)
	}
}

func TestRunMemoryLifecycleSafeApplyDryRunDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC().Add(-96 * time.Hour)
	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-old-machine-fact",
			Layer:          memory.LayerShort,
			Status:         memory.StatusActive,
			Domain:         memory.DomainResources,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "temporary dns and https probe succeeded",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	report, err := app.RunMemoryLifecycleSafeApply("amber", false)
	if err != nil {
		t.Fatalf("run safe lifecycle dry-run: %v", err)
	}
	if report.Apply || report.CandidateCount != 1 || report.AppliedCount != 0 || report.SkippedCount != 0 {
		t.Fatalf("unexpected safe lifecycle dry-run report: %#v", report)
	}
	record, ok, err := store.GetAbstractMemory("amber", "amber-old-machine-fact")
	if err != nil || !ok {
		t.Fatalf("get memory after dry-run: ok=%v err=%v", ok, err)
	}
	if record.Status != memory.StatusActive || record.Layer != memory.LayerShort {
		t.Fatalf("dry-run mutated memory: %#v", record)
	}
}

func TestRunMemoryLifecycleSafeApplyOnlyDecaysSafeCandidates(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC().Add(-96 * time.Hour)
	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-old-machine-fact",
			Layer:          memory.LayerShort,
			Status:         memory.StatusActive,
			Domain:         memory.DomainResources,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "temporary dns and https probe succeeded",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert safe memory: %v", err)
	}
	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-old-relationship",
			Layer:          memory.LayerShort,
			Status:         memory.StatusActive,
			Domain:         memory.DomainRelationships,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "world-facing thread opened with Chenglin",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert skipped memory: %v", err)
	}

	report, err := app.RunMemoryLifecycleSafeApply("amber", true)
	if err != nil {
		t.Fatalf("run safe lifecycle apply: %v", err)
	}
	if !report.Apply || report.CandidateCount != 1 || report.AppliedCount != 1 || report.SkippedCount != 1 {
		t.Fatalf("unexpected safe lifecycle apply report: %#v", report)
	}
	if len(report.AppliedMemoryIDs) != 1 || report.AppliedMemoryIDs[0] != "amber-old-machine-fact" {
		t.Fatalf("unexpected applied ids: %#v", report.AppliedMemoryIDs)
	}
	safe, ok, err := store.GetAbstractMemory("amber", "amber-old-machine-fact")
	if err != nil || !ok {
		t.Fatalf("get safe memory after apply: ok=%v err=%v", ok, err)
	}
	if safe.Status != memory.StatusDecaying || safe.Layer != memory.LayerInstant {
		t.Fatalf("safe memory was not decayed: %#v", safe)
	}
	if safe.Governance.ReviewReason != "operator_safe_lifecycle_decay" {
		t.Fatalf("missing audit reason: %#v", safe.Governance)
	}
	skipped, ok, err := store.GetAbstractMemory("amber", "amber-old-relationship")
	if err != nil || !ok {
		t.Fatalf("get skipped memory after apply: ok=%v err=%v", ok, err)
	}
	if skipped.Status != memory.StatusActive || skipped.Layer != memory.LayerShort {
		t.Fatalf("skipped memory mutated: %#v", skipped)
	}
}

func TestRunMemoryLifecycleSafeApplySettlesStaleDecayingReviewSchedule(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC()
	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-decaying-machine-fact",
			Layer:          memory.LayerInstant,
			Status:         memory.StatusDecaying,
			Domain:         memory.DomainResources,
			CreatedAt:      now.Add(-96 * time.Hour),
			UpdatedAt:      now.Add(-96 * time.Hour),
			LastAccessedAt: now,
			ReviewAt:       now.Add(-24 * time.Hour),
			ReviewAfter:    now.Add(-48 * time.Hour),
			ExpiresAt:      now.Add(3 * time.Hour),
			HardExpiresAt:  now.Add(7 * time.Hour),
		},
		Resident:       "amber",
		Summary:        "temporary dns and https probe succeeded",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert decaying memory: %v", err)
	}

	report, err := app.RunMemoryLifecycleSafeApply("amber", true)
	if err != nil {
		t.Fatalf("run safe lifecycle apply: %v", err)
	}
	if report.CandidateCount != 1 || report.AppliedCount != 1 || report.SkippedCount != 0 {
		t.Fatalf("unexpected stale decaying apply report: %#v", report)
	}
	updated, ok, err := store.GetAbstractMemory("amber", "amber-decaying-machine-fact")
	if err != nil || !ok {
		t.Fatalf("get decaying memory after apply: ok=%v err=%v", ok, err)
	}
	if updated.Status != memory.StatusDecaying || updated.Layer != memory.LayerInstant {
		t.Fatalf("expected memory to remain decaying instant: %#v", updated.Record)
	}
	if !updated.ReviewAt.IsZero() || !updated.ReviewAfter.IsZero() {
		t.Fatalf("expected stale review schedule to be cleared: %#v", updated.Record)
	}
}

func TestRunMemoryMaintenanceSummaryAggregatesDryRuns(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC().Add(-96 * time.Hour)

	for _, id := range []string{"group-a", "group-b"} {
		if err := store.UpsertHistoryGroup(memory.HistoryGroup{
			GroupUUID:    id,
			Resident:     "amber",
			CreatedAt:    now,
			SourceKind:   "dialogue_window",
			State:        memory.HistoryGroupClosed,
			EventCount:   2,
			RawEventRefs: []string{"evt-1", "evt-2"},
		}); err != nil {
			t.Fatalf("upsert history group: %v", err)
		}
	}
	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "amber-instant-old",
			Layer:          memory.LayerInstant,
			Status:         memory.StatusActive,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "temporary note",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	summary := app.RunMemoryMaintenanceSummary()
	if summary.ApplyMode != "dry_run_only" {
		t.Fatalf("expected dry-run-only summary, got %#v", summary)
	}
	if summary.ResidentCount == 0 || summary.ResidentsAttention != 1 {
		t.Fatalf("unexpected resident counts: %#v", summary)
	}
	if summary.LifecycleAttention != 1 || summary.DuplicateHistoryGroups != 1 {
		t.Fatalf("unexpected maintenance totals: %#v", summary)
	}
	if summary.BeforeHistoryGroups != 2 || summary.AfterHistoryGroups != 1 {
		t.Fatalf("unexpected compaction totals: %#v", summary)
	}
	if len(summary.Residents) != 1 || summary.Residents[0].ResidentID != "amber" {
		t.Fatalf("expected amber maintenance row, got %#v", summary.Residents)
	}
	if summary.Residents[0].RecommendedAction != "lifecycle_then_compaction_dry_run" {
		t.Fatalf("unexpected recommendation: %#v", summary.Residents[0])
	}
}

func TestMemoryMaintenanceRecommendation(t *testing.T) {
	action, summary := memoryMaintenanceRecommendation(ResidentMemoryMaintenance{
		LifecycleAttention:     2,
		DuplicateHistoryGroups: 3,
	})
	if action != "lifecycle_then_compaction_dry_run" || summary == "" {
		t.Fatalf("unexpected combined recommendation: %q %q", action, summary)
	}

	action, _ = memoryMaintenanceRecommendation(ResidentMemoryMaintenance{LifecycleAttention: 1})
	if action != "lifecycle_dry_run" {
		t.Fatalf("unexpected lifecycle recommendation: %q", action)
	}

	action, _ = memoryMaintenanceRecommendation(ResidentMemoryMaintenance{DuplicateHistoryGroups: 1})
	if action != "compaction_dry_run" {
		t.Fatalf("unexpected compaction recommendation: %q", action)
	}
}
