package tokenledger

import "testing"

func TestComputeStrain(t *testing.T) {
	cfg := DefaultConfig()
	usage := Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
	}
	penalties := Penalties{
		ToolCallCount: 2,
	}

	got := ComputeStrain(cfg, usage, penalties)
	if got.UncachedInputTokens != 400 {
		t.Fatalf("uncached input = %d, want 400", got.UncachedInputTokens)
	}
	if got.Rounded != 935 {
		t.Fatalf("rounded strain = %d, want 935", got.Rounded)
	}
}

func TestComputeCost(t *testing.T) {
	cfg := DefaultConfig()
	usage := Usage{
		InputTokens:  1200,
		CachedTokens: 800,
		OutputTokens: 300,
		Model:        "gpt-5.4",
	}

	got, err := ComputeCost(cfg, usage)
	if err != nil {
		t.Fatalf("compute cost: %v", err)
	}
	if got.TotalUSD <= 0 {
		t.Fatalf("total usd must be positive")
	}
	if got.SparkCost <= 0 {
		t.Fatalf("spark cost must be positive")
	}
	if got.SparkDisplay <= 0 {
		t.Fatalf("spark display must be positive")
	}
}

func TestDisplaySpark(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{in: 12.34567, want: 12.3},
		{in: 3.71888, want: 3.72},
		{in: 0.57888, want: 0.579},
		{in: 0.02344, want: 0.0234},
	}

	for _, tc := range cases {
		got := displaySpark(tc.in)
		if got != tc.want {
			t.Fatalf("displaySpark(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestApplyQuota(t *testing.T) {
	state := QuotaState{
		Window6HCap:  1000,
		Window6HUsed: 300,
		DayCap:       5000,
		DayUsed:      2000,
		WeekCap:      30000,
		WeekUsed:     12000,
	}

	update := ApplyQuota(state, 800)
	if !update.Exceeded {
		t.Fatalf("expected exceed true")
	}
	if len(update.ExceededTiers) != 1 || update.ExceededTiers[0] != "6h" {
		t.Fatalf("unexpected exceeded tiers: %#v", update.ExceededTiers)
	}
}

func TestComputeFatigue(t *testing.T) {
	cfg := DefaultConfig()
	strain := Strain{Rounded: 1000}

	got, err := ComputeFatigue(cfg, ActivityDeepWork, strain)
	if err != nil {
		t.Fatalf("compute fatigue: %v", err)
	}
	if got.FatigueGain != 1200 {
		t.Fatalf("fatigue gain = %d, want 1200", got.FatigueGain)
	}
}
