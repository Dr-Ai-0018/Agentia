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
	ResidentID     string `json:"resident_id"`
	InstanceName   string `json:"instance_name"`
	Status         string `json:"status"`
	Type           string `json:"type"`
	VCPU           int    `json:"vcpu"`
	MemoryLimitMiB int64  `json:"memory_limit_mib"`
	DiskGiB        int64  `json:"disk_gib"`
	IPv4           string `json:"ipv4,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

type InventorySnapshot struct {
	CollectedAt string                  `json:"collected_at"`
	Residents   []ResidentInventoryFact `json:"residents"`
}

type ResidentRuntimeFact struct {
	ResidentID               string   `json:"resident_id"`
	InstanceName             string   `json:"instance_name"`
	Status                   string   `json:"status"`
	Type                     string   `json:"type"`
	ConfiguredVCPU           int      `json:"configured_vcpu"`
	ObservedVCPU             int      `json:"observed_vcpu"`
	ConfiguredMemoryMiB      int64    `json:"configured_memory_mib"`
	ObservedMemoryMiB        int64    `json:"observed_memory_mib"`
	ConfiguredDiskGiB        int64    `json:"configured_disk_gib"`
	ObservedDiskGiB          int64    `json:"observed_disk_gib"`
	IPv4                     string   `json:"ipv4,omitempty"`
	UpdatedAt                string   `json:"updated_at"`
	HostQEMUPID              int      `json:"host_qemu_pid,omitempty"`
	HostQEMURSSMiB           int64    `json:"host_qemu_rss_mib,omitempty"`
	IncusMemoryCurrentMiB    int64    `json:"incus_memory_current_mib,omitempty"`
	GuestMemAvailableMiB     int64    `json:"guest_mem_available_mib,omitempty"`
	GuestMemFreeMiB          int64    `json:"guest_mem_free_mib,omitempty"`
	GuestBuffCacheMiB        int64    `json:"guest_buff_cache_mib,omitempty"`
	GuestTopMemoryProcess    string   `json:"guest_top_memory_process,omitempty"`
	HostRSSHighGuestUsageLow bool     `json:"host_rss_high_guest_usage_low,omitempty"`
	LiveMetricsError         string   `json:"live_metrics_error,omitempty"`
	DriftFields              []string `json:"drift_fields,omitempty"`
}

type ResidentLiveRuntimeFact struct {
	ResidentID            string
	InstanceName          string
	HostQEMUPID           int
	HostQEMURSSMiB        int64
	IncusMemoryCurrentMiB int64
	GuestMemAvailableMiB  int64
	GuestMemFreeMiB       int64
	GuestBuffCacheMiB     int64
	GuestTopMemoryProcess string
	Error                 string
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
			ResidentID:     resident.ResidentID,
			InstanceName:   resident.InstanceName,
			VCPU:           resident.VCPU,
			MemoryLimitMiB: resident.MemoryMiB,
			DiskGiB:        resident.DiskGiB,
			UpdatedAt:      now.Format(time.RFC3339),
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

func BuildResidentRuntimeFacts(cfg Config, snapshot InventorySnapshot, live map[string]ResidentLiveRuntimeFact) []ResidentRuntimeFact {
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
		if liveFact, ok := live[binding.ResidentID]; ok {
			fact.HostQEMUPID = liveFact.HostQEMUPID
			fact.HostQEMURSSMiB = liveFact.HostQEMURSSMiB
			fact.IncusMemoryCurrentMiB = liveFact.IncusMemoryCurrentMiB
			fact.GuestMemAvailableMiB = liveFact.GuestMemAvailableMiB
			fact.GuestMemFreeMiB = liveFact.GuestMemFreeMiB
			fact.GuestBuffCacheMiB = liveFact.GuestBuffCacheMiB
			fact.GuestTopMemoryProcess = liveFact.GuestTopMemoryProcess
			fact.LiveMetricsError = liveFact.Error
			fact.HostRSSHighGuestUsageLow = classifyHostRSSHighGuestUsageLow(fact)
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

func CollectResidentLiveRuntimeFacts(cfg Config) map[string]ResidentLiveRuntimeFact {
	out := make(map[string]ResidentLiveRuntimeFact, len(cfg.Residents))
	for _, binding := range cfg.Residents {
		fact := ResidentLiveRuntimeFact{
			ResidentID:   binding.ResidentID,
			InstanceName: binding.InstanceName,
		}
		if strings.TrimSpace(binding.InstanceName) == "" {
			fact.Error = "missing instance name"
			out[binding.ResidentID] = fact
			continue
		}
		state, err := queryIncusInstanceState(binding.InstanceName)
		if err != nil {
			fact.Error = appendMetricError(fact.Error, err.Error())
		} else {
			fact.HostQEMUPID = state.PID
			fact.IncusMemoryCurrentMiB = bytesToMiB(state.Memory.Usage)
		}
		if fact.HostQEMUPID > 0 {
			rss, err := readProcessRSSMiB(fact.HostQEMUPID)
			if err != nil {
				fact.Error = appendMetricError(fact.Error, err.Error())
			} else {
				fact.HostQEMURSSMiB = rss
			}
		}
		mem, err := readGuestMeminfo(binding.InstanceName)
		if err != nil {
			fact.Error = appendMetricError(fact.Error, err.Error())
		} else {
			fact.GuestMemAvailableMiB = mem.MemAvailableMiB
			fact.GuestMemFreeMiB = mem.MemFreeMiB
			fact.GuestBuffCacheMiB = mem.BuffCacheMiB
		}
		top, err := readGuestTopMemoryProcess(binding.InstanceName)
		if err != nil {
			fact.Error = appendMetricError(fact.Error, err.Error())
		} else {
			fact.GuestTopMemoryProcess = top
		}
		out[binding.ResidentID] = fact
	}
	return out
}

type incusInstanceState struct {
	PID    int `json:"pid"`
	Memory struct {
		Usage int64 `json:"usage"`
	} `json:"memory"`
}

func queryIncusInstanceState(instance string) (incusInstanceState, error) {
	cmd := exec.Command("incus", "query", "/1.0/instances/"+instance+"/state")
	out, err := cmd.Output()
	if err != nil {
		return incusInstanceState{}, fmt.Errorf("query incus state for %s: %w", instance, err)
	}
	var state incusInstanceState
	if err := json.Unmarshal(out, &state); err != nil {
		return incusInstanceState{}, fmt.Errorf("decode incus state for %s: %w", instance, err)
	}
	return state, nil
}

func readProcessRSSMiB(pid int) (int64, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "statm"))
	if err != nil {
		return 0, fmt.Errorf("read qemu rss for pid %d: %w", pid, err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0, fmt.Errorf("read qemu rss for pid %d: malformed statm", pid)
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("read qemu rss for pid %d: %w", pid, err)
	}
	return pages * int64(os.Getpagesize()) / 1024 / 1024, nil
}

type guestMeminfo struct {
	MemAvailableMiB int64
	MemFreeMiB      int64
	BuffCacheMiB    int64
}

func readGuestMeminfo(instance string) (guestMeminfo, error) {
	cmd := exec.Command("incus", "exec", instance, "--", "cat", "/proc/meminfo")
	out, err := cmd.Output()
	if err != nil {
		return guestMeminfo{}, fmt.Errorf("read guest meminfo for %s: %w", instance, err)
	}
	return parseGuestMeminfo(string(out)), nil
}

func parseGuestMeminfo(raw string) guestMeminfo {
	values := map[string]int64{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value / 1024
	}
	return guestMeminfo{
		MemAvailableMiB: values["MemAvailable"],
		MemFreeMiB:      values["MemFree"],
		BuffCacheMiB:    values["Buffers"] + values["Cached"],
	}
}

func readGuestTopMemoryProcess(instance string) (string, error) {
	cmd := exec.Command("incus", "exec", instance, "--", "sh", "-c", "ps -eo pid=,comm=,rss= --sort=-rss | head -1")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read guest top memory process for %s: %w", instance, err)
	}
	return strings.Join(strings.Fields(string(out)), " "), nil
}

func classifyHostRSSHighGuestUsageLow(fact ResidentRuntimeFact) bool {
	if fact.ConfiguredMemoryMiB <= 0 || fact.HostQEMURSSMiB <= 0 || fact.IncusMemoryCurrentMiB <= 0 || fact.GuestMemAvailableMiB <= 0 {
		return false
	}
	hostRSSHigh := fact.HostQEMURSSMiB*100 >= fact.ConfiguredMemoryMiB*80
	guestUsageLow := fact.IncusMemoryCurrentMiB*100 <= fact.ConfiguredMemoryMiB*25
	guestAvailableHigh := fact.GuestMemAvailableMiB*100 >= fact.ConfiguredMemoryMiB*50
	return hostRSSHigh && guestUsageLow && guestAvailableHigh
}

func appendMetricError(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return next
	}
	return existing + "; " + next
}

func bytesToMiB(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value / 1024 / 1024
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
