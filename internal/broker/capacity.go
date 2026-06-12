package broker

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type HostCapacityConfig struct {
	ReserveVCPU      int   `json:"reserve_vcpu"`
	ReserveMemoryMiB int64 `json:"reserve_memory_mib"`
	ReserveDiskGiB   int64 `json:"reserve_disk_gib"`
}

type ResourcePoolSummary struct {
	Resource          string `json:"resource"`
	HostTotal         int64  `json:"host_total"`
	HostAvailable     int64  `json:"host_available"`
	Reserved          int64  `json:"reserved"`
	ResidentAllocated int64  `json:"resident_allocated"`
	AllocatableTotal  int64  `json:"allocatable_total"`
	AllocatableFree   int64  `json:"allocatable_free"`
	Unit              string `json:"unit"`
}

type HostCapacityReport struct {
	Pools []ResourcePoolSummary `json:"pools"`
}

func DefaultHostCapacityConfig() HostCapacityConfig {
	return HostCapacityConfig{
		ReserveVCPU:      2,
		ReserveMemoryMiB: 4096,
		ReserveDiskGiB:   40,
	}
}

func BuildHostCapacityReport(cfg Config) (HostCapacityReport, error) {
	hostCPU, err := detectHostCPUCount()
	if err != nil {
		return HostCapacityReport{}, err
	}
	memTotalMiB, memAvailMiB, err := detectHostMemoryMiB()
	if err != nil {
		return HostCapacityReport{}, err
	}
	diskTotalGiB, diskAvailGiB, err := detectRootDiskGiB()
	if err != nil {
		return HostCapacityReport{}, err
	}

	capCfg := cfg.HostCapacity
	if capCfg.ReserveVCPU == 0 && capCfg.ReserveMemoryMiB == 0 && capCfg.ReserveDiskGiB == 0 {
		capCfg = DefaultHostCapacityConfig()
	}

	var allocatedCPU int64
	var allocatedMemoryMiB int64
	var allocatedDiskGiB int64
	for _, resident := range cfg.Residents {
		allocatedCPU += int64(resident.VCPU)
		allocatedMemoryMiB += resident.MemoryMiB
		allocatedDiskGiB += resident.DiskGiB
	}

	return HostCapacityReport{
		Pools: []ResourcePoolSummary{
			makePool("cpu", int64(hostCPU), int64(hostCPU), int64(capCfg.ReserveVCPU), allocatedCPU, "vcpu"),
			makePool("memory", memTotalMiB, memAvailMiB, capCfg.ReserveMemoryMiB, allocatedMemoryMiB, "MiB"),
			makePool("disk", diskTotalGiB, diskAvailGiB, capCfg.ReserveDiskGiB, allocatedDiskGiB, "GiB"),
		},
	}, nil
}

func makePool(resource string, total, available, reserved, allocated int64, unit string) ResourcePoolSummary {
	allocatableTotal := total - reserved
	if allocatableTotal < 0 {
		allocatableTotal = 0
	}
	allocatableFree := available - reserved
	if allocatableFree < 0 {
		allocatableFree = 0
	}
	if allocatableFree > allocatableTotal-allocated {
		allocatableFree = allocatableTotal - allocated
	}
	if allocatableFree < 0 {
		allocatableFree = 0
	}
	return ResourcePoolSummary{
		Resource:          resource,
		HostTotal:         total,
		HostAvailable:     available,
		Reserved:          reserved,
		ResidentAllocated: allocated,
		AllocatableTotal:  allocatableTotal,
		AllocatableFree:   allocatableFree,
		Unit:              unit,
	}
}

func detectHostCPUCount() (int, error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "processor") {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, fmt.Errorf("host cpu count not found")
	}
	return count, nil
}

func detectHostMemoryMiB() (int64, int64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	var totalKiB, availableKiB int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKiB = parseMeminfoKiB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			availableKiB = parseMeminfoKiB(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if totalKiB == 0 {
		return 0, 0, fmt.Errorf("host memory total not found")
	}
	return totalKiB / 1024, availableKiB / 1024, nil
}

func parseMeminfoKiB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	value, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func detectRootDiskGiB() (int64, int64, error) {
	var stat syscallStatfs
	if err := statfs("/", &stat); err != nil {
		return 0, 0, err
	}
	total := int64(stat.Blocks) * int64(stat.Bsize) / (1024 * 1024 * 1024)
	available := int64(stat.Bavail) * int64(stat.Bsize) / (1024 * 1024 * 1024)
	return total, available, nil
}
