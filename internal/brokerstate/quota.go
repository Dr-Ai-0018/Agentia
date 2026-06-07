package brokerstate

type QuotaSnapshot struct {
	Window6HCap           int    `json:"window_6h_cap"`
	Window6HUsed          int    `json:"window_6h_used"`
	Window6HRemaining     int    `json:"window_6h_remaining"`
	EffectiveWindow6HCap  int    `json:"effective_window_6h_cap"`
	EffectiveWindow6HRemaining int `json:"effective_window_6h_remaining"`
	DayCap                int    `json:"day_cap"`
	DayUsed               int    `json:"day_used"`
	DayRemaining          int    `json:"day_remaining"`
	EffectiveDayCap       int    `json:"effective_day_cap"`
	EffectiveDayRemaining int    `json:"effective_day_remaining"`
	WeekCap               int    `json:"week_cap"`
	WeekUsed              int    `json:"week_used"`
	WeekRemaining         int    `json:"week_remaining"`
	EffectiveWeekCap      int    `json:"effective_week_cap"`
	EffectiveWeekRemaining int   `json:"effective_week_remaining"`
	WorkAllowedNow        bool   `json:"work_allowed_now"`
	BlockingReason        string `json:"blocking_reason,omitempty"`
	BlockingSummary       string `json:"blocking_summary,omitempty"`
	NextRecoveryAt        string `json:"next_recovery_at,omitempty"`
	RecoveryTickMinutes   int    `json:"recovery_tick_minutes"`
}

func BuildQuotaSnapshot(status ResidentStatus) QuotaSnapshot {
	out := QuotaSnapshot{
		Window6HCap:              status.Window6HCap,
		Window6HUsed:             status.Window6HUsed,
		Window6HRemaining:        maxInt(0, status.Window6HCap-status.Window6HUsed),
		EffectiveWindow6HCap:     status.EffectiveWindow6HCap,
		EffectiveWindow6HRemaining: maxInt(0, status.EffectiveWindow6HCap-status.Window6HUsed),
		DayCap:                   status.DayCap,
		DayUsed:                  status.DayUsed,
		DayRemaining:             maxInt(0, status.DayCap-status.DayUsed),
		EffectiveDayCap:          status.EffectiveDayCap,
		EffectiveDayRemaining:    maxInt(0, status.EffectiveDayCap-status.DayUsed),
		WeekCap:                  status.WeekCap,
		WeekUsed:                 status.WeekUsed,
		WeekRemaining:            maxInt(0, status.WeekCap-status.WeekUsed),
		EffectiveWeekCap:         status.EffectiveWeekCap,
		EffectiveWeekRemaining:   maxInt(0, status.EffectiveWeekCap-status.WeekUsed),
		NextRecoveryAt:           status.NextRecoveryAt,
		RecoveryTickMinutes:      status.RecoveryTickMinutes,
	}
	out.WorkAllowedNow, out.BlockingReason, out.BlockingSummary = quotaAvailability(status, out)
	return out
}

func quotaAvailability(status ResidentStatus, snapshot QuotaSnapshot) (bool, string, string) {
	switch {
	case status.DebtActive:
		return false, "spark_debt_active", "Work is locked because spark debt is active."
	case snapshot.EffectiveWindow6HRemaining <= 0:
		return false, "effective_window_exhausted", "Work is locked because the current effective 6h quota is exhausted."
	case status.SparkBalance <= 0:
		return false, "spark_exhausted", "Work is locked because spark balance is depleted."
	default:
		return true, "", ""
	}
}
