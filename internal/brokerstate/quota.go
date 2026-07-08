package brokerstate

type QuotaSnapshot struct {
	Window6HCap                int    `json:"window_6h_cap"`
	Window6HUsed               int    `json:"window_6h_used"`
	RollingWindow6HUsed        int    `json:"rolling_6h_used,omitempty"`
	Window6HRemaining          int    `json:"window_6h_remaining"`
	EffectiveWindow6HCap       int    `json:"effective_window_6h_cap"`
	EffectiveWindow6HRemaining int    `json:"effective_window_6h_remaining"`
	DayCap                     int    `json:"day_cap"`
	DayUsed                    int    `json:"day_used"`
	RollingDayUsed             int    `json:"rolling_day_used,omitempty"`
	RollingDayRemaining        int    `json:"rolling_day_remaining,omitempty"`
	DayRemaining               int    `json:"day_remaining"`
	EffectiveDayCap            int    `json:"effective_day_cap"`
	EffectiveDayRemaining      int    `json:"effective_day_remaining"`
	WeekCap                    int    `json:"week_cap"`
	WeekUsed                   int    `json:"week_used"`
	RollingWeekUsed            int    `json:"rolling_week_used,omitempty"`
	RollingWeekRemaining       int    `json:"rolling_week_remaining,omitempty"`
	WeekRemaining              int    `json:"week_remaining"`
	EffectiveWeekCap           int    `json:"effective_week_cap"`
	EffectiveWeekRemaining     int    `json:"effective_week_remaining"`
	WorkAllowedNow             bool   `json:"work_allowed_now"`
	BlockingReason             string `json:"blocking_reason,omitempty"`
	BlockingSummary            string `json:"blocking_summary,omitempty"`
	RecoveryMode               string `json:"recovery_mode,omitempty"`
	NextRecoveryAt             string `json:"next_recovery_at,omitempty"`
	RecoveryTickMinutes        int    `json:"recovery_tick_minutes"`
}

func BuildQuotaSnapshot(status ResidentStatus) QuotaSnapshot {
	out := QuotaSnapshot{
		Window6HCap:                status.Window6HCap,
		Window6HUsed:               status.Window6HUsed,
		RollingWindow6HUsed:        status.RollingWindow6HUsed,
		Window6HRemaining:          maxInt(0, status.Window6HCap-status.Window6HUsed),
		EffectiveWindow6HCap:       status.EffectiveWindow6HCap,
		EffectiveWindow6HRemaining: maxInt(0, status.EffectiveWindow6HCap-status.Window6HUsed),
		DayCap:                     status.DayCap,
		DayUsed:                    status.DayUsed,
		RollingDayUsed:             status.RollingDayUsed,
		RollingDayRemaining:        maxInt(0, status.DayCap-status.RollingDayUsed),
		DayRemaining:               maxInt(0, status.DayCap-status.DayUsed),
		EffectiveDayCap:            status.EffectiveDayCap,
		EffectiveDayRemaining:      maxInt(0, status.EffectiveDayCap-status.DayUsed),
		WeekCap:                    status.WeekCap,
		WeekUsed:                   status.WeekUsed,
		RollingWeekUsed:            status.RollingWeekUsed,
		RollingWeekRemaining:       maxInt(0, status.WeekCap-status.RollingWeekUsed),
		WeekRemaining:              maxInt(0, status.WeekCap-status.WeekUsed),
		EffectiveWeekCap:           status.EffectiveWeekCap,
		EffectiveWeekRemaining:     maxInt(0, status.EffectiveWeekCap-status.WeekUsed),
		RecoveryMode:               status.RecoveryMode,
		NextRecoveryAt:             status.NextRecoveryAt,
		RecoveryTickMinutes:        status.RecoveryTickMinutes,
	}
	out.WorkAllowedNow, out.BlockingReason, out.BlockingSummary = quotaAvailability(status, out)
	return out
}

func quotaAvailability(status ResidentStatus, snapshot QuotaSnapshot) (bool, string, string) {
	switch {
	case status.DebtActive:
		return false, "spark_debt_active", "Work is locked because spark debt is active."
	case status.SparkBalance <= 0:
		return false, "spark_exhausted", "Work is locked because spark balance is depleted."
	case status.DayCap > 0 && status.RollingDayUsed >= status.DayCap:
		return false, "day_quota_exhausted", "Work is locked because the rolling 24h quota is exhausted."
	case status.WeekCap > 0 && status.RollingWeekUsed >= status.WeekCap:
		return false, "week_quota_exhausted", "Work is locked because the rolling 7d quota is exhausted."
	default:
		return true, "", ""
	}
}
