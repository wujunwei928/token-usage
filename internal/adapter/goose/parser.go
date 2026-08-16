package goose

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
)

// gooseRow is one row of the Goose sessions query. Column order matches
// GOOSE_SESSION_QUERY in loader.go.
type gooseRow struct {
	ID           string
	ModelConfig  string
	ProviderName *string
	CreatedAt    string
	TotalTokens  tokenValue
	InputTokens  tokenValue
	OutputTokens tokenValue
	AccumTotal   tokenValue
	AccumInput   tokenValue
	AccumOutput  tokenValue
}

// tokenValue is an optional i64 column (NULL or non-integer storage reads as
// absent, like the sqlite crate's read::<i64> error).
type tokenValue struct {
	value int64
	ok    bool
}

// gooseModelConfig holds the per-session model selection JSON blob; only the
// human-readable model name is consumed.
type gooseModelConfig struct {
	ModelName *string `json:"model_name"`
}

// rowToEntry turns one sessions row into a loaded entry; nil rows are skipped
// like the reference's early returns.
func rowToEntry(row *gooseRow, tz *time.Location, pricing *core.PricingMap) *core.LoadedEntry {
	timestamp, ok := parseGooseTimestamp(row.CreatedAt)
	if !ok {
		return nil
	}
	model := parseGooseModelConfig(row.ModelConfig)
	if model == nil {
		return nil
	}

	inputTokens := firstToken(row.AccumInput, row.InputTokens)
	outputTokens := firstToken(row.AccumOutput, row.OutputTokens)
	totalTokens := firstToken(row.AccumTotal, row.TotalTokens, positiveToken(uint64(sumInt64(int64(inputTokens), int64(outputTokens)))))
	if inputTokens == 0 && outputTokens == 0 && totalTokens == 0 {
		return nil
	}

	// reasoning = total - (input + output), saturating at zero.
	known := sumInt64(int64(inputTokens), int64(outputTokens))
	var reasoningTokens uint64
	if int64(totalTokens) > known {
		reasoningTokens = uint64(int64(totalTokens) - known)
	}

	providerID := normalizeProvider(row.ProviderName, *model)
	usage := core.TokenUsageRaw{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	timestampText := core.FormatRFC3339Millis(timestamp)
	entryID := row.ID
	data := core.UsageEntry{
		SessionID: &entryID,
		Timestamp: timestampText,
		Message: core.UsageMessage{
			Usage: usage,
			Model: model,
			ID:    &entryID,
		},
	}
	cost := calculateGooseCost(*model, providerID, usage, reasoningTokens, pricing)
	missingPricingModel := missingGoosePricing(*model, providerID, usage, reasoningTokens, pricing)
	return &core.LoadedEntry{
		Date:                core.FormatDateTZ(timestamp, tz),
		Timestamp:           timestamp,
		Project:             "goose",
		SessionID:           entryID,
		ProjectPath:         "Goose",
		Cost:                cost,
		ExtraTotalTokens:    reasoningTokens,
		Model:               model,
		MissingPricingModel: missingPricingModel,
		Data:                data,
	}
}

// firstToken returns the first positive value among candidates (read_token_value
// filters values <= 0).
func firstToken(candidates ...tokenValue) uint64 {
	for _, candidate := range candidates {
		if candidate.ok && candidate.value > 0 {
			return uint64(candidate.value)
		}
	}
	return 0
}

func positiveToken(value uint64) tokenValue {
	if value == 0 {
		return tokenValue{}
	}
	return tokenValue{value: int64(value), ok: true}
}

func sumInt64(a, b int64) int64 {
	sum := a + b
	if (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum > 0) {
		if sum < 0 {
			return int64(^uint64(0) >> 1)
		}
		return -int64(^uint64(0)>>1) - 1
	}
	return sum
}

// parseGooseModelConfig extracts the model name from the model config blob;
// unparsable blobs and missing names yield nil.
func parseGooseModelConfig(value string) *string {
	var config gooseModelConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return nil
	}
	if config.ModelName == nil || strings.TrimSpace(*config.ModelName) == "" {
		return nil
	}
	name := strings.TrimSpace(*config.ModelName)
	return &name
}

// parseGooseTimestamp accepts unix seconds/millis, RFC3339-ish strings,
// "YYYY-MM-DD HH:MM:SS" (space or T), and bare dates.
func parseGooseTimestamp(value string) (int64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	if number, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		millis := number
		if number <= 1_000_000_000_000 {
			millis = number * 1000
		}
		return millis, millis > 0
	}
	if timestamp, ok := core.ParseTSTimestamp(trimmed); ok {
		return timestamp, true
	}
	if len(trimmed) == 19 && trimmed[4] == '-' && trimmed[7] == '-' && (trimmed[10] == ' ' || trimmed[10] == 'T') {
		return core.ParseTSTimestamp(trimmed[:10] + "T" + trimmed[11:] + "Z")
	}
	if len(trimmed) == 10 && trimmed[4] == '-' && trimmed[7] == '-' {
		return core.ParseTSTimestamp(trimmed + "T00:00:00Z")
	}
	return 0, false
}

// normalizeProvider maps the session provider (or model prefix heuristics) to
// a pricing namespace.
func normalizeProvider(provider *string, model string) string {
	if provider != nil {
		trimmed := strings.TrimSpace(*provider)
		if trimmed != "" {
			return strings.ReplaceAll(trimmed, "-", "_")
		}
	}
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "chatgpt-") || strings.HasPrefix(model, "o"):
		return "openai"
	case strings.HasPrefix(model, "gemini-"):
		return "google"
	case strings.HasPrefix(strings.ToLower(model), "qwen"):
		return "openrouter"
	default:
		return "goose"
	}
}

// calculateGooseCost prices the row (mode-independent: Goose never records a
// display cost), folding reasoning tokens into the output count. When the bare
// model does not resolve (and the provider is real), provider/model is tried.
func calculateGooseCost(model, providerID string, usage core.TokenUsageRaw, reasoningTokens uint64, pricing *core.PricingMap) float64 {
	costUsage := usage
	costUsage.OutputTokens = saturatingAddU64(usage.OutputTokens, reasoningTokens)
	raw := core.CalculateCostForUsage(&model, costUsage, nil, core.ModeCalculate, pricing)
	if raw > 0 || providerID == "goose" {
		return raw
	}
	candidate := providerID + "/" + model
	return core.CalculateCostForUsage(&candidate, costUsage, nil, core.ModeCalculate, pricing)
}

// missingGoosePricing reports the model when neither the bare model nor the
// provider-qualified name carries pricing.
func missingGoosePricing(model, providerID string, usage core.TokenUsageRaw, reasoningTokens uint64, pricing *core.PricingMap) *string {
	costUsage := usage
	costUsage.OutputTokens = saturatingAddU64(usage.OutputTokens, reasoningTokens)
	candidates := []string{model}
	if providerID != "goose" {
		candidates = append(candidates, providerID+"/"+model)
	}
	return missingPricingModelForCandidates(model, candidates, core.TotalUsageTokens(costUsage), pricing)
}

// missingPricingModelForCandidates ports the core helper: the resolved model
// name is reported only when every candidate lacks pricing.
func missingPricingModelForCandidates(model string, candidates []string, totalTokens uint64, pricing *core.PricingMap) *string {
	if totalTokens == 0 {
		return nil
	}
	if pricing == nil {
		return nil
	}
	for _, candidate := range candidates {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

func saturatingAddU64(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return ^uint64(0)
	}
	return sum
}
