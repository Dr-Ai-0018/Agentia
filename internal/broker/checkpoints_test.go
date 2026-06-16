package broker

import (
	"testing"
	"time"
)

func TestCheckpointNamingHelpers(t *testing.T) {
	if got := BaselineSnapshotName(); got != "clean-base" {
		t.Fatalf("unexpected baseline snapshot name: %s", got)
	}
	now := time.Date(2026, 6, 12, 3, 30, 0, 0, time.UTC)
	if got := HostCheckpointName("amber", now); got != "checkpoint-amber-20260612T033000Z" {
		t.Fatalf("unexpected host checkpoint name: %s", got)
	}
	if got := ResidentSelfSnapshotName("amber", "Before Upgrade"); got != "self-amber-before-upgrade" {
		t.Fatalf("unexpected resident self snapshot name: %s", got)
	}
	if got := ResidentSelfSnapshotName("Amber", " Before: Upgrade!!! "); got != "self-amber-before-upgrade" {
		t.Fatalf("unexpected sanitized resident self snapshot name: %s", got)
	}
	if got := NormalizeResidentSelfSnapshotName("amber", "before upgrade"); got != "self-amber-before-upgrade" {
		t.Fatalf("unexpected normalized self snapshot name: %s", got)
	}
	if got := NormalizeResidentSelfSnapshotName("amber", "checkpoint-amber-20260612T033000Z"); got != "checkpoint-amber-20260612T033000Z" {
		t.Fatalf("host checkpoint name should pass through, got %s", got)
	}
}
