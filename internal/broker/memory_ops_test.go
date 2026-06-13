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

	report, err := app.RunMemoryLifecycle("amber")
	if err != nil {
		t.Fatalf("run memory lifecycle: %v", err)
	}
	if report.Total != 1 || len(report.Items) != 1 || report.Items[0].ID != "amber-note" {
		t.Fatalf("unexpected lifecycle report: %#v", report)
	}
}
