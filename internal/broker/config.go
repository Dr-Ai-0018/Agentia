package broker

import (
	"encoding/json"
	"os"
	"strings"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

type Config struct {
	Root         string             `json:"root"`
	Runtime      runtimecore.Config `json:"runtime"`
	HostCapacity HostCapacityConfig `json:"host_capacity"`
	Residents    []ResidentBinding  `json:"residents"`
}

type ResidentBinding struct {
	ResidentID   string  `json:"resident_id"`
	InstanceName string  `json:"instance_name"`
	InitialGrant float64 `json:"initial_grant"`
	Window6HCap  int     `json:"window_6h_cap"`
	DayCap       int     `json:"day_cap"`
	WeekCap      int     `json:"week_cap"`
	VCPU         int     `json:"vcpu"`
	MemoryMiB    int64   `json:"memory_mib"`
	DiskGiB      int64   `json:"disk_gib"`
}

func DefaultConfig(root string) Config {
	return Config{
		Root:         strings.TrimSpace(root),
		Runtime:      brokerstate.DefaultRuntimeConfig(),
		HostCapacity: DefaultHostCapacityConfig(),
		Residents: []ResidentBinding{
			{
				ResidentID:   "jade",
				InstanceName: "jade",
				InitialGrant: 665.0,
				Window6HCap:  7_100_000,
				DayCap:       24_000_000,
				WeekCap:      168_000_000,
				VCPU:         1,
				MemoryMiB:    2048,
				DiskGiB:      12,
			},
			{
				ResidentID:   "amber",
				InstanceName: "amber",
				InitialGrant: 160.0,
				Window6HCap:  3_800_000,
				DayCap:       5_500_000,
				WeekCap:      38_500_000,
				VCPU:         1,
				MemoryMiB:    2048,
				DiskGiB:      12,
			},
			{
				ResidentID:   "onyx",
				InstanceName: "onyx",
				InitialGrant: 505.0,
				Window6HCap:  14_000_000,
				DayCap:       19_000_000,
				WeekCap:      133_000_000,
				VCPU:         1,
				MemoryMiB:    2048,
				DiskGiB:      12,
			},
		},
	}
}

func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Root = strings.TrimSpace(cfg.Root)
	return cfg, nil
}

func (c Config) ResidentProfiles() []brokerstate.ResidentProfile {
	out := make([]brokerstate.ResidentProfile, 0, len(c.Residents))
	for _, resident := range c.Residents {
		out = append(out, brokerstate.ResidentProfile{
			ResidentID:   resident.ResidentID,
			InitialGrant: resident.InitialGrant,
			InitialQuota: brokerstateQuota(resident),
		})
	}
	return out
}

func brokerstateQuota(binding ResidentBinding) tokenledger.QuotaState {
	return tokenledger.QuotaState{
		Window6HCap:  binding.Window6HCap,
		Window6HUsed: 0,
		DayCap:       binding.DayCap,
		DayUsed:      0,
		WeekCap:      binding.WeekCap,
		WeekUsed:     0,
	}
}
