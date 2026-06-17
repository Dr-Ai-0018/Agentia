package broker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"ai-arena/internal/memory"
)

func (a *App) RunMemoryCompact(residentID string, apply bool) (memory.CompactReport, error) {
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	return store.CompactResidentWithReport(strings.TrimSpace(residentID), apply)
}

func (a *App) RunMemoryLifecycle(residentID string, apply bool) (memory.LifecycleReport, error) {
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	return store.LifecycleReportWithApply(strings.TrimSpace(residentID), time.Now().UTC(), memory.DefaultPolicy(), apply)
}

func (a *App) RunMemoryLifecycleSafeApply(residentID string, apply bool) (MemoryLifecycleSafeApplyReport, error) {
	residentID = strings.TrimSpace(residentID)
	now := time.Now().UTC()
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	lifecycle, err := store.LifecycleReport(residentID, now, memory.DefaultPolicy())
	if err != nil {
		return MemoryLifecycleSafeApplyReport{}, err
	}
	out := MemoryLifecycleSafeApplyReport{
		ResidentID: residentID,
		Apply:      apply,
		CheckedAt:  now.Format(time.RFC3339Nano),
		Policy:     "only lifecycle items recommended as decay_ok_after_spot_check are eligible; relationship, continuity, identity, rule, history, audit, and private items remain skipped for human review.",
	}
	for _, item := range lifecycle.Items {
		if !item.NeedsAttention {
			continue
		}
		if !safeLifecycleDecayCandidate(item, now) {
			out.Skipped = append(out.Skipped, item)
			continue
		}
		out.CandidateCount++
		out.CandidateMemoryIDs = append(out.CandidateMemoryIDs, item.ID)
		if !apply {
			continue
		}
		if _, err := store.ReviewAbstractMemory(residentID, item.ID, now, memory.MemoryReviewRequest{
			Action:     memory.ActionDecay,
			ReasonNote: "operator_safe_lifecycle_decay",
		}); err != nil {
			return MemoryLifecycleSafeApplyReport{}, err
		}
		out.AppliedCount++
		out.AppliedMemoryIDs = append(out.AppliedMemoryIDs, item.ID)
	}
	out.SkippedCount = len(out.Skipped)
	post, err := store.LifecycleReport(residentID, now, memory.DefaultPolicy())
	if err != nil {
		return MemoryLifecycleSafeApplyReport{}, err
	}
	out.PostLifecycleReport = post
	return out, nil
}

func (a *App) RunMemoryReview(input MemoryReviewInput) (MemoryReviewReport, error) {
	input.ResidentID = strings.TrimSpace(input.ResidentID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.Action = strings.TrimSpace(input.Action)
	if input.ResidentID == "" {
		return MemoryReviewReport{}, errors.New("resident id is required")
	}
	if input.MemoryID == "" {
		return MemoryReviewReport{}, errors.New("memory id is required")
	}
	now := time.Now().UTC()
	fileStore := memory.NewFileStore(filepath.Join(a.root, "memory"))
	before, ok, err := fileStore.GetAbstractMemory(input.ResidentID, input.MemoryID)
	if err != nil {
		return MemoryReviewReport{}, err
	}
	if !ok {
		return MemoryReviewReport{}, errors.New("memory record not found")
	}
	if input.Action == "mark" {
		after, err := markMemoryForResidentReview(before, now, input)
		if err != nil {
			return MemoryReviewReport{}, err
		}
		if input.Apply {
			if err := fileStore.UpsertAbstractMemory(after); err != nil {
				return MemoryReviewReport{}, err
			}
		}
		return MemoryReviewReport{
			ResidentID: input.ResidentID,
			MemoryID:   input.MemoryID,
			Apply:      input.Apply,
			CheckedAt:  now.Format(time.RFC3339Nano),
			Action:     input.Action,
			Reason:     strings.TrimSpace(input.Reason),
			Before:     before,
			After:      after,
		}, nil
	}
	review, err := memoryReviewRequestFromInput(input)
	if err != nil {
		return MemoryReviewReport{}, err
	}
	var after memory.AbstractMemory
	if input.Apply {
		after, err = fileStore.ReviewAbstractMemory(input.ResidentID, input.MemoryID, now, review)
		if err != nil {
			return MemoryReviewReport{}, err
		}
	} else {
		dryRun := memory.NewMemoryStore()
		if err := dryRun.UpsertAbstractMemory(before); err != nil {
			return MemoryReviewReport{}, err
		}
		after, err = dryRun.ReviewAbstractMemory(input.ResidentID, input.MemoryID, now, review)
		if err != nil {
			return MemoryReviewReport{}, err
		}
	}
	return MemoryReviewReport{
		ResidentID: input.ResidentID,
		MemoryID:   input.MemoryID,
		Apply:      input.Apply,
		CheckedAt:  now.Format(time.RFC3339Nano),
		Action:     input.Action,
		Reason:     strings.TrimSpace(input.Reason),
		Before:     before,
		After:      after,
	}, nil
}

func memoryReviewRequestFromInput(input MemoryReviewInput) (memory.MemoryReviewRequest, error) {
	action, err := memoryReviewActionFromString(input.Action)
	if err != nil {
		return memory.MemoryReviewRequest{}, err
	}
	return memory.MemoryReviewRequest{
		Action:       action,
		NewSummary:   input.Summary,
		NewText:      input.Text,
		TargetLayer:  memory.Layer(strings.TrimSpace(input.Layer)),
		ReasonNote:   strings.TrimSpace(input.Reason),
		ResidentNote: "operator_memory_review",
		Reviewer:     "operator",
	}, nil
}

func markMemoryForResidentReview(record memory.AbstractMemory, now time.Time, input MemoryReviewInput) (memory.AbstractMemory, error) {
	if len(record.Governance.HostMay) > 0 && !containsTrimmed(record.Governance.HostMay, "mark") {
		return memory.AbstractMemory{}, errors.New("memory governance does not allow host mark")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "Operator marked this memory for resident review."
	}
	record.Governance.ReviewState = "needs_resident_review"
	record.Governance.ReviewReason = reason
	record.Governance.FlaggedBy = "operator"
	record.Governance.FlaggedAt = now
	record.UpdatedAt = now
	record.Tags = append(record.Tags, "operator_marked_for_resident_review")
	record.Tags = uniqueStrings(record.Tags)
	return record, nil
}

func containsTrimmed(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func memoryReviewActionFromString(action string) (memory.Action, error) {
	switch strings.TrimSpace(action) {
	case "keep":
		return memory.ActionRetain, nil
	case "rewrite":
		return memory.ActionUpdate, nil
	case "compress":
		return memory.ActionSummarize, nil
	case "demote":
		return memory.ActionDecay, nil
	case "delete":
		return memory.ActionDelete, nil
	default:
		return "", errors.New("memory action must be one of: mark, keep, rewrite, compress, demote, delete")
	}
}

func safeLifecycleDecayCandidate(item memory.LifecycleItem, now time.Time) bool {
	if item.RecommendedOperatorAction == "decay_ok_after_spot_check" {
		return true
	}
	return item.Status == memory.StatusDecaying &&
		item.Layer == memory.LayerInstant &&
		item.Action == memory.ActionRetain &&
		!item.ReviewAt.IsZero() &&
		!item.ReviewAt.After(now) &&
		!item.ExpiresAt.IsZero() &&
		item.ExpiresAt.After(now)
}

func (a *App) RunMemoryMaintenanceSummary() MemoryMaintenanceSummary {
	now := time.Now().UTC()
	residents := a.buildMemoryMaintenanceReportsAt(now)
	out := MemoryMaintenanceSummary{
		CheckedAt:      now.Format(time.RFC3339Nano),
		ApplyMode:      "dry_run_only",
		OperatorPolicy: "v0 memory maintenance is operator-only: inspect lifecycle first, then apply per resident only after reviewing dry-run output; no automatic scheduler is enabled.",
		Residents:      residents,
		ResidentCount:  len(a.cfg.Residents),
	}
	for _, item := range residents {
		if item.NeedsAttention {
			out.ResidentsAttention++
		}
		out.LifecycleAttention += item.LifecycleAttention
		out.OperatorDecayCandidates += item.OperatorDecayCandidates
		out.ResidentReviewQueue += item.ResidentReviewQueue
		out.OperatorReviewRequired += item.OperatorReviewRequired
		out.DuplicateHistoryGroups += item.DuplicateHistoryGroups
		out.BeforeHistoryGroups += item.BeforeHistoryGroups
		out.AfterHistoryGroups += item.AfterHistoryGroups
	}
	return out
}

func (a *App) buildMemoryLifecycleReports() []memory.LifecycleReport {
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	now := time.Now().UTC()
	out := make([]memory.LifecycleReport, 0, len(a.cfg.Residents))
	for _, resident := range a.cfg.Residents {
		report, err := store.LifecycleReport(resident.ResidentID, now, memory.DefaultPolicy())
		if err != nil {
			continue
		}
		out = append(out, report)
	}
	return out
}

func (a *App) buildMemoryMaintenanceReports() []ResidentMemoryMaintenance {
	return a.buildMemoryMaintenanceReportsAt(time.Now().UTC())
}

func (a *App) buildMemoryMaintenanceReportsAt(now time.Time) []ResidentMemoryMaintenance {
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	out := make([]ResidentMemoryMaintenance, 0, len(a.cfg.Residents))
	for _, resident := range a.cfg.Residents {
		item := ResidentMemoryMaintenance{ResidentID: resident.ResidentID}
		if report, err := store.LifecycleReport(resident.ResidentID, now, memory.DefaultPolicy()); err == nil {
			item.LifecycleAttention = report.NeedsAttention
			item.OperatorDecayCandidates, item.ResidentReviewQueue, item.OperatorReviewRequired = classifyLifecycleAttention(report.Items, now)
		}
		if report, err := store.CompactResidentWithReport(resident.ResidentID, false); err == nil {
			item.BeforeHistoryGroups = report.BeforeHistoryGroups
			item.AfterHistoryGroups = report.AfterHistoryGroups
			if report.BeforeHistoryGroups > report.AfterHistoryGroups {
				item.DuplicateHistoryGroups = report.BeforeHistoryGroups - report.AfterHistoryGroups
			}
		}
		item.NeedsAttention = item.LifecycleAttention > 0 || item.DuplicateHistoryGroups > 0
		item.RecommendedAction, item.Summary = memoryMaintenanceRecommendation(item)
		if item.NeedsAttention {
			out = append(out, item)
		}
	}
	return out
}

func memoryMaintenanceRecommendation(item ResidentMemoryMaintenance) (string, string) {
	switch {
	case item.LifecycleAttention > 0 && item.DuplicateHistoryGroups > 0:
		return "lifecycle_then_compaction_dry_run", fmt.Sprintf("%d lifecycle items need review and %d duplicate history groups can be compacted; inspect lifecycle first, then compact after confirming summaries.", item.LifecycleAttention, item.DuplicateHistoryGroups)
	case item.OperatorDecayCandidates > 0:
		return "lifecycle_safe_decay_dry_run", fmt.Sprintf("%d memory lifecycle items are safe decay candidates; inspect with memory-lifecycle-safe-apply before applying.", item.OperatorDecayCandidates)
	case item.ResidentReviewQueue > 0 && item.OperatorReviewRequired == 0:
		return "resident_memory_review_queue", fmt.Sprintf("%d memory lifecycle items are marked for resident self-review; do not rewrite protected memories from the host side.", item.ResidentReviewQueue)
	case item.LifecycleAttention > 0:
		return "lifecycle_operator_review", fmt.Sprintf("%d memory lifecycle items need operator review before further action.", item.OperatorReviewRequired)
	case item.DuplicateHistoryGroups > 0:
		return "compaction_dry_run", fmt.Sprintf("%d duplicate history groups can be compacted after reviewing the before/after report.", item.DuplicateHistoryGroups)
	default:
		return "", ""
	}
}

func classifyLifecycleAttention(items []memory.LifecycleItem, now time.Time) (operatorDecayCandidates int, residentReviewQueue int, operatorReviewRequired int) {
	for _, item := range items {
		if !item.NeedsAttention {
			continue
		}
		if safeLifecycleDecayCandidate(item, now) {
			operatorDecayCandidates++
			continue
		}
		if item.RecommendedOperatorAction == "review_for_promotion_or_rewrite" {
			residentReviewQueue++
			continue
		}
		operatorReviewRequired++
	}
	return operatorDecayCandidates, residentReviewQueue, operatorReviewRequired
}
