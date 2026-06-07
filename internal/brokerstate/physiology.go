package brokerstate

import (
	"fmt"
	"time"
)

type ResidentMode string

const (
	ModeAwake      ResidentMode = "awake"
	ModeFocused    ResidentMode = "focused"
	ModeTired      ResidentMode = "tired"
	ModeResting    ResidentMode = "resting"
	ModeSleeping   ResidentMode = "sleeping"
	ModeOverloaded ResidentMode = "overloaded"
)

type ResidentPhysiology struct {
	Mode               ResidentMode `json:"mode"`
	Pressure           string       `json:"pressure"`
	SparkBalance       float64      `json:"spark_balance"`
	Fatigue            int          `json:"fatigue"`
	SleepDebt          int          `json:"sleep_debt"`
	DebtActive         bool         `json:"debt_active"`
	DebtAmount         float64      `json:"debt_amount"`
	Window6HRemaining  int          `json:"window_6h_remaining"`
	DayRemaining       int          `json:"day_remaining"`
	WeekRemaining      int          `json:"week_remaining"`
	QuotaTightestLayer string       `json:"quota_tightest_layer"`
	QuotaTightestRatio float64      `json:"quota_tightest_ratio"`
	RecoverySuggested  bool         `json:"recovery_suggested"`
	RecoveryUrgency    string       `json:"recovery_urgency"`
	SnapshotTime       string       `json:"snapshot_time"`
	SummaryLines       []string     `json:"summary_lines"`
}

func DerivePhysiology(status ResidentStatus, now time.Time) ResidentPhysiology {
	windowRemain := maxInt(0, status.Window6HCap-status.Window6HUsed)
	dayRemain := maxInt(0, status.DayCap-status.DayUsed)
	weekRemain := maxInt(0, status.WeekCap-status.WeekUsed)

	windowRatio := remainRatio(windowRemain, status.Window6HCap)
	dayRatio := remainRatio(dayRemain, status.DayCap)
	weekRatio := remainRatio(weekRemain, status.WeekCap)
	tightestLayer := "6h"
	tightestRatio := windowRatio
	if dayRatio < tightestRatio {
		tightestLayer = "day"
		tightestRatio = dayRatio
	}
	if weekRatio < tightestRatio {
		tightestLayer = "week"
		tightestRatio = weekRatio
	}

	mode := ModeAwake
	pressure := "comfortable"
	recoverySuggested := false
	recoveryUrgency := "none"

	switch {
	case status.DebtActive || tightestRatio <= 0.0:
		mode = ModeOverloaded
		pressure = "critical"
		recoverySuggested = true
		recoveryUrgency = "immediate"
	case tightestRatio <= 0.08 || status.SparkBalance < 0.5 || status.Fatigue >= 2200 || status.SleepDebt >= 18:
		mode = ModeTired
		pressure = "critical"
		recoverySuggested = true
		recoveryUrgency = "high"
	case tightestRatio <= 0.20 || status.SparkBalance < 1.5 || status.Fatigue >= 1200 || status.SleepDebt >= 10:
		mode = ModeTired
		pressure = "tight"
		recoverySuggested = true
		recoveryUrgency = "medium"
	case tightestRatio <= 0.45 || status.SparkBalance < 4.0 || status.Fatigue >= 600 || status.SleepDebt >= 4:
		mode = ModeFocused
		pressure = "watchful"
		recoverySuggested = status.Fatigue >= 900 || status.SleepDebt >= 6
		recoveryUrgency = "low"
	default:
		mode = ModeAwake
		pressure = "comfortable"
	}

	if !status.LastRecoveryAt.IsZero() {
		hoursSinceRecovery := now.Sub(status.LastRecoveryAt).Hours()
		switch {
		case hoursSinceRecovery >= 8 && mode == ModeAwake:
			mode = ModeResting
			recoverySuggested = true
			if recoveryUrgency == "none" {
				recoveryUrgency = "low"
			}
		case hoursSinceRecovery >= 16 && (mode == ModeAwake || mode == ModeFocused):
			mode = ModeSleeping
			recoverySuggested = true
			if recoveryUrgency == "none" || recoveryUrgency == "low" {
				recoveryUrgency = "medium"
			}
		}
	}

	out := ResidentPhysiology{
		Mode:               mode,
		Pressure:           pressure,
		SparkBalance:       status.SparkBalance,
		Fatigue:            status.Fatigue,
		SleepDebt:          status.SleepDebt,
		DebtActive:         status.DebtActive,
		DebtAmount:         status.DebtAmount,
		Window6HRemaining:  windowRemain,
		DayRemaining:       dayRemain,
		WeekRemaining:      weekRemain,
		QuotaTightestLayer: tightestLayer,
		QuotaTightestRatio: tightestRatio,
		RecoverySuggested:  recoverySuggested,
		RecoveryUrgency:    recoveryUrgency,
		SnapshotTime:       now.UTC().Format(time.RFC3339),
	}
	out.SummaryLines = physiologySummaryLines(out)
	return out
}

func physiologySummaryLines(p ResidentPhysiology) []string {
	lines := []string{
		"mode=" + string(p.Mode),
		"pressure=" + p.Pressure,
	}
	if p.QuotaTightestLayer != "" {
		lines = append(lines,
			quotaSummaryLine(p),
			sparkSummaryLine(p),
			fatigueSummaryLine(p),
		)
	}
	if p.DebtActive {
		lines = append(lines, debtSummaryLine(p))
	}
	if p.RecoverySuggested {
		lines = append(lines, "recovery_suggested="+boolWord(p.RecoverySuggested)+" urgency="+p.RecoveryUrgency)
	}
	return lines
}

func quotaSummaryLine(p ResidentPhysiology) string {
	return "quota_tightest=" + p.QuotaTightestLayer +
		" remaining_ratio=" + formatRatio(p.QuotaTightestRatio)
}

func sparkSummaryLine(p ResidentPhysiology) string {
	return "spark_balance=" + formatFloat4(p.SparkBalance)
}

func fatigueSummaryLine(p ResidentPhysiology) string {
	return fmt.Sprintf("fatigue=%d sleep_debt=%d", p.Fatigue, p.SleepDebt)
}

func debtSummaryLine(p ResidentPhysiology) string {
	return "debt_active=true debt_amount=" + formatFloat4(p.DebtAmount)
}

func boolWord(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func formatRatio(v float64) string {
	return formatFloat4(v)
}

func formatFloat4(v float64) string {
	return fmt.Sprintf("%.4f", v)
}

func remainRatio(remaining, cap int) float64 {
	if cap <= 0 {
		return 0
	}
	return float64(remaining) / float64(cap)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
