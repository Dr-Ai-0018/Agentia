package brokerstate

import (
	"testing"
	"time"
)

func TestQuotaEventRollingUsage(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	events := []QuotaEvent{
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 100, CreatedAt: now.Add(-5 * time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventAcceptanceCall, StrainCost: 200, CreatedAt: now.Add(-23 * time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 300, CreatedAt: now.Add(-25 * time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 400, CreatedAt: now.Add(-8 * 24 * time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventTestAllowance, StrainCost: 999, CreatedAt: now.Add(-time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 777, CreatedAt: now.Add(time.Hour)},
	}

	got := RollingQuotaUsageFromEvents(events, now)
	if got.Window6HUsed != 100 {
		t.Fatalf("rolling 6h = %d, want 100", got.Window6HUsed)
	}
	if got.DayUsed != 300 {
		t.Fatalf("rolling day = %d, want 300", got.DayUsed)
	}
	if got.WeekUsed != 600 {
		t.Fatalf("rolling week = %d, want 600", got.WeekUsed)
	}
}

func TestSixHourBurnSamplesFromEvents(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	events := []QuotaEvent{
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 10, CreatedAt: now.Add(-5*time.Hour - 45*time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventAcceptanceCall, StrainCost: 20, CreatedAt: now.Add(-5*time.Hour - 30*time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 30, CreatedAt: now.Add(-time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventFinalNotice, StrainCost: 40, CreatedAt: now},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 999, CreatedAt: now.Add(-6 * time.Hour)},
		{ResidentID: "jade", Kind: QuotaEventTestAllowance, StrainCost: 999, CreatedAt: now.Add(-15 * time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventManualAdjustment, StrainCost: 999, CreatedAt: now.Add(-15 * time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 999, CreatedAt: now.Add(time.Minute)},
		{ResidentID: "jade", Kind: QuotaEventWorkCall, StrainCost: 999, CreatedAt: now.Add(-7 * time.Hour)},
	}

	got := SixHourBurnSamplesFromEvents(events, now)
	if len(got) != SixHourBurnSampleCount {
		t.Fatalf("sample count = %d, want %d", len(got), SixHourBurnSampleCount)
	}
	if got[0] != 10 {
		t.Fatalf("bucket 0 = %d, want 10; samples=%v", got[0], got)
	}
	if got[1] != 20 {
		t.Fatalf("bucket 1 = %d, want 20; samples=%v", got[1], got)
	}
	if got[11] != 70 {
		t.Fatalf("bucket 11 = %d, want 70; samples=%v", got[11], got)
	}
	total := 0
	for _, sample := range got {
		total += sample
	}
	if total != 100 {
		t.Fatalf("sample total = %d, want 100; samples=%v", total, got)
	}
}

func TestStoreAppendAndLoadQuotaEvents(t *testing.T) {
	store := New(t.TempDir())
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	if _, err := store.AppendQuotaEvent(QuotaEvent{
		ResidentID: "amber",
		Kind:       QuotaEventWorkCall,
		StrainCost: 123,
		SparkCost:  1.25,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("append quota event: %v", err)
	}

	events, _, err := store.LoadQuotaEvents("amber")
	if err != nil {
		t.Fatalf("load quota events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].ID == "" {
		t.Fatalf("expected generated event id")
	}
	if events[0].StrainCost != 123 || events[0].SparkCost != 1.25 {
		t.Fatalf("unexpected event: %#v", events[0])
	}
}

func TestStoreMissingQuotaEventsIsEmpty(t *testing.T) {
	store := New(t.TempDir())
	events, _, err := store.LoadQuotaEvents("onyx")
	if err != nil {
		t.Fatalf("missing quota events should be empty: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}
