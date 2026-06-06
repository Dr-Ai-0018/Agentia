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
	if req.ResidentID == "" {
		return AdmitResponse{}, fmt.Errorf("resident id is required")
	}
	engine, status, err := s.sessions.LoadResident(req.ResidentID)
	if err != nil {
		return AdmitResponse{}, err
	}
	prepared, err := engine.PrepareCall(req.Kind, req.Usage, req.Penalties)
	if err != nil {
		return AdmitResponse{}, err
	}

	resp := AdmitResponse{
		BeforeStatus: status,
		Prepared:     prepared,
	}
	if !req.Apply {
		resp.Denied = !prepared.Decision.Allowed
		resp.DeniedReason = append(resp.DeniedReason, prepared.Decision.Reasons...)
		return resp, nil
	}
	if !prepared.Decision.Allowed {
		resp.Denied = true
		resp.DeniedReason = append(resp.DeniedReason, prepared.Decision.Reasons...)
		return resp, nil
	}

	applied, err := engine.ApplyCall(prepared, req.Activity)
	if err != nil {
		return AdmitResponse{}, err
	}
	path, err := s.sessions.SaveResident(engine)
	if err != nil {
		return AdmitResponse{}, err
	}
	after := BuildResidentStatus(engine, true, path)
	resp.Applied = true
	resp.AfterStatus = &after
	resp.ApplyResult = &applied
	resp.SnapshotPath = path
	return resp, nil
}
