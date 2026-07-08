package brokerstate

import (
	"errors"
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
	if status.Physiology.Mode == "" {
		t.Fatalf("expected derived physiology mode")
	}
	if status.Physiology.QuotaTightestLayer == "" {
		t.Fatalf("expected quota tightest layer")
	}
	if status.EffectiveWindow6HCap <= 0 || status.EffectiveDayCap <= 0 || status.EffectiveWeekCap <= 0 {
		t.Fatalf("expected effective caps to be populated")
	}
	if status.NextRecoveryAt == "" {
		t.Fatalf("expected next recovery timestamp")
	}
	if status.RecoveryTickMinutes != 15 {
		t.Fatalf("expected 15 minute recovery tick")
	}
	if status.RecoveryMode != "idle" {
		t.Fatalf("expected default recovery mode idle, got %s", status.RecoveryMode)
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
	if tick.RecoveryMode != "idle" {
		t.Fatalf("expected idle recovery mode, got %s", tick.RecoveryMode)
	}
}

func TestBrokerServiceSleepStartEnd(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	start := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return start }
	service := NewBrokerService(manager)

	started, err := service.StartSleep(SleepStartRequest{
		ResidentID:     "jade",
		PlannedMinutes: 90,
		StartedAt:      start,
		Reason:         "tired",
	})
	if err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	if !started.State.Active || started.State.Depth != SleepDepthRest {
		t.Fatalf("unexpected started sleep state: %#v", started.State)
	}

	ended, err := service.EndSleep(SleepEndRequest{
		ResidentID: "jade",
		EndedAt:    start.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("end sleep: %v", err)
	}
	if ended.Session.ActualMinutes != 90 || ended.Session.Depth != SleepDepthSleep {
		t.Fatalf("unexpected ended session: %#v", ended.Session)
	}
	if ended.State.Active {
		t.Fatalf("expected inactive sleep state after end: %#v", ended.State)
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
	if status.SparkBalance != DefaultResidentProfiles()[0].InitialGrant {
		t.Fatalf("unexpected reset balance: %.4f", status.SparkBalance)
	}
}

func TestBrokerServiceSparkGrantClearsDebtWhenBalancePositive(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	engine, _, _, err := manager.LoadResidentWithRevision("jade")
	if err != nil {
		t.Fatalf("load resident: %v", err)
	}
	if _, err := engine.SparkLedger().DebitAllowDebt("charge", engine.SparkLedger().Account().Balance+5.0, "test debt", now); err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	engine.ReconcileSparkDebt()
	if _, err := manager.SaveResident(engine); err != nil {
		t.Fatalf("save debt snapshot: %v", err)
	}

	resp, err := service.GrantSpark(SparkGrantRequest{
		ResidentID: "jade",
		Amount:     6.0,
		Reason:     "test allowance",
	})
	if err != nil {
		t.Fatalf("grant spark: %v", err)
	}
	if resp.BeforeStatus.DebtActive == false {
		t.Fatalf("expected debt before grant: %#v", resp.BeforeStatus)
	}
	if resp.AfterStatus.DebtActive || resp.AfterStatus.DebtAmount != 0 {
		t.Fatalf("expected debt cleared after grant: %#v", resp.AfterStatus)
	}
	if resp.AfterStatus.SparkBalance <= 0 {
		t.Fatalf("expected positive spark balance after grant: %#v", resp.AfterStatus)
	}
}

func TestBrokerServiceTestAllowanceCardClearsDebtAndBoostsQuota(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	engine, _, _, err := manager.LoadResidentWithRevision("onyx")
	if err != nil {
		t.Fatalf("load resident: %v", err)
	}
	if _, err := engine.SparkLedger().DebitAllowDebt("charge", engine.SparkLedger().Account().Balance+3.5, "test debt", now); err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	engine.ReconcileSparkDebt()
	if _, err := manager.SaveResident(engine); err != nil {
		t.Fatalf("save debt snapshot: %v", err)
	}

	resp, err := service.GrantTestAllowanceCard(TestAllowanceCardRequest{
		ResidentID:        "onyx",
		SparkAmount:       10,
		Window6HDelta:     1000,
		DayDelta:          2000,
		WeekDelta:         3000,
		ResetWindow6HUsed: true,
		ResetDayUsed:      true,
		ResetWeekUsed:     true,
		Reason:            "temporary_test_allowance",
		Operator:          "test",
	})
	if err != nil {
		t.Fatalf("grant allowance card: %v", err)
	}
	if !resp.BeforeStatus.DebtActive {
		t.Fatalf("expected debt before card")
	}
	if resp.AfterStatus.DebtActive || resp.AfterStatus.DebtAmount != 0 {
		t.Fatalf("expected card to clear debt: %#v", resp.AfterStatus)
	}
	if resp.AfterStatus.Window6HCap != resp.BeforeStatus.Window6HCap+1000 {
		t.Fatalf("expected 6h cap boost")
	}
	if resp.AfterStatus.Window6HUsed != 0 || resp.AfterStatus.DayUsed != 0 || resp.AfterStatus.WeekUsed != 0 {
		t.Fatalf("expected quota usage reset: %#v", resp.AfterStatus)
	}
	if !resp.Quota.WorkAllowedNow {
		t.Fatalf("expected work allowed after card: %#v", resp.Quota)
	}
	if resp.RevertQuotaGrant.Window6HDelta != -1000 || resp.RevertQuotaGrant.DayDelta != -2000 || resp.RevertQuotaGrant.WeekDelta != -3000 {
		t.Fatalf("unexpected revert grant: %#v", resp.RevertQuotaGrant)
	}
	events, _, err := store.LoadQuotaEvents("onyx")
	if err != nil {
		t.Fatalf("load quota events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != QuotaEventTestAllowance {
		t.Fatalf("expected one allowance quota event, got %#v", events)
	}
	rolling := RollingQuotaUsageFromEvents(events, now)
	if rolling.DayUsed != 0 || rolling.WeekUsed != 0 || rolling.Window6HUsed != 0 {
		t.Fatalf("allowance event must not count toward rolling usage: %#v", rolling)
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
	if resp.AfterStatus.SparkBalance >= resp.BeforeStatus.SparkBalance {
		t.Fatalf("expected spark balance to decrease after applied work")
	}
	events, _, err := store.LoadQuotaEvents("amber")
	if err != nil {
		t.Fatalf("load quota events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one quota event, got %d", len(events))
	}
	if events[0].Kind != QuotaEventWorkCall {
		t.Fatalf("expected work quota event, got %#v", events[0])
	}
	if events[0].StrainCost != resp.Prepared.Strain.Rounded {
		t.Fatalf("quota event strain = %d, want %d", events[0].StrainCost, resp.Prepared.Strain.Rounded)
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

func TestBrokerServiceApplyPreparedCallRejectsStaleSnapshot(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry(DefaultResidentProfiles())
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	preparedA, engineA, err := service.PrepareAdmission("amber", "work", tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		t.Fatalf("prepare A: %v", err)
	}

	preparedB, engineB, err := service.PrepareAdmission("amber", "work", tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(2 * time.Minute),
	}, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		t.Fatalf("prepare B: %v", err)
	}

	if _, _, _, err := service.ApplyPreparedCall(engineA, preparedA, tokenledger.ActivityNormalWork); err != nil {
		t.Fatalf("apply A: %v", err)
	}
	if _, _, _, err := service.ApplyPreparedCall(engineB, preparedB, tokenledger.ActivityNormalWork); !errors.Is(err, ErrSnapshotVersionConflict) {
		t.Fatalf("expected version conflict on stale apply, got %v", err)
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

func TestBrokerServiceRollingDayQuotaDeniesAdmission(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{{
		ResidentID:   "jade",
		InitialGrant: 5,
		InitialQuota: tokenledger.QuotaState{
			Window6HCap: 100,
			DayCap:      1000,
			WeekCap:     10000,
		},
	}})
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	if _, err := store.AppendQuotaEvent(QuotaEvent{
		ResidentID: "jade",
		Kind:       QuotaEventWorkCall,
		StrainCost: 980,
		CreatedAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("append rolling usage: %v", err)
	}

	prepared, _, err := service.PrepareAdmission("jade", "work", tokenledger.Usage{
		InputTokens:  40,
		OutputTokens: 20,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(time.Minute),
	}, tokenledger.Penalties{})
	if err != nil {
		t.Fatalf("prepare admission: %v", err)
	}
	if !prepared.Denied {
		t.Fatalf("expected rolling day quota to deny admission")
	}
	if len(prepared.DeniedReason) != 1 || prepared.DeniedReason[0] != "work_would_exceed_day_quota" {
		t.Fatalf("unexpected denied reason: %#v", prepared.DeniedReason)
	}
	if prepared.Prepared.Decision.WouldExceedQuota != true || !prepared.Prepared.Decision.WouldExceedDay {
		t.Fatalf("expected day quota projection: %#v", prepared.Prepared.Decision)
	}
}

func TestBrokerServiceExhaustedSixHourObservationDoesNotDenyAdmission(t *testing.T) {
	store := New(t.TempDir())
	registry := NewRegistry([]ResidentProfile{{
		ResidentID:   "jade",
		InitialGrant: 5,
		InitialQuota: tokenledger.QuotaState{
			Window6HCap: 100,
			DayCap:      10000,
			WeekCap:     50000,
		},
	}})
	manager := NewSessionManager(store, registry, DefaultRuntimeConfig())
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	manager.rootNow = func() time.Time { return now }
	service := NewBrokerService(manager)

	if _, err := store.AppendQuotaEvent(QuotaEvent{
		ResidentID: "jade",
		Kind:       QuotaEventWorkCall,
		StrainCost: 500,
		CreatedAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("append rolling usage: %v", err)
	}

	prepared, _, err := service.PrepareAdmission("jade", "work", tokenledger.Usage{
		InputTokens:  40,
		OutputTokens: 20,
		Model:        "gpt-5.4",
		FinishedAt:   now.Add(time.Minute),
	}, tokenledger.Penalties{})
	if err != nil {
		t.Fatalf("prepare admission: %v", err)
	}
	if prepared.Denied {
		t.Fatalf("expected 6h observation exhaustion not to deny admission: %#v", prepared.DeniedReason)
	}
	if prepared.BeforeStatus.RollingWindow6HUsed != 500 {
		t.Fatalf("expected rolling 6h usage on status, got %d", prepared.BeforeStatus.RollingWindow6HUsed)
	}
	if prepared.Prepared.Decision.WouldExceedQuota {
		t.Fatalf("6h observation should not count as hard quota projection: %#v", prepared.Prepared.Decision)
	}
}
