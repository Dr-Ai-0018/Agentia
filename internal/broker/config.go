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
	Root      string              `json:"root"`
	Runtime   runtimecore.Config  `json:"runtime"`
	Residents []ResidentBinding   `json:"residents"`
}

type ResidentBinding struct {
	ResidentID   string  `json:"resident_id"`
	InstanceName string  `json:"instance_name"`
	InitialGrant float64 `json:"initial_grant"`
	Window6HCap  int     `json:"window_6h_cap"`
	DayCap       int     `json:"day_cap"`
	WeekCap      int     `json:"week_cap"`
}

func DefaultConfig(root string) Config {
	return Config{
		Root:    strings.TrimSpace(root),
		Runtime: brokerstate.DefaultRuntimeConfig(),
		Residents: []ResidentBinding{
			{
				ResidentID:   "jade",
				InstanceName: "jade",
				InitialGrant: 4.5,
				Window6HCap:  12000,
				DayCap:       60000,
				WeekCap:      150000,
			},
			{
				ResidentID:   "amber",
				InstanceName: "amber",
				InitialGrant: 8.0,
				Window6HCap:  15000,
				DayCap:       75000,
				WeekCap:      150000,
			},
			{
				ResidentID:   "onyx",
				InstanceName: "onyx",
				InitialGrant: 3.0,
				Window6HCap:  10000,
				DayCap:       50000,
				WeekCap:      150000,
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
