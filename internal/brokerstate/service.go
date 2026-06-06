package brokerstate

import (
	"fmt"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type BrokerService struct {
	sessions *SessionManager
}

type PreparedAdmission struct {
	ResidentID   string                   `json:"resident_id"`
	BeforeStatus ResidentStatus           `json:"before_status"`
	Prepared     runtimecore.PreparedCall `json:"prepared"`
	Denied       bool                     `json:"denied"`
	DeniedReason []string                 `json:"denied_reason,omitempty"`
}

type AdmitRequest struct {
	ResidentID string
	Kind       runtimeguard.CallKind
	Usage      tokenledger.Usage
	Penalties  tokenledger.Penalties
	Activity   tokenledger.ActivityType
	Apply      bool
}

type AdmitResponse struct {
	BeforeStatus ResidentStatus           `json:"before_status"`
	Prepared     runtimecore.PreparedCall `json:"prepared"`
	Applied      bool                     `json:"applied"`
	AfterStatus  *ResidentStatus          `json:"after_status,omitempty"`
	ApplyResult  *runtimecore.AppliedCall `json:"apply_result,omitempty"`
	SnapshotPath string                   `json:"snapshot_path,omitempty"`
	Denied       bool                     `json:"denied"`
	DeniedReason []string                 `json:"denied_reason,omitempty"`
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
	if residentID == "" {
		return ResidentStatus{}, recovery.TickResult{}, "", fmt.Errorf("resident id is required")
	}
	engine, _, err := s.sessions.LoadResident(residentID)
	if err != nil {
		return ResidentStatus{}, recovery.TickResult{}, "", err
	}
	tick := engine.TickRecovery(now)
	path, err := s.sessions.SaveResident(engine)
	if err != nil {
		return ResidentStatus{}, recovery.TickResult{}, "", err
	}
	status := BuildResidentStatus(engine, true, path)
	return status, tick, path, nil
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
	status := BuildResidentStatus(engine, false, path)
	return status, path, nil
}

func (s *BrokerService) AdmitCall(req AdmitRequest) (AdmitResponse, error) {
	prepared, engine, err := s.PrepareAdmission(req.ResidentID, req.Kind, req.Usage, req.Penalties)
	if err != nil {
		return AdmitResponse{}, err
	}

	resp := AdmitResponse{
		BeforeStatus: prepared.BeforeStatus,
		Prepared:     prepared.Prepared,
		Denied:       prepared.Denied,
		DeniedReason: append([]string(nil), prepared.DeniedReason...),
	}
	if !req.Apply {
		return resp, nil
	}
	if prepared.Denied {
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
	engine, status, err := s.sessions.LoadResident(residentID)
	if err != nil {
		return PreparedAdmission{}, nil, err
	}
	prepared, err := engine.PrepareCall(kind, usage, penalties)
	if err != nil {
		return PreparedAdmission{}, nil, err
	}
	resp := PreparedAdmission{
		ResidentID:   residentID,
		BeforeStatus: status,
		Prepared:     prepared,
		Denied:       !prepared.Decision.Allowed,
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
	path, err := s.sessions.SaveResident(engine)
	if err != nil {
		return runtimecore.AppliedCall{}, ResidentStatus{}, "", err
	}
	after := BuildResidentStatus(engine, true, path)
	return applied, after, path, nil
}
