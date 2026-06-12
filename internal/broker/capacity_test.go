package broker

import "testing"

func TestBuildHostCapacityReportUsesResidentAllocations(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	report, err := BuildHostCapacityReport(cfg)
	if err != nil {
		t.Fatalf("build host capacity report: %v", err)
	}
	if len(report.Pools) != 3 {
		t.Fatalf("expected 3 pools, got %d", len(report.Pools))
	}

	var cpu, memory, disk ResourcePoolSummary
	for _, pool := range report.Pools {
		switch pool.Resource {
		case "cpu":
			cpu = pool
		case "memory":
			memory = pool
		case "disk":
			disk = pool
		}
	}
	if cpu.ResidentAllocated != 3 {
		t.Fatalf("expected 3 resident vcpu allocated, got %d", cpu.ResidentAllocated)
	}
	if memory.ResidentAllocated != 6144 {
		t.Fatalf("expected 6144 MiB resident memory allocated, got %d", memory.ResidentAllocated)
	}
	if disk.ResidentAllocated != 36 {
		t.Fatalf("expected 36 GiB resident disk allocated, got %d", disk.ResidentAllocated)
	}
	if cpu.AllocatableFree < 0 || memory.AllocatableFree < 0 || disk.AllocatableFree < 0 {
		t.Fatalf("allocatable free should never be negative: %#v", report.Pools)
	}
}
