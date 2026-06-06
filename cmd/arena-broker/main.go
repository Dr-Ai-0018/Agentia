package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type demoOutput struct {
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

type recoveryOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	Recovery     recovery.TickResult        `json:"recovery"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type resetOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type admitOutput = brokerstate.AdmitResponse

func main() {
	mode := flag.String("mode", "demo", "Mode: demo|status|recover|reset|admit")
	residentID := flag.String("resident", "jade", "Resident ID")
	hours := flag.Float64("hours", 1, "Recovery hours to advance for recover mode")
	kind := flag.String("kind", "work", "Call kind for admit mode: work|final_notice")
	apply := flag.Bool("apply", false, "Whether admit mode should actually apply the call")
	flag.Parse()

	switch *mode {
	case "demo":
		runDemo(*residentID)
	case "status":
		runStatus(*residentID)
	case "recover":
		runRecover(*residentID, *hours)
	case "reset":
		runReset(*residentID)
	case "admit":
		runAdmit(*residentID, *kind, *apply)
	default:
		exitf("unknown mode: %s", *mode)
	}
}

func runDemo(residentID string) {
	start := time.Now().UTC()
	cfg := brokerstate.DefaultRuntimeConfig()
	stateStore := brokerstate.New(filepathJoin(".agents", "brokerstate-demo"))
	if err := stateStore.DeleteResidentSnapshot(residentID); err != nil {
		exitf("%v", err)
	}
	registry := brokerstate.NewRegistry(brokerstate.DefaultResidentProfiles())
	manager := brokerstate.NewSessionManager(stateStore, registry, cfg)
	engine, status, err := manager.LoadResident(residentID)
	if err != nil {
		exitf("%v", err)
	}

	workUsage := tokenledger.Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		TotalTokens:  1500,
		Model:        "gpt-5.4",
		ResponseID:   "resp_plan_1",
		StartedAt:    start.Add(5 * time.Minute),
		FinishedAt:   start.Add(5*time.Minute + 4*time.Second),
	}
	preparedWork, err := engine.PrepareCall(runtimeguard.CallKindWork, workUsage, tokenledger.Penalties{ToolCallCount: 2})
	if err != nil {
		exitf("%v", err)
	}

	finalUsage := tokenledger.Usage{
		InputTokens:  700,
		CachedTokens: 300,
		OutputTokens: 600,
		TotalTokens:  1300,
		Model:        "gpt-5.4",
		ResponseID:   "resp_final_notice",
		StartedAt:    start.Add(45 * time.Minute),
		FinishedAt:   start.Add(45*time.Minute + 3*time.Second),
	}
	preparedFinal, err := engine.PrepareCall(runtimeguard.CallKindFinalNotice, finalUsage, tokenledger.Penalties{ToolCallCount: 1})
	if err != nil {
		exitf("%v", err)
	}
	appliedFinal, err := engine.ApplyCall(preparedFinal, tokenledger.ActivityNormalWork)
	if err != nil {
		exitf("%v", err)
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
		exitf("%v", err)
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
		exitf("%v", err)
	}

	snapshotPath, err := manager.SaveResident(engine)
	if err != nil {
		exitf("%v", err)
	}

	printJSON(demoOutput{
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
	})
}

func runStatus(residentID string) {
	service := newBrokerService(".agents", false)
	status, err := service.SelfStatus(residentID)
	if err != nil {
		exitf("%v", err)
	}
	printJSON(status)
}

func runRecover(residentID string, hours float64) {
	service := newBrokerService(".agents", false)
	status, err := service.SelfStatus(residentID)
	if err != nil {
		exitf("%v", err)
	}
	base := status.LastRecoveryAt
	if base.IsZero() {
		base = time.Now().UTC()
	}
	target := base.Add(time.Duration(hours * float64(time.Hour)))
	updatedStatus, tick, path, err := service.RecoveryTick(residentID, target)
	if err != nil {
		exitf("%v", err)
	}
	printJSON(recoveryOutput{
		Status:       updatedStatus,
		Recovery:     tick,
		SnapshotPath: path,
	})
}

func runReset(residentID string) {
	service := newBrokerService(".agents", false)
	status, path, err := service.ResetResident(residentID, time.Now().UTC())
	if err != nil {
		exitf("%v", err)
	}
	printJSON(resetOutput{
		Status:       status,
		SnapshotPath: path,
	})
}

func runAdmit(residentID, kind string, apply bool) {
	service := newBrokerService(".agents", false)
	callKind := runtimeguard.CallKind(kind)
	now := time.Now().UTC()

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
		exitf("unknown admit kind: %s", kind)
	}

	resp, err := service.AdmitCall(brokerstate.AdmitRequest{
		ResidentID: residentID,
		Kind:       callKind,
		Usage:      usage,
		Penalties:  penalties,
		Activity:   activity,
		Apply:      apply,
	})
	if err != nil {
		exitf("%v", err)
	}
	printJSON(admitOutput(resp))
}

func newBrokerService(root string, demo bool) *brokerstate.BrokerService {
	cfg := brokerstate.DefaultRuntimeConfig()
	storeDir := filepathJoin(root, "brokerstate")
	if demo {
		storeDir = filepathJoin(root, "brokerstate-demo")
	}
	store := brokerstate.New(storeDir)
	registry := brokerstate.NewRegistry(brokerstate.DefaultResidentProfiles())
	manager := brokerstate.NewSessionManager(store, registry, cfg)
	return brokerstate.NewBrokerService(manager)
}

func printJSON(v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		exitf("marshal json: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func filepathJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += "/" + part
	}
	return out
}
