package brokerstate

import (
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

	engine, loaded, path, err := registry.LoadOrInitEngine(store, cfg, "jade", time.Now().UTC())
	if err != nil {
		t.Fatalf("load or init: %v", err)
	}
	if loaded {
		t.Fatalf("expected fresh init, not loaded snapshot")
	}
	if path != "" {
		t.Fatalf("expected empty snapshot path on init")
	}
	if engine.SparkLedger().Account().Balance != 1.5 {
		t.Fatalf("unexpected initial grant balance")
	}
}
