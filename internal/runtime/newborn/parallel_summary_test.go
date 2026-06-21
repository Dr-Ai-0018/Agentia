package newborn

import "testing"

func TestSummarizeParallelReports(t *testing.T) {
	summary := SummarizeParallelReports([]FinalReport{
		{Resident: "jade", Model: "gpt-5.4", Rounds: 1, StoppedReason: "broker_preflight_denied: effective_window_exhausted"},
		{Resident: "amber", Model: "gpt-5.5", Rounds: 0, StoppedReason: "broker_preflight_denied: spark_debt_active"},
		{Resident: "onyx", Model: "gpt-5.4", Rounds: 1, StoppedReason: "broker_preflight_denied: work_would_enter_debt"},
	})
	if summary.Residents != 3 {
		t.Fatalf("expected 3 residents, got %d", summary.Residents)
	}
	if summary.UsefulRuns != 2 {
		t.Fatalf("expected 2 useful runs, got %d", summary.UsefulRuns)
	}
	if summary.BudgetBlockedRuns != 3 {
		t.Fatalf("expected 3 budget blocked runs, got %d", summary.BudgetBlockedRuns)
	}
	if !summary.Assessments[1].BudgetBlocked {
		t.Fatalf("expected amber assessment to be budget blocked")
	}
}

func TestSummarizeParallelReportsIgnoresShortWindowAsBudgetBlock(t *testing.T) {
	summary := SummarizeParallelReports([]FinalReport{
		{Resident: "jade", Model: "gpt-5.4", Rounds: 0, StoppedReason: "duration_window_too_short"},
	})
	if summary.BudgetBlockedRuns != 0 {
		t.Fatalf("expected no budget block for short window, got %d", summary.BudgetBlockedRuns)
	}
	if summary.Assessments[0].BudgetBlocked {
		t.Fatalf("short window should not be marked as budget blocked")
	}
}
