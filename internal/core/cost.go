package core

// Cost calculation ported from rust/crates/ccusage-core/src/cost.rs.

// cacheCreate1hInputMultiplier prices 1h ephemeral cache writes at 2x input.
const cacheCreate1hInputMultiplier = 2.0

// CalculateCost resolves an entry's cost under the given mode.
func CalculateCost(data *UsageEntry, mode CostMode, pricing *PricingMap) float64 {
	return CalculateCostForUsage(data.Message.Model, data.Message.Usage, data.CostUSD, mode, pricing)
}

// CalculateCostForUsage resolves cost for one usage object: display trusts
// costUSD (default 0), calculate prices tokens, auto prefers costUSD.
func CalculateCostForUsage(model *string, usage TokenUsageRaw, costUSD *float64, mode CostMode, pricing *PricingMap) float64 {
	switch mode {
	case ModeDisplay:
		if costUSD != nil {
			return *costUSD
		}
		return 0
	case ModeAuto:
		if costUSD != nil {
			return *costUSD
		}
		return calculateCostFromTokens(model, usage, pricing)
	default:
		return calculateCostFromTokens(model, usage, pricing)
	}
}

// MissingPricingModelForUsage reports the model name when its cost had to be
// computed from tokens but no price exists. Display mode and auto mode with a
// present costUSD never consult pricing.
func MissingPricingModelForUsage(model *string, usage TokenUsageRaw, costUSD *float64, mode CostMode, pricing *PricingMap) *string {
	if mode == ModeDisplay || (mode == ModeAuto && costUSD != nil) {
		return nil
	}
	return missingPricingModelForTokenTotal(model, TotalUsageTokens(usage), pricing)
}

func missingPricingModelForTokenTotal(model *string, total uint64, pricing *PricingMap) *string {
	if total == 0 {
		return nil
	}
	if model == nil || pricing == nil {
		return nil
	}
	if pricing.Find(*model) != nil {
		return nil
	}
	resolved := ResolveModelName(*model)
	return &resolved
}

func calculateCostFromTokens(model *string, usage TokenUsageRaw, pricing *PricingMap) float64 {
	if model == nil {
		return 0
	}
	found := pricing.Find(*model)
	if found == nil {
		return 0
	}
	multiplier := 1.0
	if usage.Speed != nil && *usage.Speed == SpeedFast {
		multiplier = found.FastMultiplier
	}
	return CalculateCostFromPricing(usage, *found) * multiplier
}

// CalculateCostFromPricing prices one usage object against a model's rates.
func CalculateCostFromPricing(usage TokenUsageRaw, pricing Pricing) float64 {
	cacheCreate5mTokens := usage.CacheCreationInputTokens
	cacheCreate1hTokens := uint64(0)
	if usage.CacheCreation != nil {
		cacheCreate5mTokens = usage.CacheCreation.Ephemeral5mInputTokens
		cacheCreate1hTokens = usage.CacheCreation.Ephemeral1hInputTokens
	}
	cacheCreate1hCost := pricing.Input * cacheCreate1hInputMultiplier
	var cacheCreate1hCostAbove200k *float64
	if pricing.InputAbove200k != nil {
		above := *pricing.InputAbove200k * cacheCreate1hInputMultiplier
		cacheCreate1hCostAbove200k = &above
	}

	// OpenAI two-stage pricing: a per-model LongContextThreshold means the
	// request's input size selects the tier and every bucket is billed
	// entirely at that tier's rate. The whole request switches once input
	// exceeds the threshold, so this is not a marginal breakpoint.
	if pricing.LongContextThreshold != nil {
		threshold := *pricing.LongContextThreshold
		longContext := usage.InputTokens > threshold
		rate := func(base float64, above *float64) float64 {
			if longContext && above != nil {
				return *above
			}
			return base
		}
		return float64(usage.InputTokens)*rate(pricing.Input, pricing.InputAbove200k) +
			float64(usage.OutputTokens)*rate(pricing.Output, pricing.OutputAbove200k) +
			float64(cacheCreate5mTokens)*rate(pricing.CacheCreate, pricing.CacheCreateAbove200k) +
			float64(cacheCreate1hTokens)*rate(cacheCreate1hCost, cacheCreate1hCostAbove200k) +
			float64(usage.CacheReadInputTokens)*rate(pricing.CacheRead, pricing.CacheReadAbove200k)
	}

	// LiteLLM `*_above_200k_tokens` data keeps its marginal above-threshold
	// semantics at the default 200K boundary.
	threshold := DefaultLongContextThresholdTokens
	return TieredCost(usage.InputTokens, pricing.Input, pricing.InputAbove200k, threshold) +
		TieredCost(usage.OutputTokens, pricing.Output, pricing.OutputAbove200k, threshold) +
		TieredCost(cacheCreate5mTokens, pricing.CacheCreate, pricing.CacheCreateAbove200k, threshold) +
		TieredCost(cacheCreate1hTokens, cacheCreate1hCost, cacheCreate1hCostAbove200k, threshold) +
		TieredCost(usage.CacheReadInputTokens, pricing.CacheRead, pricing.CacheReadAbove200k, threshold)
}

// TieredCost prices tokens at base up to threshold, then at the above rate.
func TieredCost(tokens uint64, base float64, above *float64, threshold uint64) float64 {
	if tokens == 0 {
		return 0
	}
	if above != nil && tokens > threshold {
		return float64(threshold)*base + float64(tokens-threshold)**above
	}
	return float64(tokens) * base
}
