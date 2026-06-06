package brokerstate

import (
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/runtimecore"
	"ai-arena/internal/sparkledger"
)

func TestSaveAndLoadResidentSnapshot(t *testing.T) {
	root := t.TempDir()
	store := New(root)

	snapshot := runtimecore.Snapshot{
		Version: "runtimecore/v1",
		SavedAt: time.Now().UTC(),
		State: runtimecore.ResidentState{
			ResidentID: "jade",
		},
		SparkAccount: sparkledger.Account{
			ResidentID:   "jade",
			Balance:      1.2345,
			BalanceUnits: 12345,
		},
	}

	path, err := store.SaveResidentSnapshot("jade", snapshot)
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if path != filepath.Join(root, "jade", "runtime-state.json") {
		t.Fatalf("unexpected path: %s", path)
	}

	loaded, loadedPath, err := store.LoadResidentSnapshot("jade")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if loadedPath != path {
		t.Fatalf("loaded path mismatch")
	}
	if loaded.State.ResidentID != "jade" {
		t.Fatalf("resident id mismatch after load")
	}
	if loaded.SparkAccount.Balance != 1.2345 {
		t.Fatalf("spark balance mismatch after load")
	}
}

func TestDeleteResidentSnapshot(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	snapshot := runtimecore.Snapshot{
		Version: "runtimecore/v1",
		SavedAt: time.Now().UTC(),
		State: runtimecore.ResidentState{ResidentID: "jade"},
	}
	if _, err := store.SaveResidentSnapshot("jade", snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := store.DeleteResidentSnapshot("jade"); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	_, _, err := store.LoadResidentSnapshot("jade")
	if err == nil {
		t.Fatalf("expected load to fail after delete")
	}
}
