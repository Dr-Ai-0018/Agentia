package broker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigResidentProfiles(t *testing.T) {
	cfg := DefaultConfig(".agents")
	profiles := cfg.ResidentProfiles()
	if len(profiles) != 3 {
		t.Fatalf("expected 3 resident profiles, got %d", len(profiles))
	}
	if profiles[0].ResidentID != "jade" {
		t.Fatalf("expected jade first, got %s", profiles[0].ResidentID)
	}
	if profiles[1].InitialQuota.Window6HCap != 15000 {
		t.Fatalf("unexpected amber 6h cap: %d", profiles[1].InitialQuota.Window6HCap)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broker.json")
	raw := `{
  "root": ".agents",
  "runtime": {
    "ReserveSpark": 0.08,
    "ReserveStrain": 300
  },
  "residents": [
    {
      "resident_id": "jade",
      "instance_name": "jade-vm",
      "initial_grant": 5,
      "window_6h_cap": 123,
      "day_cap": 456,
      "week_cap": 789
    }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Root != ".agents" {
		t.Fatalf("unexpected root: %s", cfg.Root)
	}
	if len(cfg.Residents) != 1 || cfg.Residents[0].InstanceName != "jade-vm" {
		t.Fatalf("unexpected resident bindings: %#v", cfg.Residents)
	}
}

func TestResidentRegistryBinding(t *testing.T) {
	registry := NewResidentRegistry([]ResidentBinding{
		{ResidentID: "jade", InstanceName: "jade-vm"},
	})
	binding, ok := registry.Binding("Jade")
	if !ok {
		t.Fatalf("expected binding to exist")
	}
	if binding.InstanceName != "jade-vm" {
		t.Fatalf("unexpected instance name: %s", binding.InstanceName)
	}
}
