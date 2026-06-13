package broker

import (
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
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	now := time.Now().UTC()
	out := make([]ResidentMemoryMaintenance, 0, len(a.cfg.Residents))
	for _, resident := range a.cfg.Residents {
		item := ResidentMemoryMaintenance{ResidentID: resident.ResidentID}
		if report, err := store.LifecycleReport(resident.ResidentID, now, memory.DefaultPolicy()); err == nil {
			item.LifecycleAttention = report.NeedsAttention
		}
		if report, err := store.CompactResidentWithReport(resident.ResidentID, false); err == nil && report.BeforeHistoryGroups > report.AfterHistoryGroups {
			item.DuplicateHistoryGroups = report.BeforeHistoryGroups - report.AfterHistoryGroups
		}
		item.NeedsAttention = item.LifecycleAttention > 0 || item.DuplicateHistoryGroups > 0
		if item.NeedsAttention {
			out = append(out, item)
		}
	}
	return out
}
