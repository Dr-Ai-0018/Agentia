package broker

import (
	"testing"

	"ai-arena/internal/auth"
)

type fakeMachineControl struct {
	rebootedInstance string
	snapshotInstance string
	snapshotName     string
	restoreInstance  string
	restoreName      string
	deletedInstance  string
	deletedName      string
	memoryInstance   string
	memoryMiB        int64
	memoryErr        error
}

func (f *fakeMachineControl) Reboot(instance string) error {
	f.rebootedInstance = instance
	return nil
}

func (f *fakeMachineControl) Snapshot(instance, name string) error {
	f.snapshotInstance = instance
	f.snapshotName = name
	return nil
}

func (f *fakeMachineControl) Restore(instance, snapshot string) error {
	f.restoreInstance = instance
	f.restoreName = snapshot
	return nil
}

func (f *fakeMachineControl) SetMemory(instance string, memoryMiB int64) error {
	f.memoryInstance = instance
	f.memoryMiB = memoryMiB
	return f.memoryErr
}

func (f *fakeMachineControl) ListSnapshots(instance string) ([]MachineSnapshot, error) {
	return []MachineSnapshot{
		{Name: "clean-base", CreatedAt: "2026-06-01T00:00:00Z"},
		{Name: "self-amber-before-upgrade", CreatedAt: "2026-06-10T00:00:00Z"},
		{Name: "checkpoint-amber-20260612T010000Z", CreatedAt: "2026-06-12T01:00:00Z"},
		{Name: "checkpoint-amber-20260612T020000Z", CreatedAt: "2026-06-12T02:00:00Z"},
		{Name: "checkpoint-amber-20260612T030000Z", CreatedAt: "2026-06-12T03:00:00Z"},
	}, nil
}

func (f *fakeMachineControl) DeleteSnapshot(instance, name string) error {
	f.deletedInstance = instance
	f.deletedName = name
	return nil
}

func TestSelfServiceRequestReboot(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)
	fake := &fakeMachineControl{}
	service.machine = fake

	result, err := service.RequestReboot(auth.ResidentClaim{ResidentID: "amber"})
	if err != nil {
		t.Fatalf("request reboot: %v", err)
	}
	if result.Action != "reboot" || fake.rebootedInstance != "amber" {
		t.Fatalf("unexpected reboot result: %#v fake=%#v", result, fake)
	}
}

func TestSelfServiceRequestSnapshotAndRestore(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)
	fake := &fakeMachineControl{}
	service.machine = fake

	snapshot, err := service.RequestSnapshot(auth.ResidentClaim{ResidentID: "jade"}, "before-upgrade")
	if err != nil {
		t.Fatalf("request snapshot: %v", err)
	}
	if snapshot.Action != "snapshot" || fake.snapshotInstance != "jade" || fake.snapshotName != "before-upgrade" {
		t.Fatalf("unexpected snapshot result: %#v fake=%#v", snapshot, fake)
	}

	restore, err := service.RequestRestore(auth.ResidentClaim{ResidentID: "jade"}, "before-upgrade")
	if err != nil {
		t.Fatalf("request restore: %v", err)
	}
	if restore.Action != "restore" || fake.restoreInstance != "jade" || fake.restoreName != "before-upgrade" {
		t.Fatalf("unexpected restore result: %#v fake=%#v", restore, fake)
	}
}
