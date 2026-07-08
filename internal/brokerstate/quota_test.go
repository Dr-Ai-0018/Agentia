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

func testNow() time.Time {
	return time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
}
