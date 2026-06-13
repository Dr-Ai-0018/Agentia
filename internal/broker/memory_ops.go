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
