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
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}
}

func DefaultResidentProfiles() []ResidentProfile {
	return []ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 0.62,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  4000,
				Window6HUsed: 300,
				DayCap:       20000,
				DayUsed:      1500,
				WeekCap:      150000,
				WeekUsed:     8000,
			},
		},
		{
			ResidentID:   "amber",
			InitialGrant: 0.8,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  4200,
				Window6HUsed: 250,
				DayCap:       21000,
				DayUsed:      1200,
				WeekCap:      150000,
				WeekUsed:     7000,
			},
		},
		{
			ResidentID:   "onyx",
			InitialGrant: 0.5,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  3800,
				Window6HUsed: 400,
				DayCap:       19000,
				DayUsed:      1800,
				WeekCap:      150000,
				WeekUsed:     8500,
			},
		},
	}
}
