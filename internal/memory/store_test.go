package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyDecisionPromotion(t *testing.T) {
	now := time.Now()
	record := Record{Layer: LayerShort, Status: StatusActive, CreatedAt: now.Add(-time.Hour)}
	updated := ApplyDecision(now, record, Decision{
		Action:      ActionPromote,
		TargetLayer: LayerLong,
		TTL:         24 * time.Hour,
		ReviewAfter: 12 * time.Hour,
	})

	if updated.Layer != LayerLong || updated.Status != StatusActive {
		t.Fatalf("unexpected state: %s %s", updated.Layer, updated.Status)
	}
	if updated.ExpiresAt.IsZero() || updated.ReviewAfter.IsZero() {
		t.Fatal("expected ttl and review timestamps")
	}
}

func TestMemoryStoreUpsertAbstractMemoryAndSnapshot(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()

	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "a",
			Layer:     LayerLong,
			Status:    StatusActive,
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
		Resident:       "onyx",
		Summary:        "long-term leverage lesson",
		ResidentText:   "I should stop paying for the false edge.",
		Semantic:       SemanticMemory{MemoryKind: "lesson", Salience: 4, RetentionIntent: "keep_long"},
		DecisionAction: ActionCreate,
		SourceGroupIDs: []string{"group-1"},
	})
	if err != nil {
		t.Fatalf("upsert abstract memory failed: %v", err)
	}

	err = store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "b",
			Layer:     LayerShort,
			Status:    StatusDeleted,
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		Resident:       "onyx",
		Summary:        "deleted note",
		DecisionAction: ActionDelete,
	})
	if err != nil {
		t.Fatalf("upsert abstract memory failed: %v", err)
	}

	records, err := store.ListAbstractMemories("onyx")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	snapshot := BuildSnapshot(records, 10)
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 active snapshot entry, got %d", len(snapshot))
	}
	if snapshot[0].ID != "a" {
		t.Fatalf("unexpected snapshot id: %s", snapshot[0].ID)
	}
}

func TestMemoryStoreUpsertHistoryGroup(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()

	err := store.UpsertHistoryGroup(HistoryGroup{
		GroupUUID:       "group-1",
		Resident:        "amber",
		CreatedAt:       now,
		ClosedAt:        now.Add(time.Hour),
		LastEventAt:     now.Add(time.Hour),
		SourceKind:      "dialogue_window",
		State:           HistoryGroupClosed,
		CloseReason:     "event_count_threshold",
		EventCount:      8,
		Tags:            []string{"failure", "admin_feedback"},
		SummaryHint:     "handoff structure changed",
		RawEventRefs:    []string{"evt-1", "evt-2"},
		ExtractedLayers: []string{"long"},
	})
	if err != nil {
		t.Fatalf("upsert history group failed: %v", err)
	}

	groups, err := store.ListHistoryGroups("amber")
	if err != nil {
		t.Fatalf("list history groups failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 history group, got %d", len(groups))
	}
	if groups[0].GroupUUID != "group-1" {
		t.Fatalf("unexpected history group uuid: %s", groups[0].GroupUUID)
	}
}

func TestFileStoreRoundTripBundle(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Now()

	am := AbstractMemory{
		Record: Record{
			ID:        "jade-memory-1",
			Layer:     LayerPermanent,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:        "jade",
		Summary:         "stable engineering law",
		ResidentText:    "I should keep the narrow recovery path close.",
		Semantic:        SemanticMemory{MemoryKind: "rule", TimeScope: "durable", RetentionIntent: "keep_permanent"},
		DecisionAction:  ActionCreate,
		SourceGroupIDs:  []string{"group-1"},
		ParentMemoryIDs: []string{},
	}
	if err := store.UpsertAbstractMemory(am); err != nil {
		t.Fatalf("file upsert abstract memory failed: %v", err)
	}

	group := HistoryGroup{
		GroupUUID:       "group-1",
		Resident:        "jade",
		CreatedAt:       now.Add(-time.Hour),
		ClosedAt:        now,
		LastEventAt:     now,
		SourceKind:      "dialogue_window",
		State:           HistoryGroupClosed,
		CloseReason:     "event_count_threshold",
		EventCount:      10,
		Tags:            []string{"failure"},
		SummaryHint:     "root cause discovered",
		RawEventRefs:    []string{"evt-1"},
		ExtractedLayers: []string{"permanent"},
	}
	if err := store.UpsertHistoryGroup(group); err != nil {
		t.Fatalf("file upsert history group failed: %v", err)
	}

	memories, err := store.ListAbstractMemories("jade")
	if err != nil {
		t.Fatalf("file list abstract memories failed: %v", err)
	}
	if len(memories) != 1 || memories[0].ID != "jade-memory-1" {
		t.Fatalf("unexpected abstract memories: %#v", memories)
	}
	if memories[0].ResidentText != "I should keep the narrow recovery path close." {
		t.Fatalf("unexpected resident text: %q", memories[0].ResidentText)
	}
	if memories[0].Semantic.RetentionIntent != "keep_permanent" {
		t.Fatalf("unexpected semantic retention_intent: %q", memories[0].Semantic.RetentionIntent)
	}

	groups, err := store.ListHistoryGroups("jade")
	if err != nil {
		t.Fatalf("file list history groups failed: %v", err)
	}
	if len(groups) != 1 || groups[0].GroupUUID != "group-1" {
		t.Fatalf("unexpected history groups: %#v", groups)
	}
}

func TestFileStoreNormalizesLegacyHistoryGroup(t *testing.T) {
	root := t.TempDir()
	raw := `{
  "history_groups": [
    {
      "group_uuid": "legacy-group",
      "resident": "jade",
      "created_at": "2026-06-02T10:30:00Z",
      "closed_at": "2026-06-02T18:00:00Z",
      "source_kind": "dialogue_window",
      "event_count": 5,
      "tags": ["scenario:baseline"],
      "summary_hint": "legacy group",
      "raw_event_refs": ["evt-1", "evt-2"]
    }
  ],
  "abstract_memories": []
}`
	if err := os.WriteFile(filepath.Join(root, "jade.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy bundle: %v", err)
	}

	store := NewFileStore(root)
	groups, err := store.ListHistoryGroups("jade")
	if err != nil {
		t.Fatalf("list groups failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].State != HistoryGroupClosed {
		t.Fatalf("expected normalized closed state, got %q", groups[0].State)
	}
	if groups[0].CloseReason == "" {
		t.Fatal("expected normalized close reason")
	}
	if groups[0].LastEventAt.IsZero() {
		t.Fatal("expected normalized last_event_at")
	}

	out, err := json.Marshal(groups[0])
	if err != nil {
		t.Fatalf("marshal normalized group: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected normalized group to marshal")
	}
}

func TestCompactResidentMergesDuplicateHistoryGroupsAndRemapsMemory(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Date(2026, 6, 2, 18, 0, 0, 0, time.UTC)

	g1 := HistoryGroup{
		GroupUUID:       "group-a",
		Resident:        "onyx",
		CreatedAt:       now.Add(-2 * time.Hour),
		ClosedAt:        now,
		LastEventAt:     now,
		SourceKind:      "dialogue_window",
		State:           HistoryGroupClosed,
		CloseReason:     "legacy_closed_group",
		EventCount:      5,
		Tags:            []string{"scenario:baseline", "layer:permanent"},
		SummaryHint:     "",
		RawEventRefs:    []string{"evt-1", "evt-2"},
		ExtractedLayers: nil,
	}
	g2 := HistoryGroup{
		GroupUUID:       "group-b",
		Resident:        "onyx",
		CreatedAt:       now.Add(-2 * time.Hour),
		ClosedAt:        now,
		LastEventAt:     now,
		SourceKind:      "dialogue_window",
		State:           HistoryGroupClosed,
		CloseReason:     "event_count_threshold",
		EventCount:      5,
		Tags:            []string{"category:failure"},
		SummaryHint:     "stronger summary",
		RawEventRefs:    []string{"evt-1", "evt-2"},
		ExtractedLayers: []string{"permanent"},
	}
	if err := store.UpsertHistoryGroup(g1); err != nil {
		t.Fatalf("upsert g1: %v", err)
	}
	if err := store.UpsertHistoryGroup(g2); err != nil {
		t.Fatalf("upsert g2: %v", err)
	}

	record := AbstractMemory{
		Record: Record{
			ID:        "m1",
			Layer:     LayerPermanent,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:       "onyx",
		Summary:        "memory",
		ResidentText:   "The real edge was narrower than the approval made it look.",
		Semantic:       SemanticMemory{MemoryKind: "lesson", EmotionTone: "wary", RetentionIntent: "keep_long"},
		DecisionAction: ActionUpdate,
		SourceGroupIDs: []string{"group-a", "group-b"},
	}
	if err := store.UpsertAbstractMemory(record); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	if err := store.CompactResident("onyx"); err != nil {
		t.Fatalf("compact resident: %v", err)
	}

	groups, err := store.ListHistoryGroups("onyx")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 compacted group, got %d", len(groups))
	}
	if groups[0].GroupUUID != "group-b" {
		t.Fatalf("expected stronger group-b to survive, got %q", groups[0].GroupUUID)
	}
	if groups[0].SummaryHint != "stronger summary" {
		t.Fatalf("expected summary to survive merge, got %q", groups[0].SummaryHint)
	}

	memories, err := store.ListAbstractMemories("onyx")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	if len(memories[0].SourceGroupIDs) != 1 || memories[0].SourceGroupIDs[0] != "group-b" {
		t.Fatalf("expected remapped source_group_ids to group-b, got %#v", memories[0].SourceGroupIDs)
	}
}

func TestAbstractMemoryEffectiveSummaryFallbacks(t *testing.T) {
	record := AbstractMemory{
		ResidentText: "resident-facing note",
	}
	if got := record.EffectiveSummary(); got != "resident-facing note" {
		t.Fatalf("expected resident_text fallback, got %q", got)
	}

	record.ResidentText = ""
	if got := record.EffectiveSummary(); got != "" {
		t.Fatalf("expected empty fallback with no resident_text, got %q", got)
	}
}
