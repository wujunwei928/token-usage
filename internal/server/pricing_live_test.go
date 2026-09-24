package server

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

func liveTestTable(t *testing.T, doc string) *PricingTable {
	t.Helper()
	table, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	m := core.NewPricingMap()
	if m.LoadJSON(doc) == 0 {
		t.Fatal("live fixture must load")
	}
	table.live = func() *core.PricingMap { return m }
	return table
}

// Ticket 09: the live models.dev tier sits between the embedded seed and the
// family fallback — published catalog rates beat derived estimates, never the
// seed snapshot or the user's overrides.
func TestLiveModelsDevResolveTier(t *testing.T) {
	table := liveTestTable(t, `{
		"glm-5.3": {
			"input_cost_per_token": 0.0000014,
			"output_cost_per_token": 0.0000044,
			"cache_read_input_token_cost": 0.00000026
		},
		"glm-4.7": {
			"input_cost_per_token": 0.0000009,
			"output_cost_per_token": 0.0000009,
			"cache_read_input_token_cost": 0.0000009
		}
	}`)

	// Unknown to the seed, known to the live catalog → SourceLive, per-1M rates.
	card, ok := table.resolve("glm-5.3")
	if !ok || card.Source != SourceLive {
		t.Fatalf("glm-5.3 must resolve via the live tier: %+v ok=%v", card, ok)
	}
	if card.Input != 1.4 || card.Output != 4.4 || card.CacheRead != 0.26 {
		t.Fatalf("live rates wrong (want 1.4/4.4/0.26): %+v", card)
	}
	if table.Estimated("glm-5.3") {
		t.Fatal("live catalog rates are published prices, not family estimates")
	}

	// The embedded seed wins over the live catalog for models it knows.
	card, _ = table.resolve("glm-4.7")
	if card.Source != SourceSeed || card.Input != 0.6 || card.CacheRead != 0.11 {
		t.Fatalf("seed snapshot must beat the live catalog: %+v", card)
	}

	// The user's override file beats everything, live included.
	table.overrides["glm-5.3"] = ModelPrice{Model: "glm-5.3", Source: SourceOverride,
		Input: 1.0, Output: 2.0, CacheRead: 0.3}
	card, _ = table.resolve("glm-5.3")
	if card.Source != SourceOverride || card.Input != 1.0 {
		t.Fatalf("override must beat the live tier: %+v", card)
	}

	// Models missing from every tier still fall through to the family
	// estimate with derived cache rates.
	card, ok = table.resolve("glm-9.9-unheard-of")
	if !ok || card.Source != SourceFamily || card.CacheRead == 0 {
		t.Fatalf("family fallback must stay last and derive rates: %+v", card)
	}
}

// EnableLiveModelsDev(warm=true) with no HTTP client installed (test process,
// or an unreachable network) must disable the tier instead of letting request
// handlers stall on retries.
func TestEnableLiveModelsDevDisablesOnWarmFailure(t *testing.T) {
	table, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	if table.EnableLiveModelsDev(true) {
		t.Fatal("warm fetch must fail without an HTTP client")
	}
	if table.live != nil {
		t.Fatal("failed warm fetch must disable the live tier")
	}
	if _, ok := table.resolve("glm-5.3"); !ok {
		t.Fatal("resolve must still work (seed/family) with the tier disabled")
	}
}
