package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryWrite(t *testing.T) {
	root := t.TempDir()
	history := New(root)
	if err := history.Write(HistoryEntry{
		ResidentID: "amber",
		Kind:       "resource_request",
		Summary:    "Amber requested more memory.",
	}); err != nil {
		t.Fatalf("write history entry: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "world", "public-history-*.jsonl"))
	if err != nil {
		t.Fatalf("glob history files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 public history file, got %d", len(files))
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected history file content")
	}
}
