package recovery

import (
	"math"
	"time"

	"ai-arena/internal/tokenledger"
)

type Policy struct {
	SparkRecoveryPerHour  float64
	StrainRecoveryPerHour int
	DayRecoveryPerHour    int
	WeekRecoveryPerHour   int
	FatigueRecoveryPerHour int
	SleepDebtRecoveryPerHour int
}

type State struct {
	SparkBalance float64
	Quota        tokenledger.QuotaState
	Fatigue      int
	SleepDebt    int
	DebtActive   bool
	DebtAmount   float64
	LastTickAt   time.Time
}

type TickResult struct {
	HoursElapsed        float64               `json:"hours_elapsed"`
	SparkRecovered      float64               `json:"spark_recovered"`
	AppliedToDebt       float64               `json:"applied_to_debt"`
	NewSparkBalance     float64               `json:"new_spark_balance"`
	DebtActive          bool                  `json:"debt_active"`
	DebtAmount          float64               `json:"debt_amount"`
	RecoveredStrain     int                   `json:"recovered_strain"`
	FatigueBefore       int                   `json:"fatigue_before"`
	FatigueAfter        int                   `json:"fatigue_after"`
	RecoveredFatigue    int                   `json:"recovered_fatigue"`
	SleepDebtBefore     int                   `json:"sleep_debt_before"`
	SleepDebtAfter      int                   `json:"sleep_debt_after"`
	RecoveredSleepDebt  int                   `json:"recovered_sleep_debt"`
	QuotaBefore         tokenledger.QuotaState `json:"quota_before"`
	QuotaAfter          tokenledger.QuotaState `json:"quota_after"`
	NextRecoveryAt      string                `json:"next_recovery_at,omitempty"`
	RecoveryTickMinutes int                   `json:"recovery_tick_minutes"`
}

func Apply(policy Policy, state State, now time.Time) TickResult {
	if now.Before(state.LastTickAt) {
		now = state.LastTickAt
	}

	hours := now.Sub(state.LastTickAt).Hours()
	if hours < 0 {
		hours = 0
	}

	sparkRecovered := roundToPrecision(hours*policy.SparkRecoveryPerHour, 4)
	recoveredStrain := int(math.Floor(hours * float64(policy.StrainRecoveryPerHour)))
	recoveredFatigue := int(math.Floor(hours * float64(policy.FatigueRecoveryPerHour)))
	recoveredSleepDebt := int(math.Floor(hours * float64(policy.SleepDebtRecoveryPerHour)))

	before := state.Quota
	after := state.Quota
	after.Window6HUsed = maxInt(0, after.Window6HUsed-recoveredStrain)
	after.DayUsed = maxInt(0, after.DayUsed-int(math.Floor(hours*float64(policy.DayRecoveryPerHour))))
	after.WeekUsed = maxInt(0, after.WeekUsed-int(math.Floor(hours*float64(policy.WeekRecoveryPerHour))))
	fatigueAfter := maxInt(0, state.Fatigue-recoveredFatigue)
	sleepDebtAfter := maxInt(0, state.SleepDebt-recoveredSleepDebt)

	newBalance := state.SparkBalance
	debtAmount := state.DebtAmount
	debtActive := state.DebtActive
	appliedToDebt := 0.0

	if sparkRecovered > 0 {
		if debtActive && debtAmount > 0 {
			appliedToDebt = minFloat(debtAmount, sparkRecovered)
			debtAmount = roundToPrecision(debtAmount-appliedToDebt, 4)
			newBalance = roundToPrecision(state.SparkBalance+appliedToDebt, 4)
			if debtAmount <= 0 {
				debtAmount = 0
				debtActive = false
			}
		}

		remaining := roundToPrecision(sparkRecovered-appliedToDebt, 4)
		if remaining > 0 {
			newBalance = roundToPrecision(newBalance+remaining, 4)
		}
	}

	return TickResult{
		HoursElapsed:    roundToPrecision(hours, 4),
		SparkRecovered:  sparkRecovered,
		AppliedToDebt:   appliedToDebt,
		NewSparkBalance: newBalance,
		DebtActive:      debtActive,
		DebtAmount:      debtAmount,
		RecoveredStrain: recoveredStrain,
		FatigueBefore:   state.Fatigue,
		FatigueAfter:    fatigueAfter,
		RecoveredFatigue: recoveredFatigue,
		SleepDebtBefore: state.SleepDebt,
		SleepDebtAfter:  sleepDebtAfter,
		RecoveredSleepDebt: recoveredSleepDebt,
		QuotaBefore:     before,
		QuotaAfter:      after,
		NextRecoveryAt:  NextRecoveryAt(now).Format(time.RFC3339),
		RecoveryTickMinutes: 15,
	}
}

func NextRecoveryAt(now time.Time) time.Time {
	now = now.UTC()
	return now.Truncate(15 * time.Minute).Add(15 * time.Minute)
}

func roundToPrecision(v float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(v*scale) / scale
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
