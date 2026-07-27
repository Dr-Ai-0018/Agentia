package brokerstate

import (
	"testing"
	"time"
)

func TestBuildQuotaSnapshot(t *testing.T) {
	snapshot := BuildQuotaSnapshot(ResidentStatus{
		SparkBalance:         3.5,
		Window6HCap:          12000,
		Window6HUsed:         4000,
		DayCap:               60000,
		DayUsed:              9000,
		WeekCap:              150000,
		WeekUsed:             20000,
		EffectiveWindow6HCap: 4000,
		EffectiveDayCap:      54000,
		EffectiveWeekCap:     130000,
		RecoveryMode:         "idle",
		NextRecoveryAt:       "2026-06-07T09:00:00Z",
		RecoveryTickMinutes:  15,
	})

	if snapshot.Window6HRemaining != 8000 {
		t.Fatalf("window remaining = %d", snapshot.Window6HRemaining)
	}
	if snapshot.EffectiveWindow6HRemaining != 0 {
		t.Fatalf("effective window remaining = %d", snapshot.EffectiveWindow6HRemaining)
	}
	if snapshot.NextRecoveryAt != "2026-06-07T09:00:00Z" {
		t.Fatalf("unexpected next recovery at: %s", snapshot.NextRecoveryAt)
	}
	if !snapshot.WorkAllowedNow {
		t.Fatalf("effective 6h exhaustion should not block work now: %#v", snapshot)
	}
	if snapshot.RecoveryMode != "idle" {
		t.Fatalf("unexpected recovery mode: %s", snapshot.RecoveryMode)
	}
}

func TestBuildQuotaSnapshotBlocksOnRollingDayQuota(t *testing.T) {
	snapshot := BuildQuotaSnapshot(ResidentStatus{
		SparkBalance:         3.5,
		Window6HCap:          12000,
		DayCap:               60000,
		RollingDayUsed:       60000,
		WeekCap:              150000,
		RollingWeekUsed:      20000,
		EffectiveWindow6HCap: 4000,
		EffectiveDayCap:      54000,
		EffectiveWeekCap:     130000,
	})

	if snapshot.WorkAllowedNow {
		t.Fatalf("expected work to be blocked when rolling day quota is exhausted")
	}
	if snapshot.BlockingReason != "day_quota_exhausted" {
		t.Fatalf("unexpected blocking reason: %s", snapshot.BlockingReason)
	}
}

func TestBuildQuotaSnapshotMatchesFatigueAdmissionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fatigue int
		allowed bool
	}{
		{name: "below cap", fatigue: 2_499_999, allowed: true},
		{name: "at cap", fatigue: 2_500_000, allowed: false},
		{name: "above cap", fatigue: 2_500_001, allowed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := BuildQuotaSnapshot(ResidentStatus{
				SparkBalance: 10,
				Fatigue:      tc.fatigue,
				FatigueCap:   2_500_000,
			})
			if snapshot.WorkAllowedNow != tc.allowed {
				t.Fatalf("work allowed = %t, want %t: %#v", snapshot.WorkAllowedNow, tc.allowed, snapshot)
			}
			if !tc.allowed && snapshot.BlockingReason != "fatigue_exhausted" {
				t.Fatalf("blocking reason = %q, want fatigue_exhausted", snapshot.BlockingReason)
			}
		})
	}
}

func TestPhysiologyTightestQuotaIgnoresSixHourObservation(t *testing.T) {
	status := ResidentStatus{
		SparkBalance:         3.5,
		Window6HCap:          12000,
		Window6HUsed:         12000,
		DayCap:               60000,
		DayUsed:              6000,
		WeekCap:              150000,
		WeekUsed:             60000,
		EffectiveWindow6HCap: 12000,
		EffectiveDayCap:      60000,
		EffectiveWeekCap:     150000,
	}

	physiology := DerivePhysiology(status, testNow())
	if physiology.QuotaTightestLayer == "6h" {
		t.Fatalf("6h observation must not be reported as tightest gate: %#v", physiology)
	}
	if physiology.QuotaTightestLayer != "week" {
		t.Fatalf("tightest layer = %s, want week", physiology.QuotaTightestLayer)
	}

	snapshot := BuildQuotaSnapshot(status)
	if !snapshot.WorkAllowedNow {
		t.Fatalf("6h observation exhaustion must not block work: %#v", snapshot)
	}
}

func TestPhysiologyUsesNormalizedFatigueForPressure(t *testing.T) {
	status := ResidentStatus{
		SparkBalance:         20,
		Fatigue:              139441,
		SleepDebt:            0,
		DayCap:               1400000,
		WeekCap:              5005000,
		EffectiveDayCap:      1400000,
		EffectiveWeekCap:     5005000,
		RecoveryMode:         "idle",
		RecoveryTickMinutes:  15,
		EffectiveWindow6HCap: 240000,
	}

	physiology := DerivePhysiology(status, testNow())
	if physiology.Pressure == "critical" {
		t.Fatalf("low normalized fatigue should not read as critical: %#v", physiology)
	}
}

func TestDefaultFatigueRecoveryMatchesHumanScale(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	perHour := cfg.RecoveryPolicy.FatigueRecoveryPerHour
	if perHour < 50_000 || perHour > 150_000 {
		t.Fatalf("default fatigue recovery remains dimensionally implausible: %d/hour", perHour)
	}
	sleepMultiplier := cfg.RecoveryPolicy.ActivityMultipliers["sleep"]
	hoursFromCap := float64(cfg.FatigueCap) / (float64(perHour) * sleepMultiplier)
	if hoursFromCap < 6 || hoursFromCap > 16 {
		t.Fatalf("full fatigue recovery under sleep = %.2fh, want night-scale", hoursFromCap)
	}
}

func testNow() time.Time {
	return time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
}
