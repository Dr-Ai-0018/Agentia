package brokerstate

import (
	"fmt"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
)

type SessionManager struct {
	store    *Store
	registry *Registry
	cfg      runtimecore.Config
	rootNow  func() time.Time
}

type ResidentStatus struct {
	ResidentID           string             `json:"resident_id"`
	LoadedFromSnapshot   bool               `json:"loaded_from_snapshot"`
	SnapshotPath         string             `json:"snapshot_path,omitempty"`
	SparkBalance         float64            `json:"spark_balance"`
	SparkBalanceUnits    int64              `json:"spark_balance_units"`
	Fatigue              int                `json:"fatigue"`
	SleepDebt            int                `json:"sleep_debt"`
	DebtActive           bool               `json:"debt_active"`
	DebtAmount           float64            `json:"debt_amount"`
	FinalNoticeUsed      bool               `json:"final_notice_used"`
	RecoveryMode         string             `json:"recovery_mode"`
	Window6HCap          int                `json:"window_6h_cap"`
	Window6HUsed         int                `json:"window_6h_used"`
	DayCap               int                `json:"day_cap"`
	DayUsed              int                `json:"day_used"`
	WeekCap              int                `json:"week_cap"`
	WeekUsed             int                `json:"week_used"`
	EffectiveWindow6HCap int                `json:"effective_window_6h_cap"`
	EffectiveDayCap      int                `json:"effective_day_cap"`
	EffectiveWeekCap     int                `json:"effective_week_cap"`
	NextRecoveryAt       string             `json:"next_recovery_at,omitempty"`
	RecoveryTickMinutes  int                `json:"recovery_tick_minutes"`
	LastRecoveryAt       time.Time          `json:"last_recovery_at"`
	Physiology           ResidentPhysiology `json:"physiology"`
}

func NewSessionManager(store *Store, registry *Registry, cfg runtimecore.Config) *SessionManager {
	return &SessionManager{
		store:    store,
		registry: registry,
		cfg:      cfg,
		rootNow:  time.Now().UTC,
	}
}

func (m *SessionManager) LoadResident(residentID string) (*runtimecore.Engine, ResidentStatus, error) {
	now := m.rootNow()
	engine, loaded, path, err := m.registry.LoadOrInitEngine(m.store, m.cfg, residentID, now)
	if err != nil {
		return nil, ResidentStatus{}, err
	}
	status := BuildResidentStatusAt(engine, loaded, path, now)
	return engine, status, nil
}

func (m *SessionManager) SaveResident(engine *runtimecore.Engine) (string, error) {
	state := engine.State()
	if state.ResidentID == "" {
		return "", fmt.Errorf("resident id is empty")
	}
	return m.store.SaveResidentSnapshot(state.ResidentID, engine.Snapshot(m.rootNow()))
}

func BuildResidentStatus(engine *runtimecore.Engine, loaded bool, snapshotPath string) ResidentStatus {
	return BuildResidentStatusAt(engine, loaded, snapshotPath, time.Now().UTC())
}

func BuildResidentStatusAt(engine *runtimecore.Engine, loaded bool, snapshotPath string, now time.Time) ResidentStatus {
	state := engine.State()
	account := engine.SparkLedger().Account()
	effective := runtimeguard.DeriveEffectiveQuota(runtimeguard.State{
		Quota:      state.Quota,
		Fatigue:    state.Fatigue,
		SleepDebt:  state.SleepDebt,
		DebtActive: state.DebtActive,
	})
	return ResidentStatus{
		ResidentID:           state.ResidentID,
		LoadedFromSnapshot:   loaded,
		SnapshotPath:         snapshotPath,
		SparkBalance:         account.Balance,
		SparkBalanceUnits:    account.BalanceUnits,
		Fatigue:              state.Fatigue,
		SleepDebt:            state.SleepDebt,
		DebtActive:           state.DebtActive,
		DebtAmount:           state.DebtAmount,
		FinalNoticeUsed:      state.FinalNoticeUsed,
		RecoveryMode:         state.RecoveryMode,
		Window6HCap:          state.Quota.Window6HCap,
		Window6HUsed:         state.Quota.Window6HUsed,
		DayCap:               state.Quota.DayCap,
		DayUsed:              state.Quota.DayUsed,
		WeekCap:              state.Quota.WeekCap,
		WeekUsed:             state.Quota.WeekUsed,
		EffectiveWindow6HCap: effective.Window6HCap,
		EffectiveDayCap:      effective.DayCap,
		EffectiveWeekCap:     effective.WeekCap,
		NextRecoveryAt:       recovery.NextRecoveryAt(now).Format(time.RFC3339),
		RecoveryTickMinutes:  15,
		LastRecoveryAt:       state.LastRecoveryAt,
		Physiology: DerivePhysiology(ResidentStatus{
			ResidentID:           state.ResidentID,
			SparkBalance:         account.Balance,
			Fatigue:              state.Fatigue,
			SleepDebt:            state.SleepDebt,
			DebtActive:           state.DebtActive,
			DebtAmount:           state.DebtAmount,
			RecoveryMode:         state.RecoveryMode,
			Window6HCap:          state.Quota.Window6HCap,
			Window6HUsed:         state.Quota.Window6HUsed,
			DayCap:               state.Quota.DayCap,
			DayUsed:              state.Quota.DayUsed,
			WeekCap:              state.Quota.WeekCap,
			WeekUsed:             state.Quota.WeekUsed,
			EffectiveWindow6HCap: effective.Window6HCap,
			EffectiveDayCap:      effective.DayCap,
			EffectiveWeekCap:     effective.WeekCap,
			NextRecoveryAt:       recovery.NextRecoveryAt(now).Format(time.RFC3339),
			RecoveryTickMinutes:  15,
			LastRecoveryAt:       state.LastRecoveryAt,
		}, now),
	}
}
