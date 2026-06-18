package broker

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadRecentMaintenanceRunRecords(root string, limit int) ([]MaintenanceRunRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(root, "operations", "maintenance-runs-*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	records := []MaintenanceRunRecord{}
	for _, file := range files {
		items, err := readMaintenanceRunRecordFile(file)
		if err != nil {
			return nil, err
		}
		records = append(records, items...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt != records[j].CreatedAt {
			return records[i].CreatedAt > records[j].CreatedAt
		}
		return records[i].ID > records[j].ID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func readMaintenanceRunRecordFile(path string) ([]MaintenanceRunRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	out := []MaintenanceRunRecord{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record MaintenanceRunRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
