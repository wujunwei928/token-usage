package core

// Tests ported from the cost.rs test module of the reference implementation.

import (
	"encoding/json"
	"math"
	"testing"
)

func testCostPricing() *PricingMap {
	pricing := NewPricingMap()
	pricing.LoadJSON(`{
        "test-model": {
            "input_cost_per_token": 1.0,
            "output_cost_per_token": 10.0,
            "cache_creation_input_token_cost": 1.25,
            "cache_read_input_token_cost": 0.1,
            "input_cost_per_token_above_200k_tokens": 2.0,
            "cache_creation_input_token_cost_above_200k_tokens": 1.5
        }
    }`)
	return pricing
}

func TestPricesCacheCreationBreakdownByDuration(t *testing.T) {
	usage := TokenUsageRaw{
		CacheCreationInputTokens: 999,
		CacheReadInputTokens:     30,
		CacheCreation: &CacheCreationRaw{
			Ephemeral5mInputTokens: 10,
			Ephemeral1hInputTokens: 20,
		},
	}
	model := "test-model"
	cost := CalculateCostForUsage(&model, usage, nil, ModeCalculate, testCostPricing())
	if math.Abs(cost-55.5) > 1e-12 {
		t.Fatalf("cost = %v want 55.5", cost)
	}
}

func TestFallsBackToFlatCacheCreationRateWithoutBreakdown(t *testing.T) {
	usage := TokenUsageRaw{CacheCreationInputTokens: 10}
	model := "test-model"
	cost := CalculateCostForUsage(&model, usage, nil, ModeCalculate, testCostPricing())
	if math.Abs(cost-12.5) > 1e-12 {
		t.Fatalf("cost = %v want 12.5", cost)
	}
}

func TestPricesTwoStageModelAsWholeRequestAtLongContextRates(t *testing.T) {
	pricing := LoadEmbedded()
	model := "gpt-5.6-sol"

	// gpt-5.6-sol has a 272K threshold with long-context rates of $10/$45
	// per 1M input/output tokens and a $1 per 1M cache-read rate.
	long := TokenUsageRaw{InputTokens: 300_000, OutputTokens: 1_000, CacheReadInputTokens: 100}
	cost := CalculateCostForUsage(&model, long, nil, ModeCalculate, pricing)
	// The whole request switches to long rates once input exceeds 272K,
	// including the output and cache-read buckets that are individually far
	// below the threshold: 3.0 + 0.045 + 0.0001.
	if math.Abs(cost-3.0451) > 1e-9 {
		t.Fatalf("long-context cost = %v want 3.0451", cost)
	}

	// Below the threshold every bucket stays on the short-context rates:
	// 0.5 + 0.03 + 0.00005.
	short := TokenUsageRaw{InputTokens: 100_000, OutputTokens: 1_000, CacheReadInputTokens: 100}
	cost = CalculateCostForUsage(&model, short, nil, ModeCalculate, pricing)
	if math.Abs(cost-0.53005) > 1e-9 {
		t.Fatalf("short-context cost = %v want 0.53005", cost)
	}
}

func TestParsesCacheCreationBreakdownFromUsageJSON(t *testing.T) {
	var usage TokenUsageRaw
	if err := json.Unmarshal([]byte(`{
        "input_tokens": 1,
        "output_tokens": 2,
        "cache_creation_input_tokens": 300,
        "cache_creation": {
            "ephemeral_5m_input_tokens": 100,
            "ephemeral_1h_input_tokens": 200
        }
    }`), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.CacheCreationTokenCount() != 300 {
		t.Fatalf("cache creation token count = %d", usage.CacheCreationTokenCount())
	}
}

// Marginal above-200K tiering for LiteLLM `*_above_200k_tokens` data: the
// first 200K tokens bill at the base rate, the remainder at the tier rate.
func TestTieredCostMarginalAbove200k(t *testing.T) {
	pricing := testCostPricing()
	model := "test-model"

	cases := []struct {
		name  string
		usage TokenUsageRaw
		want  float64
	}{
		{
			name:  "input below threshold",
			usage: TokenUsageRaw{InputTokens: 200_000},
			want:  200_000 * 1.0,
		},
		{
			name:  "input above threshold prices marginally",
			usage: TokenUsageRaw{InputTokens: 300_000},
			want:  200_000*1.0 + 100_000*2.0,
		},
		{
			name:  "cache create above threshold uses tier rate",
			usage: TokenUsageRaw{CacheCreationInputTokens: 250_000},
			want:  200_000*1.25 + 50_000*1.5,
		},
		{
			name: "cache read has no tier rate so stays flat",
			usage: TokenUsageRaw{
				InputTokens:              1,
				OutputTokens:             1,
				CacheCreationInputTokens: 1,
				CacheReadInputTokens:     500_000,
			},
			want: 1*1.0 + 1*10.0 + 1*1.25 + 500_000*0.1,
		},
	}
	for _, tc := range cases {
		cost := CalculateCostForUsage(&model, tc.usage, nil, ModeCalculate, pricing)
		if math.Abs(cost-tc.want) > 1e-9 {
			t.Fatalf("%s: cost = %v want %v", tc.name, cost, tc.want)
		}
	}
}

// The 1h cache-write bucket bills at 2x input, including in the above-200K
// tier (2x the tier's input rate when the entry does not carry an explicit
// 1h tier rate).
func TestOneHourCacheCreationCostsDoubleInput(t *testing.T) {
	pricing := testCostPricing()
	model := "test-model"

	below := TokenUsageRaw{
		CacheCreation: &CacheCreationRaw{Ephemeral1hInputTokens: 100},
	}
	cost := CalculateCostForUsage(&model, below, nil, ModeCalculate, pricing)
	// test-model's input is 1.0, so 1h writes bill at 2.0 per token.
	if math.Abs(cost-200.0) > 1e-9 {
		t.Fatalf("1h cache below threshold cost = %v want 200", cost)
	}

	above := TokenUsageRaw{
		CacheCreation: &CacheCreationRaw{Ephemeral1hInputTokens: 250_000},
	}
	cost = CalculateCostForUsage(&model, above, nil, ModeCalculate, pricing)
	// Marginal tiering at 200K: 200_000*2.0 + 50_000*(2.0*2.0).
	if math.Abs(cost-(400_000+200_000)) > 1e-9 {
		t.Fatalf("1h cache above threshold cost = %v want 600000", cost)
	}
}

// Auto mode prefers costUSD and falls back to token pricing; display mode
// never prices tokens.
func TestCostModesResolveCostSources(t *testing.T) {
	pricing := testCostPricing()
	model := "test-model"
	usage := TokenUsageRaw{InputTokens: 100}
	costUSD := 0.5

	if got := CalculateCostForUsage(&model, usage, &costUSD, ModeAuto, pricing); got != 0.5 {
		t.Fatalf("auto with costUSD = %v", got)
	}
	if got := CalculateCostForUsage(&model, usage, nil, ModeAuto, pricing); got != 100 {
		t.Fatalf("auto without costUSD = %v", got)
	}
	if got := CalculateCostForUsage(&model, usage, nil, ModeDisplay, pricing); got != 0 {
		t.Fatalf("display without costUSD = %v", got)
	}
	if got := CalculateCostForUsage(&model, usage, &costUSD, ModeDisplay, pricing); got != 0.5 {
		t.Fatalf("display with costUSD = %v", got)
	}
	if got := CalculateCostForUsage(&model, usage, &costUSD, ModeCalculate, pricing); got != 100 {
		t.Fatalf("calculate ignores costUSD = %v", got)
	}
}

// Missing-pricing detection only flags models whose token total is positive
// and whose cost had to be computed from tokens.
func TestMissingPricingModelForUsageFollowsModeAndTotals(t *testing.T) {
	pricing := testCostPricing()
	model := "test-model"
	unknown := "totally-unknown-model"
	costUSD := 0.5
	usage := TokenUsageRaw{InputTokens: 10}
	zero := TokenUsageRaw{}

	if got := MissingPricingModelForUsage(&unknown, usage, nil, ModeCalculate, pricing); got == nil || *got != "totally-unknown-model" {
		t.Fatalf("calculate with unknown model = %v", got)
	}
	if got := MissingPricingModelForUsage(&unknown, usage, &costUSD, ModeAuto, pricing); got != nil {
		t.Fatalf("auto with costUSD = %v", got)
	}
	if got := MissingPricingModelForUsage(&unknown, usage, nil, ModeDisplay, pricing); got != nil {
		t.Fatalf("display = %v", got)
	}
	if got := MissingPricingModelForUsage(&unknown, zero, nil, ModeCalculate, pricing); got != nil {
		t.Fatalf("zero total = %v", got)
	}
	if got := MissingPricingModelForUsage(&model, usage, nil, ModeCalculate, pricing); got != nil {
		t.Fatalf("known model = %v", got)
	}
	if got := MissingPricingModelForUsage(nil, usage, nil, ModeCalculate, pricing); got != nil {
		t.Fatalf("nil model = %v", got)
	}
}
