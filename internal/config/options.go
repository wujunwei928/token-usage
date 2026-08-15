// Option extraction over raw config maps, ported from
// rust/crates/ccusage-config/src/config_schema.rs. Type mismatches are ignored
// exactly like the reference from_map helpers: a value of the wrong JSON type
// yields "absent", never an error.
package config

import (
	"encoding/json"
	"strconv"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// NamedPiStore is the shared shape of a pi.stores entry.
type NamedPiStore = core.NamedPiStore

// CodexSpeed selects the Codex speed normalization strategy.
type CodexSpeed int

// Codex speeds.
const (
	CodexSpeedAuto CodexSpeed = iota
	CodexSpeedStandard
	CodexSpeedFast
)

// ParseCodexSpeed maps the config string onto a CodexSpeed; ok is false for
// unknown values (which the reference silently ignores).
func ParseCodexSpeed(value string) (CodexSpeed, bool) {
	switch value {
	case "auto":
		return CodexSpeedAuto, true
	case "standard":
		return CodexSpeedStandard, true
	case "fast":
		return CodexSpeedFast, true
	}
	return CodexSpeedAuto, false
}

func (s CodexSpeed) String() string {
	switch s {
	case CodexSpeedStandard:
		return "standard"
	case CodexSpeedFast:
		return "fast"
	default:
		return "auto"
	}
}

// VisualBurnRate selects the statusline burn-rate display mode.
type VisualBurnRate int

// Visual burn-rate modes.
const (
	VisualBurnRateOff VisualBurnRate = iota
	VisualBurnRateEmoji
	VisualBurnRateText
	VisualBurnRateEmojiText
)

// ParseVisualBurnRate maps the kebab-case config string; ok is false for
// unknown values.
func ParseVisualBurnRate(value string) (VisualBurnRate, bool) {
	switch value {
	case "off":
		return VisualBurnRateOff, true
	case "emoji":
		return VisualBurnRateEmoji, true
	case "text":
		return VisualBurnRateText, true
	case "emoji-text":
		return VisualBurnRateEmojiText, true
	}
	return VisualBurnRateOff, false
}

func (v VisualBurnRate) String() string {
	switch v {
	case VisualBurnRateEmoji:
		return "emoji"
	case VisualBurnRateText:
		return "text"
	case VisualBurnRateEmojiText:
		return "emoji-text"
	default:
		return "off"
	}
}

// CostSource selects the statusline cost calculation source.
type CostSource int

// Cost sources.
const (
	CostSourceAuto CostSource = iota
	CostSourceCcusage
	CostSourceCc
	CostSourceBoth
)

// ParseCostSource maps the config string; ok is false for unknown values.
func ParseCostSource(value string) (CostSource, bool) {
	switch value {
	case "auto":
		return CostSourceAuto, true
	case "ccusage":
		return CostSourceCcusage, true
	case "cc":
		return CostSourceCc, true
	case "both":
		return CostSourceBoth, true
	}
	return CostSourceAuto, false
}

func (c CostSource) String() string {
	switch c {
	case CostSourceCcusage:
		return "ccusage"
	case CostSourceCc:
		return "cc"
	case CostSourceBoth:
		return "both"
	default:
		return "auto"
	}
}

// ConfigPricingOverride is one model's pricingOverrides entry; nil pointers
// are fields the section does not set.
type ConfigPricingOverride struct {
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

// SharedOptions holds the option keys every report understands.
type SharedOptions struct {
	Since            *string
	Until            *string
	JSON             *bool
	Mode             *core.CostMode
	Debug            *bool
	DebugSamples     *uint64
	Order            *core.SortOrder
	Breakdown        *bool
	Offline          *bool
	NoOffline        *bool
	Color            *bool
	NoColor          *bool
	Timezone         *string
	JQ               *string
	All              *bool
	Compact          *bool
	SingleThread     *bool
	NoCost           *bool
	PricingOverrides map[string]ConfigPricingOverride
}

// DailySpecificOptions holds daily-only keys.
type DailySpecificOptions struct {
	Instances      *bool
	Project        *string
	ProjectAliases *string
}

// WeeklySpecificOptions holds weekly-only keys.
type WeeklySpecificOptions struct {
	StartOfWeek *core.WeekDay
}

// BlocksSpecificOptions holds blocks-only keys.
type BlocksSpecificOptions struct {
	Active        *bool
	Recent        *bool
	TokenLimit    *string
	SessionLength *float64
}

// StatuslineSpecificOptions holds statusline-only keys.
type StatuslineSpecificOptions struct {
	Offline                *bool
	NoOffline              *bool
	VisualBurnRate         *VisualBurnRate
	CostSource             *CostSource
	Cache                  *bool
	NoCache                *bool
	RefreshInterval        *uint64
	ContextLowThreshold    *uint64
	ContextMediumThreshold *uint64
	Timezone               *string
	Debug                  *bool
	ModelLabelAliases      map[string]string
}

// CodexSpecificOptions holds the codex speed key.
type CodexSpecificOptions struct {
	Speed *CodexSpeed
}

// PiSpecificOptions holds the piPath key.
type PiSpecificOptions struct {
	PIPath *string
}

// OpenClawSpecificOptions holds the openClawPath key.
type OpenClawSpecificOptions struct {
	OpenClawPath *string
}

// SharedOptionsFromMap extracts shared options from one config map.
func SharedOptionsFromMap(m map[string]any) SharedOptions {
	return SharedOptions{
		Since:            stringOption(m, "since"),
		Until:            stringOption(m, "until"),
		JSON:             boolOption(m, "json"),
		Mode:             costModeOption(m, "mode"),
		Debug:            boolOption(m, "debug"),
		DebugSamples:     u64Option(m, "debugSamples"),
		Order:            sortOrderOption(m, "order"),
		Breakdown:        boolOption(m, "breakdown"),
		Offline:          boolOption(m, "offline"),
		NoOffline:        boolOption(m, "noOffline"),
		Color:            boolOption(m, "color"),
		NoColor:          boolOption(m, "noColor"),
		Timezone:         stringOption(m, "timezone"),
		JQ:               stringOption(m, "jq"),
		All:              boolOption(m, "all"),
		Compact:          boolOption(m, "compact"),
		SingleThread:     boolOption(m, "singleThread"),
		NoCost:           boolOption(m, "noCost"),
		PricingOverrides: pricingOverrideMapOption(m, "pricingOverrides"),
	}
}

// DailySpecificOptionsFromMap extracts daily options from one config map.
func DailySpecificOptionsFromMap(m map[string]any) DailySpecificOptions {
	return DailySpecificOptions{
		Instances:      boolOption(m, "instances"),
		Project:        stringOption(m, "project"),
		ProjectAliases: stringOption(m, "projectAliases"),
	}
}

// WeeklySpecificOptionsFromMap extracts weekly options from one config map.
func WeeklySpecificOptionsFromMap(m map[string]any) WeeklySpecificOptions {
	return WeeklySpecificOptions{StartOfWeek: weekDayOption(m, "startOfWeek")}
}

// BlocksSpecificOptionsFromMap extracts blocks options from one config map.
func BlocksSpecificOptionsFromMap(m map[string]any) BlocksSpecificOptions {
	return BlocksSpecificOptions{
		Active:        boolOption(m, "active"),
		Recent:        boolOption(m, "recent"),
		TokenLimit:    stringOption(m, "tokenLimit"),
		SessionLength: f64Option(m, "sessionLength"),
	}
}

// StatuslineSpecificOptionsFromMap extracts statusline options from one map.
func StatuslineSpecificOptionsFromMap(m map[string]any) StatuslineSpecificOptions {
	return StatuslineSpecificOptions{
		Offline:                boolOption(m, "offline"),
		NoOffline:              boolOption(m, "noOffline"),
		VisualBurnRate:         visualBurnRateOption(m, "visualBurnRate"),
		CostSource:             costSourceOption(m, "costSource"),
		Cache:                  boolOption(m, "cache"),
		NoCache:                boolOption(m, "noCache"),
		RefreshInterval:        u64Option(m, "refreshInterval"),
		ContextLowThreshold:    u64Option(m, "contextLowThreshold"),
		ContextMediumThreshold: u64Option(m, "contextMediumThreshold"),
		Timezone:               stringOption(m, "timezone"),
		Debug:                  boolOption(m, "debug"),
		ModelLabelAliases:      stringMapOption(m, "modelLabelAliases"),
	}
}

// CodexSpecificOptionsFromMap extracts codex options from one config map.
func CodexSpecificOptionsFromMap(m map[string]any) CodexSpecificOptions {
	return CodexSpecificOptions{Speed: codexSpeedOption(m, "speed")}
}

// PiSpecificOptionsFromMap extracts pi options from one config map.
func PiSpecificOptionsFromMap(m map[string]any) PiSpecificOptions {
	return PiSpecificOptions{PIPath: stringOption(m, "piPath")}
}

// OpenClawSpecificOptionsFromMap extracts openclaw options from one map.
func OpenClawSpecificOptionsFromMap(m map[string]any) OpenClawSpecificOptions {
	return OpenClawSpecificOptions{OpenClawPath: stringOption(m, "openClawPath")}
}

// ---------------------------------------------------------------------------
// Typed value accessors (serde semantics)
// ---------------------------------------------------------------------------

func stringOption(m map[string]any, key string) *string {
	if m == nil {
		return nil
	}
	if value, ok := m[key].(string); ok {
		return &value
	}
	return nil
}

func boolOption(m map[string]any, key string) *bool {
	if m == nil {
		return nil
	}
	if value, ok := m[key].(bool); ok {
		return &value
	}
	return nil
}

// u64Option mirrors Value::as_u64: only pure-integer JSON literals (no sign,
// fraction, or exponent) deserialize; everything else is "absent".
func u64Option(m map[string]any, key string) *uint64 {
	if m == nil {
		return nil
	}
	number, ok := m[key].(json.Number)
	if !ok {
		return nil
	}
	value, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// f64Option mirrors Value::as_f64: any JSON number literal.
func f64Option(m map[string]any, key string) *float64 {
	if m == nil {
		return nil
	}
	number, ok := m[key].(json.Number)
	if !ok {
		return nil
	}
	value, err := number.Float64()
	if err != nil {
		return nil
	}
	return &value
}

func costModeOption(m map[string]any, key string) *core.CostMode {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	mode, ok := core.ParseCostMode(*value)
	if !ok {
		return nil
	}
	return &mode
}

func sortOrderOption(m map[string]any, key string) *core.SortOrder {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	order, ok := core.ParseSortOrder(*value)
	if !ok {
		return nil
	}
	return &order
}

func weekDayOption(m map[string]any, key string) *core.WeekDay {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	day, ok := core.ParseWeekDay(*value)
	if !ok {
		return nil
	}
	return &day
}

func codexSpeedOption(m map[string]any, key string) *CodexSpeed {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	speed, ok := ParseCodexSpeed(*value)
	if !ok {
		return nil
	}
	return &speed
}

func visualBurnRateOption(m map[string]any, key string) *VisualBurnRate {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	rate, ok := ParseVisualBurnRate(*value)
	if !ok {
		return nil
	}
	return &rate
}

func costSourceOption(m map[string]any, key string) *CostSource {
	value := stringOption(m, key)
	if value == nil {
		return nil
	}
	source, ok := ParseCostSource(*value)
	if !ok {
		return nil
	}
	return &source
}

// stringMapOption mirrors hashmap_option: an object whose string values are
// kept; non-string values are skipped; an empty result is "absent".
func stringMapOption(m map[string]any, key string) map[string]string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// pricingOverrideMapOption mirrors pricing_override_map_option: the whole map
// deserializes or it is "absent" — one bad entry drops every override in the
// section, exactly like a serde parse failure.
func pricingOverrideMapOption(m map[string]any, key string) map[string]ConfigPricingOverride {
	if m == nil {
		return nil
	}
	raw, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]ConfigPricingOverride{}
	for model, value := range raw {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		override, ok := parsePricingOverrideEntry(entry)
		if !ok {
			return nil
		}
		result[model] = override
	}
	return result
}

func parsePricingOverrideEntry(entry map[string]any) (ConfigPricingOverride, bool) {
	var out ConfigPricingOverride
	for key, value := range entry {
		// Unknown fields are skipped without type-checking their values,
		// matching serde's handling of unrecognized keys.
		switch key {
		case "maxInputTokens",
			"inputCostPerToken",
			"outputCostPerToken",
			"cacheCreationInputTokenCost",
			"cacheReadInputTokenCost",
			"inputCostPerTokenAbove200kTokens",
			"outputCostPerTokenAbove200kTokens",
			"cacheCreationInputTokenCostAbove200kTokens",
			"cacheReadInputTokenCostAbove200kTokens",
			"fastMultiplier":
		default:
			continue
		}
		if value == nil {
			continue // JSON null deserializes as None for every Option field
		}
		number, ok := value.(json.Number)
		if !ok {
			return ConfigPricingOverride{}, false
		}
		if key == "maxInputTokens" {
			parsed, err := strconv.ParseUint(number.String(), 10, 64)
			if err != nil {
				return ConfigPricingOverride{}, false
			}
			out.MaxInputTokens = &parsed
			continue
		}
		parsed, err := number.Float64()
		if err != nil {
			return ConfigPricingOverride{}, false
		}
		switch key {
		case "inputCostPerToken":
			out.InputCostPerToken = &parsed
		case "outputCostPerToken":
			out.OutputCostPerToken = &parsed
		case "cacheCreationInputTokenCost":
			out.CacheCreationInputTokenCost = &parsed
		case "cacheReadInputTokenCost":
			out.CacheReadInputTokenCost = &parsed
		case "inputCostPerTokenAbove200kTokens":
			out.InputCostPerTokenAbove200kTokens = &parsed
		case "outputCostPerTokenAbove200kTokens":
			out.OutputCostPerTokenAbove200kTokens = &parsed
		case "cacheCreationInputTokenCostAbove200kTokens":
			out.CacheCreationInputTokenCostAbove200kTokens = &parsed
		case "cacheReadInputTokenCostAbove200kTokens":
			out.CacheReadInputTokenCostAbove200kTokens = &parsed
		case "fastMultiplier":
			out.FastMultiplier = &parsed
		default:
			// Unknown fields are ignored, matching serde's default behavior.
		}
	}
	return out, true
}

// ToCore converts a config override into the pricing-override shape used by
// the pricing map loader.
func (o ConfigPricingOverride) ToCore() core.PricingOverride {
	return core.PricingOverride{
		InputCostPerToken:                          o.InputCostPerToken,
		OutputCostPerToken:                         o.OutputCostPerToken,
		CacheCreationInputTokenCost:                o.CacheCreationInputTokenCost,
		CacheReadInputTokenCost:                    o.CacheReadInputTokenCost,
		InputCostPerTokenAbove200kTokens:           o.InputCostPerTokenAbove200kTokens,
		OutputCostPerTokenAbove200kTokens:          o.OutputCostPerTokenAbove200kTokens,
		CacheCreationInputTokenCostAbove200kTokens: o.CacheCreationInputTokenCostAbove200kTokens,
		CacheReadInputTokenCostAbove200kTokens:     o.CacheReadInputTokenCostAbove200kTokens,
		MaxInputTokens:                             o.MaxInputTokens,
		FastMultiplier:                             o.FastMultiplier,
	}
}

// MergePricingOverrides merges incoming per-model overrides into current at
// the field level: fields the incoming entry sets replace the target's, unset
// fields are preserved (merge_pricing_overrides).
func MergePricingOverrides(current map[string]core.PricingOverride, incoming map[string]ConfigPricingOverride) {
	for model, override := range incoming {
		target := current[model]
		if override.InputCostPerToken != nil {
			target.InputCostPerToken = override.InputCostPerToken
		}
		if override.OutputCostPerToken != nil {
			target.OutputCostPerToken = override.OutputCostPerToken
		}
		if override.CacheCreationInputTokenCost != nil {
			target.CacheCreationInputTokenCost = override.CacheCreationInputTokenCost
		}
		if override.CacheReadInputTokenCost != nil {
			target.CacheReadInputTokenCost = override.CacheReadInputTokenCost
		}
		if override.InputCostPerTokenAbove200kTokens != nil {
			target.InputCostPerTokenAbove200kTokens = override.InputCostPerTokenAbove200kTokens
		}
		if override.OutputCostPerTokenAbove200kTokens != nil {
			target.OutputCostPerTokenAbove200kTokens = override.OutputCostPerTokenAbove200kTokens
		}
		if override.CacheCreationInputTokenCostAbove200kTokens != nil {
			target.CacheCreationInputTokenCostAbove200kTokens = override.CacheCreationInputTokenCostAbove200kTokens
		}
		if override.CacheReadInputTokenCostAbove200kTokens != nil {
			target.CacheReadInputTokenCostAbove200kTokens = override.CacheReadInputTokenCostAbove200kTokens
		}
		if override.MaxInputTokens != nil {
			target.MaxInputTokens = override.MaxInputTokens
		}
		if override.FastMultiplier != nil {
			target.FastMultiplier = override.FastMultiplier
		}
		current[model] = target
	}
}
