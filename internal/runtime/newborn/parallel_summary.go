package newborn

import "strings"

type ParallelRunAssessment struct {
	Resident        string `json:"resident"`
	Model           string `json:"model,omitempty"`
	Rounds          int    `json:"rounds"`
	StoppedReason   string `json:"stopped_reason,omitempty"`
	BudgetBlocked   bool   `json:"budget_blocked"`
	CompletedUseful bool   `json:"completed_useful"`
}

type ParallelRunSummary struct {
	Residents            int                     `json:"residents"`
	UsefulRuns           int                     `json:"useful_runs"`
	BudgetBlockedRuns    int                     `json:"budget_blocked_runs"`
	Assessments          []ParallelRunAssessment `json:"assessments"`
}

func SummarizeParallelReports(reports []FinalReport) ParallelRunSummary {
	out := ParallelRunSummary{
		Residents:   len(reports),
		Assessments: make([]ParallelRunAssessment, 0, len(reports)),
	}
	for _, report := range reports {
		item := ParallelRunAssessment{
			Resident:      report.Resident,
			Model:         report.Model,
			Rounds:        report.Rounds,
			StoppedReason: report.StoppedReason,
		}
		if report.Rounds > 0 {
			item.CompletedUseful = true
			out.UsefulRuns++
		}
		if hasBudgetBlock(report.StoppedReason) {
			item.BudgetBlocked = true
			out.BudgetBlockedRuns++
		}
		out.Assessments = append(out.Assessments, item)
	}
	return out
}

func hasBudgetBlock(reason string) bool {
	if reason == "" {
		return false
	}
	return strings.Contains(reason, "spark_reserved_for_final_notice") ||
		strings.Contains(reason, "quota_reserved_for_final_notice")
}
