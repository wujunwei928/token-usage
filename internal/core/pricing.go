package core

// Pricing system ported from rust/crates/ccusage-core/src/pricing.rs:
// the model price table (LiteLLM snapshot + built-ins + models.dev
// fallbacks), the fuzzy model resolution chain, the live-refresh fetch
// plumbing, and the built-in long-context tier overlay.
//
// The embedded snapshots in pricingdata/ come from the reference repository:
// litellm-pricing.json is the LiteLLM model_prices_and_context_window.json
// pinned by the reference flake.lock (rev f99d0a4b389c6142977c21f4d7e5d9bf9a051c8f),
// compacted exactly like the reference build.rs (prefix-filtered models,
// renamed fields, sorted keys); models-dev-pricing.json and
// fast-multiplier-overrides.json are verbatim copies.

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed pricingdata/litellm-pricing.json
var litellmPricingJSON string

//go:embed pricingdata/models-dev-pricing.json
var modelsDevPricingJSON string

//go:embed pricingdata/fast-multiplier-overrides.json
var fastMultiplierOverridesJSON string

const (
	litellmPricingURL          = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	modelsDevAPIURL            = "https://models.dev/api.json"
	modelsDevFailureRetryAfter = 60 * time.Second
	// Anthropic date-suffixed model aliases use YYYYMMDD, while other numeric
	// suffixes are treated as distinct model versions.
	modelDateSuffixDigits = 8
)

// DefaultLongContextThresholdTokens is the default tier boundary for LiteLLM
// `*_above_200k_tokens` pricing fields.
const DefaultLongContextThresholdTokens uint64 = 200_000

// openAILongContextThresholdTokens is OpenAI's long-context pricing boundary:
// requests with more than 272K input tokens (GPT-5's maximum short-context
// input size) are billed at long-context rates.
const openAILongContextThresholdTokens uint64 = 272_000

// Pricing holds per-token USD rates for one model.
type Pricing struct {
	Input                float64
	Output               float64
	CacheCreate          float64
	CacheRead            float64
	CacheReadExplicit    bool
	InputAbove200k       *float64
	OutputAbove200k      *float64
	CacheCreateAbove200k *float64
	CacheReadAbove200k   *float64
	// LongContextThreshold is the token count above which the `*Above200k`
	// rates apply. The field names keep the LiteLLM `_above_200k_tokens`
	// suffix for JSON compatibility, but some providers switch tiers at a
	// different point (OpenAI long-context pricing starts above 272K input
	// tokens), so the threshold is per model.
	LongContextThreshold *uint64
	FastMultiplier       float64
}

func emptyPricing() Pricing {
	return Pricing{FastMultiplier: 1.0}
}

// PricingMap resolves model names to Pricing entries.
type PricingMap struct {
	entries                         map[string]Pricing
	contextLimits                   map[string]uint64
	enableModelsDevFallback         bool
	enableEmbeddedModelsDevFallback bool

	findMu    sync.Mutex
	findCache map[string]*Pricing
}

// NewPricingMap returns an empty map.
func NewPricingMap() *PricingMap {
	return &PricingMap{
		entries:       map[string]Pricing{},
		contextLimits: map[string]uint64{},
	}
}

// LoadEmbedded builds the offline pricing table: the embedded LiteLLM
// snapshot, the built-in model table, and the embedded models.dev fallback.
func LoadEmbedded() *PricingMap {
	m := NewPricingMap()
	fastOverrides := loadFastMultiplierOverrides()
	m.loadJSONWithOverrides(litellmPricingJSON, fastOverrides)
	m.putBuiltinPricing(fastOverrides)
	m.applyBuiltinLongContextRates()
	// Resolve models that LiteLLM and the built-in table miss from the
	// embedded models.dev snapshot. This works offline, unlike the network
	// source gated by enableModelsDevFallback.
	m.enableEmbeddedModelsDevFallback = true
	return m
}

// PricingOverride is a per-model user override (CLI --pricing-override).
type PricingOverride struct {
	InputCostPerToken                          *float64
	OutputCostPerToken                         *float64
	CacheCreationInputTokenCost                *float64
	CacheReadInputTokenCost                    *float64
	InputCostPerTokenAbove200kTokens           *float64
	OutputCostPerTokenAbove200kTokens          *float64
	CacheCreationInputTokenCostAbove200kTokens *float64
	CacheReadInputTokenCostAbove200kTokens     *float64
	MaxInputTokens                             *uint64
	FastMultiplier                             *float64
}

// LoadWithOverrides mirrors PricingMap::load_with_overrides: refresh pricing
// from LiteLLM when online, re-apply the built-in long-context overlay, then
// user overrides. The log flag gates the progress spinner around the refresh
// (stderr-only, so JSON output stays clean).
func LoadWithOverrides(offline, log bool, overrides map[string]PricingOverride) *PricingMap {
	m := LoadEmbedded()
	if !offline {
		var (
			body string
			err  error
		)
		TrackStatus(log, "Refreshing model pricing from LiteLLM...", func() {
			body, err = fetchJSONURL(litellmPricingURL)
		})
		if err != nil {
			if shouldLogPricingRefreshDetails() {
				fmt.Fprintf(os.Stderr, "WARN  Failed to fetch LiteLLM pricing (%v); using embedded pricing.\n", err)
			}
		} else if loaded := m.LoadJSON(body); loaded == 0 && shouldLogPricingRefreshDetails() {
			fmt.Fprintln(os.Stderr, "WARN  Failed to parse LiteLLM pricing; using embedded pricing.")
		}
	}

	// A live LiteLLM refresh replaces whole entries, so re-apply the
	// built-in long-context rates it does not publish before user overrides
	// get the final word.
	m.applyBuiltinLongContextRates()
	m.enableModelsDevFallback = !offline
	m.applyOverrides(overrides)
	return m
}

// ---------------------------------------------------------------------------
// JSON loading (LiteLLM shapes and models.dev shapes)
// ---------------------------------------------------------------------------

type providerSpecificEntry struct {
	Fast *float64 `json:"fast"`
}

type liteLlmPricing struct {
	InputCostPerToken                          *float64               `json:"input_cost_per_token"`
	OutputCostPerToken                         *float64               `json:"output_cost_per_token"`
	CacheCreationInputTokenCost                *float64               `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost                    *float64               `json:"cache_read_input_token_cost"`
	InputCostPerTokenAbove200kTokens           *float64               `json:"input_cost_per_token_above_200k_tokens"`
	OutputCostPerTokenAbove200kTokens          *float64               `json:"output_cost_per_token_above_200k_tokens"`
	CacheCreationInputTokenCostAbove200kTokens *float64               `json:"cache_creation_input_token_cost_above_200k_tokens"`
	CacheReadInputTokenCostAbove200kTokens     *float64               `json:"cache_read_input_token_cost_above_200k_tokens"`
	MaxInputTokens                             *uint64                `json:"max_input_tokens"`
	ProviderSpecificEntry                      *providerSpecificEntry `json:"provider_specific_entry"`
}

type compactLiteLlmPricing struct {
	I    float64  `json:"i"`
	O    float64  `json:"o"`
	CC   *float64 `json:"cc"`
	CR   *float64 `json:"cr"`
	IA   *float64 `json:"ia"`
	OA   *float64 `json:"oa"`
	CCA  *float64 `json:"cca"`
	CRA  *float64 `json:"cra"`
	CTX  *uint64  `json:"ctx"`
	Fast *float64 `json:"fast"`
}

// parseLiteLlmPricing accepts both the compact embedded shape ({"i":..,"o":..})
// and the full LiteLLM shape; ok=false when the value fits neither.
func parseLiteLlmPricing(raw json.RawMessage) (liteLlmPricing, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return liteLlmPricing{}, false
	}
	if _, hasI := fields["i"]; hasI {
		if _, hasO := fields["o"]; hasO {
			var compact compactLiteLlmPricing
			if err := json.Unmarshal(raw, &compact); err == nil {
				out := liteLlmPricing{
					InputCostPerToken:                          &compact.I,
					OutputCostPerToken:                         &compact.O,
					CacheCreationInputTokenCost:                compact.CC,
					CacheReadInputTokenCost:                    compact.CR,
					InputCostPerTokenAbove200kTokens:           compact.IA,
					OutputCostPerTokenAbove200kTokens:          compact.OA,
					CacheCreationInputTokenCostAbove200kTokens: compact.CCA,
					CacheReadInputTokenCostAbove200kTokens:     compact.CRA,
					MaxInputTokens:                             compact.CTX,
				}
				if compact.Fast != nil {
					fast := *compact.Fast
					out.ProviderSpecificEntry = &providerSpecificEntry{Fast: &fast}
				}
				return out, true
			}
		}
	}
	var full liteLlmPricing
	if err := json.Unmarshal(raw, &full); err != nil {
		return liteLlmPricing{}, false
	}
	if full.InputCostPerToken == nil || full.OutputCostPerToken == nil {
		return liteLlmPricing{}, false
	}
	return full, true
}

// LoadJSON merges a LiteLLM pricing document into the map, returning the
// number of entries loaded.
func (m *PricingMap) LoadJSON(doc string) int {
	return m.loadJSONWithOverrides(doc, loadFastMultiplierOverrides())
}

func (m *PricingMap) loadJSONWithOverrides(doc string, fastOverrides *fastMultiplierOverrides) int {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &raw); err != nil {
		return 0
	}
	loaded := 0
	for model, value := range raw {
		parsed, ok := parseLiteLlmPricing(value)
		if !ok || parsed.InputCostPerToken == nil || parsed.OutputCostPerToken == nil {
			continue
		}
		input := *parsed.InputCostPerToken
		output := *parsed.OutputCostPerToken
		cacheReadExplicit := parsed.CacheReadInputTokenCost != nil
		fastMultiplier := 1.0
		if parsed.ProviderSpecificEntry != nil && parsed.ProviderSpecificEntry.Fast != nil {
			fastMultiplier = *parsed.ProviderSpecificEntry.Fast
		} else if override := fastOverrides.multiplierFor(model); override != nil {
			fastMultiplier = *override
		}
		cacheCreate := input * 1.25
		if parsed.CacheCreationInputTokenCost != nil {
			cacheCreate = *parsed.CacheCreationInputTokenCost
		}
		cacheRead := input * 0.1
		if parsed.CacheReadInputTokenCost != nil {
			cacheRead = *parsed.CacheReadInputTokenCost
		}
		m.entries[model] = Pricing{
			Input:                input,
			Output:               output,
			CacheCreate:          cacheCreate,
			CacheRead:            cacheRead,
			CacheReadExplicit:    cacheReadExplicit,
			InputAbove200k:       parsed.InputCostPerTokenAbove200kTokens,
			OutputAbove200k:      parsed.OutputCostPerTokenAbove200kTokens,
			CacheCreateAbove200k: parsed.CacheCreationInputTokenCostAbove200kTokens,
			CacheReadAbove200k:   parsed.CacheReadInputTokenCostAbove200kTokens,
			FastMultiplier:       fastMultiplier,
		}
		if parsed.MaxInputTokens != nil {
			m.contextLimits[model] = *parsed.MaxInputTokens
		}
		loaded++
	}
	m.clearFindCache()
	return loaded
}

// ---------------------------------------------------------------------------
// models.dev loading
// ---------------------------------------------------------------------------

type modelsDevCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type modelsDevLimit struct {
	Context *uint64 `json:"context"`
}

type modelsDevModel struct {
	ID    *string         `json:"id"`
	Cost  *modelsDevCost  `json:"cost"`
	Limit *modelsDevLimit `json:"limit"`
}

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevJSON struct {
	providers map[string]modelsDevProvider
	models    map[string]modelsDevModel
}

func parseModelsDevJSON(doc string) (*modelsDevJSON, bool) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &entries); err != nil || entries == nil {
		return nil, false
	}
	hasModelsField := false
	for _, value := range entries {
		if modelsDevEntryHasModelsField(value) {
			hasModelsField = true
			break
		}
	}
	if hasModelsField {
		for _, value := range entries {
			if !modelsDevEntryHasModelsField(value) {
				return nil, false
			}
		}
		var providers map[string]modelsDevProvider
		if err := json.Unmarshal([]byte(doc), &providers); err != nil {
			return nil, false
		}
		return &modelsDevJSON{providers: providers}, true
	}
	for _, value := range entries {
		if !modelsDevEntryHasRequiredCost(value) {
			return nil, false
		}
	}
	var models map[string]modelsDevModel
	if err := json.Unmarshal([]byte(doc), &models); err != nil {
		return nil, false
	}
	return &modelsDevJSON{models: models}, true
}

func modelsDevEntryHasModelsField(value json.RawMessage) bool {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(value, &entry); err != nil {
		return false
	}
	models, ok := entry["models"]
	if !ok {
		return false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(models, &obj) != nil {
		return false
	}
	// A JSON null unmarshals into a nil map; the reference requires an object.
	return obj != nil
}

func modelsDevEntryHasRequiredCost(value json.RawMessage) bool {
	var entry struct {
		Cost *struct {
			Input  *float64 `json:"input"`
			Output *float64 `json:"output"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(value, &entry); err != nil {
		return false
	}
	return entry.Cost != nil && entry.Cost.Input != nil && entry.Cost.Output != nil
}

// loadModelsDevJSONMissing merges a models.dev document, filling only models
// the primary table does not already carry. ok=false when the document does
// not parse as either shape.
func (m *PricingMap) loadModelsDevJSONMissing(doc string) (int, bool) {
	parsed, ok := parseModelsDevJSON(doc)
	if !ok {
		return 0, false
	}
	loaded := 0
	if parsed.providers != nil {
		for _, provider := range parsed.providers {
			loaded += m.loadModelsDevModels(provider.Models)
		}
		return loaded, true
	}
	return m.loadModelsDevModels(parsed.models), true
}

func (m *PricingMap) loadModelsDevModels(models map[string]modelsDevModel) int {
	loaded := 0
	for modelKey, model := range models {
		modelID := modelKey
		if model.ID != nil {
			modelID = *model.ID
		}
		if _, exists := m.entries[modelID]; exists {
			continue
		}
		if model.Cost == nil || model.Cost.Input == nil || model.Cost.Output == nil {
			continue
		}
		input := *model.Cost.Input / 1_000_000.0
		output := *model.Cost.Output / 1_000_000.0
		cacheReadExplicit := model.Cost.CacheRead != nil
		cacheCreate := input * 1.25
		if model.Cost.CacheWrite != nil {
			cacheCreate = *model.Cost.CacheWrite / 1_000_000.0
		}
		cacheRead := input * 0.1
		if model.Cost.CacheRead != nil {
			cacheRead = *model.Cost.CacheRead / 1_000_000.0
		}
		m.entries[modelID] = Pricing{
			Input:             input,
			Output:            output,
			CacheCreate:       cacheCreate,
			CacheRead:         cacheRead,
			CacheReadExplicit: cacheReadExplicit,
			FastMultiplier:    1.0,
		}
		if model.Limit != nil && model.Limit.Context != nil {
			m.contextLimits[modelID] = *model.Limit.Context
		}
		loaded++
	}
	m.clearFindCache()
	return loaded
}

// ---------------------------------------------------------------------------
// Lookup chain
// ---------------------------------------------------------------------------

// Find resolves a model through the full chain: exact entry, static pricing
// alias, fuzzy suffix matching, CCUSAGE_MODEL_ALIASES, live models.dev, then
// the embedded models.dev snapshot. A nil result means no pricing exists.
func (m *PricingMap) Find(model string) *Pricing {
	if m == nil {
		return nil
	}
	m.findMu.Lock()
	if m.findCache != nil {
		if cached, ok := m.findCache[model]; ok {
			m.findMu.Unlock()
			return cached
		}
	}
	m.findMu.Unlock()

	resolvedAlias := ResolveModelName(model)
	var result *Pricing
	if p := m.findEntryOrAlias(model); p != nil {
		result = p
	}
	if result == nil && resolvedAlias != model {
		result = m.findEntryOrAlias(resolvedAlias)
	}
	if result == nil && m.enableModelsDevFallback {
		if dev := modelsDevPricing(); dev != nil {
			result = dev.findEntryOrAlias(resolvedAlias)
		}
	}
	// The embedded models.dev snapshot is a separate map, so it only resolves
	// models the primary table misses and never perturbs its fuzzy alias
	// matching. It works offline, unlike the network source.
	if result == nil && m.enableEmbeddedModelsDevFallback {
		result = embeddedModelsDevPricing().findEntryOrAlias(resolvedAlias)
	}

	m.findMu.Lock()
	if m.findCache == nil {
		m.findCache = map[string]*Pricing{}
	}
	m.findCache[model] = result
	m.findMu.Unlock()
	return result
}

// FindExact returns the entry stored under model verbatim.
func (m *PricingMap) FindExact(model string) *Pricing {
	if m == nil {
		return nil
	}
	if p, ok := m.entries[model]; ok {
		return &p
	}
	return nil
}

// Models returns the sorted model keys of the primary pricing table (the
// leaderboard server's pricing page renders this roster).
func (m *PricingMap) Models() []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m.entries))
	for key := range m.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// findEntry is the primary-table lookup without any models.dev fallback;
// exposed for tests mirroring the Rust find_entry test helper.
func (m *PricingMap) findEntry(model string) *Pricing {
	if p, ok := m.entries[model]; ok {
		return &p
	}
	normalizedModel := normalizedPricingKey(model)
	var best string
	var bestPricing Pricing
	found := false
	for candidate, pricing := range m.entries {
		if !pricingKeyMatches(candidate, model, normalizedModel) {
			continue
		}
		if !found || len(candidate) > len(best) || (len(candidate) == len(best) && candidate < best) {
			best, bestPricing, found = candidate, pricing, true
		}
	}
	if !found {
		return nil
	}
	return &bestPricing
}

func (m *PricingMap) findEntryOrAlias(model string) *Pricing {
	if p, ok := m.entries[model]; ok {
		return &p
	}
	if alias, isAlias := pricingAlias(model); isAlias {
		if p := m.findEntry(alias); p != nil {
			return p
		}
	}
	return m.findEntry(model)
}

// ContextLimit resolves the max-input-token limit through the same chain.
func (m *PricingMap) ContextLimit(model string) (uint64, bool) {
	if m == nil {
		return 0, false
	}
	resolvedAlias := ResolveModelName(model)
	if limit, ok := m.contextLimitEntryOrAlias(model); ok {
		return limit, true
	}
	if resolvedAlias != model {
		if limit, ok := m.contextLimitEntryOrAlias(resolvedAlias); ok {
			return limit, true
		}
	}
	if m.enableModelsDevFallback {
		if dev := modelsDevPricing(); dev != nil {
			if limit, ok := dev.contextLimitEntryOrAlias(resolvedAlias); ok {
				return limit, true
			}
		}
	}
	if m.enableEmbeddedModelsDevFallback {
		if limit, ok := embeddedModelsDevPricing().contextLimitEntryOrAlias(resolvedAlias); ok {
			return limit, true
		}
	}
	return 0, false
}

func (m *PricingMap) contextLimitEntryOrAlias(model string) (uint64, bool) {
	if limit, ok := m.contextLimits[model]; ok {
		return limit, true
	}
	if alias, isAlias := pricingAlias(model); isAlias {
		if limit, ok := m.contextLimitEntry(alias); ok {
			return limit, true
		}
	}
	return m.contextLimitEntry(model)
}

func (m *PricingMap) contextLimitEntry(model string) (uint64, bool) {
	if limit, ok := m.contextLimits[model]; ok {
		return limit, true
	}
	normalizedModel := normalizedPricingKey(model)
	var best string
	bestLimit := uint64(0)
	found := false
	for candidate, limit := range m.contextLimits {
		if !pricingKeyMatches(candidate, model, normalizedModel) {
			continue
		}
		if !found || len(candidate) > len(best) || (len(candidate) == len(best) && candidate < best) {
			best, bestLimit, found = candidate, limit, true
		}
	}
	return bestLimit, found
}

func (m *PricingMap) clearFindCache() {
	m.findMu.Lock()
	m.findCache = nil
	m.findMu.Unlock()
}

func (m *PricingMap) len() int {
	return len(m.entries)
}

func (m *PricingMap) modelsDevFallbackEnabled() bool {
	return m.enableModelsDevFallback
}

// ---------------------------------------------------------------------------
// Fuzzy key matching (pricing_key_matches and friends)
// ---------------------------------------------------------------------------

// pricingKeyMatches matches pricing keys across provider/model aliases while
// preserving version boundaries.
func pricingKeyMatches(candidate, model, normalizedModel string) bool {
	if containsPricingKey(model, candidate) || containsPricingKey(candidate, model) {
		return true
	}
	normalizedCandidate := normalizedPricingKey(candidate)
	return containsPricingKey(normalizedModel, normalizedCandidate) ||
		containsPricingKey(normalizedCandidate, normalizedModel)
}

// containsPricingKey finds a key only when the surrounding bytes are
// non-alphanumeric boundaries.
func containsPricingKey(value, key string) bool {
	offset := 0
	for {
		index := strings.Index(value[offset:], key)
		if index < 0 {
			return false
		}
		index += offset
		before := byte(0)
		hasBefore := index > 0
		if hasBefore {
			before = value[index-1]
		}
		suffix := value[index+len(key):]
		if (!hasBefore || isPricingKeyBoundary(before)) && suffixAllowsPricingKeyMatch(key, suffix) {
			return true
		}
		offset = index + len(key)
		if offset >= len(value) {
			return false
		}
	}
}

// isPricingKeyBoundary treats punctuation separators as boundaries, but not
// adjacent version digits.
func isPricingKeyBoundary(b byte) bool {
	return !isASCIIAlphanumeric(b)
}

func isASCIIAlphanumeric(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func suffixAllowsPricingKeyMatch(key, suffix string) bool {
	if suffix == "" {
		return true
	}
	separator := suffix[0]
	if !isPricingKeyBoundary(separator) {
		return false
	}
	return !suffixStartsWithNumericModelVersion(key, suffix)
}

func suffixStartsWithNumericModelVersion(key, suffix string) bool {
	if len(key) == 0 || !isASCIIDigit(key[len(key)-1]) {
		return false
	}
	if len(suffix) == 0 || (suffix[0] != '-' && suffix[0] != '.') {
		return false
	}
	rest := suffix[1:]
	digitLen := 0
	for digitLen < len(rest) && isASCIIDigit(rest[digitLen]) {
		digitLen++
	}
	if digitLen == 0 {
		return false
	}
	if digitLen == len(rest) {
		// Digits run to the end: an exact-length date suffix is allowed.
		return !(digitLen == modelDateSuffixDigits)
	}
	afterDigits := rest[digitLen]
	return !(digitLen == modelDateSuffixDigits && isPricingKeyBoundary(afterDigits))
}

// normalizedPricingKey normalizes known model separator variants.
func normalizedPricingKey(value string) string {
	if strings.ContainsAny(value, ".@") {
		return strings.NewReplacer(".", "-", "@", "-").Replace(value)
	}
	return value
}

// modelWithoutDateSuffix strips a trailing release-date suffix (-YYYY-MM-DD
// or -YYYYMMDD) so date-pinned pricing keys share their base model's
// long-context rates.
func modelWithoutDateSuffix(model string) string {
	// -YYYY-MM-DD (OpenAI style, e.g. gpt-5.5-2026-04-23)
	if len(model) > 11 {
		suffix := model[len(model)-11:]
		if suffix[0] == '-' &&
			isASCIIDigit(suffix[1]) && isASCIIDigit(suffix[2]) && isASCIIDigit(suffix[3]) && isASCIIDigit(suffix[4]) &&
			suffix[5] == '-' &&
			isASCIIDigit(suffix[6]) && isASCIIDigit(suffix[7]) &&
			suffix[8] == '-' &&
			isASCIIDigit(suffix[9]) && isASCIIDigit(suffix[10]) {
			return model[:len(model)-11]
		}
	}
	// -YYYYMMDD (Anthropic style, e.g. claude-3-5-haiku-20241022)
	if len(model) > 9 {
		suffix := model[len(model)-9:]
		if suffix[0] == '-' && isASCIIDigit(suffix[1]) && isASCIIDigit(suffix[2]) && isASCIIDigit(suffix[3]) &&
			isASCIIDigit(suffix[4]) && isASCIIDigit(suffix[5]) && isASCIIDigit(suffix[6]) && isASCIIDigit(suffix[7]) &&
			isASCIIDigit(suffix[8]) {
			return model[:len(model)-9]
		}
	}
	return model
}

// pricingAlias maps model aliases to canonical pricing keys before fuzzy
// matching.
func pricingAlias(model string) (string, bool) {
	switch model {
	case "gpt-5.6":
		return "gpt-5.6-sol", true
	case "gpt-5.3-spark":
		return "gpt-5.3-codex-spark", true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Built-in long-context tier rates
// ---------------------------------------------------------------------------

type longContextRates struct {
	threshold   uint64
	input       *float64
	output      *float64
	cacheCreate *float64
	cacheRead   *float64
}

// builtinLongContextRates holds built-in two-stage rates that upstream pricing
// sources do not publish. Source: https://platform.openai.com/docs/pricing
// (Standard tier); OpenAI bills requests with more than 272K input tokens at
// long-context rates.
func builtinLongContextRates(baseModel string) *longContextRates {
	// A default family alias bills at the variant it points to, so it must
	// pick up that variant's tier rates. Upstream pricing data publishes the
	// alias as its own entry without long-context rates, which would
	// otherwise leave the alias on flat pricing.
	if alias, ok := pricingAlias(baseModel); ok {
		baseModel = alias
	}
	openai := func(input, output float64, cacheCreate, cacheRead *float64) *longContextRates {
		return &longContextRates{
			threshold:   openAILongContextThresholdTokens,
			input:       &input,
			output:      &output,
			cacheCreate: cacheCreate,
			cacheRead:   cacheRead,
		}
	}
	value := func(v float64) *float64 { return &v }
	switch baseModel {
	case "gpt-5.6-sol":
		return openai(10e-6, 45e-6, value(12.5e-6), value(1e-6))
	case "gpt-5.6-terra":
		return openai(5e-6, 22.5e-6, value(6.25e-6), value(0.5e-6))
	case "gpt-5.6-luna":
		return openai(2e-6, 9e-6, value(2.5e-6), value(0.2e-6))
	// gpt-5.5 and gpt-5.4 have no separate cache-write price, so cache writes
	// are billed as regular input in both tiers.
	case "gpt-5.5":
		return openai(10e-6, 45e-6, value(10e-6), value(1e-6))
	case "gpt-5.4":
		return openai(5e-6, 22.5e-6, value(5e-6), value(0.5e-6))
	// The pro models have no prompt-caching prices, so only the input and
	// output rates change in the long-context tier.
	case "gpt-5.5-pro", "gpt-5.4-pro":
		return openai(60e-6, 270e-6, nil, nil)
	}
	return nil
}

// LongContextSplitThreshold returns the input-token boundary above which a
// request is billed at a model's long-context tier, falling back to the
// default 200K boundary used for LiteLLM `*_above_200k_tokens` data.
func LongContextSplitThreshold(model string) uint64 {
	if rates := builtinLongContextRates(modelWithoutDateSuffix(model)); rates != nil {
		return rates.threshold
	}
	return DefaultLongContextThresholdTokens
}

// applyBuiltinLongContextRates fills in long-context tier rates for models
// whose upstream pricing entries only carry the flat rates. Runs after every
// pricing load (embedded and live) because LiteLLM refreshes replace whole
// entries. Entries that already carry any tier rate are left untouched so
// upstream data wins once it exists. The check is deliberately all-or-nothing
// rather than per field: upstream `*_above_200k_tokens` values assume the
// 200K boundary, so mixing them with built-in rates that assume the OpenAI
// 272K boundary would price both tiers wrong.
func (m *PricingMap) applyBuiltinLongContextRates() {
	for model, pricing := range m.entries {
		if pricing.InputAbove200k != nil || pricing.OutputAbove200k != nil ||
			pricing.CacheCreateAbove200k != nil || pricing.CacheReadAbove200k != nil {
			continue
		}
		rates := builtinLongContextRates(modelWithoutDateSuffix(model))
		if rates == nil {
			continue
		}
		threshold := rates.threshold
		pricing.InputAbove200k = rates.input
		pricing.OutputAbove200k = rates.output
		pricing.CacheCreateAbove200k = rates.cacheCreate
		pricing.CacheReadAbove200k = rates.cacheRead
		pricing.LongContextThreshold = &threshold
		m.entries[model] = pricing
	}
	m.clearFindCache()
}

// ---------------------------------------------------------------------------
// User overrides
// ---------------------------------------------------------------------------

func (m *PricingMap) applyOverrides(overrides map[string]PricingOverride) {
	for model, override := range overrides {
		m.applyOverride(model, override)
	}
	m.clearFindCache()
}

func (m *PricingMap) applyOverride(model string, override PricingOverride) {
	base, ok := m.entries[model]
	if !ok {
		if alias, isAlias := pricingAlias(model); isAlias {
			if aliased, aliasOk := m.entries[alias]; aliasOk {
				base = aliased
				ok = true
			}
		}
	}
	if !ok {
		base = emptyPricing()
	}

	newInput := base.Input
	if override.InputCostPerToken != nil {
		newInput = *override.InputCostPerToken
	}

	// When input cost is overridden but cache fields are not explicitly
	// provided, and the base cache values were derived from input (indicated
	// by !cache_read_explicit), scale cache costs proportionally by
	// new_input / old_input. When cache_read_explicit is true, the base cache
	// values were independently set (from LiteLLM data or a prior override),
	// so preserve them unchanged.
	shouldScale := override.InputCostPerToken != nil && base.Input > 0 && !base.CacheReadExplicit
	scale := 1.0
	if shouldScale {
		scale = newInput / base.Input
	}

	cacheCreate := base.CacheCreate
	if override.CacheCreationInputTokenCost != nil {
		cacheCreate = *override.CacheCreationInputTokenCost
	} else if shouldScale && base.CacheCreate > 0 {
		cacheCreate = base.CacheCreate * scale
	}

	cacheRead := base.CacheRead
	if override.CacheReadInputTokenCost != nil {
		cacheRead = *override.CacheReadInputTokenCost
	} else if shouldScale && base.CacheRead > 0 {
		cacheRead = base.CacheRead * scale
	}

	cacheCreateAbove200k := base.CacheCreateAbove200k
	if overrideValue := override.CacheCreationInputTokenCostAbove200kTokens; overrideValue != nil {
		cacheCreateAbove200k = overrideValue
	} else if shouldScale && base.CacheCreateAbove200k != nil {
		scaled := *base.CacheCreateAbove200k * scale
		cacheCreateAbove200k = &scaled
	}

	cacheReadAbove200k := base.CacheReadAbove200k
	if overrideValue := override.CacheReadInputTokenCostAbove200kTokens; overrideValue != nil {
		cacheReadAbove200k = overrideValue
	} else if shouldScale && base.CacheReadAbove200k != nil {
		scaled := *base.CacheReadAbove200k * scale
		cacheReadAbove200k = &scaled
	}

	cacheReadExplicit := base.CacheReadExplicit
	if override.CacheReadInputTokenCost != nil {
		cacheReadExplicit = true
	}

	output := base.Output
	if override.OutputCostPerToken != nil {
		output = *override.OutputCostPerToken
	}

	inputAbove200k := base.InputAbove200k
	if override.InputCostPerTokenAbove200kTokens != nil {
		inputAbove200k = override.InputCostPerTokenAbove200kTokens
	}
	outputAbove200k := base.OutputAbove200k
	if override.OutputCostPerTokenAbove200kTokens != nil {
		outputAbove200k = override.OutputCostPerTokenAbove200kTokens
	}
	fastMultiplier := base.FastMultiplier
	if override.FastMultiplier != nil {
		fastMultiplier = *override.FastMultiplier
	}

	m.entries[model] = Pricing{
		Input:                newInput,
		Output:               output,
		CacheCreate:          cacheCreate,
		CacheRead:            cacheRead,
		CacheReadExplicit:    cacheReadExplicit,
		InputAbove200k:       inputAbove200k,
		OutputAbove200k:      outputAbove200k,
		CacheCreateAbove200k: cacheCreateAbove200k,
		CacheReadAbove200k:   cacheReadAbove200k,
		LongContextThreshold: base.LongContextThreshold,
		FastMultiplier:       fastMultiplier,
	}
	if override.MaxInputTokens != nil {
		m.contextLimits[model] = *override.MaxInputTokens
	}
}

// ---------------------------------------------------------------------------
// Fast multiplier overrides
// ---------------------------------------------------------------------------

type fastMultiplierOverrides struct {
	exact            map[string]float64
	normalizedPrefix map[string]float64
}

func loadFastMultiplierOverrides() *fastMultiplierOverrides {
	var raw struct {
		Exact            map[string]float64 `json:"exact"`
		NormalizedPrefix map[string]float64 `json:"normalized_prefix"`
	}
	if err := json.Unmarshal([]byte(fastMultiplierOverridesJSON), &raw); err != nil {
		panic("parse embedded fast-multiplier-overrides.json: " + err.Error())
	}
	return &fastMultiplierOverrides{
		exact:            raw.Exact,
		normalizedPrefix: raw.NormalizedPrefix,
	}
}

func (o *fastMultiplierOverrides) multiplierFor(model string) *float64 {
	if o == nil {
		return nil
	}
	if multiplier, ok := o.exact[model]; ok {
		return &multiplier
	}
	// A default family alias bills at the variant it points to, so it shares
	// that variant's Fast multiplier.
	if alias, isAlias := pricingAlias(model); isAlias {
		if multiplier, ok := o.exact[alias]; ok {
			return &multiplier
		}
	}
	normalized := strings.NewReplacer(".", "-", "@", "-").Replace(model)
	prefixes := make([]string, 0, len(o.normalizedPrefix))
	for base := range o.normalizedPrefix {
		prefixes = append(prefixes, base)
	}
	sort.Strings(prefixes)
	for _, part := range strings.FieldsFunc(normalized, func(r rune) bool { return r == '/' || r == ':' }) {
		for _, base := range prefixes {
			if matchesModelSuffix(part, base) {
				multiplier := o.normalizedPrefix[base]
				return &multiplier
			}
		}
	}
	return nil
}

func matchesModelSuffix(part, base string) bool {
	index := strings.LastIndex(part, base)
	if index < 0 {
		return false
	}
	suffix := part[index:]
	if suffix == base {
		return true
	}
	return len(suffix) > len(base) && suffix[len(base)] == '-'
}

// ---------------------------------------------------------------------------
// Built-in pricing table
// ---------------------------------------------------------------------------

func (m *PricingMap) putBuiltinPricing(fastOverrides *fastMultiplierOverrides) {
	fastFor := func(model string) float64 {
		if multiplier := fastOverrides.multiplierFor(model); multiplier != nil {
			return *multiplier
		}
		return 1.0
	}
	insert := func(model string, p Pricing) {
		m.entries[model] = p
	}
	anthropic := func(input, output, cacheCreate, cacheRead float64, fast float64) Pricing {
		return Pricing{
			Input: input, Output: output, CacheCreate: cacheCreate, CacheRead: cacheRead,
			CacheReadExplicit: true, FastMultiplier: fast,
		}
	}
	insert("claude-opus-4-5", anthropic(5e-6, 25e-6, 6.25e-6, 0.5e-6, 1.0))
	insert("claude-opus-4-6", anthropic(5e-6, 25e-6, 6.25e-6, 0.5e-6, fastFor("claude-opus-4-6")))
	insert("claude-opus-4-7", anthropic(5e-6, 25e-6, 6.25e-6, 0.5e-6, fastFor("claude-opus-4-7")))
	insert("claude-opus-4-8", anthropic(5e-6, 25e-6, 6.25e-6, 0.5e-6, fastFor("claude-opus-4-8")))
	insert("claude-haiku-4-5", anthropic(1e-6, 5e-6, 1.25e-6, 0.1e-6, 1.0))
	insert("claude-opus-4", anthropic(15e-6, 75e-6, 18.75e-6, 1.5e-6, 1.0))
	insert("claude-sonnet-4-6", anthropic(3e-6, 15e-6, 3.75e-6, 0.3e-6, 1.0))
	insert("claude-sonnet-4", Pricing{
		Input: 3e-6, Output: 15e-6, CacheCreate: 3.75e-6, CacheRead: 0.3e-6,
		CacheReadExplicit:    true,
		InputAbove200k:       floatPtr(6e-6),
		OutputAbove200k:      floatPtr(22.5e-6),
		CacheCreateAbove200k: floatPtr(7.5e-6),
		CacheReadAbove200k:   floatPtr(0.6e-6),
		FastMultiplier:       1.0,
	})
	claude35Haiku := anthropic(0.8e-6, 4e-6, 1e-6, 0.08e-6, 1.0)
	insert("claude-3-5-haiku", claude35Haiku)
	insert("claude-3-5-haiku-20241022", claude35Haiku)
	insert("claude-3-opus", anthropic(15e-6, 75e-6, 18.75e-6, 1.5e-6, 1.0))
	insert("claude-3-sonnet", anthropic(3e-6, 15e-6, 3.75e-6, 0.3e-6, 1.0))
	insert("claude-3-haiku", anthropic(0.25e-6, 1.25e-6, 0.3e-6, 0.03e-6, 1.0))
	insert("gpt-5", anthropic(1.25e-6, 10e-6, 1.25e-6, 0.125e-6, 1.0))
	insert("gpt-5.5", anthropic(5e-6, 30e-6, 5e-6, 0.5e-6, fastFor("gpt-5.5")))
	insert("grok-4.3", Pricing{
		Input: 1.25e-6, Output: 2.5e-6, CacheCreate: 1.25e-6, CacheRead: 0.125e-6,
		CacheReadExplicit: false, FastMultiplier: 1.0,
	})
	// Source: https://platform.kimi.ai/docs/pricing/chat-k25
	insert("moonshot/kimi-k2.5", anthropic(0.6e-6, 3e-6, 0.75e-6, 0.1e-6, 1.0))
	// Source: https://platform.kimi.ai/docs/pricing/chat-k26
	insert("moonshot/kimi-k2.6", anthropic(0.95e-6, 4e-6, 1.1875e-6, 0.16e-6, 1.0))
	gpt51 := anthropic(1.25e-6, 10e-6, 1.25e-6, 0.125e-6, 1.0)
	insert("gpt-5.1", gpt51)
	insert("gpt-5.1-codex", gpt51)
	gpt52Codex := anthropic(1.75e-6, 14e-6, 1.75e-6, 0.175e-6, 1.0)
	insert("gpt-5.2-codex", gpt52Codex)
	insert("gpt-5.3-codex", anthropic(1.75e-6, 14e-6, 1.75e-6, 0.175e-6, fastFor("gpt-5.3-codex")))
	insert("gpt-5.2", gpt52Codex)
	insert("gpt-5.4", anthropic(2.5e-6, 15e-6, 2.5e-6, 0.25e-6, fastFor("gpt-5.4")))
	insert("gpt-5.4-mini", anthropic(0.75e-6, 4.5e-6, 0.75e-6, 0.075e-6, 1.0))
	insert("gpt-5.4-nano", anthropic(0.2e-6, 1.25e-6, 0.2e-6, 0.02e-6, 1.0))
	// Source: https://platform.openai.com/docs/pricing (Standard tier, short
	// context). The long-context tier rates live in
	// builtinLongContextRates, which runs after every pricing load.
	for model, rates := range map[string][4]float64{
		"gpt-5.6-sol":   {5e-6, 30e-6, 6.25e-6, 0.5e-6},
		"gpt-5.6-terra": {2.5e-6, 15e-6, 3.125e-6, 0.25e-6},
		"gpt-5.6-luna":  {1e-6, 6e-6, 1.25e-6, 0.1e-6},
	} {
		fast := fastFor(model)
		insert(model, anthropic(rates[0], rates[1], rates[2], rates[3], fast))
	}
	// Source: https://docs.z.ai/guides/overview/pricing
	glm := func(input, output, cacheRead float64) Pricing {
		return Pricing{
			Input: input, Output: output, CacheCreate: 0, CacheRead: cacheRead,
			CacheReadExplicit: true, FastMultiplier: 1.0,
		}
	}
	glmBase := glm(0.6e-6, 2.2e-6, 0.11e-6)
	insert("glm-4.5", glmBase)
	insert("zai/glm-4.5", glmBase)
	insert("zai/glm-4.5-x", glm(2.2e-6, 8.9e-6, 0.45e-6))
	insert("zai/glm-4.5-air", glm(0.2e-6, 1.1e-6, 0.03e-6))
	insert("zai/glm-4.5-airx", glm(1.1e-6, 4.5e-6, 0.22e-6))
	insert("zai/glm-4.5v", glm(0.6e-6, 1.8e-6, 0.11e-6))
	insert("zai/glm-4-32b-0414-128k", glm(0.1e-6, 0.1e-6, 0))
	insert("zai/glm-4.5-flash", glm(0, 0, 0))
	insert("glm-4.6", glmBase)
	insert("glm-4.7", glmBase)
	insert("glm-5", Pricing{
		Input: 1.0e-6, Output: 3.2e-6, CacheRead: 0.2e-6,
		CacheCreate: glmBase.CacheCreate, CacheReadExplicit: glmBase.CacheReadExplicit,
		FastMultiplier: glmBase.FastMultiplier,
	})
	insert("glm-5-turbo", Pricing{
		Input: 1.2e-6, Output: 4.0e-6, CacheRead: 0.24e-6,
		CacheCreate: glmBase.CacheCreate, CacheReadExplicit: glmBase.CacheReadExplicit,
		FastMultiplier: glmBase.FastMultiplier,
	})
	insert("glm-5.1", Pricing{
		Input: 1.4e-6, Output: 4.4e-6, CacheRead: 0.26e-6,
		CacheCreate: glmBase.CacheCreate, CacheReadExplicit: glmBase.CacheReadExplicit,
		FastMultiplier: glmBase.FastMultiplier,
	})

	m.contextLimits["gpt-5.5"] = 1_050_000
	m.contextLimits["grok-4.3"] = 1_000_000
	m.contextLimits["gpt-5.4"] = 1_050_000
	// The gpt-5.6 family shares the 1,050,000-token window of the other
	// long-context GPT-5 flagship models until upstream data lands.
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		m.contextLimits[model] = 1_050_000
	}
	for _, model := range []string{
		"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6",
	} {
		m.contextLimits[model] = 1_000_000
	}
	m.contextLimits["moonshot/kimi-k2.5"] = 262_144
	m.contextLimits["moonshot/kimi-k2.6"] = 262_144
	for _, model := range []string{
		"claude-opus-4-5", "claude-haiku-4-5", "claude-opus-4", "claude-sonnet-4",
		"claude-3-5-haiku", "claude-3-5-haiku-20241022", "claude-3-opus",
		"claude-3-sonnet", "claude-3-haiku",
	} {
		m.contextLimits[model] = 200_000
	}
}

func floatPtr(v float64) *float64 { return &v }

// ---------------------------------------------------------------------------
// models.dev sources and JSON fetch plumbing
// ---------------------------------------------------------------------------

// modelsDevPricingCache lazily loads the live models.dev pricing table,
// throttling retries after a failure.
type modelsDevPricingCache struct {
	mu                sync.Mutex
	pricing           *PricingMap
	lastFailure       *time.Time
	failureRetryAfter time.Duration
}

func newModelsDevPricingCache(failureRetryAfter time.Duration) *modelsDevPricingCache {
	return &modelsDevPricingCache{failureRetryAfter: failureRetryAfter}
}

func (c *modelsDevPricingCache) getOrTryLoad(fetchJSON func() (string, error)) *PricingMap {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pricing != nil {
		return c.pricing
	}
	if c.lastFailure != nil && time.Since(*c.lastFailure) < c.failureRetryAfter {
		return nil
	}
	m, ok := loadModelsDevPricing(fetchJSON)
	if !ok {
		now := time.Now()
		c.lastFailure = &now
		return nil
	}
	c.pricing = m
	c.lastFailure = nil
	return m
}

var liveModelsDevCache = newModelsDevPricingCache(modelsDevFailureRetryAfter)

func modelsDevPricing() *PricingMap {
	return liveModelsDevCache.getOrTryLoad(func() (string, error) {
		return fetchJSONURL(modelsDevAPIURL)
	})
}

func loadModelsDevPricing(fetchJSON func() (string, error)) (*PricingMap, bool) {
	body, err := fetchJSON()
	if err != nil {
		if shouldLogPricingRefreshDetails() {
			fmt.Fprintf(os.Stderr, "WARN  Failed to fetch models.dev pricing (%v); using LiteLLM pricing.\n", err)
		}
		return nil, false
	}
	m := NewPricingMap()
	if _, ok := m.loadModelsDevJSONMissing(body); !ok {
		if shouldLogPricingRefreshDetails() {
			fmt.Fprintln(os.Stderr, "WARN  Failed to parse models.dev pricing; using LiteLLM pricing.")
		}
		return nil, false
	}
	return m, true
}

var embeddedModelsDevOnce sync.Once
var embeddedModelsDevMap *PricingMap

// embeddedModelsDevPricing is built from the models.dev snapshot embedded at
// build time. Unlike the network source this is always available, so it lets
// offline runs price models that LiteLLM and the built-in table do not cover.
// It is kept separate from the primary table so it never participates in that
// table's fuzzy alias matching.
func embeddedModelsDevPricing() *PricingMap {
	embeddedModelsDevOnce.Do(func() {
		m := NewPricingMap()
		if _, ok := m.loadModelsDevJSONMissing(modelsDevPricingJSON); !ok {
			panic("embedded models-dev-pricing.json must parse")
		}
		embeddedModelsDevMap = m
	})
	return embeddedModelsDevMap
}

func shouldLogPricingRefreshDetails() bool {
	level := LogLevel()
	return level != nil && *level >= 4
}

// JSONFetcher fetches a JSON document over HTTP for the pricing refresh. The
// client lives with the binary rather than here so that the net/http stack is
// not a dependency of the core package every adapter builds against.
type JSONFetcher func(url string) (string, error)

var jsonFetcherMu sync.Mutex
var jsonFetcher JSONFetcher

// SetJSONFetcher installs the HTTP client used to refresh pricing; the first
// installation wins, mirroring the reference OnceLock.
func SetJSONFetcher(fetcher JSONFetcher) {
	jsonFetcherMu.Lock()
	defer jsonFetcherMu.Unlock()
	if jsonFetcher == nil {
		jsonFetcher = fetcher
	}
}

func fetchJSONURL(url string) (string, error) {
	jsonFetcherMu.Lock()
	fetcher := jsonFetcher
	jsonFetcherMu.Unlock()
	if fetcher == nil {
		return "", errors.New("no HTTP client installed for pricing refresh")
	}
	return fetcher(url)
}
