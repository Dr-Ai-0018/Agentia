package broker

import (
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
	case item.LifecycleAttention > 0:
		return "lifecycle_dry_run", fmt.Sprintf("%d memory lifecycle items need review before any automatic decay/deletion is enabled.", item.LifecycleAttention)
	case item.DuplicateHistoryGroups > 0:
		return "compaction_dry_run", fmt.Sprintf("%d duplicate history groups can be compacted after reviewing the before/after report.", item.DuplicateHistoryGroups)
	default:
		return "", ""
	}
}
