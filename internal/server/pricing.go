package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// Pricing is the server-side Cost Estimate engine: the CLI domain's embedded
// seed table (LiteLLM snapshot + built-ins + models.dev fallback) optionally
// overridden by a model-prices.json file, plus the model-family fallback that
// estimates unknown models from their family prefix (marked estimated).
//
// Cost semantics are shared with the CLI domain: input/output/cache-read unit
// rates, cache write 5m at the cache-create rate, cache write 1h at 2x input,
// the >200K tiering, and OpenAI long-context tier switching.

// PriceSource labels where a model's rates came from.
type PriceSource string

// Price sources.
const (
	SourceOverride PriceSource = "override"  // model-prices.json entry
	SourceSeed     PriceSource = "official"  // embedded LiteLLM/built-in snapshot
	SourceLive     PriceSource = "live"      // models.dev catalog fetched at startup
	SourceFamily   PriceSource = "estimated" // family-prefix estimate
)

// ModelPrice is the per-model rate card used for display and costing.
type ModelPrice struct {
	Model        string      `json:"model"`
	Input        float64     `json:"input"`        // USD per 1M input tokens
	Output       float64     `json:"output"`       // USD per 1M output tokens
	CacheRead    float64     `json:"cacheRead"`    // USD per 1M cache-read tokens
	CacheWrite5m float64     `json:"cacheWrite5m"` // USD per 1M 5m cache-write tokens
	CacheWrite1h float64     `json:"cacheWrite1h"` // USD per 1M 1h cache-write tokens
	Source       PriceSource `json:"source"`
}

// rateEntry is one override-row in model-prices.json (per-1M-token USD).
type rateEntry struct {
	Input        *float64 `json:"input"`
	Output       *float64 `json:"output"`
	CacheRead    *float64 `json:"cacheRead"`
	CacheWrite5m *float64 `json:"cacheWrite5m"`
	CacheWrite1h *float64 `json:"cacheWrite1h"`
}

type overrideDoc struct {
	Models map[string]rateEntry `json:"models"`
}

// PricingTable resolves model names to rates and computes Cost Estimates.
type PricingTable struct {
	seed      *core.PricingMap
	overrides map[string]ModelPrice
	// live is the optional models.dev catalog tier, consulted between the
	// seed and the family fallback. nil means the tier is disabled (offline
	// or unreachable), keeping request paths free of network stalls.
	live func() *core.PricingMap
}

// EnableLiveModelsDev arms the live models.dev tier. With warm=true it
// fetches once immediately: on success later lookups hit the cached catalog;
// on failure the tier is disabled for the process so request handlers never
// stall on the network. Returns whether the catalog is live.
func (t *PricingTable) EnableLiveModelsDev(warm bool) bool {
	t.live = core.LiveModelsDevPricing
	if warm && t.live() == nil {
		t.live = nil
		return false
	}
	return true
}

// ConfigPricingPath returns the ADR 0007-namespaced pricing override file
// and whether it exists; absence is the normal no-override case, so callers
// only pass the path to LoadPricing when it is there.
func ConfigPricingPath() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(dir, "token-usage", "model-prices.json")
	if _, err := os.Stat(path); err != nil {
		return path, false
	}
	return path, true
}

// LoadPricing builds the server pricing table. overridePath may be empty; a
// missing or malformed override file falls back to the seed table with a
// warning to stderr (a bad price file must not take the server down).
func LoadPricing(overridePath string) (*PricingTable, error) {
	t := &PricingTable{
		seed:      core.LoadEmbedded(),
		overrides: map[string]ModelPrice{},
	}
	if overridePath == "" {
		return t, nil
	}
	raw, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[pricing] override file %s not found; using embedded seed\n", overridePath)
			return t, nil
		}
		return nil, err
	}
	var doc overrideDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "[pricing] ignoring malformed %s: %v\n", overridePath, err)
		return t, nil
	}
	for model, entry := range doc.Models {
		base := ModelPrice{Model: model, Source: SourceOverride}
		if seed := t.seed.Find(model); seed != nil {
			base = pricingToCard(model, seed, SourceOverride)
		}
		if entry.Input != nil {
			base.Input = *entry.Input
		}
		if entry.Output != nil {
			base.Output = *entry.Output
		}
		if entry.CacheRead != nil {
			base.CacheRead = *entry.CacheRead
		}
		if entry.CacheWrite5m != nil {
			base.CacheWrite5m = *entry.CacheWrite5m
		}
		if entry.CacheWrite1h != nil {
			base.CacheWrite1h = *entry.CacheWrite1h
		}
		t.overrides[model] = base
	}
	return t, nil
}

// pricingToCard converts a core Pricing entry (per-token rates) into the
// per-1M display card, deriving the 1h write rate the way costs do (2x input).
func pricingToCard(model string, p *core.Pricing, source PriceSource) ModelPrice {
	return ModelPrice{
		Model:        model,
		Input:        p.Input * 1e6,
		Output:       p.Output * 1e6,
		CacheRead:    p.CacheRead * 1e6,
		CacheWrite5m: p.CacheCreate * 1e6,
		CacheWrite1h: p.Input * 1e6 * 2,
		Source:       source,
	}
}

// resolve returns the rate card for a model: override first, then the seed
// table's fuzzy match, then the family-prefix estimate.
func (t *PricingTable) resolve(model string) (ModelPrice, bool) {
	if card, ok := t.overrides[model]; ok {
		return card, true
	}
	if p := t.seed.Find(model); p != nil {
		return pricingToCard(model, p, SourceSeed), true
	}
	// Live models.dev tier: startup-fetched catalog entries win over family
	// estimates (published rates beat derived ones) but never over the
	// embedded snapshot or the user's overrides.
	if t.live != nil {
		if p := t.live().Find(model); p != nil {
			return pricingToCard(model, p, SourceLive), true
		}
	}
	// Family fallback: strip trailing '-'-segments (newest-version suffixes
	// first) and retry the fuzzy chain on each ancestor name. Ancestor cards
	// with $0 cache rates are unusable for cache-heavy traffic (a whole day of
	// reads would price at zero), so zero cache rates are derived from the
	// input price instead: read 0.1×, write 5m 1.25×, write 1h 2× (the last
	// matches pricingToCard's standing derivation).
	if model != "" {
		parts := strings.Split(model, "-")
		for end := len(parts) - 1; end >= 1; end-- {
			family := strings.Join(parts[:end], "-")
			if p := t.seed.Find(family); p != nil {
				card := pricingToCard(model, p, SourceFamily)
				if card.CacheRead == 0 && card.Input > 0 {
					card.CacheRead = 0.1 * card.Input
				}
				if card.CacheWrite5m == 0 && card.Input > 0 {
					card.CacheWrite5m = 1.25 * card.Input
				}
				return card, true
			}
		}
	}
	return ModelPrice{}, false
}

// Exists reports whether any rate card (exact, fuzzy, or family) is known.
func (t *PricingTable) Exists(model string) bool {
	_, ok := t.resolve(model)
	return ok
}

// Estimated reports whether the model's cost is a family estimate.
func (t *PricingTable) Estimated(model string) bool {
	card, ok := t.resolve(model)
	return ok && card.Source == SourceFamily
}

// CostForHourRow prices one Hourly Usage cell. Unknown models cost 0 (the
// missing-pricing signal travels separately via Exists).
func (t *PricingTable) CostForHourRow(r *HourRow) float64 {
	card, ok := t.resolve(r.Model)
	if !ok {
		return 0
	}
	return (float64(r.Input)*card.Input +
		float64(r.Output)*card.Output +
		float64(r.CacheRead)*card.CacheRead +
		float64(r.Cache5m)*card.CacheWrite5m +
		float64(r.Cache1h)*card.CacheWrite1h) / 1e6
}

// CacheSavingsForHourRow prices one cell's Cache Savings: what the cache
// reads saved by billing at the read rate instead of the input rate, minus
// the write premium paid to populate the cache. Unknown models contribute 0
// (no rate card, no honest estimate).
func (t *PricingTable) CacheSavingsForHourRow(r *HourRow) float64 {
	card, ok := t.resolve(r.Model)
	if !ok {
		return 0
	}
	saved := float64(r.CacheRead) * (card.Input - card.CacheRead)
	premium := float64(r.Cache5m)*(card.CacheWrite5m-card.Input) +
		float64(r.Cache1h)*(card.CacheWrite1h-card.Input)
	return (saved - premium) / 1e6
}

// Card returns the display card for a model.
func (t *PricingTable) Card(model string) (ModelPrice, bool) {
	return t.resolve(model)
}

// Roster lists the rate cards for the pricing page: every override plus every
// seed model, overrides winning key collisions, sorted by model name.
func (t *PricingTable) Roster() []ModelPrice {
	seen := map[string]bool{}
	out := make([]ModelPrice, 0, 64)
	for _, model := range t.seed.Models() {
		if card, ok := t.resolve(model); ok && card.Source != SourceFamily {
			out = append(out, card)
			seen[model] = true
		}
	}
	var overrideKeys []string
	for model := range t.overrides {
		overrideKeys = append(overrideKeys, model)
	}
	sort.Strings(overrideKeys)
	for _, model := range overrideKeys {
		if !seen[model] {
			out = append(out, t.overrides[model])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// DumpPricing writes the current effective table to path as model-prices.json
// (per-1M rates), so operators can edit a baseline and feed it back via the
// -pricing flag.
func DumpPricing(overridePath, path string) error {
	table, err := LoadPricing(overridePath)
	if err != nil {
		return err
	}
	doc := map[string]any{"models": map[string]any{}}
	models := doc["models"].(map[string]any)
	for _, card := range table.Roster() {
		models[card.Model] = map[string]any{
			"input":        card.Input,
			"output":       card.Output,
			"cacheRead":    card.CacheRead,
			"cacheWrite5m": card.CacheWrite5m,
			"cacheWrite1h": card.CacheWrite1h,
		}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
