package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/sparkledger"
	"ai-arena/internal/tokenledger"
)

type turn struct {
	Name      string
	Usage     tokenledger.Usage
	Penalties tokenledger.Penalties
	Activity  tokenledger.ActivityType
	CallKind  runtimeguard.CallKind
}

type turnResult struct {
	Name        string                    `json:"name"`
	CallKind    runtimeguard.CallKind     `json:"call_kind"`
	Decision    runtimeguard.Decision     `json:"decision"`
	Usage       tokenledger.Usage         `json:"usage,omitempty"`
	Cost        tokenledger.CostBreakdown `json:"cost,omitempty"`
	Strain      tokenledger.Strain        `json:"strain,omitempty"`
	Quota       tokenledger.QuotaUpdate   `json:"quota,omitempty"`
	Fatigue     tokenledger.FatigueUpdate `json:"fatigue,omitempty"`
	SparkEntry  *sparkledger.Entry        `json:"spark_entry,omitempty"`
	Skipped     bool                      `json:"skipped"`
}

type result struct {
	Resident          string                `json:"resident"`
	InitialQuota      tokenledger.QuotaState `json:"initial_quota"`
	FinalQuota        tokenledger.QuotaState `json:"final_quota"`
	FinalAccount      sparkledger.Account   `json:"final_account"`
	SparkEntries      []sparkledger.Entry   `json:"spark_entries"`
	Turns             []turnResult          `json:"turns"`
	DebtActive        bool                  `json:"debt_active"`
	DebtAmount        float64               `json:"debt_amount"`
	FinalNoticeUsed   bool                  `json:"final_notice_used"`
}

func main() {
	cfg := tokenledger.DefaultConfig()
	now := time.Now().UTC()
	resident := "jade"

	quota := tokenledger.QuotaState{
		Window6HCap:  4000,
		Window6HUsed: 300,
		DayCap:       20000,
		DayUsed:      1500,
		WeekCap:      150000,
		WeekUsed:     8000,
	}

	spark := sparkledger.New(resident)
	must(spark.Credit(sparkledger.EntryGrant, 0.6200, "start-of-shift allowance", now))

	turns := []turn{
		{
			Name: "planning-turn",
			Usage: tokenledger.Usage{
				InputTokens:  1200,
				CachedTokens: 800,
				OutputTokens: 300,
				TotalTokens:  1500,
				Model:        "gpt-5.4",
				ResponseID:   "resp_plan_1",
				StartedAt:    now.Add(5 * time.Minute),
				FinishedAt:   now.Add(5*time.Minute + 4*time.Second),
			},
			Penalties: tokenledger.Penalties{ToolCallCount: 2},
			Activity:  tokenledger.ActivityNormalWork,
			CallKind:  runtimeguard.CallKindWork,
		},
		{
			Name: "deep-analysis-burst",
			Usage: tokenledger.Usage{
				InputTokens:  2200,
				CachedTokens: 200,
				OutputTokens: 900,
				TotalTokens:  3100,
				Model:        "gpt-5.5",
				ResponseID:   "resp_deep_1",
				StartedAt:    now.Add(30 * time.Minute),
				FinishedAt:   now.Add(30*time.Minute + 9*time.Second),
			},
			Penalties: tokenledger.Penalties{ToolCallCount: 1},
			Activity:  tokenledger.ActivityDeepWork,
			CallKind:  runtimeguard.CallKindWork,
		},
		{
			Name: "final-exhaustion-notice",
			Usage: tokenledger.Usage{
				InputTokens:  700,
				CachedTokens: 300,
				OutputTokens: 600,
				TotalTokens:  1300,
				Model:        "gpt-5.4",
				ResponseID:   "resp_final_notice",
				StartedAt:    now.Add(45 * time.Minute),
				FinishedAt:   now.Add(45*time.Minute + 3*time.Second),
			},
			Penalties: tokenledger.Penalties{ToolCallCount: 1},
			Activity:  tokenledger.ActivityNormalWork,
			CallKind:  runtimeguard.CallKindFinalNotice,
		},
		{
			Name: "forbidden-extra-call-after-debt",
			Usage: tokenledger.Usage{
				InputTokens:  100,
				CachedTokens: 80,
				OutputTokens: 50,
				TotalTokens:  150,
				Model:        "gpt-5.4-mini",
				ResponseID:   "resp_forbidden",
				StartedAt:    now.Add(55 * time.Minute),
				FinishedAt:   now.Add(55*time.Minute + 2*time.Second),
			},
			Penalties: tokenledger.Penalties{},
			Activity:  tokenledger.ActivityStatusCheck,
			CallKind:  runtimeguard.CallKindWork,
		},
	}

	out := result{
		Resident:     resident,
		InitialQuota: quota,
		Turns:        make([]turnResult, 0, len(turns)),
	}

	finalNoticeUsed := false
	debtActive := false
	debtAmount := 0.0
	const reserveSpark = 0.08
	const reserveStrain = 300

	for _, turn := range turns {
		cost, err := tokenledger.ComputeCost(cfg, turn.Usage)
		if err != nil {
			exitf("%v", err)
		}
		strain := tokenledger.ComputeStrain(cfg, turn.Usage, turn.Penalties)
		decision := runtimeguard.Evaluate(runtimeguard.State{
			SparkBalance:    spark.Account().Balance,
			Quota:           quota,
			ReserveSpark:    reserveSpark,
			ReserveStrain:   reserveStrain,
			DebtActive:      debtActive,
			DebtAmount:      debtAmount,
			FinalNoticeUsed: finalNoticeUsed,
		}, runtimeguard.Request{
			Kind:       turn.CallKind,
			SparkCost:  cost.SparkCost,
			StrainCost: strain.Rounded,
		})

		if !decision.Allowed {
			out.Turns = append(out.Turns, turnResult{
				Name:     turn.Name,
				CallKind: turn.CallKind,
				Decision: decision,
				Skipped:  true,
			})
			continue
		}

		quotaUpdate := tokenledger.ApplyQuota(quota, strain.Rounded)
		fatigue, err := tokenledger.ComputeFatigue(cfg, turn.Activity, strain)
		if err != nil {
			exitf("%v", err)
		}

		var entry sparkledger.Entry
		if turn.CallKind == runtimeguard.CallKindFinalNotice {
			entry, err = spark.DebitAllowDebt(
				sparkledger.EntryCharge,
				cost.SparkCost,
				fmt.Sprintf("%s via %s", turn.Name, turn.Usage.Model),
				turn.Usage.FinishedAt,
			)
			finalNoticeUsed = true
		} else {
			entry, err = spark.Debit(
				sparkledger.EntryCharge,
				cost.SparkCost,
				fmt.Sprintf("%s via %s", turn.Name, turn.Usage.Model),
				turn.Usage.FinishedAt,
			)
		}
		if err != nil {
			exitf("%v", err)
		}

		quota = quotaUpdate.After
		if spark.Account().Balance < 0 {
			debtActive = true
			debtAmount = -spark.Account().Balance
		}

		entryCopy := entry
		out.Turns = append(out.Turns, turnResult{
			Name:       turn.Name,
			CallKind:   turn.CallKind,
			Decision:   decision,
			Usage:      turn.Usage,
			Cost:       cost,
			Strain:     strain,
			Quota:      quotaUpdate,
			Fatigue:    fatigue,
			SparkEntry: &entryCopy,
		})
	}

	out.FinalQuota = quota
	out.FinalAccount = spark.Account()
	out.SparkEntries = spark.Entries()
	out.DebtActive = debtActive
	out.DebtAmount = debtAmount
	out.FinalNoticeUsed = finalNoticeUsed

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		exitf("marshal result: %v", err)
	}
	fmt.Println(string(raw))
}

func must(_ sparkledger.Entry, err error) {
	if err != nil {
		exitf("%v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
