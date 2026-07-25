package brokerstate

import (
	"fmt"
	"strconv"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/sparkledger"
	"ai-arena/internal/tokenledger"
)

type BrokerService struct {
	sessions *SessionManager
}

type PreparedAdmission struct {
	ResidentID       string                   `json:"resident_id"`
	SnapshotRevision uint64                   `json:"snapshot_revision"`
	BeforeStatus     ResidentStatus           `json:"before_status"`
	Quota            QuotaSnapshot            `json:"quota"`
	Prepared         runtimecore.PreparedCall `json:"prepared"`
	Denied           bool                     `json:"denied"`
	DeniedReason     []string                 `json:"denied_reason,omitempty"`
}

type AdmitRequest struct {
	ResidentID                 string
	Kind                       runtimeguard.CallKind
	Usage                      tokenledger.Usage
	Penalties                  tokenledger.Penalties
	Activity                   tokenledger.ActivityType
	Apply                      bool
	RecordProviderCostOnDenied bool
}

type AdmitResponse struct {
	BeforeStatus         ResidentStatus           `json:"before_status"`
	Quota                QuotaSnapshot            `json:"quota"`
	Prepared             runtimecore.PreparedCall `json:"prepared"`
	Applied              bool                     `json:"applied"`
	AfterStatus          *ResidentStatus          `json:"after_status,omitempty"`
	ApplyResult          *runtimecore.AppliedCall `json:"apply_result,omitempty"`
	SnapshotPath         string                   `json:"snapshot_path,omitempty"`
	Denied               bool                     `json:"denied"`
	DeniedReason         []string                 `json:"denied_reason,omitempty"`
	ProviderCostRecorded bool                     `json:"provider_cost_recorded,omitempty"`
}

type QuotaGrantRequest struct {
	ResidentID    string `json:"resident_id"`
	Window6HDelta int    `json:"window_6h_delta,omitempty"`
	DayDelta      int    `json:"day_delta,omitempty"`
	WeekDelta     int    `json:"week_delta,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type QuotaGrantResponse struct {
	ResidentID       string         `json:"resident_id"`
	Reason           string         `json:"reason,omitempty"`
	Window6HDelta    int            `json:"window_6h_delta,omitempty"`
	DayDelta         int            `json:"day_delta,omitempty"`
	WeekDelta        int            `json:"week_delta,omitempty"`
	BeforeStatus     ResidentStatus `json:"before_status"`
	AfterStatus      ResidentStatus `json:"after_status"`
	SnapshotPath     string         `json:"snapshot_path"`
	SnapshotRevision uint64         `json:"snapshot_revision"`
}

type SparkGrantRequest struct {
	ResidentID string  `json:"resident_id"`
	Amount     float64 `json:"amount"`
	Reason     string  `json:"reason,omitempty"`
}

type SparkGrantResponse struct {
	ResidentID       string            `json:"resident_id"`
	Amount           float64           `json:"amount"`
	Reason           string            `json:"reason,omitempty"`
	Entry            sparkledger.Entry `json:"entry"`
	BeforeStatus     ResidentStatus    `json:"before_status"`
	AfterStatus      ResidentStatus    `json:"after_status"`
	SnapshotPath     string            `json:"snapshot_path"`
	SnapshotRevision uint64            `json:"snapshot_revision"`
}

type TestAllowanceCardRequest struct {
	ResidentID        string  `json:"resident_id"`
	SparkAmount       float64 `json:"spark_amount"`
	Window6HDelta     int     `json:"window_6h_delta,omitempty"`
	DayDelta          int     `json:"day_delta,omitempty"`
	WeekDelta         int     `json:"week_delta,omitempty"`
	ResetWindow6HUsed bool    `json:"reset_window_6h_used,omitempty"`
	ResetDayUsed      bool    `json:"reset_day_used,omitempty"`
	ResetWeekUsed     bool    `json:"reset_week_used,omitempty"`
	Reason            string  `json:"reason,omitempty"`
	Operator          string  `json:"operator,omitempty"`
}

type TestAllowanceCardResponse struct {
	ResidentID        string             `json:"resident_id"`
	SparkAmount       float64            `json:"spark_amount"`
	Window6HDelta     int                `json:"window_6h_delta,omitempty"`
	DayDelta          int                `json:"day_delta,omitempty"`
	WeekDelta         int                `json:"week_delta,omitempty"`
	ResetWindow6HUsed bool               `json:"reset_window_6h_used,omitempty"`
	ResetDayUsed      bool               `json:"reset_day_used,omitempty"`
	ResetWeekUsed     bool               `json:"reset_week_used,omitempty"`
	Reason            string             `json:"reason,omitempty"`
	Operator          string             `json:"operator,omitempty"`
	SparkEntry        *sparkledger.Entry `json:"spark_entry,omitempty"`
	BeforeStatus      ResidentStatus     `json:"before_status"`
	AfterStatus       ResidentStatus     `json:"after_status"`
	Quota             QuotaSnapshot      `json:"quota"`
	RevertQuotaGrant  QuotaGrantRequest  `json:"revert_quota_grant,omitempty"`
	SnapshotPath      string             `json:"snapshot_path"`
	SnapshotRevision  uint64             `json:"snapshot_revision"`
}

type SleepStartRequest struct {
	ResidentID     string    `json:"resident_id"`
	PlannedMinutes int       `json:"planned_minutes,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	Reason         string    `json:"reason,omitempty"`
}

type SleepEndRequest struct {
	ResidentID string    `json:"resident_id"`
	EndedAt    time.Time `json:"ended_at"`
}

type SleepRecordResponse struct {
	ResidentID string             `json:"resident_id"`
	Session    SleepSession       `json:"session"`
	State      SleepStateSnapshot `json:"state"`
	Path       string             `json:"path,omitempty"`
}

func NewBrokerService(sessions *SessionManager) *BrokerService {
	return &BrokerService{sessions: sessions}
}

func ErrUnknownCallKind(kind runtimeguard.CallKind) error {
	return fmt.Errorf("unknown call kind: %s", kind)
}

func (s *BrokerService) SelfStatus(residentID string) (ResidentStatus, error) {
	if residentID == "" {
		return ResidentStatus{}, fmt.Errorf("resident id is required")
	}
	_, status, err := s.sessions.LoadResident(residentID)
	if err != nil {
		return ResidentStatus{}, err
	}
	return status, nil
}

func (s *BrokerService) RecoveryTick(residentID string, now time.Time) (ResidentStatus, recovery.TickResult, string, error) {
	return s.RecoveryTickWithMode(residentID, now, "idle")
}

func (s *BrokerService) RecoveryTickWithMode(residentID string, now time.Time, mode string) (ResidentStatus, recovery.TickResult, string, error) {
	if residentID == "" {
		return ResidentStatus{}, recovery.TickResult{}, "", fmt.Errorf("resident id is required")
	}
	engine, _, err := s.sessions.LoadResident(residentID)
	if err != nil {
		return ResidentStatus{}, recovery.TickResult{}, "", err
	}
	debt, err := s.sessions.store.SleepDebt(residentID, now)
	if err != nil {
		return ResidentStatus{}, recovery.TickResult{}, "", err
	}
	engine.SetSleepDebtHours(debt.DebtHours)
	engine.SetRecoveryMode(mode)
	tick := engine.TickRecovery(now)
	path, err := s.sessions.SaveResident(engine)
	if err != nil {
		return ResidentStatus{}, recovery.TickResult{}, "", err
	}
	status := s.sessions.BuildResidentStatus(engine, true, path)
	return status, tick, path, nil
}

func (s *BrokerService) StartSleep(req SleepStartRequest) (SleepRecordResponse, error) {
	if req.ResidentID == "" {
		return SleepRecordResponse{}, fmt.Errorf("resident id is required")
	}
	session, path, err := s.sessions.store.RecordSleepStart(req.ResidentID, req.PlannedMinutes, req.StartedAt, req.Reason)
	if err != nil {
		return SleepRecordResponse{}, err
	}
	state := s.sessions.store.CurrentSleepState(req.ResidentID, session.StartedAt)
	return SleepRecordResponse{ResidentID: req.ResidentID, Session: session, State: state, Path: path}, nil
}

func (s *BrokerService) EndSleep(req SleepEndRequest) (SleepRecordResponse, error) {
	if req.ResidentID == "" {
		return SleepRecordResponse{}, fmt.Errorf("resident id is required")
	}
	session, path, err := s.sessions.store.RecordSleepEnd(req.ResidentID, req.EndedAt)
	if err != nil {
		return SleepRecordResponse{}, err
	}
	state := s.sessions.store.CurrentSleepState(req.ResidentID, session.EndedAt)
	return SleepRecordResponse{ResidentID: req.ResidentID, Session: session, State: state, Path: path}, nil
}

func (s *BrokerService) ResetResident(residentID string, now time.Time) (ResidentStatus, string, error) {
	if residentID == "" {
		return ResidentStatus{}, "", fmt.Errorf("resident id is required")
	}
	if err := s.sessions.store.DeleteResidentSnapshot(residentID); err != nil {
		return ResidentStatus{}, "", err
	}
	engine, _, err := s.sessions.LoadResident(residentID)
	if err != nil {
		return ResidentStatus{}, "", err
	}
	path, err := s.sessions.SaveResident(engine)
	if err != nil {
		return ResidentStatus{}, "", err
	}
	status := s.sessions.BuildResidentStatus(engine, false, path)
	return status, path, nil
}

func (s *BrokerService) GrantQuota(req QuotaGrantRequest) (QuotaGrantResponse, error) {
	if req.ResidentID == "" {
		return QuotaGrantResponse{}, fmt.Errorf("resident id is required")
	}
	if req.Window6HDelta == 0 && req.DayDelta == 0 && req.WeekDelta == 0 {
		return QuotaGrantResponse{}, fmt.Errorf("at least one quota delta is required")
	}
	engine, before, revision, err := s.sessions.LoadResidentWithRevision(req.ResidentID)
	if err != nil {
		return QuotaGrantResponse{}, err
	}
	engine.AdjustQuotaCaps(req.Window6HDelta, req.DayDelta, req.WeekDelta)
	path, err := s.sessions.SaveResidentExpected(engine, revision, true)
	if err != nil {
		return QuotaGrantResponse{}, err
	}
	after := s.sessions.BuildResidentStatus(engine, true, path)
	return QuotaGrantResponse{
		ResidentID:       req.ResidentID,
		Reason:           req.Reason,
		Window6HDelta:    req.Window6HDelta,
		DayDelta:         req.DayDelta,
		WeekDelta:        req.WeekDelta,
		BeforeStatus:     before,
		AfterStatus:      after,
		SnapshotPath:     path,
		SnapshotRevision: revision + 1,
	}, nil
}

func (s *BrokerService) GrantSpark(req SparkGrantRequest) (SparkGrantResponse, error) {
	if req.ResidentID == "" {
		return SparkGrantResponse{}, fmt.Errorf("resident id is required")
	}
	if req.Amount <= 0 {
		return SparkGrantResponse{}, fmt.Errorf("spark grant amount must be positive")
	}
	engine, before, revision, err := s.sessions.LoadResidentWithRevision(req.ResidentID)
	if err != nil {
		return SparkGrantResponse{}, err
	}
	entry, err := engine.SparkLedger().Credit(sparkledger.EntryGrant, req.Amount, req.Reason, time.Now().UTC())
	if err != nil {
		return SparkGrantResponse{}, err
	}
	engine.ReconcileSparkDebt()
	path, err := s.sessions.SaveResidentExpected(engine, revision, true)
	if err != nil {
		return SparkGrantResponse{}, err
	}
	after := s.sessions.BuildResidentStatus(engine, true, path)
	return SparkGrantResponse{
		ResidentID:       req.ResidentID,
		Amount:           req.Amount,
		Reason:           req.Reason,
		Entry:            entry,
		BeforeStatus:     before,
		AfterStatus:      after,
		SnapshotPath:     path,
		SnapshotRevision: revision + 1,
	}, nil
}

func (s *BrokerService) GrantTestAllowanceCard(req TestAllowanceCardRequest) (TestAllowanceCardResponse, error) {
	if req.ResidentID == "" {
		return TestAllowanceCardResponse{}, fmt.Errorf("resident id is required")
	}
	if req.SparkAmount <= 0 && req.Window6HDelta == 0 && req.DayDelta == 0 && req.WeekDelta == 0 && !req.ResetWindow6HUsed && !req.ResetDayUsed && !req.ResetWeekUsed {
		return TestAllowanceCardResponse{}, fmt.Errorf("spark amount or quota delta is required")
	}
	if req.SparkAmount < 0 {
		return TestAllowanceCardResponse{}, fmt.Errorf("spark amount cannot be negative")
	}

	engine, before, revision, err := s.sessions.LoadResidentWithRevision(req.ResidentID)
	if err != nil {
		return TestAllowanceCardResponse{}, err
	}

	var sparkEntry *sparkledger.Entry
	if req.SparkAmount > 0 {
		entry, err := engine.SparkLedger().Credit(sparkledger.EntryGrant, req.SparkAmount, req.Reason, time.Now().UTC())
		if err != nil {
			return TestAllowanceCardResponse{}, err
		}
		sparkEntry = &entry
		engine.ReconcileSparkDebt()
	}
	if req.Window6HDelta != 0 || req.DayDelta != 0 || req.WeekDelta != 0 {
		engine.AdjustQuotaCaps(req.Window6HDelta, req.DayDelta, req.WeekDelta)
	}
	if req.ResetWindow6HUsed || req.ResetDayUsed || req.ResetWeekUsed {
		engine.ResetQuotaUsage(req.ResetWindow6HUsed, req.ResetDayUsed, req.ResetWeekUsed)
	}

	path, err := s.sessions.SaveResidentExpected(engine, revision, true)
	if err != nil {
		return TestAllowanceCardResponse{}, err
	}
	if _, err := s.sessions.store.AppendQuotaEvent(QuotaEvent{
		ResidentID: req.ResidentID,
		Kind:       QuotaEventTestAllowance,
		SparkCost:  -req.SparkAmount,
		CreatedAt:  time.Now().UTC(),
		Metadata: map[string]interface{}{
			"reason":               req.Reason,
			"operator":             req.Operator,
			"window_6h_delta":      req.Window6HDelta,
			"day_delta":            req.DayDelta,
			"week_delta":           req.WeekDelta,
			"reset_window_6h_used": req.ResetWindow6HUsed,
			"reset_day_used":       req.ResetDayUsed,
			"reset_week_used":      req.ResetWeekUsed,
		},
	}); err != nil {
		return TestAllowanceCardResponse{}, err
	}
	after := s.sessions.BuildResidentStatus(engine, true, path)
	quota := BuildQuotaSnapshot(after)
	revert := QuotaGrantRequest{
		ResidentID:    req.ResidentID,
		Window6HDelta: -req.Window6HDelta,
		DayDelta:      -req.DayDelta,
		WeekDelta:     -req.WeekDelta,
		Reason:        "revert " + req.Reason,
	}
	if req.Window6HDelta == 0 && req.DayDelta == 0 && req.WeekDelta == 0 {
		revert = QuotaGrantRequest{}
	}
	return TestAllowanceCardResponse{
		ResidentID:        req.ResidentID,
		SparkAmount:       req.SparkAmount,
		Window6HDelta:     req.Window6HDelta,
		DayDelta:          req.DayDelta,
		WeekDelta:         req.WeekDelta,
		ResetWindow6HUsed: req.ResetWindow6HUsed,
		ResetDayUsed:      req.ResetDayUsed,
		ResetWeekUsed:     req.ResetWeekUsed,
		Reason:            req.Reason,
		Operator:          req.Operator,
		SparkEntry:        sparkEntry,
		BeforeStatus:      before,
		AfterStatus:       after,
		Quota:             quota,
		RevertQuotaGrant:  revert,
		SnapshotPath:      path,
		SnapshotRevision:  revision + 1,
	}, nil
}

func (s *BrokerService) AdmitCall(req AdmitRequest) (AdmitResponse, error) {
	prepared, engine, err := s.PrepareAdmission(req.ResidentID, req.Kind, req.Usage, req.Penalties)
	if err != nil {
		return AdmitResponse{}, err
	}

	resp := AdmitResponse{
		BeforeStatus: prepared.BeforeStatus,
		Quota:        prepared.Quota,
		Prepared:     prepared.Prepared,
		Denied:       prepared.Denied,
		DeniedReason: append([]string(nil), prepared.DeniedReason...),
	}
	if !req.Apply {
		return resp, nil
	}
	if prepared.Denied {
		if req.RecordProviderCostOnDenied {
			if _, err := s.sessions.store.AppendQuotaEvent(quotaEventFromDeniedProviderUsage(prepared, req.Activity)); err != nil {
				return AdmitResponse{}, err
			}
			resp.ProviderCostRecorded = true
		}
		return resp, nil
	}

	applied, after, path, err := s.ApplyPreparedCall(engine, prepared, req.Activity)
	if err != nil {
		return AdmitResponse{}, err
	}
	resp.Applied = true
	resp.AfterStatus = &after
	resp.ApplyResult = &applied
	resp.SnapshotPath = path
	return resp, nil
}

func (s *BrokerService) PrepareAdmission(residentID string, kind runtimeguard.CallKind, usage tokenledger.Usage, penalties tokenledger.Penalties) (PreparedAdmission, *runtimecore.Engine, error) {
	if residentID == "" {
		return PreparedAdmission{}, nil, fmt.Errorf("resident id is required")
	}
	engine, status, revision, err := s.sessions.LoadResidentWithRevision(residentID)
	if err != nil {
		return PreparedAdmission{}, nil, err
	}
	prepared, err := engine.PrepareCallWithQuotaContext(kind, usage, penalties, runtimecore.QuotaContext{
		RollingUsageValid:   true,
		RollingWindow6HUsed: status.RollingWindow6HUsed,
		RollingDayUsed:      status.RollingDayUsed,
		RollingWeekUsed:     status.RollingWeekUsed,
	})
	if err != nil {
		return PreparedAdmission{}, nil, err
	}
	resp := PreparedAdmission{
		ResidentID:       residentID,
		SnapshotRevision: revision,
		BeforeStatus:     status,
		Quota:            BuildQuotaSnapshot(status),
		Prepared:         prepared,
		Denied:           !prepared.Decision.Allowed,
	}
	if !prepared.Decision.Allowed {
		resp.DeniedReason = append(resp.DeniedReason, prepared.Decision.Reasons...)
	}
	return resp, engine, nil
}

func (s *BrokerService) ApplyPreparedCall(engine *runtimecore.Engine, prepared PreparedAdmission, activity tokenledger.ActivityType) (runtimecore.AppliedCall, ResidentStatus, string, error) {
	if engine == nil {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", fmt.Errorf("engine is required")
	}
	if prepared.Denied {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", fmt.Errorf("prepared call is denied")
	}
	applied, err := engine.ApplyCall(prepared.Prepared, activity)
	if err != nil {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", err
	}
	path, err := s.sessions.SaveResidentExpected(engine, prepared.SnapshotRevision, true)
	if err != nil {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", err
	}
	if _, err := s.sessions.store.AppendQuotaEvent(quotaEventFromApplied(prepared, applied, activity)); err != nil {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", err
	}
	after := s.sessions.BuildResidentStatus(engine, true, path)
	return applied, after, path, nil
}

func quotaEventFromApplied(prepared PreparedAdmission, applied runtimecore.AppliedCall, activity tokenledger.ActivityType) QuotaEvent {
	kind := QuotaEventWorkCall
	switch prepared.Prepared.Kind {
	case runtimeguard.CallKindAcceptance:
		kind = QuotaEventAcceptanceCall
	case runtimeguard.CallKindFinalNotice:
		kind = QuotaEventFinalNotice
	}
	finishedAt := prepared.Prepared.Usage.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	return QuotaEvent{
		ResidentID: prepared.ResidentID,
		Kind:       kind,
		ResponseID: prepared.Prepared.Usage.ResponseID,
		Action:     string(activity),
		StrainCost: prepared.Prepared.Strain.Rounded,
		SparkCost:  prepared.Prepared.Cost.SparkCost,
		CreatedAt:  finishedAt,
		Metadata: map[string]interface{}{
			"model":             prepared.Prepared.Usage.Model,
			"call_kind":         string(prepared.Prepared.Kind),
			"activity":          string(activity),
			"snapshot_revision": strconv.FormatUint(prepared.SnapshotRevision, 10),
			"fatigue_gain":      applied.Fatigue.FatigueGain,
		},
	}
}

func quotaEventFromDeniedProviderUsage(prepared PreparedAdmission, activity tokenledger.ActivityType) QuotaEvent {
	finishedAt := prepared.Prepared.Usage.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	return QuotaEvent{
		ResidentID: prepared.ResidentID,
		Kind:       QuotaEventProviderDenied,
		ResponseID: prepared.Prepared.Usage.ResponseID,
		Action:     string(activity),
		StrainCost: prepared.Prepared.Strain.Rounded,
		SparkCost:  prepared.Prepared.Cost.SparkCost,
		CreatedAt:  finishedAt,
		Metadata: map[string]interface{}{
			"model":             prepared.Prepared.Usage.Model,
			"call_kind":         string(prepared.Prepared.Kind),
			"activity":          string(activity),
			"snapshot_revision": strconv.FormatUint(prepared.SnapshotRevision, 10),
			"denied_reasons":    append([]string(nil), prepared.DeniedReason...),
			"cost_class":        "provider_cost_record_only",
		},
	}
}
