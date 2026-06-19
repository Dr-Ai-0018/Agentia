package broker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OrchestratorInspectionDigest struct {
	RunID             string                                 `json:"run_id"`
	RetryOf           string                                 `json:"retry_of,omitempty"`
	Mode              string                                 `json:"mode,omitempty"`
	StartedAt         string                                 `json:"started_at,omitempty"`
	EndedAt           string                                 `json:"ended_at,omitempty"`
	Duration          string                                 `json:"duration,omitempty"`
	ResidentsPlanned  []string                               `json:"residents_planned,omitempty"`
	ResidentsFinished int                                    `json:"residents_finished"`
	ResidentsErrored  int                                    `json:"residents_errored"`
	TransientBlocked  int                                    `json:"transient_blocked"`
	UsefulRuns        int                                    `json:"useful_runs"`
	BudgetBlockedRuns int                                    `json:"budget_blocked_runs"`
	Residents         []OrchestratorResidentInspectionDigest `json:"residents,omitempty"`
}

type OrchestratorResidentInspectionDigest struct {
	Resident         string `json:"resident"`
	Status           string `json:"status"`
	Rounds           int    `json:"rounds,omitempty"`
	StoppedReason    string `json:"stopped_reason,omitempty"`
	BudgetBlocked    bool   `json:"budget_blocked,omitempty"`
	CompletedUseful  bool   `json:"completed_useful,omitempty"`
	TransientBlocked bool   `json:"transient_blocked,omitempty"`
	Error            string `json:"error,omitempty"`
}

func LoadLatestOrchestratorInspection(root string) (*OrchestratorInspectionDigest, error) {
	recent, err := LoadRecentOrchestratorInspections(root, 1)
	if err != nil || len(recent) == 0 {
		return nil, err
	}
	return &recent[0], nil
}

func LoadRecentOrchestratorInspections(root string, limit int) ([]OrchestratorInspectionDigest, error) {
	runRoot := filepath.Join(root, "orchestrator-runs")
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.TrimSpace(entry.Name()) != "" {
			runIDs = append(runIDs, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runIDs)))
	out := make([]OrchestratorInspectionDigest, 0, minPositive(limit, len(runIDs)))
	for _, runID := range runIDs {
		path := filepath.Join(runRoot, runID, "inspection-report.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item OrchestratorInspectionDigest
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.RunID == "" {
			item.RunID = runID
		}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func minPositive(a, b int) int {
	if a <= 0 || b < a {
		return b
	}
	return a
}
