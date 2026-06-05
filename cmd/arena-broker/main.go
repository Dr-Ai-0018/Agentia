package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type demoOutput struct {
	PreparedWork        runtimecore.PreparedCall `json:"prepared_work"`
	PreparedFinalNotice runtimecore.PreparedCall `json:"prepared_final_notice"`
	AppliedFinalNotice  runtimecore.AppliedCall  `json:"applied_final_notice"`
	RecoveryAfter2H     recovery.TickResult      `json:"recovery_after_2h"`
	PreparedAfter2H     runtimecore.PreparedCall `json:"prepared_after_2h"`
	RecoveryAfter3H     recovery.TickResult      `json:"recovery_after_3h"`
	PreparedAfter3H     runtimecore.PreparedCall `json:"prepared_after_3h"`
	FinalState          runtimecore.ResidentState `json:"final_state"`
}

func main() {
	mode := flag.String("mode", "demo", "Mode: demo")
	flag.Parse()

	switch *mode {
	case "demo":
		runDemo()
	default:
		exitf("unknown mode: %s", *mode)
	}
}

func runDemo() {
	start := time.Now().UTC()
	engine := runtimecore.New(runtimecore.Config{
		TokenPolicy:    tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
			DayRecoveryPerHour:    50,
			WeekRecoveryPerHour:   25,
		},
		ReserveSpark:   0.08,
		ReserveStrain:  300,
	}, "jade", tokenledger.QuotaState{
		Window6HCap:  4000,
		Window6HUsed: 300,
		DayCap:       20000,
		DayUsed:      1500,
		WeekCap:      150000,
		WeekUsed:     8000,
	}, start)

	if _, err := engine.SparkLedger().Credit("grant", 0.62, "start-of-shift allowance", start); err != nil {
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

	out := demoOutput{
		PreparedWork:        preparedWork,
		PreparedFinalNotice: preparedFinal,
		AppliedFinalNotice:  appliedFinal,
		RecoveryAfter2H:     recovery2h,
		PreparedAfter2H:     preparedAfter2h,
		RecoveryAfter3H:     recovery3h,
		PreparedAfter3H:     preparedAfter3h,
		FinalState:          engine.State(),
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		exitf("marshal demo output: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
