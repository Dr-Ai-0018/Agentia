package worldstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadRecentForResident(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if _, err := store.AppendResidentToChenglin("amber", "hello", now); err != nil {
		t.Fatalf("append amber message: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("jade", "ignore me", now.Add(time.Second)); err != nil {
		t.Fatalf("append jade message: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("amber", "second", now.Add(2*time.Second)); err != nil {
		t.Fatalf("append amber second message: %v", err)
	}

	got, err := store.ReadRecentForResident("amber", 8)
	if err != nil {
		t.Fatalf("read recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 amber messages, got %d", len(got))
	}
	if got[0].Body != "second" {
		t.Fatalf("expected newest first, got %q", got[0].Body)
	}

	dayFile := filepath.Join(root, "world", "messages", "2026-06-06.jsonl")
	if err := osStat(dayFile); err != nil {
		t.Fatalf("expected message log file: %v", err)
	}
}

func osStat(path string) error {
	_, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	return err
}
