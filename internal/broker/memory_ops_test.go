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
