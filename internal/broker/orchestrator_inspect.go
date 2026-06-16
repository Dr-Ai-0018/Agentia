package broker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LatestOrchestratorInspection struct {
	RunID             string                                 `json:"run_id"`
	RetryOf           string                                 `json:"retry_of,omitempty"`
	Mode              string                                 `json:"mode,omitempty"`
	StartedAt         string                                 `json:"started_at,omitempty"`
	EndedAt           string                                 `json:"ended_at,omitempty"`
	Duration          string                                 `json:"duration,omitempty"`
	ResidentsPlanned  []string                               `json:"residents_planned,omitempty"`
	ResidentsFinished int                                    `json:"residents_finished"`
	ResidentsErrored  int                                    `json:"residents_errored"`
	UsefulRuns        int                                    `json:"useful_runs"`
	BudgetBlockedRuns int                                    `json:"budget_blocked_runs"`
	Residents         []LatestOrchestratorResidentInspection `json:"residents,omitempty"`
}

type LatestOrchestratorResidentInspection struct {
	Resident        string `json:"resident"`
	Status          string `json:"status"`
	Rounds          int    `json:"rounds,omitempty"`
	StoppedReason   string `json:"stopped_reason,omitempty"`
	BudgetBlocked   bool   `json:"budget_blocked,omitempty"`
	CompletedUseful bool   `json:"completed_useful,omitempty"`
	Error           string `json:"error,omitempty"`
}

func LoadLatestOrchestratorInspection(root string) (*LatestOrchestratorInspection, error) {
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
	for _, runID := range runIDs {
		path := filepath.Join(runRoot, runID, "inspection-report.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var out LatestOrchestratorInspection
		if err := json.Unmarshal(raw, &out); err != nil {
			continue
		}
		if out.RunID == "" {
			out.RunID = runID
		}
		return &out, nil
	}
	return nil, nil
}
