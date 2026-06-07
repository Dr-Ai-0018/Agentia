package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Event struct {
	ID         string         `json:"id"`
	CreatedAt  string         `json:"created_at"`
	Actor      string         `json:"actor"`
	ResidentID string         `json:"resident_id,omitempty"`
	Kind       string         `json:"kind"`
	TargetID   string         `json:"target_id,omitempty"`
	Summary    string         `json:"summary"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Logger struct {
	root string
}

func New(root string) *Logger {
	return &Logger{root: root}
}

func (l *Logger) Write(event Event) error {
	now := strings.TrimSpace(event.CreatedAt)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
		event.CreatedAt = now
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "audit-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	if err := os.MkdirAll(filepath.Join(l.root, "audit"), 0o755); err != nil {
		return err
	}
	file := filepath.Join(l.root, "audit", "events-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}
