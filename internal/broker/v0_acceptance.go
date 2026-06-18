package broker

import (
	"fmt"
	"strings"
	"time"
)

const (
	v0AcceptancePass    = "pass"
	v0AcceptanceWarn    = "warn"
	v0AcceptanceFail    = "fail"
	v0AcceptancePending = "pending"
)

func (a *App) RunV0Acceptance(limit int) (V0AcceptanceOutput, error) {
	readiness, err := a.RunV0Readiness(limit)
	if err != nil {
		return V0AcceptanceOutput{}, err
	}
	evidence, err := LoadRecentV0AcceptanceEvidence(a.root, 64)
	if err != nil {
		return V0AcceptanceOutput{}, err
	}
	return BuildV0Acceptance(readiness, BuildV0Runbook(time.Now().UTC()), evidence, time.Now().UTC(), "live"), nil
}

func (a *App) RunV0AcceptanceFromSnapshot(limit int) (V0AcceptanceOutput, error) {
	readiness, err := a.RunV0ReadinessFromSnapshot(limit)
	if err != nil {
		return V0AcceptanceOutput{}, err
	}
	evidence, err := LoadRecentV0AcceptanceEvidence(a.root, 64)
	if err != nil {
		return V0AcceptanceOutput{}, err
	}
	return BuildV0Acceptance(readiness, BuildV0Runbook(time.Now().UTC()), evidence, time.Now().UTC(), "cached"), nil
}

func BuildV0Acceptance(readiness V0ReadinessOutput, runbook V0RunbookOutput, evidence []V0AcceptanceEvidenceRecord, now time.Time, source string) V0AcceptanceOutput {
	evidenceByCheck := latestPassingV0AcceptanceEvidence(evidence)
	out := V0AcceptanceOutput{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Source:      strings.TrimSpace(source),
		Summary: V0AcceptanceSummary{
			WeightedPercent:      readiness.Completion.WeightedPercent,
			EvidenceRecords:      len(evidence),
			RunbookSections:      len(runbook.Sections),
			RunbookApprovalSteps: countRunbookApprovalSteps(runbook),
		},
	}
	if out.Source == "" {
		out.Source = "summary"
	}
	for _, item := range readiness.Items {
		addAcceptanceCheck(&out, acceptanceCheckFromReadinessItem(item))
	}
	addAcceptanceCheck(&out, V0AcceptanceCheck{
		ID:       "operator_runbook",
		Title:    "Operator runbook is available as structured output",
		Category: "automatic",
		Status:   runbookAvailabilityStatus(runbook),
		Required: true,
		Evidence: []string{
			fmt.Sprintf("sections=%d approval_steps=%d", len(runbook.Sections), countRunbookApprovalSteps(runbook)),
		},
		Command: "arena-broker --mode v0-runbook",
	})
	for _, step := range readiness.Completion.RecommendedSteps {
		if !step.RequiresApproval && !step.BlocksRelease {
			continue
		}
		addAcceptanceCheck(&out, acceptanceCheckFromRecommendedStep(step, evidenceByCheck[step.ID]))
	}
	finalizeV0Acceptance(&out)
	return out
}

func acceptanceCheckFromReadinessItem(item V0ReadinessItem) V0AcceptanceCheck {
	check := V0AcceptanceCheck{
		ID:       item.ID,
		Title:    item.Title,
		Category: "automatic",
		Status:   item.Status,
		Required: item.Required,
		Evidence: append([]string(nil), item.Evidence...),
	}
	if item.Required && item.Status == v0ReadinessFail {
		check.BlocksRelease = true
	}
	return check
}

func acceptanceCheckFromRecommendedStep(step V0RecommendedStep, evidence *V0AcceptanceEvidenceRecord) V0AcceptanceCheck {
	status := v0AcceptancePending
	if !step.BlocksRelease {
		status = v0AcceptanceWarn
	}
	check := V0AcceptanceCheck{
		ID:               step.ID,
		Title:            step.Title,
		Category:         "manual",
		Status:           status,
		Required:         step.BlocksRelease,
		BlocksRelease:    step.BlocksRelease,
		RequiresApproval: step.RequiresApproval,
		Evidence:         []string{step.Reason},
		Command:          step.Command,
		EvidenceCommand:  v0AcceptanceEvidenceCommandTemplate(step.ID),
	}
	if evidence != nil {
		check.Status = v0AcceptancePass
		check.BlocksRelease = false
		check.RequiresApproval = false
		check.Evidence = append(check.Evidence,
			fmt.Sprintf("accepted evidence %s recorded_at=%s operator=%s", evidence.ID, evidence.RecordedAt, evidence.Operator),
		)
		if evidence.Summary != "" {
			check.Evidence = append(check.Evidence, evidence.Summary)
		}
		check.Evidence = append(check.Evidence, evidence.Evidence...)
	}
	return check
}

func runbookAvailabilityStatus(runbook V0RunbookOutput) string {
	if len(runbook.Sections) == 0 || strings.TrimSpace(runbook.Scope) == "" {
		return v0AcceptanceFail
	}
	return v0AcceptancePass
}

func addAcceptanceCheck(out *V0AcceptanceOutput, check V0AcceptanceCheck) {
	out.Checks = append(out.Checks, check)
	switch check.Category {
	case "manual":
		if check.Status == v0AcceptancePending {
			out.Summary.ManualPending++
		}
		if check.BlocksRelease {
			out.Summary.ManualBlocking++
		}
		if check.RequiresApproval {
			out.Summary.ApprovalRequired++
		}
	default:
		switch check.Status {
		case v0AcceptancePass:
			out.Summary.AutomaticPassed++
		case v0AcceptanceWarn:
			out.Summary.AutomaticWarnings++
		case v0AcceptanceFail:
			out.Summary.AutomaticFailed++
		}
	}
}

func finalizeV0Acceptance(out *V0AcceptanceOutput) {
	switch {
	case out.Summary.AutomaticFailed > 0:
		out.Gate = "blocked_by_automatic_failures"
	case out.Summary.ManualBlocking > 0:
		out.Gate = "blocked_by_manual_validation"
	case out.Summary.AutomaticWarnings > 0 || out.Summary.ManualPending > 0:
		out.Gate = "ready_with_warnings"
	default:
		out.Gate = "accepted"
	}
	for _, check := range out.Checks {
		if check.Status == v0AcceptancePass {
			continue
		}
		if check.BlocksRelease || check.Status == v0AcceptanceFail || check.Status == v0AcceptancePending {
			out.NextActions = append(out.NextActions, fmt.Sprintf("%s: %s", check.ID, check.Title))
		}
	}
}

func countRunbookApprovalSteps(runbook V0RunbookOutput) int {
	count := 0
	for _, section := range runbook.Sections {
		for _, step := range section.Steps {
			if step.RequiresApproval {
				count++
			}
		}
	}
	return count
}

func latestPassingV0AcceptanceEvidence(records []V0AcceptanceEvidenceRecord) map[string]*V0AcceptanceEvidenceRecord {
	out := map[string]*V0AcceptanceEvidenceRecord{}
	for i := range records {
		record := records[i]
		if strings.TrimSpace(record.CheckID) == "" || !strings.EqualFold(strings.TrimSpace(record.Status), "passed") {
			continue
		}
		current, ok := out[record.CheckID]
		if ok && current.RecordedAt >= record.RecordedAt {
			continue
		}
		copyRecord := record
		out[record.CheckID] = &copyRecord
	}
	return out
}
