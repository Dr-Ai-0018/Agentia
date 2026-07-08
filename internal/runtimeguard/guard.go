package runtimeguard

import "ai-arena/internal/tokenledger"

type CallKind string

const (
	CallKindWork        CallKind = "work"
	CallKindAcceptance  CallKind = "acceptance"
	CallKindFinalNotice CallKind = "final_notice"
)

type State struct {
	SparkBalance        float64
	Quota               tokenledger.QuotaState
	RollingUsageValid   bool
	RollingWindow6HUsed int
	RollingDayUsed      int
	RollingWeekUsed     int
	Fatigue             int
	FatigueCap          int
	SleepDebt           int
	ReserveSpark        float64
	ReserveStrain       int
	DebtActive          bool
	DebtAmount          float64
	FinalNoticeUsed     bool
}

type Request struct {
	Kind       CallKind
	SparkCost  float64
	StrainCost int
}

type Decision struct {
	Allowed           bool     `json:"allowed"`
	AllowDebt         bool     `json:"allow_debt"`
	ConsumesReserve   bool     `json:"consumes_reserve"`
	Reasons           []string `json:"reasons,omitempty"`
	RemainingSpark    float64  `json:"remaining_spark"`
	Remaining6H       int      `json:"remaining_6h"`
	RemainingDay      int      `json:"remaining_day"`
	RemainingWeek     int      `json:"remaining_week"`
	WouldExceedQuota  bool     `json:"would_exceed_quota"`
	WouldExceedDay    bool     `json:"would_exceed_day"`
	WouldExceedWeek   bool     `json:"would_exceed_week"`
	WouldEnterDebt    bool     `json:"would_enter_debt"`
	LockAfterThisCall bool     `json:"lock_after_this_call"`
}

func Evaluate(state State, req Request) Decision {
	effective := DeriveEffectiveQuota(state)
	remaining6H := RemainingWindow6H(effective)
	if remaining6H < 0 {
		remaining6H = 0
	}

	decision := Decision{
		RemainingSpark: state.SparkBalance,
		Remaining6H:    remaining6H,
		RemainingDay:   hardRemaining(state.Quota.DayCap, rollingDayUsed(state)),
		RemainingWeek:  hardRemaining(state.Quota.WeekCap, rollingWeekUsed(state)),
	}

	if state.DebtActive {
		decision.Reasons = append(decision.Reasons, "spark_debt_active")
		if req.Kind != CallKindFinalNotice {
			return decision
		}
		if state.FinalNoticeUsed {
			decision.Reasons = append(decision.Reasons, "final_notice_already_used")
			return decision
		}
	}

	if req.Kind == CallKindWork || req.Kind == CallKindAcceptance {
		if state.SparkBalance <= 0 {
			decision.Reasons = append(decision.Reasons, "spark_exhausted")
			return decision
		}
		if quotaExhausted(state.Quota.DayCap, rollingDayUsed(state)) {
			decision.Reasons = append(decision.Reasons, "day_quota_exhausted")
			return decision
		}
		if quotaExhausted(state.Quota.WeekCap, rollingWeekUsed(state)) {
			decision.Reasons = append(decision.Reasons, "week_quota_exhausted")
			return decision
		}
		if FatigueHardCapped(state.Fatigue, state.FatigueCap) {
			decision.Reasons = append(decision.Reasons, "fatigue_exhausted")
			return decision
		}
		decision.RemainingSpark = state.SparkBalance - req.SparkCost
		decision.Remaining6H = remaining6H - req.StrainCost
		decision.RemainingDay = projectedRemaining(state.Quota.DayCap, rollingDayUsed(state), req.StrainCost)
		decision.RemainingWeek = projectedRemaining(state.Quota.WeekCap, rollingWeekUsed(state), req.StrainCost)
		decision.WouldEnterDebt = decision.RemainingSpark < 0
		decision.WouldExceedDay = quotaWouldExceed(state.Quota.DayCap, rollingDayUsed(state), req.StrainCost)
		decision.WouldExceedWeek = quotaWouldExceed(state.Quota.WeekCap, rollingWeekUsed(state), req.StrainCost)
		decision.WouldExceedQuota = decision.WouldExceedDay || decision.WouldExceedWeek
		decision.LockAfterThisCall = decision.WouldEnterDebt || decision.WouldExceedQuota
		if decision.WouldEnterDebt {
			decision.Reasons = append(decision.Reasons, string(req.Kind)+"_would_enter_debt")
			return decision
		}
		if decision.WouldExceedDay {
			decision.Reasons = append(decision.Reasons, string(req.Kind)+"_would_exceed_day_quota")
			return decision
		}
		if decision.WouldExceedWeek {
			decision.Reasons = append(decision.Reasons, string(req.Kind)+"_would_exceed_week_quota")
			return decision
		}
		if decision.RemainingSpark < state.ReserveSpark {
			decision.ConsumesReserve = true
			decision.Reasons = append(decision.Reasons, string(req.Kind)+"_would_consume_spark_reserve")
			return decision
		}
		decision.Allowed = true
		return decision
	}

	if req.Kind == CallKindFinalNotice {
		if state.FinalNoticeUsed {
			decision.Reasons = append(decision.Reasons, "final_notice_already_used")
			return decision
		}
		decision.Allowed = true
		decision.ConsumesReserve = true
		decision.RemainingSpark = state.SparkBalance - req.SparkCost
		decision.Remaining6H = remaining6H - req.StrainCost
		decision.RemainingDay = projectedRemaining(state.Quota.DayCap, rollingDayUsed(state), req.StrainCost)
		decision.RemainingWeek = projectedRemaining(state.Quota.WeekCap, rollingWeekUsed(state), req.StrainCost)
		decision.WouldExceedDay = quotaWouldExceed(state.Quota.DayCap, rollingDayUsed(state), req.StrainCost)
		decision.WouldExceedWeek = quotaWouldExceed(state.Quota.WeekCap, rollingWeekUsed(state), req.StrainCost)
		decision.WouldExceedQuota = decision.WouldExceedDay || decision.WouldExceedWeek
		decision.WouldEnterDebt = decision.RemainingSpark < 0
		decision.AllowDebt = decision.WouldEnterDebt
		decision.LockAfterThisCall = true
		if decision.WouldEnterDebt {
			decision.Reasons = append(decision.Reasons, "final_notice_allowed_to_enter_debt")
		}
		if decision.WouldExceedQuota {
			decision.Reasons = append(decision.Reasons, "final_notice_allowed_after_quota_exhaustion")
		}
		return decision
	}

	decision.Reasons = append(decision.Reasons, "unknown_call_kind")
	return decision
}

func rollingDayUsed(state State) int {
	if state.RollingUsageValid {
		return state.RollingDayUsed
	}
	return state.Quota.DayUsed
}

func rollingWeekUsed(state State) int {
	if state.RollingUsageValid {
		return state.RollingWeekUsed
	}
	return state.Quota.WeekUsed
}

func hardRemaining(cap, used int) int {
	if cap <= 0 {
		return 0
	}
	remaining := cap - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func projectedRemaining(cap, used, spend int) int {
	if cap <= 0 {
		return 0
	}
	return cap - used - spend
}

func quotaExhausted(cap, used int) bool {
	return cap > 0 && used >= cap
}

func quotaWouldExceed(cap, used, spend int) bool {
	return cap > 0 && used+spend > cap
}

func FatigueHardCapped(fatigue, cap int) bool {
	return cap > 0 && fatigue >= cap
}

func FatigueStrainMultiplier(fatigue, cap int) float64 {
	if fatigue <= 0 || cap <= 0 {
		return 1.0
	}
	ratio := float64(fatigue) / float64(cap)
	switch {
	case ratio >= 0.80:
		return 1.35
	case ratio >= 0.55:
		return 1.15
	case ratio >= 0.30:
		return 1.05
	default:
		return 1.0
	}
}

func FatigueZone(fatigue, cap int) string {
	if fatigue <= 0 || cap <= 0 {
		return "fresh"
	}
	ratio := float64(fatigue) / float64(cap)
	switch {
	case ratio >= 1.0:
		return "hard_cap"
	case ratio >= 0.80:
		return "exhausted"
	case ratio >= 0.55:
		return "tired"
	case ratio >= 0.30:
		return "warming_up"
	default:
		return "fresh"
	}
}
