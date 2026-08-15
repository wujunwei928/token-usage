package core

// Tests ported from the pricing.rs and model_aliases.rs test modules of the
// reference implementation, keeping their values.

import (
	"strings"
	"testing"
	"time"
)

func mustFind(t *testing.T, m *PricingMap, model string) Pricing {
	t.Helper()
	found := m.Find(model)
	if found == nil {
		t.Fatalf("expected pricing for %q", model)
	}
	return *found
}

func floatEq(t *testing.T, name string, got, want float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 1e-12 {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func optEq(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
	if got != nil {
		floatEq(t, name, *got, *want)
	}
}

func TestLoadsEmbeddedClaudePricing(t *testing.T) {
	pricing := LoadEmbedded()
	if pricing.len() == 0 {
		t.Fatal("embedded pricing is empty")
	}
	if pricing.Find("claude-sonnet-4-20250514") == nil {
		t.Fatal("expected pricing for claude-sonnet-4-20250514")
	}
}

func TestEmbeddedBuildTimePricingIsCompact(t *testing.T) {
	if len(litellmPricingJSON) >= 200_000 {
		t.Fatalf("embedded pricing snapshot is %d bytes", len(litellmPricingJSON))
	}
	if strings.Contains(litellmPricingJSON, "\"source\"") {
		t.Fatal("embedded snapshot still carries source fields")
	}
	if strings.Contains(litellmPricingJSON, "vertex_ai/") {
		t.Fatal("embedded snapshot still carries vertex_ai entries")
	}
	if !strings.Contains(litellmPricingJSON, "claude-opus-4-6") {
		t.Fatal("embedded snapshot is missing claude-opus-4-6")
	}
}

func TestEmbeddedPricingIncludesHermesFrontierModels(t *testing.T) {
	pricing := LoadEmbedded()
	if pricing.Find("gpt-5.5") == nil {
		t.Fatal("expected pricing for gpt-5.5")
	}
	if pricing.Find("grok-4.3") == nil {
		t.Fatal("expected pricing for grok-4.3")
	}
	if limit, ok := pricing.ContextLimit("grok-4.3"); !ok || limit != 1_000_000 {
		t.Fatalf("grok-4.3 context limit = %d,%v", limit, ok)
	}
}

func TestEmbeddedPricingIncludesMoonshotKimiForOfflineReports(t *testing.T) {
	pricing := LoadEmbedded()
	k25 := mustFind(t, pricing, "moonshot/kimi-k2.5")
	floatEq(t, "kimi-k2.5 input", k25.Input, 0.6e-6)
	floatEq(t, "kimi-k2.5 output", k25.Output, 3e-6)
	floatEq(t, "kimi-k2.5 cache read", k25.CacheRead, 0.1e-6)
	if !k25.CacheReadExplicit {
		t.Fatal("kimi-k2.5 cache read should be explicit")
	}
	k26 := mustFind(t, pricing, "moonshot/kimi-k2.6")
	floatEq(t, "kimi-k2.6 input", k26.Input, 0.95e-6)
	floatEq(t, "kimi-k2.6 output", k26.Output, 4e-6)
	floatEq(t, "kimi-k2.6 cache read", k26.CacheRead, 0.16e-6)
	if !k26.CacheReadExplicit {
		t.Fatal("kimi-k2.6 cache read should be explicit")
	}
	if limit, ok := pricing.ContextLimit("moonshot/kimi-k2.5"); !ok || limit != 262_144 {
		t.Fatalf("kimi-k2.5 context limit = %d,%v", limit, ok)
	}
	if limit, ok := pricing.ContextLimit("moonshot/kimi-k2.6"); !ok || limit != 262_144 {
		t.Fatalf("kimi-k2.6 context limit = %d,%v", limit, ok)
	}
}

func TestOfflinePricesKimiK3FromEmbeddedModelsDev(t *testing.T) {
	pricing := LoadEmbedded()
	kimiK3 := pricing.Find("moonshot/kimi-k3")
	if kimiK3 == nil {
		kimiK3 = pricing.Find("kimi-k3")
	}
	if kimiK3 == nil {
		t.Fatal("embedded models.dev should include kimi-k3 pricing")
	}
	floatEq(t, "kimi-k3 input", kimiK3.Input, 3e-6)
	floatEq(t, "kimi-k3 output", kimiK3.Output, 15e-6)
	floatEq(t, "kimi-k3 cache read", kimiK3.CacheRead, 0.3e-6)
	if !kimiK3.CacheReadExplicit {
		t.Fatal("kimi-k3 cache read should be explicit")
	}
	limit, ok := pricing.ContextLimit("moonshot/kimi-k3")
	if !ok {
		limit, ok = pricing.ContextLimit("kimi-k3")
	}
	if !ok || limit != 1_048_576 {
		t.Fatalf("kimi-k3 context limit = %d,%v", limit, ok)
	}
}

func TestOfflinePricesNewAnthropicModelFromEmbeddedModelsDev(t *testing.T) {
	if embeddedModelsDevPricing().findEntry("claude-fable-5") == nil {
		t.Fatal("embedded models.dev snapshot should include claude-fable-5")
	}
	offline := LoadWithOverrides(true, false, nil)
	if offline.Find("claude-fable-5") == nil {
		t.Fatal("offline pricing should resolve claude-fable-5")
	}
}

func TestEmbeddedPricingIncludesZAiGlmModelsForOfflineReports(t *testing.T) {
	pricing := LoadEmbedded()

	glm51 := mustFind(t, pricing, "glm-5.1")
	floatEq(t, "glm-5.1 input", glm51.Input, 1.4e-6)
	floatEq(t, "glm-5.1 output", glm51.Output, 4.4e-6)
	floatEq(t, "glm-5.1 cache create", glm51.CacheCreate, 0)
	floatEq(t, "glm-5.1 cache read", glm51.CacheRead, 0.26e-6)
	if !glm51.CacheReadExplicit {
		t.Fatal("glm-5.1 cache read should be explicit")
	}

	glm5 := mustFind(t, pricing, "glm-5")
	floatEq(t, "glm-5 input", glm5.Input, 1.0e-6)
	floatEq(t, "glm-5 output", glm5.Output, 3.2e-6)
	floatEq(t, "glm-5 cache create", glm5.CacheCreate, 0)
	floatEq(t, "glm-5 cache read", glm5.CacheRead, 0.2e-6)
	if limit, ok := pricing.ContextLimit("zai/glm-5"); !ok || limit != 200_000 {
		t.Fatalf("zai/glm-5 context limit = %d,%v", limit, ok)
	}

	glm5Turbo := mustFind(t, pricing, "glm-5-turbo")
	floatEq(t, "glm-5-turbo input", glm5Turbo.Input, 1.2e-6)
	floatEq(t, "glm-5-turbo output", glm5Turbo.Output, 4.0e-6)
	floatEq(t, "glm-5-turbo cache create", glm5Turbo.CacheCreate, 0)
	floatEq(t, "glm-5-turbo cache read", glm5Turbo.CacheRead, 0.24e-6)

	glm47 := mustFind(t, pricing, "glm-4.7")
	floatEq(t, "glm-4.7 input", glm47.Input, 0.6e-6)
	floatEq(t, "glm-4.7 output", glm47.Output, 2.2e-6)
	floatEq(t, "glm-4.7 cache read", glm47.CacheRead, 0.11e-6)

	glm46 := mustFind(t, pricing, "glm-4.6")
	floatEq(t, "glm-4.6 input", glm46.Input, 0.6e-6)
	floatEq(t, "glm-4.6 output", glm46.Output, 2.2e-6)
	floatEq(t, "glm-4.6 cache read", glm46.CacheRead, 0.11e-6)

	glm45 := mustFind(t, pricing, "glm-4.5")
	floatEq(t, "glm-4.5 input", glm45.Input, 0.6e-6)
	floatEq(t, "glm-4.5 output", glm45.Output, 2.2e-6)
	floatEq(t, "glm-4.5 cache read", glm45.CacheRead, 0.11e-6)

	zaiGlm45 := mustFind(t, pricing, "zai/glm-4.5")
	floatEq(t, "zai/glm-4.5 input", zaiGlm45.Input, 0.6e-6)
	floatEq(t, "zai/glm-4.5 output", zaiGlm45.Output, 2.2e-6)
	floatEq(t, "zai/glm-4.5 cache read", zaiGlm45.CacheRead, 0.11e-6)
	if limit, ok := pricing.ContextLimit("zai/glm-4.5"); !ok || limit != 128_000 {
		t.Fatalf("zai/glm-4.5 context limit = %d,%v", limit, ok)
	}
}

func TestEmbeddedPricingPatchesZAiGlmEntriesWithoutLiteLLMCacheRates(t *testing.T) {
	pricing := LoadEmbedded()

	air := mustFind(t, pricing, "zai/glm-4.5-air")
	floatEq(t, "zai/glm-4.5-air input", air.Input, 0.2e-6)
	floatEq(t, "zai/glm-4.5-air output", air.Output, 1.1e-6)
	floatEq(t, "zai/glm-4.5-air cache create", air.CacheCreate, 0)
	floatEq(t, "zai/glm-4.5-air cache read", air.CacheRead, 0.03e-6)

	x := mustFind(t, pricing, "zai/glm-4.5-x")
	floatEq(t, "zai/glm-4.5-x input", x.Input, 2.2e-6)
	floatEq(t, "zai/glm-4.5-x output", x.Output, 8.9e-6)
	floatEq(t, "zai/glm-4.5-x cache create", x.CacheCreate, 0)
	floatEq(t, "zai/glm-4.5-x cache read", x.CacheRead, 0.45e-6)

	v := mustFind(t, pricing, "zai/glm-4.5v")
	floatEq(t, "zai/glm-4.5v input", v.Input, 0.6e-6)
	floatEq(t, "zai/glm-4.5v output", v.Output, 1.8e-6)
	floatEq(t, "zai/glm-4.5v cache read", v.CacheRead, 0.11e-6)
}

func TestRecordsWhetherCacheReadRateCameFromLiteLLMPricing(t *testing.T) {
	pricing := NewPricingMap()
	loaded := pricing.LoadJSON(`{
        "gpt-with-cache": {
            "input_cost_per_token": 0.000001,
            "output_cost_per_token": 0.000010,
            "cache_read_input_token_cost": 0.0000001
        },
        "gpt-without-cache": {
            "input_cost_per_token": 0.000001,
            "output_cost_per_token": 0.000010
        }
    }`)
	if loaded != 2 {
		t.Fatalf("loaded = %d", loaded)
	}
	if !mustFind(t, pricing, "gpt-with-cache").CacheReadExplicit {
		t.Fatal("explicit cache read should be recorded")
	}
	if mustFind(t, pricing, "gpt-without-cache").CacheReadExplicit {
		t.Fatal("derived cache read should not be recorded as explicit")
	}
}

func TestSkipsInvalidLiteLLMEntriesWithoutDiscardingValidPricing(t *testing.T) {
	pricing := NewPricingMap()
	loaded := pricing.LoadJSON(`{
        "sample_spec": {
            "max_input_tokens": "max input tokens, if the provider specifies it"
        },
        "gpt-valid": {
            "input_cost_per_token": 0.000001,
            "output_cost_per_token": 0.000010,
            "max_input_tokens": 123
        }
    }`)
	if loaded != 1 {
		t.Fatalf("loaded = %d", loaded)
	}
	if pricing.Find("gpt-valid") == nil {
		t.Fatal("expected pricing for gpt-valid")
	}
	if limit, ok := pricing.ContextLimit("gpt-valid"); !ok || limit != 123 {
		t.Fatalf("gpt-valid context limit = %d,%v", limit, ok)
	}
}

func TestLoadsCompactLiteLLMPricingJSON(t *testing.T) {
	pricing := NewPricingMap()
	loaded := pricing.LoadJSON(`{
        "gpt-compact": {
            "i": 0.000001,
            "o": 0.000010,
            "cc": 0.00000125,
            "cr": 0.0000001,
            "ia": 0.000002,
            "oa": 0.000020,
            "cca": 0.0000025,
            "cra": 0.0000002,
            "ctx": 123456,
            "fast": 1.5
        }
    }`)
	if loaded != 1 {
		t.Fatalf("loaded = %d", loaded)
	}
	compact := mustFind(t, pricing, "gpt-compact")
	floatEq(t, "compact input", compact.Input, 1e-6)
	floatEq(t, "compact output", compact.Output, 10e-6)
	floatEq(t, "compact cache create", compact.CacheCreate, 1.25e-6)
	floatEq(t, "compact cache read", compact.CacheRead, 0.1e-6)
	if !compact.CacheReadExplicit {
		t.Fatal("compact cache read should be explicit")
	}
	optEq(t, "compact input above", compact.InputAbove200k, floatPtr(2e-6))
	optEq(t, "compact output above", compact.OutputAbove200k, floatPtr(20e-6))
	optEq(t, "compact cache create above", compact.CacheCreateAbove200k, floatPtr(2.5e-6))
	optEq(t, "compact cache read above", compact.CacheReadAbove200k, floatPtr(0.2e-6))
	floatEq(t, "compact fast", compact.FastMultiplier, 1.5)
	if limit, ok := pricing.ContextLimit("gpt-compact"); !ok || limit != 123456 {
		t.Fatalf("gpt-compact context limit = %d,%v", limit, ok)
	}
}

func TestFallsBackToFullLiteLLMPricingWhenCompactShapeIsIncomplete(t *testing.T) {
	pricing := NewPricingMap()
	loaded := pricing.LoadJSON(`{
        "gpt-full-with-extra-i": {
            "i": "provider metadata",
            "o": "provider metadata",
            "input_cost_per_token": 0.000001,
            "output_cost_per_token": 0.000010
        }
    }`)
	if loaded != 1 {
		t.Fatalf("loaded = %d", loaded)
	}
	full := mustFind(t, pricing, "gpt-full-with-extra-i")
	floatEq(t, "full input", full.Input, 1e-6)
	floatEq(t, "full output", full.Output, 10e-6)
}

func TestKeepsModelsDevFallbackDisabledForEmbeddedAndOfflinePricing(t *testing.T) {
	if LoadEmbedded().modelsDevFallbackEnabled() {
		t.Fatal("embedded pricing must not enable the live models.dev fallback")
	}
	if LoadWithOverrides(true, false, nil).modelsDevFallbackEnabled() {
		t.Fatal("offline pricing must not enable the live models.dev fallback")
	}
}

func TestRetriesModelsDevPricingAfterFetchFailure(t *testing.T) {
	cache := newModelsDevPricingCache(0)

	if failed := cache.getOrTryLoad(func() (string, error) {
		return "", &fakeNetError{"temporary failure"}
	}); failed != nil {
		t.Fatal("expected first attempt to fail")
	}

	pricing := cache.getOrTryLoad(func() (string, error) {
		return `{
            "openai": {
                "id": "openai",
                "name": "OpenAI",
                "models": {
                    "gpt-retry": {
                        "id": "gpt-retry",
                        "name": "GPT Retry",
                        "cost": {"input": 1.0, "output": 2.0},
                        "limit": {"context": 42}
                    }
                }
            }
        }`, nil
	})
	if pricing == nil {
		t.Fatal("models.dev retry should cache successful pricing")
	}
	gptRetry := pricing.findEntry("gpt-retry")
	if gptRetry == nil {
		t.Fatal("successful retry should load pricing")
	}
	floatEq(t, "gpt-retry input", gptRetry.Input, 0.000001)
	floatEq(t, "gpt-retry output", gptRetry.Output, 0.000002)
	if limit, ok := pricing.contextLimitEntry("gpt-retry"); !ok || limit != 42 {
		t.Fatalf("gpt-retry context limit = %d,%v", limit, ok)
	}
}

func TestBacksOffModelsDevPricingAfterFetchFailure(t *testing.T) {
	cache := newModelsDevPricingCache(60 * time.Second)
	attempts := 0

	if failed := cache.getOrTryLoad(func() (string, error) {
		attempts++
		return "", &fakeNetError{"temporary failure"}
	}); failed != nil {
		t.Fatal("expected first attempt to fail")
	}

	if skipped := cache.getOrTryLoad(func() (string, error) {
		attempts++
		return `{"openai": {"models": {"gpt-skipped": {"cost": {"input": 2.0, "output": 8.0}}}}}`, nil
	}); skipped != nil {
		t.Fatal("second attempt should be throttled")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d", attempts)
	}
}

type fakeNetError struct{ msg string }

func (e *fakeNetError) Error() string { return e.msg }

func TestLoadsMissingModelsDevPricingWithoutOverridingLiteLLM(t *testing.T) {
	pricing := NewPricingMap()
	pricing.LoadJSON(`{
        "gpt-primary": {
            "input_cost_per_token": 0.000001,
            "output_cost_per_token": 0.000010,
            "cache_read_input_token_cost": 0.0000001,
            "max_input_tokens": 123
        },
        "openrouter/gpt-alias": {
            "input_cost_per_token": 0.000003,
            "output_cost_per_token": 0.000030,
            "max_input_tokens": 321
        }
    }`)

	modelsDevJSON := `{
        "openai": {
            "id": "openai",
            "name": "OpenAI",
            "models": {
                "gpt-primary": {
                    "id": "gpt-primary",
                    "name": "GPT Primary",
                    "cost": {"input": 9.0, "output": 90.0, "cache_read": 0.9, "cache_write": 11.25},
                    "limit": {"context": 999}
                },
                "gpt-fallback": {
                    "id": "gpt-fallback",
                    "name": "GPT Fallback",
                    "cost": {"input": 2.0, "output": 8.0, "cache_read": 0.2, "cache_write": 2.5},
                    "limit": {"context": 456}
                },
                "gpt-alias": {
                    "id": "gpt-alias",
                    "name": "GPT Alias",
                    "cost": {"input": 4.0, "output": 16.0},
                    "limit": {"context": 654}
                }
            }
        }
    }`

	loaded, ok := pricing.loadModelsDevJSONMissing(modelsDevJSON)
	if !ok || loaded != 2 {
		t.Fatalf("loaded = %d,%v", loaded, ok)
	}

	primary := mustFind(t, pricing, "gpt-primary")
	fallback := mustFind(t, pricing, "gpt-fallback")
	alias := pricing.entries["gpt-alias"]

	floatEq(t, "primary input", primary.Input, 1e-6)
	floatEq(t, "primary output", primary.Output, 10e-6)
	floatEq(t, "primary cache read", primary.CacheRead, 0.1e-6)
	if limit, ok := pricing.ContextLimit("gpt-primary"); !ok || limit != 123 {
		t.Fatalf("gpt-primary context limit = %d,%v", limit, ok)
	}
	floatEq(t, "fallback input", fallback.Input, 2e-6)
	floatEq(t, "fallback output", fallback.Output, 8e-6)
	floatEq(t, "fallback cache create", fallback.CacheCreate, 2.5e-6)
	floatEq(t, "fallback cache read", fallback.CacheRead, 0.2e-6)
	if !fallback.CacheReadExplicit {
		t.Fatal("fallback cache read should be explicit")
	}
	if fallback.InputAbove200k != nil || fallback.OutputAbove200k != nil {
		t.Fatal("models.dev fallback should not carry tier rates")
	}
	floatEq(t, "fallback fast", fallback.FastMultiplier, 1.0)
	if limit, ok := pricing.ContextLimit("gpt-fallback"); !ok || limit != 456 {
		t.Fatalf("gpt-fallback context limit = %d,%v", limit, ok)
	}
	floatEq(t, "alias input", alias.Input, 4e-6)
	if limit := pricing.contextLimits["gpt-alias"]; limit != 654 {
		t.Fatalf("gpt-alias context limit = %d", limit)
	}
}

func TestRejectsMalformedModelsDevProviderPayload(t *testing.T) {
	doc := `{
        "openai": {
            "models": {
                "gpt-fallback": {
                    "cost": {"input": 2.0, "output": 8.0}
                }
            }
        },
        "broken-provider": {
            "name": "Broken Provider"
        }
    }`
	pricing := NewPricingMap()
	if _, ok := pricing.loadModelsDevJSONMissing(doc); ok {
		t.Fatal("malformed providers payload should be rejected")
	}
	if pricing.len() != 0 {
		t.Fatalf("len = %d", pricing.len())
	}
}

func TestLoadsFlatModelsDevPricingSnapshot(t *testing.T) {
	pricing := NewPricingMap()
	doc := `{
        "claude-fallback": {
            "cost": {"input": 3.0, "output": 15.0, "cache_read": 0.3, "cache_write": 3.75},
            "limit": {"context": 200000}
        }
    }`
	loaded, ok := pricing.loadModelsDevJSONMissing(doc)
	if !ok || loaded != 1 {
		t.Fatalf("loaded = %d,%v", loaded, ok)
	}
	fallback := mustFind(t, pricing, "claude-fallback")
	floatEq(t, "fallback input", fallback.Input, 3e-6)
	floatEq(t, "fallback output", fallback.Output, 15e-6)
	floatEq(t, "fallback cache create", fallback.CacheCreate, 3.75e-6)
	floatEq(t, "fallback cache read", fallback.CacheRead, 0.3e-6)
	if limit, ok := pricing.ContextLimit("claude-fallback"); !ok || limit != 200000 {
		t.Fatalf("claude-fallback context limit = %d,%v", limit, ok)
	}
}

func TestEmbeddedModelsDevSnapshotIsParseable(t *testing.T) {
	m := NewPricingMap()
	if _, ok := m.loadModelsDevJSONMissing(modelsDevPricingJSON); !ok {
		t.Fatal("embedded models.dev snapshot must parse")
	}
}

func TestOfflineResolvesModelsOnlyInEmbeddedModelsDev(t *testing.T) {
	offline := LoadWithOverrides(true, false, nil)
	// Pick an embedded model the primary table (LiteLLM + built-ins) cannot
	// resolve on its own. findEntry never consults the fallback.
	var model string
	for candidate := range embeddedModelsDevPricing().entries {
		if offline.findEntry(candidate) == nil {
			model = candidate
			break
		}
	}
	if model == "" {
		t.Skip("embedded models.dev adds no models beyond the primary table")
	}
	// The primary table alone misses it, but the offline embedded fallback
	// resolves it; a bare map without the fallback flag must not.
	if offline.findEntry(model) != nil {
		t.Fatalf("primary table unexpectedly resolves %q", model)
	}
	if offline.Find(model) == nil {
		t.Fatalf("offline pricing should resolve %q", model)
	}
	if NewPricingMap().Find(model) != nil {
		t.Fatalf("bare map must not resolve %q", model)
	}
}

func TestEmbeddedPricingResolvesOverlappingModelKeysExactly(t *testing.T) {
	pricing := LoadEmbedded()
	sonnet4 := mustFind(t, pricing, "claude-sonnet-4-20250514")
	sonnet45 := mustFind(t, pricing, "claude-sonnet-4-5-20250929")

	if got := mustFind(t, pricing, "claude-sonnet-4-20250514").Input; got != sonnet4.Input {
		t.Fatalf("sonnet-4 input = %v", got)
	}
	if got := mustFind(t, pricing, "claude-sonnet-4-5-20250929").Input; got != sonnet45.Input {
		t.Fatalf("sonnet-4-5 input = %v", got)
	}
	if got := mustFind(t, pricing, "anthropic.claude-sonnet-4-20250514-v1:0").Input; got != sonnet4.Input {
		t.Fatalf("bedrock sonnet-4 input = %v", got)
	}
	floatEq(t, "claude-3-5-haiku-20241022 input", mustFind(t, pricing, "claude-3-5-haiku-20241022").Input, 0.8e-6)
}

func TestEmbeddedPricingIncludesGpt55ForOfflineCodexReports(t *testing.T) {
	pricing := LoadEmbedded()
	gpt55 := mustFind(t, pricing, "gpt-5.5")
	floatEq(t, "gpt-5.5 input", gpt55.Input, 5e-6)
	floatEq(t, "gpt-5.5 output", gpt55.Output, 30e-6)
	floatEq(t, "gpt-5.5 cache read", gpt55.CacheRead, 0.5e-6)
	if !gpt55.CacheReadExplicit {
		t.Fatal("gpt-5.5 cache read should be explicit")
	}
	floatEq(t, "gpt-5.5 fast", gpt55.FastMultiplier, 2.5)
	if limit, ok := pricing.ContextLimit("gpt-5.5"); !ok || limit != 1_050_000 {
		t.Fatalf("gpt-5.5 context limit = %d,%v", limit, ok)
	}
}

func TestEmbeddedPricingIncludesGpt56FamilyWithLongContextRates(t *testing.T) {
	pricing := LoadEmbedded()

	sol := mustFind(t, pricing, "gpt-5.6-sol")
	floatEq(t, "sol input", sol.Input, 5e-6)
	floatEq(t, "sol output", sol.Output, 30e-6)
	floatEq(t, "sol cache create", sol.CacheCreate, 6.25e-6)
	floatEq(t, "sol cache read", sol.CacheRead, 0.5e-6)
	if !sol.CacheReadExplicit {
		t.Fatal("sol cache read should be explicit")
	}
	optEq(t, "sol input above", sol.InputAbove200k, floatPtr(10e-6))
	optEq(t, "sol output above", sol.OutputAbove200k, floatPtr(45e-6))
	optEq(t, "sol cache create above", sol.CacheCreateAbove200k, floatPtr(12.5e-6))
	optEq(t, "sol cache read above", sol.CacheReadAbove200k, floatPtr(1e-6))
	if sol.LongContextThreshold == nil || *sol.LongContextThreshold != 272_000 {
		t.Fatalf("sol threshold = %v", sol.LongContextThreshold)
	}
	if limit, ok := pricing.ContextLimit("gpt-5.6-sol"); !ok || limit != 1_050_000 {
		t.Fatalf("sol context limit = %d,%v", limit, ok)
	}

	terra := mustFind(t, pricing, "gpt-5.6-terra")
	floatEq(t, "terra input", terra.Input, 2.5e-6)
	floatEq(t, "terra cache create", terra.CacheCreate, 3.125e-6)
	optEq(t, "terra input above", terra.InputAbove200k, floatPtr(5e-6))
	optEq(t, "terra output above", terra.OutputAbove200k, floatPtr(22.5e-6))

	luna := mustFind(t, pricing, "gpt-5.6-luna")
	floatEq(t, "luna input", luna.Input, 1e-6)
	floatEq(t, "luna output", luna.Output, 6e-6)
	optEq(t, "luna input above", luna.InputAbove200k, floatPtr(2e-6))
	optEq(t, "luna output above", luna.OutputAbove200k, floatPtr(9e-6))
}

func TestGpt56AliasResolvesToSolAcrossPricingMetadata(t *testing.T) {
	pricing := LoadEmbedded()
	alias := mustFind(t, pricing, "gpt-5.6")
	sol := mustFind(t, pricing, "gpt-5.6-sol")

	floatEq(t, "alias input", alias.Input, sol.Input)
	floatEq(t, "alias output", alias.Output, sol.Output)
	floatEq(t, "alias cache create", alias.CacheCreate, sol.CacheCreate)
	floatEq(t, "alias cache read", alias.CacheRead, sol.CacheRead)
	optEq(t, "alias input above", alias.InputAbove200k, sol.InputAbove200k)
	optEq(t, "alias output above", alias.OutputAbove200k, sol.OutputAbove200k)
	aliasLimit, aliasOK := pricing.ContextLimit("gpt-5.6")
	solLimit, solOK := pricing.ContextLimit("gpt-5.6-sol")
	if aliasOK != solOK || aliasLimit != solLimit {
		t.Fatalf("context limits differ: %d,%v vs %d,%v", aliasLimit, aliasOK, solLimit, solOK)
	}
	if got := LongContextSplitThreshold("gpt-5.6"); got != 272_000 {
		t.Fatalf("gpt-5.6 split threshold = %d", got)
	}
}

func TestEmbeddedPricingFillsGptLongContextTierRates(t *testing.T) {
	pricing := LoadEmbedded()

	gpt55 := mustFind(t, pricing, "gpt-5.5")
	optEq(t, "gpt-5.5 input above", gpt55.InputAbove200k, floatPtr(10e-6))
	optEq(t, "gpt-5.5 output above", gpt55.OutputAbove200k, floatPtr(45e-6))
	optEq(t, "gpt-5.5 cache read above", gpt55.CacheReadAbove200k, floatPtr(1e-6))
	if gpt55.LongContextThreshold == nil || *gpt55.LongContextThreshold != 272_000 {
		t.Fatalf("gpt-5.5 threshold = %v", gpt55.LongContextThreshold)
	}

	gpt54 := mustFind(t, pricing, "gpt-5.4")
	optEq(t, "gpt-5.4 input above", gpt54.InputAbove200k, floatPtr(5e-6))
	optEq(t, "gpt-5.4 output above", gpt54.OutputAbove200k, floatPtr(22.5e-6))
	if gpt54.LongContextThreshold == nil || *gpt54.LongContextThreshold != 272_000 {
		t.Fatalf("gpt-5.4 threshold = %v", gpt54.LongContextThreshold)
	}

	// Models the pricing page lists without a long-context tier stay flat.
	mini := mustFind(t, pricing, "gpt-5.4-mini")
	if mini.InputAbove200k != nil {
		t.Fatal("gpt-5.4-mini should stay flat")
	}
	if mini.LongContextThreshold != nil {
		t.Fatal("gpt-5.4-mini should not carry a threshold")
	}
}

func TestLongContextOverlaySurvivesLiteLLMRefreshAndDefersToUpstream(t *testing.T) {
	pricing := LoadEmbedded()
	// A live LiteLLM refresh replaces whole entries with flat rates and may
	// add date-pinned keys.
	pricing.LoadJSON(`{
        "gpt-5.5": {
            "input_cost_per_token": 0.000006,
            "output_cost_per_token": 0.000031
        },
        "gpt-5.5-2026-04-23": {
            "input_cost_per_token": 0.000006,
            "output_cost_per_token": 0.000031
        }
    }`)
	pricing.applyBuiltinLongContextRates()

	gpt55 := mustFind(t, pricing, "gpt-5.5")
	floatEq(t, "gpt-5.5 input", gpt55.Input, 6e-6)
	optEq(t, "gpt-5.5 input above", gpt55.InputAbove200k, floatPtr(10e-6))
	if gpt55.LongContextThreshold == nil || *gpt55.LongContextThreshold != 272_000 {
		t.Fatalf("gpt-5.5 threshold = %v", gpt55.LongContextThreshold)
	}
	// Date-pinned keys share the base model's long-context rates.
	dated := mustFindExact(t, pricing, "gpt-5.5-2026-04-23")
	optEq(t, "dated input above", dated.InputAbove200k, floatPtr(10e-6))

	// Tier rates published upstream win over the built-in overlay.
	pricing.LoadJSON(`{
        "gpt-5.5": {
            "input_cost_per_token": 0.000006,
            "output_cost_per_token": 0.000031,
            "input_cost_per_token_above_200k_tokens": 0.000012
        }
    }`)
	pricing.applyBuiltinLongContextRates()

	gpt55 = mustFind(t, pricing, "gpt-5.5")
	optEq(t, "gpt-5.5 input above", gpt55.InputAbove200k, floatPtr(12e-6))
	if gpt55.LongContextThreshold != nil {
		t.Fatalf("gpt-5.5 threshold = %v", gpt55.LongContextThreshold)
	}
}

func mustFindExact(t *testing.T, m *PricingMap, model string) Pricing {
	t.Helper()
	found := m.FindExact(model)
	if found == nil {
		t.Fatalf("expected exact pricing for %q", model)
	}
	return *found
}

func TestLongContextSplitThresholdIsPerModel(t *testing.T) {
	cases := []struct {
		model string
		want  uint64
	}{
		{"gpt-5.6-sol", 272_000},
		{"gpt-5.5", 272_000},
		{"gpt-5.5-pro", 272_000},
		{"gpt-5.5-2026-04-23", 272_000},
		{"gpt-5", 200_000},
		{"gpt-5.4-mini", 200_000},
	}
	for _, tc := range cases {
		if got := LongContextSplitThreshold(tc.model); got != tc.want {
			t.Fatalf("LongContextSplitThreshold(%q) = %d want %d", tc.model, got, tc.want)
		}
	}
}

func TestStripsModelDateSuffixes(t *testing.T) {
	cases := []struct{ model, want string }{
		{"gpt-5.5-2026-04-23", "gpt-5.5"},
		{"gpt-5.5-pro-2026-04-23", "gpt-5.5-pro"},
		{"claude-3-5-haiku-20241022", "claude-3-5-haiku"},
		{"gpt-5.6-sol", "gpt-5.6-sol"},
		{"gpt-4-0613", "gpt-4-0613"},
	}
	for _, tc := range cases {
		if got := modelWithoutDateSuffix(tc.model); got != tc.want {
			t.Fatalf("modelWithoutDateSuffix(%q) = %q want %q", tc.model, got, tc.want)
		}
	}
}

func TestPricingLookupResolvesModelAliases(t *testing.T) {
	restore := SetModelAliasesForTests(map[string]string{"private-gpt-55": "gpt-5.5"})
	defer restore()

	pricing := LoadEmbedded()
	if got := mustFind(t, pricing, "private-gpt-55").Input; got != mustFind(t, pricing, "gpt-5.5").Input {
		t.Fatalf("aliased input = %v", got)
	}
	if limit, ok := pricing.ContextLimit("private-gpt-55"); !ok || limit != 1_050_000 {
		t.Fatalf("aliased context limit = %d,%v", limit, ok)
	}
}

func TestPricingLookupPrefersKnownOriginalModelBeforeAlias(t *testing.T) {
	restore := SetModelAliasesForTests(map[string]string{"claude-opus-4-8": "mythos-5"})
	defer restore()

	pricing := LoadEmbedded()
	original := mustFindEntry(t, pricing, "claude-opus-4-8")
	resolved := mustFind(t, pricing, "claude-opus-4-8")

	floatEq(t, "resolved input", resolved.Input, original.Input)
	entryLimit, entryOK := pricing.contextLimitEntry("claude-opus-4-8")
	gotLimit, gotOK := pricing.ContextLimit("claude-opus-4-8")
	if entryOK != gotOK || entryLimit != gotLimit {
		t.Fatalf("context limits differ: %d,%v vs %d,%v", entryLimit, entryOK, gotLimit, gotOK)
	}
}

func mustFindEntry(t *testing.T, m *PricingMap, model string) Pricing {
	t.Helper()
	found := m.findEntry(model)
	if found == nil {
		t.Fatalf("expected primary-table pricing for %q", model)
	}
	return *found
}

func TestEmbeddedPricingIncludesCodexPriorityMultiplier(t *testing.T) {
	pricing := LoadEmbedded()
	floatEq(t, "sol fast", mustFind(t, pricing, "gpt-5.6-sol").FastMultiplier, 2.0)
	floatEq(t, "terra fast", mustFind(t, pricing, "gpt-5.6-terra").FastMultiplier, 2.0)
	floatEq(t, "luna fast", mustFind(t, pricing, "gpt-5.6-luna").FastMultiplier, 2.0)
	floatEq(t, "gpt-5.5 fast", mustFind(t, pricing, "gpt-5.5").FastMultiplier, 2.5)
	floatEq(t, "gpt-5.4 fast", mustFind(t, pricing, "gpt-5.4").FastMultiplier, 2.0)
	floatEq(t, "gpt-5.3-codex fast", mustFind(t, pricing, "gpt-5.3-codex").FastMultiplier, 2.0)
}

func TestEmbeddedPricingDoesNotResolveUndatedCodexAutoReviewModel(t *testing.T) {
	pricing := LoadEmbedded()
	if pricing.Find("codex-auto-review") != nil {
		t.Fatal("codex-auto-review should not resolve")
	}
	if _, ok := pricing.ContextLimit("codex-auto-review"); ok {
		t.Fatal("codex-auto-review should not carry a context limit")
	}
}

func TestEmbeddedPricingResolvesCodexSparkShortModelAlias(t *testing.T) {
	pricing := LoadEmbedded()
	shortSpark := mustFind(t, pricing, "gpt-5.3-spark")
	codexSpark := mustFind(t, pricing, "gpt-5.3-codex-spark")
	floatEq(t, "spark input", shortSpark.Input, codexSpark.Input)
	floatEq(t, "spark output", shortSpark.Output, codexSpark.Output)
	floatEq(t, "spark cache read", shortSpark.CacheRead, codexSpark.CacheRead)
	floatEq(t, "spark fast", shortSpark.FastMultiplier, codexSpark.FastMultiplier)
}

func TestEmbeddedPricingIncludesClaudeFastMultiplierForProviderModels(t *testing.T) {
	pricing := LoadEmbedded()
	floatEq(t, "opus-4-6-v1 fast", mustFind(t, pricing, "anthropic.claude-opus-4-6-v1").FastMultiplier, 6.0)
	floatEq(t, "opus-4-7 fast", mustFind(t, pricing, "anthropic.claude-opus-4-7").FastMultiplier, 6.0)
	floatEq(t, "opus-4-8 fast", mustFind(t, pricing, "anthropic.claude-opus-4-8").FastMultiplier, 2.0)
}

func TestEmbeddedPricingResolvesOpus47DotModelNames(t *testing.T) {
	pricing := LoadEmbedded()
	floatEq(t, "opus-4.7-20260416 input", mustFind(t, pricing, "claude-opus-4.7-20260416").Input, 5e-6)
	if limit, ok := pricing.ContextLimit("claude-opus-4.7"); !ok || limit != 1_000_000 {
		t.Fatalf("claude-opus-4.7 context limit = %d,%v", limit, ok)
	}
	floatEq(t, "openrouter opus-4.7 input", mustFind(t, pricing, "openrouter/anthropic/claude-opus-4.7").Input, 5e-6)
}

func TestEmbeddedPricingResolvesOpus48DotModelNames(t *testing.T) {
	pricing := LoadEmbedded()
	opus48 := mustFind(t, pricing, "claude-opus-4.8-20260528")
	floatEq(t, "opus-4.8 input", opus48.Input, 5e-6)
	floatEq(t, "opus-4.8 output", opus48.Output, 25e-6)
	floatEq(t, "opus-4.8 cache create", opus48.CacheCreate, 6.25e-6)
	floatEq(t, "opus-4.8 cache read", opus48.CacheRead, 0.5e-6)
	if limit, ok := pricing.ContextLimit("claude-opus-4.8"); !ok || limit != 1_000_000 {
		t.Fatalf("claude-opus-4.8 context limit = %d,%v", limit, ok)
	}
}

func TestEmbeddedPricingResolvesSeparatorAliasesForOtherClaudeModels(t *testing.T) {
	pricing := LoadEmbedded()
	sonnet46 := mustFind(t, pricing, "claude-sonnet-4-6")
	haiku45 := mustFind(t, pricing, "claude-haiku-4-5")

	floatEq(t, "sonnet-4.6-20260416 input", mustFind(t, pricing, "claude-sonnet-4.6-20260416").Input, sonnet46.Input)
	floatEq(t, "haiku-4.5 input", mustFind(t, pricing, "claude-haiku-4.5").Input, haiku45.Input)
	if a, ok := pricing.ContextLimit("claude-sonnet-4.6"); !ok {
		t.Fatal("claude-sonnet-4.6 context limit missing")
	} else if b, okB := pricing.ContextLimit("claude-sonnet-4-6"); !okB || a != b {
		t.Fatalf("sonnet context limits differ: %d vs %d", a, b)
	}
	if a, ok := pricing.ContextLimit("claude-haiku-4.5"); !ok {
		t.Fatal("claude-haiku-4.5 context limit missing")
	} else if b, okB := pricing.ContextLimit("claude-haiku-4-5"); !okB || a != b {
		t.Fatalf("haiku context limits differ: %d vs %d", a, b)
	}
}

func testPricingEntry(input, output float64) Pricing {
	return Pricing{
		Input: input, Output: output,
		CacheCreate: 0, CacheRead: 0,
		CacheReadExplicit: true, FastMultiplier: 1.0,
	}
}

func TestFuzzyMatchRequiresModelKeyBoundaries(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["claude-opus-4-7"] = testPricingEntry(5e-6, 25e-6)
	pricing.entries["claude-opus-4"] = testPricingEntry(15e-6, 75e-6)

	if pricing.Find("claude-opus-4.70") != nil {
		t.Fatal("claude-opus-4.70 should not fuzzy match")
	}
}

func TestFuzzyMatchDoesNotFallBackAcrossNumericModelVersions(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["claude-opus-4"] = testPricingEntry(15e-6, 75e-6)

	for _, model := range []string{
		"claude-opus-4.8-20260528",
		"claude-opus-4-9",
		"claude-opus-5",
		"claude-opus-4.70",
	} {
		if pricing.Find(model) != nil {
			t.Fatalf("%q should not fuzzy match", model)
		}
	}
	if pricing.Find("claude-opus-4-20250514") == nil {
		t.Fatal("claude-opus-4-20250514 should fuzzy match")
	}
}

func TestFuzzyMatchAllowsDateLikeSuffixesForKnownNumericModelVersions(t *testing.T) {
	pricing := LoadEmbedded()
	if pricing.Find("claude-opus-4-8-20270898") == nil {
		t.Fatal("claude-opus-4-8-20270898 should fuzzy match")
	}
}

func TestFillsCodexFastMultiplierWhenLiteLLMPricingOmitsIt(t *testing.T) {
	pricing := NewPricingMap()
	pricing.LoadJSON(`{
        "gpt-5.5": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000030,
            "cache_read_input_token_cost": 0.0000005
        },
        "gpt-5.4": {
            "input_cost_per_token": 0.0000025,
            "output_cost_per_token": 0.000015,
            "cache_read_input_token_cost": 0.00000025
        },
        "gpt-5.3-codex": {
            "input_cost_per_token": 0.00000175,
            "output_cost_per_token": 0.000014,
            "cache_read_input_token_cost": 0.000000175
        },
        "gpt-5.2-codex": {
            "input_cost_per_token": 0.00000175,
            "output_cost_per_token": 0.000014,
            "cache_read_input_token_cost": 0.000000175
        }
    }`)

	floatEq(t, "gpt-5.5 fast", mustFind(t, pricing, "gpt-5.5").FastMultiplier, 2.5)
	floatEq(t, "gpt-5.4 fast", mustFind(t, pricing, "gpt-5.4").FastMultiplier, 2.0)
	floatEq(t, "gpt-5.3-codex fast", mustFind(t, pricing, "gpt-5.3-codex").FastMultiplier, 2.0)
	floatEq(t, "gpt-5.2-codex fast", mustFind(t, pricing, "gpt-5.2-codex").FastMultiplier, 1.0)
}

func TestFillsClaudeFastMultiplierWhenLiteLLMPricingOmitsIt(t *testing.T) {
	pricing := NewPricingMap()
	pricing.LoadJSON(`{
        "vertex_ai/claude-opus-4-7@default": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000025
        },
        "openrouter/anthropic/claude-opus-4.7": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000025
        },
        "claude-opus-4.7-20260416": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000025
        },
        "claude-opus-4.8-20260528": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000025
        },
        "claude-opus-4-70": {
            "input_cost_per_token": 0.000005,
            "output_cost_per_token": 0.000025
        }
    }`)

	floatEq(t, "vertex fast", mustFind(t, pricing, "vertex_ai/claude-opus-4-7@default").FastMultiplier, 6.0)
	floatEq(t, "openrouter fast", mustFind(t, pricing, "openrouter/anthropic/claude-opus-4.7").FastMultiplier, 6.0)
	floatEq(t, "dated fast", mustFind(t, pricing, "claude-opus-4.7-20260416").FastMultiplier, 6.0)
	floatEq(t, "opus-4.8 fast", mustFind(t, pricing, "claude-opus-4.8-20260528").FastMultiplier, 2.0)
	floatEq(t, "opus-4-70 fast", mustFind(t, pricing, "claude-opus-4-70").FastMultiplier, 1.0)
}

func TestFuzzyMatchPrefersLongestModelKey(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["claude-sonnet-4"] = testPricingEntry(1.0, 0)
	pricing.entries["claude-sonnet-4-20250514"] = testPricingEntry(2.0, 0)

	matched := mustFind(t, pricing, "claude-sonnet-4-20250514-via-bedrock")
	floatEq(t, "matched input", matched.Input, 2.0)
}

// --- overrides (ported from the pricing.rs overrides test module) ---

func buildOverrides(model string, init func(*PricingOverride)) map[string]PricingOverride {
	override := PricingOverride{}
	init(&override)
	return map[string]PricingOverride{model: override}
}

func f64(v float64) *float64 { return &v }

func TestFullOverrideCreatesNewModel(t *testing.T) {
	pricing := NewPricingMap()
	overrides := buildOverrides("custom-model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(1e-6)
		o.OutputCostPerToken = f64(2e-6)
		o.CacheCreationInputTokenCost = f64(3e-6)
		o.CacheReadInputTokenCost = f64(4e-7)
		o.FastMultiplier = f64(2.0)
		o.MaxInputTokens = uint64Ptr(123_456)
	})

	pricing.applyOverrides(overrides)

	entry := mustFind(t, pricing, "custom-model")
	floatEq(t, "input", entry.Input, 1e-6)
	floatEq(t, "output", entry.Output, 2e-6)
	floatEq(t, "cache create", entry.CacheCreate, 3e-6)
	floatEq(t, "cache read", entry.CacheRead, 4e-7)
	if !entry.CacheReadExplicit {
		t.Fatal("cache read should be explicit")
	}
	floatEq(t, "fast", entry.FastMultiplier, 2.0)
	if limit, ok := pricing.ContextLimit("custom-model"); !ok || limit != 123_456 {
		t.Fatalf("context limit = %d,%v", limit, ok)
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }

func TestExactOverrideWinsOverGpt56Alias(t *testing.T) {
	pricing := LoadEmbedded()
	sol := mustFind(t, pricing, "gpt-5.6-sol")
	overrides := buildOverrides("gpt-5.6", func(o *PricingOverride) {
		o.InputCostPerToken = f64(42e-6)
		o.MaxInputTokens = uint64Ptr(654_321)
	})

	pricing.applyOverrides(overrides)

	entry := mustFind(t, pricing, "gpt-5.6")
	floatEq(t, "input", entry.Input, 42e-6)
	floatEq(t, "output", entry.Output, sol.Output)
	floatEq(t, "cache create", entry.CacheCreate, sol.CacheCreate)
	floatEq(t, "cache read", entry.CacheRead, sol.CacheRead)
	optEq(t, "input above", entry.InputAbove200k, sol.InputAbove200k)
	optEq(t, "output above", entry.OutputAbove200k, sol.OutputAbove200k)
	optEq(t, "cache create above", entry.CacheCreateAbove200k, sol.CacheCreateAbove200k)
	optEq(t, "cache read above", entry.CacheReadAbove200k, sol.CacheReadAbove200k)
	if (entry.LongContextThreshold == nil) != (sol.LongContextThreshold == nil) ||
		(entry.LongContextThreshold != nil && *entry.LongContextThreshold != *sol.LongContextThreshold) {
		t.Fatalf("threshold = %v want %v", entry.LongContextThreshold, sol.LongContextThreshold)
	}
	floatEq(t, "fast", entry.FastMultiplier, sol.FastMultiplier)
	if limit, ok := pricing.ContextLimit("gpt-5.6"); !ok || limit != 654_321 {
		t.Fatalf("context limit = %d,%v", limit, ok)
	}
}

func TestPartialOverridePreservesExistingFields(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["existing"] = Pricing{
		Input: 10e-6, Output: 20e-6, CacheCreate: 30e-6, CacheRead: 40e-6,
		CacheReadExplicit: true, InputAbove200k: f64(15e-6), FastMultiplier: 1.5,
	}

	pricing.applyOverrides(buildOverrides("existing", func(o *PricingOverride) {
		o.InputCostPerToken = f64(99e-6)
	}))

	entry := mustFind(t, pricing, "existing")
	floatEq(t, "input", entry.Input, 99e-6)
	floatEq(t, "output", entry.Output, 20e-6)
	floatEq(t, "cache create", entry.CacheCreate, 30e-6)
	floatEq(t, "cache read", entry.CacheRead, 40e-6)
	if !entry.CacheReadExplicit {
		t.Fatal("cache read should stay explicit")
	}
	optEq(t, "input above", entry.InputAbove200k, f64(15e-6))
	floatEq(t, "fast", entry.FastMultiplier, 1.5)
}

func TestOverrideWithoutCacheReadDoesNotSetExplicit(t *testing.T) {
	pricing := NewPricingMap()
	pricing.applyOverrides(buildOverrides("new-model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(1e-6)
	}))

	entry := mustFind(t, pricing, "new-model")
	if entry.CacheReadExplicit {
		t.Fatal("cache read should not be explicit")
	}
	floatEq(t, "cache read", entry.CacheRead, 0)
}

func TestOverrideWithCacheReadSetsExplicit(t *testing.T) {
	pricing := NewPricingMap()
	pricing.applyOverrides(buildOverrides("new-model", func(o *PricingOverride) {
		o.CacheReadInputTokenCost = f64(0)
	}))

	if !mustFind(t, pricing, "new-model").CacheReadExplicit {
		t.Fatal("cache read should be explicit")
	}
}

func TestMaxInputTokensWritesContextLimits(t *testing.T) {
	pricing := NewPricingMap()
	pricing.applyOverrides(buildOverrides("with-limit", func(o *PricingOverride) {
		o.MaxInputTokens = uint64Ptr(2_000_000)
	}))
	if limit, ok := pricing.ContextLimit("with-limit"); !ok || limit != 2_000_000 {
		t.Fatalf("context limit = %d,%v", limit, ok)
	}
}

func TestMissingMaxInputTokensDoesNotClobberExistingLimit(t *testing.T) {
	pricing := NewPricingMap()
	pricing.contextLimits["model"] = 500_000
	pricing.applyOverrides(buildOverrides("model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(1e-6)
	}))
	if limit, ok := pricing.ContextLimit("model"); !ok || limit != 500_000 {
		t.Fatalf("context limit = %d,%v", limit, ok)
	}
}

func TestInputOverrideScalesCacheProportionally(t *testing.T) {
	pricing := NewPricingMap()
	// Base: input=3e-6, cache_read=3e-7 (0.1x), cache_create=3.75e-6 (1.25x).
	// CacheReadExplicit=false means these were derived from input by LiteLLM.
	pricing.entries["claude-model"] = Pricing{
		Input: 3e-6, Output: 15e-6, CacheCreate: 3.75e-6, CacheRead: 3e-7,
		CacheReadExplicit:    false,
		CacheCreateAbove200k: f64(4.6875e-6),
		CacheReadAbove200k:   f64(3.75e-7),
		FastMultiplier:       1.0,
	}

	// Override input to 2e-6 (2/3 of original), don't touch cache.
	pricing.applyOverrides(buildOverrides("claude-model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(2e-6)
	}))

	entry := mustFind(t, pricing, "claude-model")
	floatEq(t, "input", entry.Input, 2e-6)
	floatEq(t, "output", entry.Output, 15e-6)
	floatEq(t, "cache create", entry.CacheCreate, 2.5e-6)
	floatEq(t, "cache read", entry.CacheRead, 2e-7)
	floatEq(t, "cache create above", *entry.CacheCreateAbove200k, 3.125e-6)
	floatEq(t, "cache read above", *entry.CacheReadAbove200k, 2.5e-7)
}

func TestInputOverrideDoesNotScaleZeroCache(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["no-cache-model"] = Pricing{
		Input: 5e-6, Output: 10e-6, CacheCreate: 0, CacheRead: 0,
		CacheReadExplicit: false, FastMultiplier: 1.0,
	}

	pricing.applyOverrides(buildOverrides("no-cache-model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(2e-6)
	}))

	entry := mustFind(t, pricing, "no-cache-model")
	floatEq(t, "cache create", entry.CacheCreate, 0)
	floatEq(t, "cache read", entry.CacheRead, 0)
}

func TestExplicitCacheOverrideTakesPrecedenceOverScaling(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["model"] = Pricing{
		Input: 3e-6, Output: 15e-6, CacheCreate: 3.75e-6, CacheRead: 3e-7,
		CacheReadExplicit: false, FastMultiplier: 1.0,
	}

	pricing.applyOverrides(buildOverrides("model", func(o *PricingOverride) {
		o.InputCostPerToken = f64(2e-6)
		o.CacheReadInputTokenCost = f64(5e-7)
	}))

	entry := mustFind(t, pricing, "model")
	floatEq(t, "input", entry.Input, 2e-6)
	floatEq(t, "cache read", entry.CacheRead, 5e-7)
	floatEq(t, "cache create", entry.CacheCreate, 2.5e-6)
}

// --- model aliases (ported from model_aliases.rs tests) ---

func TestParsesDelimitedModelAliases(t *testing.T) {
	aliases := parseModelAliases(" private-alpha = gpt-5.5, other-alpha=claude-sonnet-4 ")
	if aliases["private-alpha"] != "gpt-5.5" {
		t.Fatalf("private-alpha = %q", aliases["private-alpha"])
	}
	if aliases["other-alpha"] != "claude-sonnet-4" {
		t.Fatalf("other-alpha = %q", aliases["other-alpha"])
	}
}

func TestParsesJSONModelAliases(t *testing.T) {
	aliases := parseModelAliases(`{"private-alpha":"gpt-5.5"}`)
	if aliases["private-alpha"] != "gpt-5.5" {
		t.Fatalf("private-alpha = %q", aliases["private-alpha"])
	}
}

func TestFallsBackToDelimitedParsingWhenJSONIsMalformed(t *testing.T) {
	aliases := parseModelAliases(`{private-alpha=gpt-5.5, other-alpha=claude-sonnet-4}`)
	if aliases["private-alpha"] != "gpt-5.5" {
		t.Fatalf("private-alpha = %q", aliases["private-alpha"])
	}
	if aliases["other-alpha"] != "claude-sonnet-4" {
		t.Fatalf("other-alpha = %q", aliases["other-alpha"])
	}
}

func TestResolvesConfiguredModelAlias(t *testing.T) {
	restore := SetModelAliasesForTests(map[string]string{"private-alpha": "gpt-5.5"})
	defer restore()

	if got := ResolveModelName("private-alpha"); got != "gpt-5.5" {
		t.Fatalf("ResolveModelName(private-alpha) = %q", got)
	}
	if got := ResolveModelName("private-alpha-fast"); got != "gpt-5.5-fast" {
		t.Fatalf("ResolveModelName(private-alpha-fast) = %q", got)
	}
	if got := ResolveModelName("gpt-5"); got != "gpt-5" {
		t.Fatalf("ResolveModelName(gpt-5) = %q", got)
	}
}

// --- resolution chain ordering (Go-side end-to-end checks) ---

func TestResolutionChainOrderExactBeforeAliasBeforeFuzzy(t *testing.T) {
	pricing := NewPricingMap()
	pricing.entries["model-x"] = testPricingEntry(1, 1)
	pricing.entries["model-x-longer"] = testPricingEntry(2, 2)

	restore := SetModelAliasesForTests(map[string]string{
		// An exact hit wins over the configured alias mapping.
		"model-x": "model-x-longer",
		// The alias resolves when the original name misses.
		"model-y": "model-x-longer",
	})
	defer restore()

	floatEq(t, "exact wins", mustFind(t, pricing, "model-x").Input, 1)
	floatEq(t, "alias resolves", mustFind(t, pricing, "model-y").Input, 2)
	// Fuzzy suffix matching fills names the table does not carry verbatim.
	floatEq(t, "fuzzy resolves", mustFind(t, pricing, "model-x-via-bedrock").Input, 1)
	// Names the table and every fallback misses stay unresolved.
	if pricing.Find("model-z") != nil {
		t.Fatal("model-z should not resolve")
	}
}

func TestFastSuffixLookupResolvesAndMultipliesCost(t *testing.T) {
	pricing := LoadEmbedded()
	// The loader suffixes -fast onto fast-speed models; the pricing table has
	// no explicit -fast keys, so resolution goes through fuzzy matching.
	fast := mustFind(t, pricing, "claude-opus-4-8-fast")
	floatEq(t, "fast multiplier", fast.FastMultiplier, 2.0)
	floatEq(t, "fast input", fast.Input, 5e-6)

	speed := SpeedFast
	model := "claude-opus-4-8"
	usage := TokenUsageRaw{InputTokens: 1000, Speed: &speed}
	cost := CalculateCostForUsage(&model, usage, nil, ModeCalculate, pricing)
	floatEq(t, "fast cost", cost, 1000*5e-6*2.0)
}
