package world

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HistoryEntry struct {
	ID         string         `json:"id"`
	CreatedAt  string         `json:"created_at"`
	ResidentID string         `json:"resident_id,omitempty"`
	Kind       string         `json:"kind"`
	Summary    string         `json:"summary"`
	Details    map[string]any `json:"details,omitempty"`
}

type History struct {
	root string
}

func New(root string) *History {
	return &History{root: root}
}

func (h *History) Write(entry HistoryEntry) error {
	now := strings.TrimSpace(entry.CreatedAt)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
		entry.CreatedAt = now
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = "history-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	if err := os.MkdirAll(filepath.Join(h.root, "world"), 0o755); err != nil {
		return err
	}
	file := filepath.Join(h.root, "world", "public-history-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}
