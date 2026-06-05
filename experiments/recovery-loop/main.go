package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type snapshot struct {
	Label         string                   `json:"label"`
	Recovery      recovery.TickResult      `json:"recovery"`
	WorkDecision  runtimeguard.Decision    `json:"work_decision"`
}

type result struct {
	Resident  string     `json:"resident"`
	Snapshots []snapshot `json:"snapshots"`
}

func main() {
	start := time.Now().UTC()
	policy := recovery.Policy{
		SparkRecoveryPerHour: 0.2,
		StrainRecoveryPerHour: 100,
	}

	state := recovery.State{
		SparkBalance: -0.3875,
		DebtActive:   true,
		DebtAmount:   0.3875,
		Quota: tokenledger.QuotaState{
			Window6HCap:  4000,
			Window6HUsed: 1520,
			DayCap:       20000,
			DayUsed:      2720,
			WeekCap:      150000,
			WeekUsed:     9220,
		},
		LastTickAt: start,
	}

	workReq := runtimeguard.Request{
		Kind:       runtimeguard.CallKindWork,
		SparkCost:  0.10,
		StrainCost: 90,
	}

	snapshots := make([]snapshot, 0, 3)
	for idx, step := range []time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour} {
		recovered := recovery.Apply(policy, state, start.Add(step))
		decision := runtimeguard.Evaluate(runtimeguard.State{
			SparkBalance: recovered.NewSparkBalance,
			Quota:        recovered.QuotaAfter,
			ReserveSpark: 0.08,
			ReserveStrain: 300,
			DebtActive:   recovered.DebtActive,
			DebtAmount:   recovered.DebtAmount,
			FinalNoticeUsed: true,
		}, workReq)

		snapshots = append(snapshots, snapshot{
			Label:        fmt.Sprintf("tick_%d", idx+1),
			Recovery:     recovered,
			WorkDecision: decision,
		})

		state = recovery.State{
			SparkBalance: recovered.NewSparkBalance,
			DebtActive:   recovered.DebtActive,
			DebtAmount:   recovered.DebtAmount,
			Quota:        recovered.QuotaAfter,
			LastTickAt:   start.Add(step),
		}
	}

	raw, err := json.MarshalIndent(result{
		Resident:  "jade",
		Snapshots: snapshots,
	}, "", "  ")
	if err != nil {
		exitf("marshal result: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
