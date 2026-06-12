package broker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListResidentCheckpoints(t *testing.T) {
	service := NewHostActionService(t.TempDir())
	fake := &fakeMachineControl{}
	service.machine = fake

	out, err := service.ListResidentCheckpoints("amber")
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(out.Checkpoints) != 5 {
		t.Fatalf("expected 5 checkpoints, got %d", len(out.Checkpoints))
	}
	kinds := map[string]string{}
	for _, item := range out.Checkpoints {
		kinds[item.Name] = item.Kind
	}
	if kinds["clean-base"] != "baseline" {
		t.Fatalf("expected baseline classification, got %#v", kinds)
	}
	if kinds["checkpoint-amber-20260612T030000Z"] != "host_checkpoint" {
		t.Fatalf("expected host checkpoint classification, got %#v", kinds)
	}
	if kinds["self-amber-before-upgrade"] != "self_snapshot" {
		t.Fatalf("expected self snapshot classification, got %#v", kinds)
	}
}

func TestCleanupResidentCheckpointsDryRunKeepsNewestHostCheckpoints(t *testing.T) {
	root := t.TempDir()
	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake

	out, err := service.CleanupResidentCheckpoints(CheckpointCleanupInput{
		ResidentID: "amber",
		Keep:       1,
		Apply:      false,
		Operator:   "chenglin",
	})
	if err != nil {
		t.Fatalf("cleanup checkpoints: %v", err)
	}
	if len(out.Deleted) != 2 {
		t.Fatalf("expected 2 deletions, got %d", len(out.Deleted))
	}
	if out.Deleted[0].Name != "checkpoint-amber-20260612T020000Z" || out.Deleted[1].Name != "checkpoint-amber-20260612T010000Z" {
		t.Fatalf("unexpected deleted checkpoints: %#v", out.Deleted)
	}
	files, err := filepath.Glob(filepath.Join(root, "audit", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob audit files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected audit entry, got %d files", len(files))
	}
}

func TestCleanupResidentCheckpointsApplyDeletesOldHostCheckpoints(t *testing.T) {
	root := t.TempDir()
	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake

	out, err := service.CleanupResidentCheckpoints(CheckpointCleanupInput{
		ResidentID: "amber",
		Keep:       2,
		Apply:      true,
		Operator:   "chenglin",
	})
	if err != nil {
		t.Fatalf("cleanup checkpoints apply: %v", err)
	}
	if len(out.Deleted) != 1 || out.Deleted[0].Name != "checkpoint-amber-20260612T010000Z" {
		t.Fatalf("unexpected deleted checkpoints: %#v", out.Deleted)
	}
	if fake.deletedInstance != "amber" || fake.deletedName != "checkpoint-amber-20260612T010000Z" {
		t.Fatalf("expected delete call, got fake=%#v", fake)
	}
	if _, err := os.Stat(filepath.Join(root, "world")); err != nil {
		t.Fatalf("expected history/audit output directories to exist: %v", err)
	}
}

func TestCreateHostCheckpoint(t *testing.T) {
	root := t.TempDir()
	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake
	now := time.Date(2026, 6, 12, 5, 0, 0, 0, time.UTC)

	out, err := service.CreateHostCheckpoint("amber", "chenglin", now)
	if err != nil {
		t.Fatalf("create host checkpoint: %v", err)
	}
	if out.Name != "checkpoint-amber-20260612T050000Z" {
		t.Fatalf("unexpected checkpoint output: %#v", out)
	}
	if fake.snapshotInstance != "amber" || fake.snapshotName != "checkpoint-amber-20260612T050000Z" {
		t.Fatalf("expected snapshot create call, got fake=%#v", fake)
	}
}
