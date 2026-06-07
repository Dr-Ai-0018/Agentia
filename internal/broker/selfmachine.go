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
	if err := s.guard.Allow(ActionReboot); err != nil {
		return MachineActionResult{}, err
	}
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
	_ = s.audit.Write(auditEvent("resident", claim.ResidentID, "self_reboot", binding.InstanceName, fmt.Sprintf("%s rebooted %s", claim.ResidentID, binding.InstanceName), nil))
	_ = s.history.Write(historyEntry(claim.ResidentID, "self_reboot", fmt.Sprintf("%s rebooted own VM", claim.ResidentID), map[string]any{
		"instance_name": binding.InstanceName,
	}))
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "reboot",
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *SelfService) RequestSnapshot(claim auth.ResidentClaim, snapshotName string) (MachineActionResult, error) {
	if err := s.guard.Allow(ActionSnapshot); err != nil {
		return MachineActionResult{}, err
	}
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
	_ = s.audit.Write(auditEvent("resident", claim.ResidentID, "self_snapshot", binding.InstanceName, fmt.Sprintf("%s created snapshot %s", claim.ResidentID, name), map[string]any{
		"snapshot_name": name,
	}))
	_ = s.history.Write(historyEntry(claim.ResidentID, "self_snapshot", fmt.Sprintf("%s created a self snapshot", claim.ResidentID), map[string]any{
		"instance_name": binding.InstanceName,
		"snapshot_name": name,
	}))
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "snapshot",
		SnapshotName: name,
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *SelfService) RequestRestore(claim auth.ResidentClaim, snapshotName string) (MachineActionResult, error) {
	if err := s.guard.Allow(ActionRestore); err != nil {
		return MachineActionResult{}, err
	}
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
	_ = s.audit.Write(auditEvent("resident", claim.ResidentID, "self_restore", binding.InstanceName, fmt.Sprintf("%s restored snapshot %s", claim.ResidentID, name), map[string]any{
		"snapshot_name": name,
	}))
	_ = s.history.Write(historyEntry(claim.ResidentID, "self_restore", fmt.Sprintf("%s restored own VM snapshot", claim.ResidentID), map[string]any{
		"instance_name": binding.InstanceName,
		"snapshot_name": name,
	}))
	return MachineActionResult{
		ResidentID:   claim.ResidentID,
		InstanceName: binding.InstanceName,
		Action:       "restore",
		SnapshotName: name,
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}
