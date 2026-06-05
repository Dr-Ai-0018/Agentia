package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/tokenledger"
)

type sample struct {
	Name      string
	Usage     tokenledger.Usage
	Penalties tokenledger.Penalties
	Activity  tokenledger.ActivityType
	Quota     tokenledger.QuotaState
}

type result struct {
	Name        string                    `json:"name"`
	Usage       tokenledger.Usage         `json:"usage"`
	Cost        tokenledger.CostBreakdown `json:"internal_cost"`
	Strain      tokenledger.Strain        `json:"internal_strain"`
	QuotaUpdate tokenledger.QuotaUpdate   `json:"quota_update"`
	Fatigue     tokenledger.FatigueUpdate `json:"fatigue"`
}

func main() {
	cfg := tokenledger.DefaultConfig()
	now := time.Now().UTC()

	samples := []sample{
		{
			Name: "cached-heavy-normal-work",
			Usage: tokenledger.Usage{
				InputTokens:  1200,
				CachedTokens: 800,
				OutputTokens: 300,
				TotalTokens:  1500,
				Model:        "gpt-5.4",
				ResponseID:   "resp_cached_heavy",
				StartedAt:    now,
				FinishedAt:   now.Add(4 * time.Second),
			},
			Penalties: tokenledger.Penalties{ToolCallCount: 2},
			Activity:  tokenledger.ActivityNormalWork,
			Quota: tokenledger.QuotaState{
				Window6HCap:  3000,
				Window6HUsed: 900,
				DayCap:       18000,
				DayUsed:      2400,
				WeekCap:      140000,
				WeekUsed:     12000,
			},
		},
		{
			Name: "deep-work-near-window-limit",
			Usage: tokenledger.Usage{
				InputTokens:  2200,
				CachedTokens: 200,
				OutputTokens: 900,
				TotalTokens:  3100,
				Model:        "gpt-5.5",
				ResponseID:   "resp_deep_work",
				StartedAt:    now.Add(10 * time.Second),
				FinishedAt:   now.Add(19 * time.Second),
			},
			Penalties: tokenledger.Penalties{ToolCallCount: 1},
			Activity:  tokenledger.ActivityDeepWork,
			Quota: tokenledger.QuotaState{
				Window6HCap:  3000,
				Window6HUsed: 1200,
				DayCap:       18000,
				DayUsed:      5300,
				WeekCap:      140000,
				WeekUsed:     26000,
			},
		},
		{
			Name: "light-status-check",
			Usage: tokenledger.Usage{
				InputTokens:  180,
				CachedTokens: 120,
				OutputTokens: 40,
				TotalTokens:  220,
				Model:        "gpt-5.4-mini",
				ResponseID:   "resp_status_check",
				StartedAt:    now.Add(30 * time.Second),
				FinishedAt:   now.Add(32 * time.Second),
			},
			Penalties: tokenledger.Penalties{},
			Activity:  tokenledger.ActivityStatusCheck,
			Quota: tokenledger.QuotaState{
				Window6HCap:  3000,
				Window6HUsed: 400,
				DayCap:       18000,
				DayUsed:      1100,
				WeekCap:      140000,
				WeekUsed:     9000,
			},
		},
	}

	results := make([]result, 0, len(samples))
	for _, sample := range samples {
		cost, err := tokenledger.ComputeCost(cfg, sample.Usage)
		if err != nil {
			exitf("%v", err)
		}
		strain := tokenledger.ComputeStrain(cfg, sample.Usage, sample.Penalties)
		quota := tokenledger.ApplyQuota(sample.Quota, strain.Rounded)
		fatigue, err := tokenledger.ComputeFatigue(cfg, sample.Activity, strain)
		if err != nil {
			exitf("%v", err)
		}

		results = append(results, result{
			Name:        sample.Name,
			Usage:       sample.Usage,
			Cost:        cost,
			Strain:      strain,
			QuotaUpdate: quota,
			Fatigue:     fatigue,
		})
	}

	raw, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		exitf("marshal result: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
