package brokerstate

import (
	"testing"
	"time"
)

func TestClassifySleepDepth(t *testing.T) {
	if got := ClassifySleepDepth(12*time.Minute, 0); got != SleepDepthRest {
		t.Fatalf("12m sleep depth = %s, want rest", got)
	}
	if got := ClassifySleepDepth(90*time.Minute, 0); got != SleepDepthSleep {
		t.Fatalf("90m sleep depth = %s, want sleep", got)
	}
	if got := ClassifySleepDepth(4*time.Hour, 1); got != SleepDepthSleep {
		t.Fatalf("4h without enough rolling sleep = %s, want sleep", got)
	}
	if got := ClassifySleepDepth(4*time.Hour, 2); got != SleepDepthDeep {
		t.Fatalf("4h with enough rolling sleep = %s, want deep_sleep", got)
	}
}

func TestStoreRecordSleepStartAndEnd(t *testing.T) {
	store := New(t.TempDir())
	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	if _, _, err := store.RecordSleepStart("jade", 90, start, "tired"); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	active, _, err := store.LoadActiveSleep("jade")
	if err != nil {
		t.Fatalf("load active sleep: %v", err)
	}
	if active.ResidentID != "jade" || active.PlannedMinutes != 90 {
		t.Fatalf("unexpected active sleep: %#v", active)
	}
	state := store.CurrentSleepState("jade", start.Add(90*time.Minute))
	if state.Depth != SleepDepthSleep {
		t.Fatalf("active sleep depth = %s, want sleep", state.Depth)
	}

	ended, _, err := store.RecordSleepEnd("jade", start.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("end sleep: %v", err)
	}
	if ended.ActualMinutes != 90 || ended.Depth != SleepDepthSleep {
		t.Fatalf("unexpected ended sleep: %#v", ended)
	}
	active, _, err = store.LoadActiveSleep("jade")
	if err != nil {
		t.Fatalf("reload active sleep: %v", err)
	}
	if active.ResidentID != "" {
		t.Fatalf("expected active sleep to be cleared, got %#v", active)
	}
	sessions, _, err := store.LoadSleepSessions("jade")
	if err != nil {
		t.Fatalf("load sleep sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sleep sessions = %d, want 1", len(sessions))
	}
}

func TestSleepDebtFromRollingWeightedSleep(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	sessions := []SleepSession{
		{ResidentID: "jade", StartedAt: now.Add(-10 * time.Hour), EndedAt: now.Add(-3 * time.Hour), ActualMinutes: 7 * 60, Depth: SleepDepthSleep},
		{ResidentID: "jade", StartedAt: now.Add(-34 * time.Hour), EndedAt: now.Add(-27 * time.Hour), ActualMinutes: 7 * 60, Depth: SleepDepthSleep},
		{ResidentID: "jade", StartedAt: now.Add(-58 * time.Hour), EndedAt: now.Add(-51 * time.Hour), ActualMinutes: 7 * 60, Depth: SleepDepthSleep},
	}
	weighted := RollingWeightedSleepFromSessions(sessions, now, 72*time.Hour)
	if weighted.WeightedHours != 21 {
		t.Fatalf("weighted hours = %.2f, want 21", weighted.WeightedHours)
	}
	if weighted.ObservedSince.IsZero() {
		t.Fatalf("expected observed sleep window start")
	}

	store := New(t.TempDir())
	for _, session := range sessions[:2] {
		if _, err := store.AppendSleepSession(session); err != nil {
			t.Fatalf("append sleep session: %v", err)
		}
	}
	debt, err := store.SleepDebt("jade", now)
	if err != nil {
		t.Fatalf("sleep debt: %v", err)
	}
	if debt.DebtHours != 0 {
		t.Fatalf("debt hours = %.2f, want 0 because observed history is fully covered", debt.DebtHours)
	}
}

func TestSleepDebtStartsNeutralWithoutSleepHistory(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	store := New(t.TempDir())

	debt, err := store.SleepDebt("jade", now)
	if err != nil {
		t.Fatalf("sleep debt: %v", err)
	}
	if debt.DebtHours != 0 {
		t.Fatalf("debt hours = %.2f, want neutral 0 without sleep history", debt.DebtHours)
	}
}

func TestSleepDebtTargetScalesWithObservedHistory(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	store := New(t.TempDir())
	if _, err := store.AppendSleepSession(SleepSession{
		ResidentID:    "jade",
		StartedAt:     now.Add(-10 * time.Hour),
		EndedAt:       now.Add(-9 * time.Hour),
		ActualMinutes: 60,
		Depth:         SleepDepthSleep,
	}); err != nil {
		t.Fatalf("append sleep session: %v", err)
	}

	debt, err := store.SleepDebt("jade", now)
	if err != nil {
		t.Fatalf("sleep debt: %v", err)
	}
	if debt.DebtHours != 1.92 {
		t.Fatalf("debt hours = %.2f, want 1.92", debt.DebtHours)
	}
}
