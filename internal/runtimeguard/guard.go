package runtimeguard

import "ai-arena/internal/tokenledger"

type CallKind string

const (
	CallKindWork        CallKind = "work"
	CallKindFinalNotice CallKind = "final_notice"
)

type State struct {
	SparkBalance       float64
	Quota              tokenledger.QuotaState
	ReserveSpark       float64
	ReserveStrain      int
	DebtActive         bool
	DebtAmount         float64
	FinalNoticeUsed    bool
}

type Request struct {
	Kind       CallKind
	SparkCost  float64
	StrainCost int
}

type Decision struct {
	Allowed            bool     `json:"allowed"`
	AllowDebt          bool     `json:"allow_debt"`
	ConsumesReserve    bool     `json:"consumes_reserve"`
	Reasons            []string `json:"reasons,omitempty"`
	RemainingSpark     float64  `json:"remaining_spark"`
	Remaining6H        int      `json:"remaining_6h"`
	WouldExceedQuota   bool     `json:"would_exceed_quota"`
	WouldEnterDebt     bool     `json:"would_enter_debt"`
	LockAfterThisCall  bool     `json:"lock_after_this_call"`
}

func Evaluate(state State, req Request) Decision {
	remaining6H := state.Quota.Window6HCap - state.Quota.Window6HUsed
	if remaining6H < 0 {
		remaining6H = 0
	}

	decision := Decision{
		RemainingSpark: state.SparkBalance,
		Remaining6H:    remaining6H,
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

	if req.Kind == CallKindWork {
		if state.SparkBalance-req.SparkCost < state.ReserveSpark {
			decision.Reasons = append(decision.Reasons, "spark_reserved_for_final_notice")
			return decision
		}
		if remaining6H-req.StrainCost < state.ReserveStrain {
			decision.Reasons = append(decision.Reasons, "quota_reserved_for_final_notice")
			return decision
		}
		decision.Allowed = true
		decision.RemainingSpark = state.SparkBalance - req.SparkCost
		decision.Remaining6H = remaining6H - req.StrainCost
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
		decision.WouldExceedQuota = decision.Remaining6H < 0
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
