package broker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveInventorySnapshot(t *testing.T) {
	root := t.TempDir()
	snapshot := InventorySnapshot{
		CollectedAt: time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Residents: []ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           1,
				MemoryLimitMiB: 2048,
				DiskGiB:        12,
				IPv4:           "10.0.0.2",
				UpdatedAt:      time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Format(time.RFC3339),
			},
		},
	}

	path, err := SaveInventorySnapshot(root, snapshot)
	if err != nil {
		t.Fatalf("save inventory snapshot: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat inventory snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read inventory snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `"resident_id": "amber"`) {
		t.Fatalf("expected amber resident in snapshot, got %s", string(raw))
	}
	if filepath.Base(path) != "incus-inventory.json" {
		t.Fatalf("unexpected snapshot filename: %s", path)
	}
}

func TestParseMemoryLimitMiB(t *testing.T) {
	if got := parseMemoryLimitMiB("2GiB", 0); got != 2048 {
		t.Fatalf("expected 2048 MiB, got %d", got)
	}
	if got := parseMemoryLimitMiB("2304MiB", 0); got != 2304 {
		t.Fatalf("expected 2304 MiB, got %d", got)
	}
	if got := parseMemoryLimitMiB("bad", 123); got != 123 {
		t.Fatalf("expected fallback 123, got %d", got)
	}
}
