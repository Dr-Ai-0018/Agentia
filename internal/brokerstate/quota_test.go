package brokerstate

import "testing"

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
	if snapshot.WorkAllowedNow {
		t.Fatalf("expected work to be blocked when effective remaining is exhausted")
	}
	if snapshot.BlockingReason != "effective_window_exhausted" {
		t.Fatalf("unexpected blocking reason: %s", snapshot.BlockingReason)
	}
	if snapshot.RecoveryMode != "idle" {
		t.Fatalf("unexpected recovery mode: %s", snapshot.RecoveryMode)
	}
}
