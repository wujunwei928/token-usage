package codex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// Speed is the CLI --speed vocabulary.
type Speed int

// Speed choices.
const (
	SpeedAuto Speed = iota
	SpeedStandard
	SpeedFast
)

// ParseSpeed maps the CLI string onto a Speed; ok is false for unknown.
func ParseSpeed(value string) (Speed, bool) {
	switch value {
	case "auto":
		return SpeedAuto, true
	case "standard":
		return SpeedStandard, true
	case "fast":
		return SpeedFast, true
	}
	return SpeedAuto, false
}

// SpeedPolicy is the resolved pricing policy: auto keeps recorded tiers with a
// config fallback for unclassified usage, forced applies one tier everywhere.
type SpeedPolicy struct {
	auto   bool
	fast   bool
	forced bool
}

// PolicyForced builds a forced policy for the given tier.
func PolicyForced(tier ServiceTier) SpeedPolicy {
	return SpeedPolicy{auto: false, fast: tier == TierFast, forced: true}
}

// PolicyAuto builds an auto policy with the fallback tier for unclassified
// usage.
func PolicyAuto(tier ServiceTier) SpeedPolicy {
	return SpeedPolicy{auto: true, fast: tier == TierFast, forced: false}
}

// ResolveSpeed maps the requested speed onto the pricing policy, consulting
// config.toml for the auto tier.
func ResolveSpeed(requested Speed) SpeedPolicy {
	switch requested {
	case SpeedStandard:
		return PolicyForced(TierStandard)
	case SpeedFast:
		return PolicyForced(TierFast)
	default:
		if detectCodexFastServiceTier() {
			return PolicyAuto(TierFast)
		}
		return PolicyAuto(TierStandard)
	}
}

func detectCodexFastServiceTier() bool {
	homes, err := CodexHomePaths()
	if err != nil {
		return false
	}
	for _, home := range homes {
		content, err := os.ReadFile(filepath.Join(home, "config.toml"))
		if err == nil && codexConfigRequestsFastServiceTier(string(content)) {
			return true
		}
	}
	return false
}

func codexConfigRequestsFastServiceTier(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		setting := line
		if index := strings.Index(setting, "#"); index >= 0 {
			setting = setting[:index]
		}
		setting = strings.TrimSpace(setting)
		key, value, found := strings.Cut(setting, "=")
		if !found || strings.TrimSpace(key) != "service_tier" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if value == "fast" || value == "priority" {
			return true
		}
	}
	return false
}

// NonCachedInputTokens returns the input tokens that were not served from
// cache.
func NonCachedInputTokens(inputTokens, cachedInputTokens uint64) uint64 {
	if cachedInputTokens >= inputTokens {
		return 0
	}
	return inputTokens - cachedInputTokens
}

// CalculateModelCost prices one model's usage under the speed policy,
// splitting long-context requests into their own bucket.
func CalculateModelCost(model string, usage *ModelUsage, pricing *core.PricingMap, speed SpeedPolicy) float64 {
	entry := pricing.Find(model)
	if entry == nil {
		return 0
	}
	totalUsage := modelUsageBucket(usage)
	standardCost := calculateBucketCost(totalUsage, entry)
	var fastUsage UsageBucket
	switch {
	case speed.auto && !speed.fast:
		fastUsage = usage.RecordedFastUsage
	case speed.auto && speed.fast:
		fastUsage = subtractBucket(totalUsage, usage.RecordedStandardUsage)
	case !speed.auto && !speed.fast:
		return standardCost
	default:
		fastUsage = totalUsage
	}
	return standardCost + calculateBucketCost(fastUsage, entry)*(entry.FastMultiplier-1.0)
}

func modelUsageBucket(usage *ModelUsage) UsageBucket {
	return UsageBucket{
		InputTokens:                  usage.InputTokens,
		CachedInputTokens:            usage.CachedInputTokens,
		OutputTokens:                 usage.OutputTokens,
		LongContextInputTokens:       usage.LongContextInputTokens,
		LongContextCachedInputTokens: usage.LongContextCachedInputTokens,
		LongContextOutputTokens:      usage.LongContextOutputTokens,
	}
}

func subtractBucket(total, excluded UsageBucket) UsageBucket {
	sub := func(a, b uint64) uint64 {
		if b >= a {
			return 0
		}
		return a - b
	}
	return UsageBucket{
		InputTokens:                  sub(total.InputTokens, excluded.InputTokens),
		CachedInputTokens:            sub(total.CachedInputTokens, excluded.CachedInputTokens),
		OutputTokens:                 sub(total.OutputTokens, excluded.OutputTokens),
		LongContextInputTokens:       sub(total.LongContextInputTokens, excluded.LongContextInputTokens),
		LongContextCachedInputTokens: sub(total.LongContextCachedInputTokens, excluded.LongContextCachedInputTokens),
		LongContextOutputTokens:      sub(total.LongContextOutputTokens, excluded.LongContextOutputTokens),
	}
}

func calculateBucketCost(usage UsageBucket, pricing *core.Pricing) float64 {
	cacheRead := pricing.CacheRead
	if !pricing.CacheReadExplicit {
		cacheRead = pricing.Input
	}
	// OpenAI bills every token of a long-context request (input above the
	// tier threshold) at the long-context rates, so the aggregated usage is
	// priced as two independent buckets; models without tier rates fall back
	// to the flat rates, keeping both buckets at the same price.
	longInputRate := pricing.Input
	if pricing.InputAbove200k != nil {
		longInputRate = *pricing.InputAbove200k
	}
	longOutputRate := pricing.Output
	if pricing.OutputAbove200k != nil {
		longOutputRate = *pricing.OutputAbove200k
	}
	longCacheRead := longInputRate
	if pricing.CacheReadExplicit {
		longCacheRead = cacheRead
		if pricing.CacheReadAbove200k != nil {
			longCacheRead = *pricing.CacheReadAbove200k
		}
	}
	min := func(a, b uint64) uint64 {
		if a < b {
			return a
		}
		return b
	}
	sub := func(a, b uint64) uint64 {
		if b >= a {
			return 0
		}
		return a - b
	}
	longInput := min(usage.LongContextInputTokens, usage.InputTokens)
	longCached := min(min(usage.LongContextCachedInputTokens, usage.CachedInputTokens), longInput)
	longOutput := min(usage.LongContextOutputTokens, usage.OutputTokens)
	shortNonCached := sub(sub(usage.InputTokens, longInput), sub(usage.CachedInputTokens, longCached))
	longNonCached := sub(longInput, longCached)
	return float64(shortNonCached)*pricing.Input +
		float64(sub(usage.CachedInputTokens, longCached))*cacheRead +
		float64(sub(usage.OutputTokens, longOutput))*pricing.Output +
		float64(longNonCached)*longInputRate +
		float64(longCached)*longCacheRead +
		float64(longOutput)*longOutputRate
}

// CalculateGroupCost prices every model in the group.
func CalculateGroupCost(group *Group, pricing *core.PricingMap, speed SpeedPolicy) float64 {
	cost := 0.0
	for model, usage := range group.Models {
		cost += CalculateModelCost(model, usage, pricing, speed)
	}
	return cost
}

// ModelMissingPricing reports whether a model with recorded tokens has no
// pricing entry.
func ModelMissingPricing(model string, usage *ModelUsage, pricing *core.PricingMap) bool {
	total := usage.TotalTokens
	if derived := usage.InputTokens + usage.OutputTokens; derived > total {
		total = derived
	}
	if total == 0 {
		return false
	}
	return pricing.Find(model) == nil
}

// MissingPricingModels collects the models across groups that lack pricing,
// deduped and sorted.
func MissingPricingModels(groups *Groups, pricing *core.PricingMap) []string {
	set := map[string]struct{}{}
	for _, entry := range groups.Sorted() {
		for model, usage := range entry.Group.Models {
			if ModelMissingPricing(model, usage, pricing) {
				set[model] = struct{}{}
			}
		}
	}
	models := make([]string, 0, len(set))
	for model := range set {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
