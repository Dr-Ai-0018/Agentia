package broker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ai-arena/internal/audit"
	"ai-arena/internal/world"
)

type ResidentCheckpoint struct {
	ResidentID   string `json:"resident_id"`
	InstanceName string `json:"instance_name"`
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at,omitempty"`
	Stateful     bool   `json:"stateful,omitempty"`
	Kind         string `json:"kind"`
}

type CheckpointListOutput struct {
	ResidentID   string               `json:"resident_id"`
	InstanceName string               `json:"instance_name"`
	Checkpoints  []ResidentCheckpoint `json:"checkpoints"`
}

type CheckpointCleanupInput struct {
	ResidentID string `json:"resident_id"`
	Keep       int    `json:"keep"`
	Apply      bool   `json:"apply"`
	Operator   string `json:"operator"`
}

type CheckpointCleanupOutput struct {
	ResidentID           string               `json:"resident_id"`
	InstanceName         string               `json:"instance_name"`
	Keep                 int                  `json:"keep"`
	Apply                bool                 `json:"apply"`
	ManualReviewRequired bool                 `json:"manual_review_required"`
	Policy               string               `json:"policy"`
	ProtectedKinds       []string             `json:"protected_kinds"`
	Deletable            int                  `json:"deletable"`
	Deleted              []ResidentCheckpoint `json:"deleted"`
	Retained             []ResidentCheckpoint `json:"retained"`
}

type CheckpointCreateOutput struct {
	ResidentID   string `json:"resident_id"`
	InstanceName string `json:"instance_name"`
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at"`
	Operator     string `json:"operator"`
	Kind         string `json:"kind"`
}

func classifyCheckpoint(residentID, name string) string {
	switch {
	case name == BaselineSnapshotName():
		return "baseline"
	case strings.HasPrefix(name, "checkpoint-"+strings.ToLower(strings.TrimSpace(residentID))+"-"):
		return "host_checkpoint"
	case strings.HasPrefix(name, "self-"+strings.ToLower(strings.TrimSpace(residentID))+"-"):
		return "self_snapshot"
	default:
		return "other"
	}
}

func (s *HostActionService) ListResidentCheckpoints(residentID string) (CheckpointListOutput, error) {
	binding, ok := s.app.Binding(strings.TrimSpace(residentID))
	if !ok {
		return CheckpointListOutput{}, fmt.Errorf("unknown resident binding: %s", residentID)
	}
	snapshots, err := s.machine.ListSnapshots(binding.InstanceName)
	if err != nil {
		return CheckpointListOutput{}, err
	}
	checkpoints := make([]ResidentCheckpoint, 0, len(snapshots))
	for _, item := range snapshots {
		checkpoints = append(checkpoints, ResidentCheckpoint{
			ResidentID:   binding.ResidentID,
			InstanceName: binding.InstanceName,
			Name:         item.Name,
			CreatedAt:    item.CreatedAt,
			Stateful:     item.Stateful,
			Kind:         classifyCheckpoint(binding.ResidentID, item.Name),
		})
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Name < checkpoints[j].Name
	})
	return CheckpointListOutput{
		ResidentID:   binding.ResidentID,
		InstanceName: binding.InstanceName,
		Checkpoints:  checkpoints,
	}, nil
}

func (s *HostActionService) CreateHostCheckpoint(residentID, operator string, now time.Time) (CheckpointCreateOutput, error) {
	binding, ok := s.app.Binding(strings.TrimSpace(residentID))
	if !ok {
		return CheckpointCreateOutput{}, fmt.Errorf("unknown resident binding: %s", residentID)
	}
	name := HostCheckpointName(binding.ResidentID, now)
	if err := s.machine.Snapshot(binding.InstanceName, name); err != nil {
		return CheckpointCreateOutput{}, err
	}
	_ = s.audit.Write(audit.Event{
		Actor:      defaultMaintenanceOperator(operator),
		ResidentID: binding.ResidentID,
		Kind:       "checkpoint_create",
		TargetID:   binding.InstanceName,
		Summary:    fmt.Sprintf("Created host checkpoint %s for %s", name, binding.ResidentID),
		Metadata: map[string]any{
			"instance_name":   binding.InstanceName,
			"checkpoint_name": name,
			"kind":            "host_checkpoint",
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: binding.ResidentID,
		Kind:       "checkpoint_create",
		Summary:    fmt.Sprintf("Chenglin created host checkpoint %s", name),
		Details: map[string]any{
			"instance_name":   binding.InstanceName,
			"checkpoint_name": name,
			"kind":            "host_checkpoint",
		},
	})
	return CheckpointCreateOutput{
		ResidentID:   binding.ResidentID,
		InstanceName: binding.InstanceName,
		Name:         name,
		CreatedAt:    now.UTC().Format(time.RFC3339),
		Operator:     defaultMaintenanceOperator(operator),
		Kind:         "host_checkpoint",
	}, nil
}

func (s *HostActionService) CleanupResidentCheckpoints(input CheckpointCleanupInput) (CheckpointCleanupOutput, error) {
	if input.Keep < 0 {
		return CheckpointCleanupOutput{}, fmt.Errorf("keep must be >= 0")
	}
	list, err := s.ListResidentCheckpoints(input.ResidentID)
	if err != nil {
		return CheckpointCleanupOutput{}, err
	}
	hostOnly := make([]ResidentCheckpoint, 0, len(list.Checkpoints))
	retained := make([]ResidentCheckpoint, 0, len(list.Checkpoints))
	for _, item := range list.Checkpoints {
		if item.Kind == "host_checkpoint" {
			hostOnly = append(hostOnly, item)
			continue
		}
		retained = append(retained, item)
	}
	sort.Slice(hostOnly, func(i, j int) bool {
		return hostCheckpointSortKey(hostOnly[i]).After(hostCheckpointSortKey(hostOnly[j]))
	})

	deleted := []ResidentCheckpoint{}
	for i, item := range hostOnly {
		if i < input.Keep {
			retained = append(retained, item)
			continue
		}
		if input.Apply {
			if err := s.machine.DeleteSnapshot(list.InstanceName, item.Name); err != nil {
				return CheckpointCleanupOutput{}, err
			}
		}
		deleted = append(deleted, item)
	}
	sort.Slice(retained, func(i, j int) bool {
		return retained[i].Name < retained[j].Name
	})
	if len(deleted) > 0 {
		s.recordCheckpointCleanup(list.ResidentID, list.InstanceName, deleted, input)
	}
	return CheckpointCleanupOutput{
		ResidentID:           list.ResidentID,
		InstanceName:         list.InstanceName,
		Keep:                 input.Keep,
		Apply:                input.Apply,
		ManualReviewRequired: !input.Apply && len(deleted) > 0,
		Policy:               fmt.Sprintf("retain newest %d host checkpoint(s); never delete baseline, resident self snapshots, or unknown snapshots", input.Keep),
		ProtectedKinds:       []string{"baseline", "self_snapshot", "other"},
		Deletable:            len(deleted),
		Deleted:              deleted,
		Retained:             retained,
	}, nil
}

func hostCheckpointSortKey(item ResidentCheckpoint) time.Time {
	if ts, ok := strings.CutPrefix(item.Name, "checkpoint-"+strings.ToLower(strings.TrimSpace(item.ResidentID))+"-"); ok {
		if parsed, err := time.Parse("20060102T150405Z", ts); err == nil {
			return parsed
		}
	}
	if parsed, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
		return parsed
	}
	return time.Time{}
}

func (s *HostActionService) recordCheckpointCleanup(residentID, instanceName string, deleted []ResidentCheckpoint, input CheckpointCleanupInput) {
	names := make([]string, 0, len(deleted))
	for _, item := range deleted {
		names = append(names, item.Name)
	}
	action := "checkpoint_cleanup_dry_run"
	summary := fmt.Sprintf("Checkpoint cleanup dry-run for %s", residentID)
	if input.Apply {
		action = "checkpoint_cleanup"
		summary = fmt.Sprintf("Checkpoint cleanup applied for %s", residentID)
	}
	_ = s.audit.Write(audit.Event{
		Actor:      defaultMaintenanceOperator(input.Operator),
		ResidentID: residentID,
		Kind:       action,
		TargetID:   instanceName,
		Summary:    summary,
		Metadata: map[string]any{
			"instance_name":     instanceName,
			"deleted_snapshots": names,
			"keep":              input.Keep,
			"apply":             input.Apply,
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: residentID,
		Kind:       action,
		Summary:    summary,
		Details: map[string]any{
			"instance_name":     instanceName,
			"deleted_snapshots": names,
			"keep":              input.Keep,
			"apply":             input.Apply,
		},
	})
}
