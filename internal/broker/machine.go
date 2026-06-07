package broker

import (
	"fmt"
	"os/exec"
	"strings"
)

type MachineControl interface {
	Reboot(instance string) error
	Snapshot(instance, name string) error
	Restore(instance, snapshot string) error
}

type IncusMachineControl struct{}

func NewIncusMachineControl() *IncusMachineControl {
	return &IncusMachineControl{}
}

func (c *IncusMachineControl) Reboot(instance string) error {
	return runIncus("restart", instance)
}

func (c *IncusMachineControl) Snapshot(instance, name string) error {
	return runIncus("snapshot", "create", instance, name)
}

func (c *IncusMachineControl) Restore(instance, snapshot string) error {
	return runIncus("snapshot", "restore", instance, snapshot)
}

func runIncus(args ...string) error {
	cmd := exec.Command("incus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("incus %s failed: %s", strings.Join(args, " "), text)
	}
	return nil
}
