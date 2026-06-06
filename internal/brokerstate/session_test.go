package brokerstate

import (
	"testing"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

func TestSessionManagerLoadResident(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 1.25,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap: 4000,
			},
		},
	})
	manager := NewSessionManager(store, registry, runtimecore.Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	})
	manager.rootNow = func() time.Time {
		return time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	}

	engine, status, err := manager.LoadResident("jade")
	if err != nil {
		t.Fatalf("load resident: %v", err)
	}
	if engine == nil {
		t.Fatalf("expected engine")
	}
	if status.ResidentID != "jade" {
		t.Fatalf("unexpected resident id")
	}
	if status.SparkBalance != 1.25 {
		t.Fatalf("unexpected spark balance")
	}
}
