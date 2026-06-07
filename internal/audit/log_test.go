package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerWrite(t *testing.T) {
	root := t.TempDir()
	logger := New(root)
	if err := logger.Write(Event{
		Actor:      "chenglin",
		ResidentID: "amber",
		Kind:       "ticket_reply",
		Summary:    "Closed a test ticket.",
	}); err != nil {
		t.Fatalf("write audit event: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "audit", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob audit files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 audit file, got %d", len(files))
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected audit file content")
	}
}
