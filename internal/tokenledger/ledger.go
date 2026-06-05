package tokenledger

import (
	"fmt"
	"math"
	"time"
)

const defaultSparkPerUSD = 100.0

type Usage struct {
	InputTokens  int
	CachedTokens int
	OutputTokens int
	TotalTokens  int
	Model        string
	ResponseID   string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type ActivityType string

const (
	ActivityStatusCheck ActivityType = "status_check"
	ActivityLightWork   ActivityType = "light_work"
	ActivityNormalWork  ActivityType = "normal_work"
	ActivityDeepWork    ActivityType = "deep_work"
)

type Weights struct {
	UncachedInputWeight float64
	CachedInputWeight   float64
	OutputWeight        float64
	ToolCallPenaltyBase float64
	RetryPenaltyBase    float64
}

type PriceBook struct {
	InputUSDPer1M  float64
	CachedUSDPer1M float64
	OutputUSDPer1M float64
}

type Config struct {
	Weights             Weights
	ActivityMultipliers map[ActivityType]float64
	SparkPerUSD         float64
	ModelPrices         map[string]PriceBook
}

type Penalties struct {
	ToolCallCount int
	RetryCount    int
}

type Strain struct {
	UncachedInputTokens int
	WeightedUncached    float64
	WeightedCached      float64
	WeightedOutput      float64
	ToolPenalty         float64
	RetryPenalty        float64
	Raw                 float64
	Rounded             int
}

type CostBreakdown struct {
	InputUSD        float64
	CachedInputUSD  float64
	OutputUSD       float64
	TotalUSD        float64
	SparkCostRaw    float64
	SparkCost       float64
	SparkDisplay    float64
	SparkPerUSD     float64
	ModelPriceFound bool
}

type QuotaState struct {
	Window6HCap  int
	Window6HUsed int
	DayCap       int
	DayUsed      int
	WeekCap      int
	WeekUsed     int
}

type QuotaUpdate struct {
	Before        QuotaState
	After         QuotaState
	Exceeded      bool
	ExceededTiers []string
}

type FatigueUpdate struct {
	Activity       ActivityType
	Multiplier     float64
	FatigueGainRaw float64
	FatigueGain    int
}

func DefaultConfig() Config {
	return Config{
		Weights: Weights{
			UncachedInputWeight: 1.00,
			CachedInputWeight:   0.10,
			OutputWeight:        1.25,
			ToolCallPenaltyBase: 40,
			RetryPenaltyBase:    0,
		},
		ActivityMultipliers: map[ActivityType]float64{
			ActivityStatusCheck: 0.40,
			ActivityLightWork:   0.60,
			ActivityNormalWork:  1.00,
			ActivityDeepWork:    1.20,
		},
		SparkPerUSD: defaultSparkPerUSD,
		ModelPrices: map[string]PriceBook{
			"gpt-5.4": {
				InputUSDPer1M:  2.50,
				CachedUSDPer1M: 0.25,
				OutputUSDPer1M: 15.00,
			},
			"gpt-5.5": {
				InputUSDPer1M:  5.00,
				CachedUSDPer1M: 0.50,
				OutputUSDPer1M: 30.00,
			},
			"gpt-5.4-mini": {
				InputUSDPer1M:  0.75,
				CachedUSDPer1M: 0.075,
				OutputUSDPer1M: 4.50,
			},
		},
	}
}

func ComputeStrain(cfg Config, usage Usage, penalties Penalties) Strain {
	uncached := usage.InputTokens - usage.CachedTokens
	if uncached < 0 {
		uncached = 0
	}

	weightedUncached := float64(uncached) * cfg.Weights.UncachedInputWeight
	weightedCached := float64(usage.CachedTokens) * cfg.Weights.CachedInputWeight
	weightedOutput := float64(usage.OutputTokens) * cfg.Weights.OutputWeight
	toolPenalty := float64(penalties.ToolCallCount) * cfg.Weights.ToolCallPenaltyBase
	retryPenalty := float64(penalties.RetryCount) * cfg.Weights.RetryPenaltyBase
	raw := weightedUncached + weightedCached + weightedOutput + toolPenalty + retryPenalty

	return Strain{
		UncachedInputTokens: uncached,
		WeightedUncached:    weightedUncached,
		WeightedCached:      weightedCached,
		WeightedOutput:      weightedOutput,
		ToolPenalty:         toolPenalty,
		RetryPenalty:        retryPenalty,
		Raw:                 raw,
		Rounded:             int(math.Ceil(raw)),
	}
}

func ComputeCost(cfg Config, usage Usage) (CostBreakdown, error) {
	price, ok := cfg.ModelPrices[usage.Model]
	if !ok {
		return CostBreakdown{}, fmt.Errorf("missing model price: %s", usage.Model)
	}

	uncached := usage.InputTokens - usage.CachedTokens
	if uncached < 0 {
		uncached = 0
	}

	inputUSD := perMillionCost(uncached, price.InputUSDPer1M)
	cachedUSD := perMillionCost(usage.CachedTokens, price.CachedUSDPer1M)
	outputUSD := perMillionCost(usage.OutputTokens, price.OutputUSDPer1M)
	totalUSD := inputUSD + cachedUSD + outputUSD
	sparkRaw := totalUSD * cfg.SparkPerUSD

	return CostBreakdown{
		InputUSD:        inputUSD,
		CachedInputUSD:  cachedUSD,
		OutputUSD:       outputUSD,
		TotalUSD:        totalUSD,
		SparkCostRaw:    sparkRaw,
		SparkCost:       roundToPrecision(sparkRaw, 4),
		SparkDisplay:    displaySpark(sparkRaw),
		SparkPerUSD:     cfg.SparkPerUSD,
		ModelPriceFound: true,
	}, nil
}

func ApplyQuota(state QuotaState, spend int) QuotaUpdate {
	before := state
	after := state
	after.Window6HUsed += spend
	after.DayUsed += spend
	after.WeekUsed += spend

	exceeded := make([]string, 0, 3)
	if after.Window6HUsed > after.Window6HCap {
		exceeded = append(exceeded, "6h")
	}
	if after.DayUsed > after.DayCap {
		exceeded = append(exceeded, "day")
	}
	if after.WeekUsed > after.WeekCap {
		exceeded = append(exceeded, "week")
	}

	return QuotaUpdate{
		Before:        before,
		After:         after,
		Exceeded:      len(exceeded) > 0,
		ExceededTiers: exceeded,
	}
}

func ComputeFatigue(cfg Config, activity ActivityType, strain Strain) (FatigueUpdate, error) {
	multiplier, ok := cfg.ActivityMultipliers[activity]
	if !ok {
		return FatigueUpdate{}, fmt.Errorf("unknown activity type: %s", activity)
	}

	raw := float64(strain.Rounded) * multiplier
	return FatigueUpdate{
		Activity:       activity,
		Multiplier:     multiplier,
		FatigueGainRaw: raw,
		FatigueGain:    int(math.Ceil(raw)),
	}, nil
}

func perMillionCost(tokens int, usdPer1M float64) float64 {
	return (float64(tokens) / 1_000_000.0) * usdPer1M
}

func roundToPrecision(v float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(v*scale) / scale
}

func displaySpark(v float64) float64 {
	switch {
	case v >= 10:
		return roundToPrecision(v, 1)
	case v >= 1:
		return roundToPrecision(v, 2)
	case v >= 0.1:
		return roundToPrecision(v, 3)
	default:
		return roundToPrecision(v, 4)
	}
}
