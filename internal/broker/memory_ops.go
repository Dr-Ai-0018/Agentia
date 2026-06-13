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

func (a *App) RunMemoryLifecycle(residentID string) (memory.LifecycleReport, error) {
	store := memory.NewFileStore(filepath.Join(a.root, "memory"))
	return store.LifecycleReport(strings.TrimSpace(residentID), time.Now().UTC(), memory.DefaultPolicy())
}
