package newborn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ReportWriter struct{}

func NewReportWriter() *ReportWriter {
	return &ReportWriter{}
}

func (w *ReportWriter) Write(outDir string, started time.Time, report FinalReport) error {
	runDir := filepath.Join(outDir, report.Resident+"-"+started.Format("20060102T150405Z"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	return os.WriteFile(filepath.Join(runDir, "report.json"), raw, 0o644)
}
