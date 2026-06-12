package broker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ResidentInventoryFact struct {
	ResidentID    string `json:"resident_id"`
	InstanceName  string `json:"instance_name"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	VCPU          int    `json:"vcpu"`
	MemoryLimitMiB int64 `json:"memory_limit_mib"`
	DiskGiB       int64  `json:"disk_gib"`
	IPv4          string `json:"ipv4,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

type InventorySnapshot struct {
	CollectedAt string                  `json:"collected_at"`
	Residents   []ResidentInventoryFact `json:"residents"`
}

type ResidentRuntimeFact struct {
	ResidentID         string   `json:"resident_id"`
	InstanceName       string   `json:"instance_name"`
	Status             string   `json:"status"`
	Type               string   `json:"type"`
	ConfiguredVCPU     int      `json:"configured_vcpu"`
	ObservedVCPU       int      `json:"observed_vcpu"`
	ConfiguredMemoryMiB int64   `json:"configured_memory_mib"`
	ObservedMemoryMiB  int64    `json:"observed_memory_mib"`
	ConfiguredDiskGiB  int64    `json:"configured_disk_gib"`
	ObservedDiskGiB    int64    `json:"observed_disk_gib"`
	IPv4               string   `json:"ipv4,omitempty"`
	UpdatedAt          string   `json:"updated_at"`
	DriftFields        []string `json:"drift_fields,omitempty"`
}

func CollectInventorySnapshot(cfg Config, now time.Time) (InventorySnapshot, error) {
	type incusInstance struct {
		Name           string            `json:"name"`
		Status         string            `json:"status"`
		Type           string            `json:"type"`
		Config         map[string]string `json:"config"`
		ExpandedConfig map[string]string `json:"expanded_config"`
		State          struct {
			Network map[string]struct {
				Addresses []struct {
					Family  string `json:"family"`
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"network"`
		} `json:"state"`
	}

	cmd := exec.Command("incus", "list", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return InventorySnapshot{}, fmt.Errorf("collect incus inventory: %w", err)
	}
	var instances []incusInstance
	if err := json.Unmarshal(out, &instances); err != nil {
		return InventorySnapshot{}, err
	}

	byName := map[string]incusInstance{}
	for _, item := range instances {
		byName[item.Name] = item
	}

	facts := make([]ResidentInventoryFact, 0, len(cfg.Residents))
	for _, resident := range cfg.Residents {
		item, ok := byName[resident.InstanceName]
		fact := ResidentInventoryFact{
			ResidentID:   resident.ResidentID,
			InstanceName: resident.InstanceName,
			VCPU:         resident.VCPU,
			MemoryLimitMiB: resident.MemoryMiB,
			DiskGiB:      resident.DiskGiB,
			UpdatedAt:    now.Format(time.RFC3339),
		}
		if ok {
			fact.Status = item.Status
			fact.Type = item.Type
			if v := firstNonEmpty(item.Config["limits.cpu"], item.ExpandedConfig["limits.cpu"]); v != "" {
				if parsed, parseErr := strconv.Atoi(strings.TrimSpace(v)); parseErr == nil {
					fact.VCPU = parsed
				}
			}
			if v := firstNonEmpty(item.Config["limits.memory"], item.ExpandedConfig["limits.memory"]); v != "" {
				fact.MemoryLimitMiB = parseMemoryLimitMiB(v, fact.MemoryLimitMiB)
			}
			fact.IPv4 = extractIPv4(item)
		}
		facts = append(facts, fact)
	}

	return InventorySnapshot{
		CollectedAt: now.Format(time.RFC3339),
		Residents:   facts,
	}, nil
}

func SaveInventorySnapshot(root string, snapshot InventorySnapshot) (string, error) {
	dir := filepath.Join(root, "inventory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "incus-inventory.json")
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func BuildResidentRuntimeFacts(cfg Config, snapshot InventorySnapshot) []ResidentRuntimeFact {
	byResident := make(map[string]ResidentInventoryFact, len(snapshot.Residents))
	for _, item := range snapshot.Residents {
		byResident[item.ResidentID] = item
	}

	facts := make([]ResidentRuntimeFact, 0, len(cfg.Residents))
	for _, binding := range cfg.Residents {
		observed := ResidentInventoryFact{
			ResidentID:     binding.ResidentID,
			InstanceName:   binding.InstanceName,
			VCPU:           binding.VCPU,
			MemoryLimitMiB: binding.MemoryMiB,
			DiskGiB:        binding.DiskGiB,
		}
		if item, ok := byResident[binding.ResidentID]; ok {
			observed = item
		}
		fact := ResidentRuntimeFact{
			ResidentID:          binding.ResidentID,
			InstanceName:        binding.InstanceName,
			Status:              observed.Status,
			Type:                observed.Type,
			ConfiguredVCPU:      binding.VCPU,
			ObservedVCPU:        observed.VCPU,
			ConfiguredMemoryMiB: binding.MemoryMiB,
			ObservedMemoryMiB:   observed.MemoryLimitMiB,
			ConfiguredDiskGiB:   binding.DiskGiB,
			ObservedDiskGiB:     observed.DiskGiB,
			IPv4:                observed.IPv4,
			UpdatedAt:           observed.UpdatedAt,
		}
		if fact.ObservedVCPU != fact.ConfiguredVCPU {
			fact.DriftFields = append(fact.DriftFields, "vcpu")
		}
		if fact.ObservedMemoryMiB != fact.ConfiguredMemoryMiB {
			fact.DriftFields = append(fact.DriftFields, "memory")
		}
		if fact.ObservedDiskGiB != fact.ConfiguredDiskGiB {
			fact.DriftFields = append(fact.DriftFields, "disk")
		}
		facts = append(facts, fact)
	}
	return facts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func parseMemoryLimitMiB(value string, fallback int64) int64 {
	value = strings.TrimSpace(strings.ToLower(value))
	switch {
	case strings.HasSuffix(value, "gib"):
		n, err := strconv.ParseInt(strings.TrimSuffix(value, "gib"), 10, 64)
		if err == nil {
			return n * 1024
		}
	case strings.HasSuffix(value, "mib"):
		n, err := strconv.ParseInt(strings.TrimSuffix(value, "mib"), 10, 64)
		if err == nil {
			return n
		}
	}
	return fallback
}

func extractIPv4(item struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	Type           string            `json:"type"`
	Config         map[string]string `json:"config"`
	ExpandedConfig map[string]string `json:"expanded_config"`
	State          struct {
		Network map[string]struct {
			Addresses []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"network"`
	} `json:"state"`
}) string {
	for _, network := range item.State.Network {
		for _, addr := range network.Addresses {
			if addr.Family == "inet" && addr.Address != "" && addr.Address != "127.0.0.1" {
				return addr.Address
			}
		}
	}
	return ""
}
