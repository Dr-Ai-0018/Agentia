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
			SparkRecoveryPerHour:     0.2,
			StrainRecoveryPerHour:    100,
			DayRecoveryPerHour:       50,
			WeekRecoveryPerHour:      25,
			FatigueRecoveryPerHour:   180,
			SleepDebtRecoveryPerHour: 2,
		},
		ReserveSpark:  0.08,
		ReserveStrain: 300,
	}
}

func DefaultResidentProfiles() []ResidentProfile {
	return []ResidentProfile{
		{
			ResidentID:   "jade",
			InitialGrant: 4.5,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  12000,
				Window6HUsed: 0,
				DayCap:       60000,
				DayUsed:      0,
				WeekCap:      150000,
				WeekUsed:     0,
			},
		},
		{
			ResidentID:   "amber",
			InitialGrant: 8.0,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  15000,
				Window6HUsed: 0,
				DayCap:       75000,
				DayUsed:      0,
				WeekCap:      150000,
				WeekUsed:     0,
			},
		},
		{
			ResidentID:   "onyx",
			InitialGrant: 3.0,
			InitialQuota: tokenledger.QuotaState{
				Window6HCap:  10000,
				Window6HUsed: 0,
				DayCap:       50000,
				DayUsed:      0,
				WeekCap:      150000,
				WeekUsed:     0,
			},
		},
	}
}
