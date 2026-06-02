package memory

import (
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

func TestMemoryStoreUpsertAndSnapshot(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()

	err := store.Upsert(StoreRecord{
		Record: Record{
			ID:        "a",
			Layer:     LayerLong,
			Status:    StatusActive,
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		},
		Resident:       "onyx",
		Summary:        "long-term leverage lesson",
		DecisionAction: ActionCreate,
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	err = store.Upsert(StoreRecord{
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
		t.Fatalf("upsert failed: %v", err)
	}

	records, err := store.List("onyx")
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

func TestFileStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	now := time.Now()

	record := StoreRecord{
		Record: Record{
			ID:        "jade-1",
			Layer:     LayerPermanent,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:       "jade",
		Summary:        "stable engineering law",
		DecisionAction: ActionCreate,
	}

	if err := store.Upsert(record); err != nil {
		t.Fatalf("file upsert failed: %v", err)
	}

	got, ok, err := store.Get("jade", "jade-1")
	if err != nil {
		t.Fatalf("file get failed: %v", err)
	}
	if !ok {
		t.Fatal("expected record to exist")
	}
	if got.Summary != record.Summary {
		t.Fatalf("unexpected summary: %s", got.Summary)
	}

	records, err := store.List("jade")
	if err != nil {
		t.Fatalf("file list failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	expectedPath := filepath.Join(root, "jade.json")
	if _, err := filepath.Abs(expectedPath); err != nil {
		t.Fatalf("abs path failed: %v", err)
	}
}
