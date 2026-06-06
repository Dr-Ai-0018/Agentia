package brokerstate

import (
	"testing"
	"time"

	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

func TestBrokerServiceSelfStatus(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 0.62,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap: 4000,
				DayCap:      20000,
				WeekCap:     150000,
			},
		},
	})
	manager := NewSessionManager(store, registry, runtimecore.Config{
		TokenPolicy:    tokenledger.DefaultConfig(),
		RecoveryPolicy: DefaultRuntimeConfig().RecoveryPolicy,
		ReserveSpark:   0.08,
		ReserveStrain:  300,
	})
	manager.rootNow = func() time.Time {
		return time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	}

	service := NewBrokerService(manager)
	status, err := service.SelfStatus("jade")
	if err != nil {
		t.Fatalf("self status: %v", err)
	}
	if status.ResidentID != "jade" {
		t.Fatalf("unexpected resident id")
	}
	if status.SparkBalance != 0.62 {
		t.Fatalf("unexpected spark balance")
	}
}

func TestBrokerServiceRecoveryTick(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 0.62,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  4000,
				Window6HUsed: 300,
				DayCap:       20000,
				DayUsed:      1500,
				WeekCap:      150000,
				WeekUsed:     8000,
			},
		},
	})
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	status, tick, _, err := service.RecoveryTick("jade", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("recovery tick: %v", err)
	}
	if tick.HoursElapsed != 2 {
		t.Fatalf("unexpected recovery hours: %v", tick.HoursElapsed)
	}
	if status.ResidentID != "jade" {
		t.Fatalf("unexpected resident id")
	}
}

func TestBrokerServiceResetResident(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	status, _, err := service.ResetResident("jade", now)
	if err != nil {
		t.Fatalf("reset resident: %v", err)
	}
	if status.ResidentID != "jade" {
		t.Fatalf("unexpected resident id")
	}
	if status.SparkBalance != 4.5 {
		t.Fatalf("unexpected reset balance")
	}
}

func TestBrokerServiceAdmitCall(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	resp, err := service.AdmitCall(AdmitRequest{
		ResidentID: "amber",
		Kind:       "work",
		Usage: tokenledger.Usage{
			InputTokens:  1200,
			CachedTokens: 800,
			OutputTokens: 300,
			TotalTokens:  1500,
			Model:        "gpt-5.4",
			FinishedAt:   now.Add(time.Minute),
		},
		Penalties: tokenledger.Penalties{ToolCallCount: 2},
		Activity:  tokenledger.ActivityNormalWork,
		Apply:     true,
	})
	if err != nil {
		t.Fatalf("admit call: %v", err)
	}
	if !resp.Prepared.Decision.Allowed {
		t.Fatalf("expected amber work call to be allowed")
	}
	if !resp.Applied {
		t.Fatalf("expected applied response")
	}
	if resp.AfterStatus == nil {
		t.Fatalf("expected after status")
	}
	if resp.AfterStatus.SparkBalance >= 8.0 {
		t.Fatalf("expected spark balance to decrease after applied work")
	}
}

func TestBrokerServicePrepareAndApplySeparated(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	prepared, engine, err := service.PrepareAdmission("amber", "work", tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		t.Fatalf("prepare admission: %v", err)
	}
	if prepared.Denied {
		t.Fatalf("expected prepared call to be allowed")
	}
	if engine == nil {
		t.Fatalf("expected engine")
	}

	applied, after, _, err := service.ApplyPreparedCall(engine, prepared, tokenledger.ActivityNormalWork)
	if err != nil {
		t.Fatalf("apply prepared call: %v", err)
	}
	if applied.State.ResidentID != "amber" {
		t.Fatalf("unexpected resident id after apply")
	}
	if after.SparkBalance >= prepared.BeforeStatus.SparkBalance {
		t.Fatalf("expected spark balance to decrease")
	}
}

func TestBrokerServicePrepareDeniedCall(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	prepared, _, err := service.PrepareAdmission("jade", "work", tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		t.Fatalf("prepare denied call: %v", err)
	}
	if prepared.Denied {
		t.Fatalf("expected jade call to be allowed under upgraded newborn baseline")
	}
	if !prepared.Prepared.Decision.Allowed {
		t.Fatalf("expected allow decision")
	}
}
