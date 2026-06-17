package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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
	if updated.ExpiresAt.IsZero() || updated.ReviewAt.IsZero() {
		t.Fatal("expected ttl and review timestamps")
	}
	if updated.HardExpiresAt.IsZero() {
		t.Fatal("expected hard expiry timestamp")
	}
	if updated.LastConfirmedAt.IsZero() {
		t.Fatal("expected last confirmed timestamp")
	}
}

func TestApplyDecisionSetsShortHardExpiryAfterSoftExpiry(t *testing.T) {
	now := time.Now()
	record := Record{Layer: LayerShort}
	updated := ApplyDecision(now, record, Decision{
		Action:      ActionCreate,
		TargetLayer: LayerShort,
		TTL:         24 * time.Hour,
		ReviewAfter: 8 * time.Hour,
	})
	if !updated.HardExpiresAt.After(updated.ExpiresAt) {
		t.Fatalf("expected hard expiry after soft expiry, got hard=%v soft=%v", updated.HardExpiresAt, updated.ExpiresAt)
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
	if snapshot[0].Visibility != VisibilityResidentPrivate {
		t.Fatalf("expected legacy memory to normalize to resident-private visibility, got %q", snapshot[0].Visibility)
	}
}

func TestResidentDigestVisible(t *testing.T) {
	visible := []Visibility{
		"",
		VisibilityPublic,
		VisibilityRelationship,
		VisibilityResidentPrivate,
	}
	for _, visibility := range visible {
		if !ResidentDigestVisible(AbstractMemory{Visibility: visibility}) {
			t.Fatalf("expected %q to be resident digest visible", visibility)
		}
	}

	hidden := []Visibility{
		VisibilityPrivateJournal,
		VisibilitySystemAudit,
		VisibilityOperatorObservation,
	}
	for _, visibility := range hidden {
		if ResidentDigestVisible(AbstractMemory{Visibility: visibility}) {
			t.Fatalf("expected %q to be hidden from resident digest", visibility)
		}
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
			ID:              "jade-memory-1",
			Layer:           LayerPermanent,
			Status:          StatusActive,
			CreatedAt:       now,
			UpdatedAt:       now,
			LastAccessedAt:  now,
			LastConfirmedAt: now,
			ReviewAt:        now.Add(30 * 24 * time.Hour),
			ExpiresAt:       now.Add(90 * 24 * time.Hour),
			HardExpiresAt:   now.Add(365 * 24 * time.Hour),
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
	if memories[0].HardExpiresAt.IsZero() {
		t.Fatal("expected hard_expires_at to survive roundtrip")
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

func TestFileStoreDoesNotLeaveTempFiles(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Now()

	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "amber-memory-1",
			Layer:     LayerShort,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:       "amber",
		Summary:        "keep exploring system state",
		DecisionAction: ActionCreate,
	})
	if err != nil {
		t.Fatalf("upsert abstract memory: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(root, "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no temp files, got %#v", matches)
	}
}

func TestFileStoreConcurrentWritesDoNotConflictOnTempName(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Now()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"amber-memory-a", "amber-memory-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			errs <- store.UpsertAbstractMemory(AbstractMemory{
				Record: Record{
					ID:        id,
					Layer:     LayerShort,
					Status:    StatusActive,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Resident:       "amber",
				Summary:        id,
				DecisionAction: ActionCreate,
			})
		}(id)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write failed: %v", err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no temp files, got %#v", matches)
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

func TestCompactResidentWithReportDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Date(2026, 6, 2, 18, 0, 0, 0, time.UTC)

	g1 := HistoryGroup{
		GroupUUID:    "group-a",
		Resident:     "onyx",
		CreatedAt:    now,
		SourceKind:   "dialogue_window",
		State:        HistoryGroupClosed,
		EventCount:   2,
		RawEventRefs: []string{"evt-1", "evt-2"},
	}
	g2 := HistoryGroup{
		GroupUUID:    "group-b",
		Resident:     "onyx",
		CreatedAt:    now,
		SourceKind:   "dialogue_window",
		State:        HistoryGroupClosed,
		EventCount:   2,
		RawEventRefs: []string{"evt-1", "evt-2"},
	}
	if err := store.UpsertHistoryGroup(g1); err != nil {
		t.Fatalf("upsert g1: %v", err)
	}
	if err := store.UpsertHistoryGroup(g2); err != nil {
		t.Fatalf("upsert g2: %v", err)
	}

	report, err := store.CompactResidentWithReport("onyx", false)
	if err != nil {
		t.Fatalf("compact dry-run: %v", err)
	}
	if !report.Changed || report.BeforeHistoryGroups != 2 || report.AfterHistoryGroups != 1 || report.Apply {
		t.Fatalf("unexpected dry-run report: %#v", report)
	}
	if len(report.MergeGroups) != 1 {
		t.Fatalf("expected one merge detail, got %#v", report.MergeGroups)
	}
	if report.MergeGroups[0].KeepGroupUUID == "" || len(report.MergeGroups[0].DropGroupUUIDs) != 1 || len(report.MergeGroups[0].GroupUUIDs) != 2 {
		t.Fatalf("unexpected merge detail: %#v", report.MergeGroups[0])
	}
	if report.MergeGroups[0].RawEventRefCount != 2 {
		t.Fatalf("expected raw event ref count 2, got %#v", report.MergeGroups[0])
	}
	groups, err := store.ListHistoryGroups("onyx")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("dry-run should not write compacted bundle, got %d groups", len(groups))
	}
}

func TestLifecycleReportFlagsExpiredMemories(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	if err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "instant-old",
			Layer:          LayerInstant,
			Status:         StatusActive,
			CreatedAt:      now.Add(-8 * time.Hour),
			UpdatedAt:      now.Add(-8 * time.Hour),
			LastAccessedAt: now.Add(-8 * time.Hour),
			ExpiresAt:      now.Add(-2 * time.Hour),
			HardExpiresAt:  now.Add(-time.Hour),
		},
		Resident:       "amber",
		Summary:        "temporary note",
		DecisionAction: ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "long-fresh",
			Layer:          LayerLong,
			Status:         StatusActive,
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now.Add(-time.Hour),
			LastAccessedAt: now.Add(-time.Hour),
		},
		Resident:       "amber",
		Summary:        "stable useful note",
		DecisionAction: ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "relationship-old",
			Layer:          LayerShort,
			Domain:         DomainRelationships,
			Status:         StatusActive,
			CreatedAt:      now.Add(-96 * time.Hour),
			UpdatedAt:      now.Add(-96 * time.Hour),
			LastAccessedAt: now.Add(-96 * time.Hour),
			ExpiresAt:      now.Add(-48 * time.Hour),
			HardExpiresAt:  now.Add(-24 * time.Hour),
		},
		Resident:       "amber",
		Summary:        "Chenglin confirmed persistence is intentional and continuity notes matter.",
		DecisionAction: ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	report, err := store.LifecycleReport("amber", now, DefaultPolicy())
	if err != nil {
		t.Fatalf("lifecycle report: %v", err)
	}
	if report.Total != 3 || report.NeedsAttention != 2 {
		t.Fatalf("unexpected lifecycle counts: %#v", report)
	}
	if report.ActionCounts[ActionDelete] != 1 || report.ActionCounts[ActionDecay] != 1 || report.ActionCounts[ActionRetain] != 1 {
		t.Fatalf("unexpected action counts: %#v", report.ActionCounts)
	}
	if len(report.Items) != 3 || report.Items[0].ID != "long-fresh" || report.Items[1].ID != "instant-old" || report.Items[2].ID != "relationship-old" {
		t.Fatalf("expected report to preserve memory listing order, got %#v", report.Items)
	}
	if !report.Items[1].HardExpired || !report.Items[1].NeedsAttention {
		t.Fatalf("expected expired instant memory to need attention: %#v", report.Items[1])
	}
	if report.Items[1].Summary != "temporary note" || report.Items[1].Visibility != VisibilityResidentPrivate || report.Items[1].RecommendedOperatorAction != "decay_ok_after_spot_check" {
		t.Fatalf("expected machine-like expired memory to include summary and decay recommendation: %#v", report.Items[1])
	}
	if report.Items[2].Domain != DomainRelationships || report.Items[2].RecommendedOperatorAction != "review_for_promotion_or_rewrite" {
		t.Fatalf("expected relationship memory to need promotion/rewrite review: %#v", report.Items[2])
	}
}

func TestLifecycleReportWithApplyMarksExpiredMemories(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	if err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "instant-old",
			Layer:          LayerInstant,
			Status:         StatusActive,
			CreatedAt:      now.Add(-8 * time.Hour),
			UpdatedAt:      now.Add(-8 * time.Hour),
			LastAccessedAt: now.Add(-8 * time.Hour),
		},
		Resident:       "amber",
		Summary:        "temporary note",
		DecisionAction: ActionCreate,
	}); err != nil {
		t.Fatalf("upsert instant memory: %v", err)
	}
	if err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "short-old",
			Layer:          LayerShort,
			Status:         StatusActive,
			CreatedAt:      now.Add(-96 * time.Hour),
			UpdatedAt:      now.Add(-96 * time.Hour),
			LastAccessedAt: now.Add(-96 * time.Hour),
		},
		Resident:       "amber",
		Summary:        "short note",
		DecisionAction: ActionCreate,
	}); err != nil {
		t.Fatalf("upsert short memory: %v", err)
	}

	report, err := store.LifecycleReportWithApply("amber", now, DefaultPolicy(), true)
	if err != nil {
		t.Fatalf("lifecycle apply: %v", err)
	}
	if !report.Apply || report.ActionCounts[ActionDelete] != 1 || report.ActionCounts[ActionDecay] != 1 {
		t.Fatalf("unexpected apply report: %#v", report)
	}

	deleted, ok, err := store.GetAbstractMemory("amber", "instant-old")
	if err != nil || !ok {
		t.Fatalf("get instant memory: ok=%v err=%v", ok, err)
	}
	if deleted.Status != StatusDeleted {
		t.Fatalf("expected instant memory deleted, got %#v", deleted.Record)
	}
	decayed, ok, err := store.GetAbstractMemory("amber", "short-old")
	if err != nil || !ok {
		t.Fatalf("get short memory: ok=%v err=%v", ok, err)
	}
	if decayed.Status != StatusDecaying || decayed.Layer != LayerInstant {
		t.Fatalf("expected short memory decayed to instant, got %#v", decayed.Record)
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

func TestReviewAbstractMemoryRewrite(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 7, 6, 0, 0, 0, time.UTC)
	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "amber-short-1",
			Layer:     LayerShort,
			Domain:    DomainLessons,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:     "amber",
		Summary:      "raw-ish summary",
		ResidentText: "raw-ish resident text",
		Governance: GovernanceMeta{
			Quality:     "low",
			ReviewState: "needs_resident_review",
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	updated, err := store.ReviewAbstractMemory("amber", "amber-short-1", now.Add(time.Hour), MemoryReviewRequest{
		Action:     ActionUpdate,
		NewSummary: "I confirmed root access and a usable local notes surface.",
		NewText:    "I rewrote this into something I can actually use later.",
		ReasonNote: "The old version read too much like a raw log.",
	})
	if err != nil {
		t.Fatalf("review memory: %v", err)
	}
	if updated.Summary != "I confirmed root access and a usable local notes surface." {
		t.Fatalf("unexpected summary: %q", updated.Summary)
	}
	if updated.Governance.ReviewState != "resolved" {
		t.Fatalf("expected resolved review state, got %#v", updated.Governance)
	}
	if updated.Status != StatusActive {
		t.Fatalf("expected active status, got %s", updated.Status)
	}
}

func TestOperatorReviewCannotRewriteProtectedMemory(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 7, 6, 0, 0, 0, time.UTC)
	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "amber-short-protected",
			Layer:     LayerShort,
			Domain:    DomainRelationships,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident: "amber",
		Summary:  "A world-facing thread was opened.",
		Governance: GovernanceMeta{
			ProtectedFrom: []string{"host_rewrite", "host_delete"},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	_, err = store.ReviewAbstractMemory("amber", "amber-short-protected", now.Add(time.Hour), MemoryReviewRequest{
		Action:     ActionUpdate,
		NewSummary: "Operator rewrite should not be allowed.",
		Reviewer:   "operator",
	})
	if err == nil {
		t.Fatalf("expected operator rewrite to be rejected")
	}

	updated, err := store.ReviewAbstractMemory("amber", "amber-short-protected", now.Add(time.Hour), MemoryReviewRequest{
		Action:     ActionUpdate,
		NewSummary: "Resident rewrite is still allowed.",
	})
	if err != nil {
		t.Fatalf("resident rewrite should still be allowed: %v", err)
	}
	if updated.Summary != "Resident rewrite is still allowed." || updated.Governance.FlaggedBy != "resident" {
		t.Fatalf("unexpected resident rewrite result: %#v", updated)
	}
}

func TestReviewAbstractMemoryDelete(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 7, 6, 0, 0, 0, time.UTC)
	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "amber-short-2",
			Layer:     LayerShort,
			Domain:    DomainLessons,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident: "amber",
		Summary:  "throwaway note",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	updated, err := store.ReviewAbstractMemory("amber", "amber-short-2", now.Add(time.Hour), MemoryReviewRequest{
		Action:     ActionDelete,
		ReasonNote: "I do not want to keep this.",
	})
	if err != nil {
		t.Fatalf("review memory: %v", err)
	}
	if updated.Status != StatusDeleted {
		t.Fatalf("expected deleted status, got %s", updated.Status)
	}
	if updated.Governance.ReviewState != "resolved" {
		t.Fatalf("expected resolved review state, got %#v", updated.Governance)
	}
}

func TestReviewAbstractMemoryDecayRefreshesAccessTime(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 7, 6, 0, 0, 0, time.UTC)
	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:             "amber-short-4",
			Layer:          LayerShort,
			Domain:         DomainLessons,
			Status:         StatusActive,
			CreatedAt:      now.Add(-96 * time.Hour),
			UpdatedAt:      now.Add(-96 * time.Hour),
			LastAccessedAt: now.Add(-96 * time.Hour),
			ReviewAt:       now.Add(-24 * time.Hour),
			ReviewAfter:    now.Add(-48 * time.Hour),
		},
		Resident: "amber",
		Summary:  "stale machine probe",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	reviewedAt := now.Add(time.Hour)
	updated, err := store.ReviewAbstractMemory("amber", "amber-short-4", reviewedAt, MemoryReviewRequest{
		Action:     ActionDecay,
		ReasonNote: "operator_safe_lifecycle_decay",
	})
	if err != nil {
		t.Fatalf("review memory: %v", err)
	}
	if updated.Status != StatusDecaying || updated.Layer != LayerInstant {
		t.Fatalf("expected decayed instant memory, got %#v", updated.Record)
	}
	if !updated.LastAccessedAt.Equal(reviewedAt) {
		t.Fatalf("expected decay review to refresh access time, got %s", updated.LastAccessedAt)
	}
	if !updated.ReviewAt.IsZero() || !updated.ReviewAfter.IsZero() {
		t.Fatalf("expected decay review to clear stale review schedule, got %#v", updated.Record)
	}
}

func TestReviewAbstractMemoryCompressAlsoRewritesResidentTextByDefault(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 7, 6, 0, 0, 0, time.UTC)
	err := store.UpsertAbstractMemory(AbstractMemory{
		Record: Record{
			ID:        "amber-short-3",
			Layer:     LayerShort,
			Domain:    DomainLessons,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:     "amber",
		Summary:      "## raw log",
		ResidentText: "## raw log with long noisy tail",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	updated, err := store.ReviewAbstractMemory("amber", "amber-short-3", now.Add(time.Hour), MemoryReviewRequest{
		Action:     ActionSummarize,
		NewSummary: "I confirmed network reachability and kept a cleaner carry-forward note.",
		ReasonNote: "The raw log version was too noisy.",
	})
	if err != nil {
		t.Fatalf("review memory: %v", err)
	}
	if updated.ResidentText != "I confirmed network reachability and kept a cleaner carry-forward note." {
		t.Fatalf("expected resident text to follow compressed summary, got %q", updated.ResidentText)
	}
}
