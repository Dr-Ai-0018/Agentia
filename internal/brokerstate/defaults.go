package brokerstate

import (
	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

func DefaultRuntimeConfig() runtimecore.Config {
	return runtimecore.Config{
		TokenPolicy: tokenledger.DefaultConfig(),
		RecoveryPolicy: recovery.Policy{
			SparkRecoveryPerHour:  0.2,
			StrainRecoveryPerHour: 100,
			DayRecoveryPerHour:    50,
			WeekRecoveryPerHour:   25,
			// A full fatigue bar should recover on the scale of a night's sleep,
			// not multiple calendar years. Sleep-depth and debt multipliers still apply.
			FatigueRecoveryPerHour:   100_000,
			SleepDebtRecoveryPerHour: 2,
			ActivityMultipliers: map[string]float64{
				"idle":       1.0,
				"normal":     1.0,
				"rest":       1.5,
				"sleep":      2.5,
				"deep_sleep": 4.0,
				"deep":       4.0,
			},
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
		FatigueCap:    2_500_000,
	}
}

func DefaultResidentProfiles() []ResidentProfile {
	return []ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 665.0,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  7_100_000,
				Window6HUsed: 0,
				DayCap:       24_000_000,
				DayUsed:      0,
				WeekCap:      168_000_000,
				WeekUsed:     0,
			},
		},
		{
			ResidentID:   "amber",
			InitialGrant: 160.0,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  3_800_000,
				Window6HUsed: 0,
				DayCap:       5_500_000,
				DayUsed:      0,
				WeekCap:      38_500_000,
				WeekUsed:     0,
			},
		},
		{
			ResidentID:   "onyx",
			InitialGrant: 505.0,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  14_000_000,
				Window6HUsed: 0,
				DayCap:       19_000_000,
				DayUsed:      0,
				WeekCap:      133_000_000,
				WeekUsed:     0,
			},
		},
	}
}
