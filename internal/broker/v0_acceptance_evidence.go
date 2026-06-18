package broker

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type V0AcceptanceEvidenceInput struct {
	CheckID  string
	Status   string
	Operator string
	Summary  string
	Evidence []string
	Command  string
	Apply    bool
}

type V0AcceptanceEvidenceOutput struct {
	Apply  bool                       `json:"apply"`
	Policy string                     `json:"policy"`
	Record V0AcceptanceEvidenceRecord `json:"record"`
	Path   string                     `json:"path,omitempty"`
}

type V0AcceptanceEvidenceListOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Limit       int                          `json:"limit"`
	Count       int                          `json:"count"`
	Records     []V0AcceptanceEvidenceRecord `json:"records"`
}

var validV0AcceptanceEvidenceChecks = map[string]struct{}{
	"cpu_maintenance_regression":          {},
	"disk_maintenance_regression":         {},
	"checkpoint_cleanup_apply_regression": {},
	"final_acceptance_manual_pass":        {},
	"longer_orchestrator_soak":            {},
}

func (a *App) RunV0AcceptanceEvidence(input V0AcceptanceEvidenceInput, now time.Time) (V0AcceptanceEvidenceOutput, error) {
	record, err := buildV0AcceptanceEvidenceRecord(input, now)
	if err != nil {
		return V0AcceptanceEvidenceOutput{}, err
	}
	out := V0AcceptanceEvidenceOutput{
		Apply:  input.Apply,
		Policy: "dry-run by default; --apply records operator-provided evidence only and does not execute validation work",
		Record: record,
	}
	if !input.Apply {
		return out, nil
	}
	dir := filepath.Join(a.root, "operations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return V0AcceptanceEvidenceOutput{}, err
	}
	path := filepath.Join(dir, "v0-acceptance-evidence-"+now.UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return V0AcceptanceEvidenceOutput{}, err
	}
	defer f.Close()
	raw, err := json.Marshal(record)
	if err != nil {
		return V0AcceptanceEvidenceOutput{}, err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return V0AcceptanceEvidenceOutput{}, err
	}
	out.Path = path
	return out, nil
}

func (a *App) RunV0AcceptanceEvidenceList(limit int, now time.Time) (V0AcceptanceEvidenceListOutput, error) {
	if limit <= 0 {
		limit = 8
	}
	records, err := LoadRecentV0AcceptanceEvidence(a.root, limit)
	if err != nil {
		return V0AcceptanceEvidenceListOutput{}, err
	}
	return V0AcceptanceEvidenceListOutput{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Limit:       limit,
		Count:       len(records),
		Records:     records,
	}, nil
}

func buildV0AcceptanceEvidenceRecord(input V0AcceptanceEvidenceInput, now time.Time) (V0AcceptanceEvidenceRecord, error) {
	checkID := strings.TrimSpace(input.CheckID)
	if checkID == "" {
		return V0AcceptanceEvidenceRecord{}, fmt.Errorf("check id is required")
	}
	if !isValidV0AcceptanceEvidenceCheck(checkID) {
		return V0AcceptanceEvidenceRecord{}, fmt.Errorf("unknown v0 acceptance check id: %s", checkID)
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "passed"
	}
	if status != "passed" && status != "failed" {
		return V0AcceptanceEvidenceRecord{}, fmt.Errorf("status must be passed or failed")
	}
	operator := defaultMaintenanceOperator(input.Operator)
	recordedAt := now.UTC().Format(time.RFC3339)
	summary := strings.TrimSpace(input.Summary)
	var evidence []string
	for _, item := range input.Evidence {
		item = strings.TrimSpace(item)
		if item != "" {
			evidence = append(evidence, item)
		}
	}
	return V0AcceptanceEvidenceRecord{
		ID:         "v0-evidence-" + checkID + "-" + now.UTC().Format("20060102T150405.000000000Z"),
		CheckID:    checkID,
		Status:     status,
		RecordedAt: recordedAt,
		Operator:   operator,
		Summary:    summary,
		Evidence:   evidence,
		Command:    strings.TrimSpace(input.Command),
	}, nil
}

func isValidV0AcceptanceEvidenceCheck(checkID string) bool {
	_, ok := validV0AcceptanceEvidenceChecks[strings.TrimSpace(checkID)]
	return ok
}

func v0AcceptanceEvidenceCommandTemplate(checkID string) string {
	checkID = strings.TrimSpace(checkID)
	if !isValidV0AcceptanceEvidenceCheck(checkID) {
		return ""
	}
	return "arena-broker --mode v0-acceptance-evidence --check-id " + checkID +
		" --status passed --operator <operator> --summary '<validated summary>' --body '<evidence lines>' --reason '<validation command or run id>' --apply"
}

func LoadRecentV0AcceptanceEvidence(root string, limit int) ([]V0AcceptanceEvidenceRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(root, "operations", "v0-acceptance-evidence-*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	var records []V0AcceptanceEvidenceRecord
	for _, file := range files {
		items, err := readV0AcceptanceEvidenceFile(file)
		if err != nil {
			return nil, err
		}
		records = append(records, items...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt > records[j].RecordedAt
		}
		return records[i].ID > records[j].ID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func readV0AcceptanceEvidenceFile(path string) ([]V0AcceptanceEvidenceRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []V0AcceptanceEvidenceRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record V0AcceptanceEvidenceRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		record.ID = strings.TrimSpace(record.ID)
		record.CheckID = strings.TrimSpace(record.CheckID)
		record.Status = strings.TrimSpace(record.Status)
		record.RecordedAt = strings.TrimSpace(record.RecordedAt)
		record.Operator = strings.TrimSpace(record.Operator)
		record.Summary = strings.TrimSpace(record.Summary)
		record.Command = strings.TrimSpace(record.Command)
		out = append(out, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
