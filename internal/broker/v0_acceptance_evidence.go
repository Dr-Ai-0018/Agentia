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

type V0AcceptanceEvidenceTemplateOutput struct {
	GeneratedAt string                         `json:"generated_at"`
	Source      string                         `json:"source"`
	Policy      []string                       `json:"policy"`
	Count       int                            `json:"count"`
	Templates   []V0AcceptanceEvidenceTemplate `json:"templates"`
}

type V0AcceptanceEvidenceTemplate struct {
	CheckID            string   `json:"check_id"`
	Title              string   `json:"title"`
	Status             string   `json:"status"`
	Required           bool     `json:"required"`
	BlocksRelease      bool     `json:"blocks_release"`
	RequiresApproval   bool     `json:"requires_approval"`
	ValidationCommand  string   `json:"validation_command,omitempty"`
	EvidenceCommand    string   `json:"evidence_command"`
	ValidationEvidence []string `json:"validation_evidence,omitempty"`
	Notes              []string `json:"notes,omitempty"`
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

func (a *App) RunV0AcceptanceEvidenceTemplate(limit int, checkID string, now time.Time) (V0AcceptanceEvidenceTemplateOutput, error) {
	checkID = strings.TrimSpace(checkID)
	if checkID != "" && !isValidV0AcceptanceEvidenceCheck(checkID) {
		return V0AcceptanceEvidenceTemplateOutput{}, fmt.Errorf("unknown v0 acceptance check id: %s", checkID)
	}
	acceptance, err := a.RunV0AcceptanceFromSnapshot(limit)
	if err != nil {
		return V0AcceptanceEvidenceTemplateOutput{}, err
	}
	return BuildV0AcceptanceEvidenceTemplate(acceptance, checkID, now), nil
}

func BuildV0AcceptanceEvidenceTemplate(acceptance V0AcceptanceOutput, checkID string, now time.Time) V0AcceptanceEvidenceTemplateOutput {
	checkID = strings.TrimSpace(checkID)
	out := V0AcceptanceEvidenceTemplateOutput{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Source:      acceptance.Source,
		Policy: []string{
			"template output is read-only and never records evidence",
			"run the validation command first; use evidence_command only after real validation succeeds",
			"approval-required checks must not be executed or recorded without explicit operator approval",
		},
	}
	for _, check := range acceptance.Checks {
		if check.Category != "manual" || strings.TrimSpace(check.EvidenceCommand) == "" {
			continue
		}
		if checkID != "" && check.ID != checkID {
			continue
		}
		out.Templates = append(out.Templates, V0AcceptanceEvidenceTemplate{
			CheckID:            check.ID,
			Title:              check.Title,
			Status:             check.Status,
			Required:           check.Required,
			BlocksRelease:      check.BlocksRelease,
			RequiresApproval:   check.RequiresApproval,
			ValidationCommand:  check.Command,
			EvidenceCommand:    check.EvidenceCommand,
			ValidationEvidence: append([]string(nil), check.Evidence...),
			Notes:              v0AcceptanceEvidenceTemplateNotes(check),
		})
	}
	out.Count = len(out.Templates)
	return out
}

func v0AcceptanceEvidenceTemplateNotes(check V0AcceptanceCheck) []string {
	notes := []string{"Evidence records document validation; they do not perform validation."}
	if check.RequiresApproval {
		notes = append(notes, "Requires explicit operator approval before validation or evidence recording.")
	}
	if check.BlocksRelease {
		notes = append(notes, "Passing evidence for this check can clear a release blocker.")
	}
	if check.Status == v0AcceptancePass {
		notes = append(notes, "Already has passing evidence in the current acceptance view.")
	}
	return notes
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
