package orchestrator

func BuildInspectionReport(summary RunSummary) InspectionReport {
	out := InspectionReport{
		RunID:             summary.RunID,
		RetryOf:           summary.Contract.RetryOf,
		Mode:              summary.Mode,
		ResidentsPlanned:  append([]string(nil), summary.Residents...),
		StartedAt:         summary.StartedAt,
		EndedAt:           summary.EndedAt,
		Duration:          summary.Duration,
		Assessment:        summary.Assessment,
		UsefulRuns:        summary.Assessment.UsefulRuns,
		BudgetBlockedRuns: summary.Assessment.BudgetBlockedRuns,
		Residents:         make([]InspectionResidentReport, 0, len(summary.Runs)),
	}
	for _, item := range summary.Runs {
		report := InspectionResidentReport{
			Resident: item.Resident,
			Status:   item.Status,
			Error:    item.Error,
		}
		if item.Report != nil {
			report.Rounds = item.Report.Rounds
			report.StoppedReason = item.Report.StoppedReason
		}
		for _, assessment := range summary.Assessment.Assessments {
			if assessment.Resident != item.Resident {
				continue
			}
			report.BudgetBlocked = assessment.BudgetBlocked
			report.CompletedUseful = assessment.CompletedUseful
			break
		}
		if item.Status == "ok" {
			out.ResidentsFinished++
		}
		if item.Status == "error" {
			out.ResidentsErrored++
		}
		out.Residents = append(out.Residents, report)
	}
	return out
}
