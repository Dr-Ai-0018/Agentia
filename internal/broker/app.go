package broker

import (
	"fmt"
	"time"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type DemoOutput struct {
	Status              brokerstate.ResidentStatus `json:"status"`
	SnapshotPath        string                     `json:"snapshot_path"`
	PreparedWork        runtimecore.PreparedCall   `json:"prepared_work"`
	PreparedFinalNotice runtimecore.PreparedCall   `json:"prepared_final_notice"`
	AppliedFinalNotice  runtimecore.AppliedCall    `json:"applied_final_notice"`
	RecoveryAfter2H     recovery.TickResult        `json:"recovery_after_2h"`
	PreparedAfter2H     runtimecore.PreparedCall   `json:"prepared_after_2h"`
	RecoveryAfter3H     recovery.TickResult        `json:"recovery_after_3h"`
	PreparedAfter3H     runtimecore.PreparedCall   `json:"prepared_after_3h"`
	FinalState          runtimecore.ResidentState  `json:"final_state"`
}

type RecoveryOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	Recovery     recovery.TickResult        `json:"recovery"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type ResetOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type App struct {
	root string
}

func New(root string) *App {
	return &App{root: root}
}

func (a *App) RunDemo(residentID string, start time.Time) (DemoOutput, error) {
	cfg := brokerstate.DefaultRuntimeConfig()
	stateStore := brokerstate.New(join(a.root, "brokerstate-demo"))
	if err := stateStore.DeleteResidentSnapshot(residentID); err != nil {
		return DemoOutput{}, err
	}
	registry := brokerstate.NewRegistry(brokerstate.DefaultResidentProfiles())
	manager := brokerstate.NewSessionManager(stateStore, registry, cfg)
	engine, status, err := manager.LoadResident(residentID)
	if err != nil {
		return DemoOutput{}, err
	}

	preparedWork, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		ResponseID:   "resp_plan_1",
		StartedAt:    start.Add(5 * time.Minute),
		FinishedAt:   start.Add(5*time.Minute + 4*time.Second),
	}, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		return DemoOutput{}, err
	}

	preparedFinal, err := engine.PrepareCall(runtimeguard.CallKindFinalNotice, tokenledger.Usage{
		InputTokens:  700,
		CachedTokens: 300,
		OutputTokens: 600,
		TotalTokens:  1300,
		Model:        "gpt-5.4",
		ResponseID:   "resp_final_notice",
		StartedAt:    start.Add(45 * time.Minute),
		FinishedAt:   start.Add(45*time.Minute + 3*time.Second),
	}, tokenledger.Penalties{ToolCallCount: 1})
	if err != nil {
		return DemoOutput{}, err
	}
	appliedFinal, err := engine.ApplyCall(preparedFinal, tokenledger.ActivityNormalWork)
	if err != nil {
		return DemoOutput{}, err
	}

	recovery2h := engine.TickRecovery(start.Add(2 * time.Hour))
	preparedAfter2h, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  100,
		CachedTokens: 80,
		OutputTokens: 50,
		TotalTokens:  150,
		Model:        "gpt-5.4-mini",
		ResponseID:   "resp_after_2h",
		StartedAt:    start.Add(2 * time.Hour),
		FinishedAt:   start.Add(2*time.Hour + 2*time.Second),
	}, tokenledger.Penalties{})
	if err != nil {
		return DemoOutput{}, err
	}

	recovery3h := engine.TickRecovery(start.Add(3 * time.Hour))
	preparedAfter3h, err := engine.PrepareCall(runtimeguard.CallKindWork, tokenledger.Usage{
		InputTokens:  100,
		CachedTokens: 80,
		OutputTokens: 50,
		TotalTokens:  150,
		Model:        "gpt-5.4-mini",
		ResponseID:   "resp_after_3h",
		StartedAt:    start.Add(3 * time.Hour),
		FinishedAt:   start.Add(3*time.Hour + 2*time.Second),
	}, tokenledger.Penalties{})
	if err != nil {
		return DemoOutput{}, err
	}

	snapshotPath, err := manager.SaveResident(engine)
	if err != nil {
		return DemoOutput{}, err
	}

	return DemoOutput{
		Status:              status,
		SnapshotPath:        snapshotPath,
		PreparedWork:        preparedWork,
		PreparedFinalNotice: preparedFinal,
		AppliedFinalNotice:  appliedFinal,
		RecoveryAfter2H:     recovery2h,
		PreparedAfter2H:     preparedAfter2h,
		RecoveryAfter3H:     recovery3h,
		PreparedAfter3H:     preparedAfter3h,
		FinalState:          engine.State(),
	}, nil
}

func (a *App) RunStatus(residentID string) (brokerstate.ResidentStatus, error) {
	return a.service(false).SelfStatus(residentID)
}

func (a *App) RunRecover(residentID string, hours float64, now time.Time) (RecoveryOutput, error) {
	service := a.service(false)
	status, err := service.SelfStatus(residentID)
	if err != nil {
		return RecoveryOutput{}, err
	}
	base := status.LastRecoveryAt
	if base.IsZero() {
		base = now
	}
	target := base.Add(time.Duration(hours * float64(time.Hour)))
	updatedStatus, tick, path, err := service.RecoveryTick(residentID, target)
	if err != nil {
		return RecoveryOutput{}, err
	}
	return RecoveryOutput{
		Status:       updatedStatus,
		Recovery:     tick,
		SnapshotPath: path,
	}, nil
}

func (a *App) RunReset(residentID string, now time.Time) (ResetOutput, error) {
	status, path, err := a.service(false).ResetResident(residentID, now)
	if err != nil {
		return ResetOutput{}, err
	}
	return ResetOutput{
		Status:       status,
		SnapshotPath: path,
	}, nil
}

func (a *App) RunAdmit(residentID string, callKind runtimeguard.CallKind, apply bool, now time.Time) (brokerstate.AdmitResponse, error) {
	var usage tokenledger.Usage
	var penalties tokenledger.Penalties
	var activity tokenledger.ActivityType

	switch callKind {
	case runtimeguard.CallKindWork:
		usage = tokenledger.Usage{
			InputTokens:  1200,
			CachedTokens: 800,
			OutputTokens: 300,
			TotalTokens:  1500,
			Model:        "gpt-5.4",
			ResponseID:   "resp_admit_work",
			StartedAt:    now,
			FinishedAt:   now.Add(4 * time.Second),
		}
		penalties = tokenledger.Penalties{ToolCallCount: 2}
		activity = tokenledger.ActivityNormalWork
	case runtimeguard.CallKindFinalNotice:
		usage = tokenledger.Usage{
			InputTokens:  700,
			CachedTokens: 300,
			OutputTokens: 600,
			TotalTokens:  1300,
			Model:        "gpt-5.4",
			ResponseID:   "resp_admit_final",
			StartedAt:    now,
			FinishedAt:   now.Add(3 * time.Second),
		}
		penalties = tokenledger.Penalties{ToolCallCount: 1}
		activity = tokenledger.ActivityNormalWork
	default:
		return brokerstate.AdmitResponse{}, fmt.Errorf("unknown admit kind: %s", callKind)
	}

	return a.service(false).AdmitCall(brokerstate.AdmitRequest{
		ResidentID: residentID,
		Kind:       callKind,
		Usage:      usage,
		Penalties:  penalties,
		Activity:   activity,
		Apply:      apply,
	})
}

func (a *App) service(demo bool) *brokerstate.BrokerService {
	cfg := brokerstate.DefaultRuntimeConfig()
	storeDir := join(a.root, "brokerstate")
	if demo {
		storeDir = join(a.root, "brokerstate-demo")
	}
	store := brokerstate.New(storeDir)
	registry := brokerstate.NewRegistry(brokerstate.DefaultResidentProfiles())
	manager := brokerstate.NewSessionManager(store, registry, cfg)
	return brokerstate.NewBrokerService(manager)
}

func join(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += "/" + part
	}
	return out
}
