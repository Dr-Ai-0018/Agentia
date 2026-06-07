package runtimeguard

import "ai-arena/internal/tokenledger"

type EffectiveQuota struct {
	Window6HCap  int `json:"window_6h_cap"`
	DayCap       int `json:"day_cap"`
	WeekCap      int `json:"week_cap"`
	Window6HUsed int `json:"window_6h_used"`
	DayUsed      int `json:"day_used"`
	WeekUsed     int `json:"week_used"`
}

func DeriveEffectiveQuota(state State) EffectiveQuota {
	windowPenalty := fatiguePenaltyRatio(state.Fatigue, state.SleepDebt, 0.60)
	dayPenalty := fatiguePenaltyRatio(state.Fatigue, state.SleepDebt, 0.45)
	weekPenalty := fatiguePenaltyRatio(state.Fatigue, state.SleepDebt, 0.30)

	return EffectiveQuota{
		Window6HCap:  applyPenaltyCap(state.Quota.Window6HCap, windowPenalty),
		DayCap:       applyPenaltyCap(state.Quota.DayCap, dayPenalty),
		WeekCap:      applyPenaltyCap(state.Quota.WeekCap, weekPenalty),
		Window6HUsed: state.Quota.Window6HUsed,
		DayUsed:      state.Quota.DayUsed,
		WeekUsed:     state.Quota.WeekUsed,
	}
}

func fatiguePenaltyRatio(fatigue, sleepDebt int, maxPenalty float64) float64 {
	fatiguePenalty := 0.0
	sleepPenalty := 0.0

	switch {
	case fatigue >= 3000:
		fatiguePenalty = 0.45
	case fatigue >= 2000:
		fatiguePenalty = 0.30
	case fatigue >= 1200:
		fatiguePenalty = 0.18
	case fatigue >= 600:
		fatiguePenalty = 0.08
	}

	switch {
	case sleepDebt >= 20:
		sleepPenalty = 0.20
	case sleepDebt >= 12:
		sleepPenalty = 0.12
	case sleepDebt >= 6:
		sleepPenalty = 0.06
	case sleepDebt >= 2:
		sleepPenalty = 0.03
	}

	total := fatiguePenalty + sleepPenalty
	if total > maxPenalty {
		return maxPenalty
	}
	return total
}

func applyPenaltyCap(cap int, penalty float64) int {
	if cap <= 0 {
		return 0
	}
	if penalty <= 0 {
		return cap
	}
	effective := int(float64(cap) * (1.0 - penalty))
	floor := cap / 4
	if floor < 1 {
		floor = 1
	}
	if effective < floor {
		return floor
	}
	return effective
}

func RemainingWindow6H(e EffectiveQuota) int {
	remaining := e.Window6HCap - e.Window6HUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func RemainingDay(e EffectiveQuota) int {
	remaining := e.DayCap - e.DayUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func RemainingWeek(e EffectiveQuota) int {
	remaining := e.WeekCap - e.WeekUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func ToQuotaState(e EffectiveQuota) tokenledger.QuotaState {
	return tokenledger.QuotaState{
		Window6HCap:  e.Window6HCap,
		Window6HUsed: e.Window6HUsed,
		DayCap:       e.DayCap,
		DayUsed:      e.DayUsed,
		WeekCap:      e.WeekCap,
		WeekUsed:     e.WeekUsed,
	}
}
