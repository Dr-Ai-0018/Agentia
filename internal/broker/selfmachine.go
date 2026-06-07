package broker

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/auth"
)

type MachineActionResult struct {
	ResidentID   string `json:"resident_id"`
	InstanceName string `json:"instance_name"`
	Action       string `json:"action"`
	SnapshotName string `json:"snapshot_name,omitempty"`
	CompletedAt  string `json:"completed_at"`
}

func (s *SelfService) RequestReboot(claim auth.ResidentClaim) (MachineActionResult, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return MachineActionResult{}, err
	}
	binding, err := s.Binding(claim)
	if err != nil {
		return MachineActionResult{}, err
	}
	if err := s.machine.Reboot(binding.InstanceName); err != nil {
		return MachineActionResult{}, err
	}
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "reboot",
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *SelfService) RequestSnapshot(claim auth.ResidentClaim, snapshotName string) (MachineActionResult, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return MachineActionResult{}, err
	}
	binding, err := s.Binding(claim)
	if err != nil {
		return MachineActionResult{}, err
	}
	name := strings.TrimSpace(snapshotName)
	if name == "" {
		return MachineActionResult{}, fmt.Errorf("snapshot name is required")
	}
	if err := s.machine.Snapshot(binding.InstanceName, name); err != nil {
		return MachineActionResult{}, err
	}
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "snapshot",
		SnapshotName: name,
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *SelfService) RequestRestore(claim auth.ResidentClaim, snapshotName string) (MachineActionResult, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return MachineActionResult{}, err
	}
	binding, err := s.Binding(claim)
	if err != nil {
		return MachineActionResult{}, err
	}
	name := strings.TrimSpace(snapshotName)
	if name == "" {
		return MachineActionResult{}, fmt.Errorf("snapshot name is required")
	}
	if err := s.machine.Restore(binding.InstanceName, name); err != nil {
		return MachineActionResult{}, err
	}
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "restore",
		SnapshotName: name,
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}
