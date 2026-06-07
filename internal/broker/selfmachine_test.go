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
