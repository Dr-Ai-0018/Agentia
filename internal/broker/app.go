package broker

import (
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

type CallSpec struct {
	Kind      runtimeguard.CallKind
	Usage     tokenledger.Usage
	Penalties tokenledger.Penalties
	Activity  tokenledger.ActivityType
}

type App struct {
	root     string
	cfg      Config
	registry *ResidentRegistry
}

func New(root string) *App {
	cfg := DefaultConfig(root)
	return &App{
		root:     root,
		cfg:      cfg,
		registry: NewResidentRegistry(cfg.Residents),
	}
}

func (a *App) RunDemo(residentID string, start time.Time) (DemoOutput, error) {
	cfg := a.cfg.Runtime
	stateStore := brokerstate.New(join(a.root, "brokerstate-demo"))
	if err := stateStore.DeleteResidentSnapshot(residentID); err != nil {
		return DemoOutput{}, err
	}
	registry := brokerstate.NewRegistry(a.cfg.ResidentProfiles())
	manager := brokerstate.NewSessionManager(stateStore, registry, cfg)
	engine, status, err := manager.LoadResident(residentID)
	if err != nil {
		return DemoOutput{}, err
	}

	workSpec := DefaultWorkSpec(start, "resp_plan_1")
	preparedWork, err := engine.PrepareCall(workSpec.Kind, workSpec.Usage, workSpec.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}

	finalSpec := DefaultFinalNoticeSpec(start.Add(40*time.Minute), "resp_final_notice")
	preparedFinal, err := engine.PrepareCall(finalSpec.Kind, finalSpec.Usage, finalSpec.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}
	appliedFinal, err := engine.ApplyCall(preparedFinal, finalSpec.Activity)
	if err != nil {
		return DemoOutput{}, err
	}

	recovery2h := engine.TickRecovery(start.Add(2 * time.Hour))
	recoverySpec2h := RecoveryProbeSpec(start.Add(2*time.Hour), "resp_after_2h")
	preparedAfter2h, err := engine.PrepareCall(recoverySpec2h.Kind, recoverySpec2h.Usage, recoverySpec2h.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}

	recovery3h := engine.TickRecovery(start.Add(3 * time.Hour))
	recoverySpec3h := RecoveryProbeSpec(start.Add(3*time.Hour), "resp_after_3h")
	preparedAfter3h, err := engine.PrepareCall(recoverySpec3h.Kind, recoverySpec3h.Usage, recoverySpec3h.Penalties)
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

func (a *App) Binding(residentID string) (ResidentBinding, bool) {
	return a.registry.Binding(residentID)
}

func (a *App) RunStatus(residentID string) (brokerstate.ResidentStatus, error) {
	return a.service(false).SelfStatus(residentID)
}

func (a *App) RunPrepareSpec(residentID string, spec CallSpec) (brokerstate.PreparedAdmission, error) {
	prepared, _, err := a.service(false).PrepareAdmission(residentID, spec.Kind, spec.Usage, spec.Penalties)
	if err != nil {
		return brokerstate.PreparedAdmission{}, err
	}
	return prepared, nil
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
	spec, err := DefaultSpecForKind(callKind, now)
	if err != nil {
		return brokerstate.AdmitResponse{}, err
	}
	return a.RunAdmitSpec(residentID, spec, apply)
}

func (a *App) RunAdmitSpec(residentID string, spec CallSpec, apply bool) (brokerstate.AdmitResponse, error) {
	return a.service(false).AdmitCall(brokerstate.AdmitRequest{
		ResidentID: residentID,
		Kind:       spec.Kind,
		Usage:      spec.Usage,
		Penalties:  spec.Penalties,
		Activity:   spec.Activity,
		Apply:      apply,
	})
}

func SpecFromUsage(kind runtimeguard.CallKind, usage tokenledger.Usage, penalties tokenledger.Penalties, activity tokenledger.ActivityType) CallSpec {
	return CallSpec{
		Kind:      kind,
		Usage:     usage,
		Penalties: penalties,
		Activity:  activity,
	}
}

func DefaultSpecForKind(callKind runtimeguard.CallKind, now time.Time) (CallSpec, error) {
	switch callKind {
	case runtimeguard.CallKindWork:
		return DefaultWorkSpec(now, "resp_admit_work"), nil
	case runtimeguard.CallKindFinalNotice:
		return DefaultFinalNoticeSpec(now, "resp_admit_final"), nil
	default:
		return CallSpec{}, brokerstate.ErrUnknownCallKind(callKind)
	}
}

func DefaultWorkSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  1200,
			CachedTokens: 800,
			OutputTokens: 300,
			TotalTokens:  1500,
			Model:        "gpt-5.4",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(4 * time.Second),
		},
		Penalties: tokenledger.Penalties{ToolCallCount: 2},
		Activity:  tokenledger.ActivityNormalWork,
	}
}

func DefaultFinalNoticeSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindFinalNotice,
		Usage: tokenledger.Usage{
			InputTokens:  700,
			CachedTokens: 300,
			OutputTokens: 600,
			TotalTokens:  1300,
			Model:        "gpt-5.4",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(3 * time.Second),
		},
		Penalties: tokenledger.Penalties{ToolCallCount: 1},
		Activity:  tokenledger.ActivityNormalWork,
	}
}

func RecoveryProbeSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  100,
			CachedTokens: 80,
			OutputTokens: 50,
			TotalTokens:  150,
			Model:        "gpt-5.4-mini",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(2 * time.Second),
		},
		Penalties: tokenledger.Penalties{},
		Activity:  tokenledger.ActivityNormalWork,
	}
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
