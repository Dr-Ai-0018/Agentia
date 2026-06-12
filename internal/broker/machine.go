package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrMemoryIncreaseRequiresStop = errors.New("memory increase requires stop")

type MachineControl interface {
	Reboot(instance string) error
	Snapshot(instance, name string) error
	Restore(instance, snapshot string) error
	SetMemory(instance string, memoryMiB int64) error
	ListSnapshots(instance string) ([]MachineSnapshot, error)
	DeleteSnapshot(instance, name string) error
}

type IncusMachineControl struct{}

type MachineSnapshot struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	Stateful  bool   `json:"stateful,omitempty"`
}

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

func (c *IncusMachineControl) SetMemory(instance string, memoryMiB int64) error {
	return runIncus("config", "set", instance, "limits.memory", fmt.Sprintf("%dMiB", memoryMiB))
}

func (c *IncusMachineControl) ListSnapshots(instance string) ([]MachineSnapshot, error) {
	cmd := exec.Command("incus", "snapshot", "list", instance, "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("incus snapshot list %s failed: %w", instance, err)
	}
	var raw []struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
		Stateful  bool   `json:"stateful"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("decode incus snapshot list %s: %w", instance, err)
	}
	snapshots := make([]MachineSnapshot, 0, len(raw))
	for _, item := range raw {
		snapshots = append(snapshots, MachineSnapshot{
			Name:      item.Name,
			CreatedAt: item.CreatedAt,
			Stateful:  item.Stateful,
		})
	}
	return snapshots, nil
}

func (c *IncusMachineControl) DeleteSnapshot(instance, name string) error {
	return runIncus("snapshot", "delete", instance, name)
}

func runIncus(args ...string) error {
	cmd := exec.Command("incus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			text = err.Error()
		}
		if strings.Contains(text, "Cannot increase memory size beyond boot time size when VM is running") {
			return fmt.Errorf("%w: %s", ErrMemoryIncreaseRequiresStop, text)
		}
		return fmt.Errorf("incus %s failed: %s", strings.Join(args, " "), text)
	}
	return nil
}
