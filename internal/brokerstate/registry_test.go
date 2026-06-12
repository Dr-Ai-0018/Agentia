package brokerstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

func TestLoadOrInitEngineInitializesWhenNoSnapshotExists(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 1.5,
			InitialQuota: tokenledger.QuotaState{Window6HCap: 4000},
		},
	})

	cfg := runtimecore.Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}

	engine, loaded, path, revision, err := registry.LoadOrInitEngine(store, cfg, "jade", time.Now().UTC())
	if err != nil {
		t.Fatalf("load or init: %v", err)
	}
	if loaded {
		t.Fatalf("expected fresh init, not loaded snapshot")
	}
	if path != "" {
		t.Fatalf("expected empty snapshot path on init")
	}
	if revision != 0 {
		t.Fatalf("expected zero revision on fresh init")
	}
	if engine.SparkLedger().Account().Balance != 1.5 {
		t.Fatalf("unexpected initial grant balance")
	}
}

func TestLoadOrInitEngineQuarantinesCorruptSnapshot(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	registry := NewRegistry([]ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 1.5,
			InitialQuota: tokenledger.QuotaState{Window6HCap: 4000},
		},
	})

	cfg := runtimecore.Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}

	dir := filepath.Join(root, "jade")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir resident dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runtime-state.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write corrupt snapshot: %v", err)
	}

	engine, loaded, path, revision, err := registry.LoadOrInitEngine(store, cfg, "jade", time.Now().UTC())
	if err != nil {
		t.Fatalf("load or init with corrupt snapshot: %v", err)
	}
	if loaded {
		t.Fatalf("expected fresh init after corrupt snapshot quarantine")
	}
	if path != "" {
		t.Fatalf("expected empty snapshot path on reinit, got %s", path)
	}
	if revision != 0 {
		t.Fatalf("expected zero revision after reinit, got %d", revision)
	}
	if engine.State().ResidentID != "jade" {
		t.Fatalf("unexpected resident id after reinit: %s", engine.State().ResidentID)
	}
	if _, err := os.Stat(filepath.Join(dir, "runtime-state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt runtime-state.json to be moved away, stat err=%v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "runtime-state.*.corrupt.json"))
	if err != nil {
		t.Fatalf("glob quarantine snapshots: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 quarantined snapshot, got %#v", matches)
	}
}
