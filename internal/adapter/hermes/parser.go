package hermes

import (
	"math"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// hermesEntry mirrors the Rust HermesEntry: one row of the sessions table.
type hermesEntry struct {
	timestamp       int64
	timestampText   string
	sessionID       string
	model           string
	provider        string
	usage           core.TokenUsageRaw
	reasoningTokens uint64
	messageCount    uint64
	costUSD         *float64
}

// sessionColumns is the SELECT list of the reference query, in order.
var sessionColumns = []string{
	"id",
	"model",
	"billing_provider",
	"started_at",
	"message_count",
	"input_tokens",
	"output_tokens",
	"cache_read_tokens",
	"cache_write_tokens",
	"reasoning_tokens",
	"estimated_cost_usd",
	"actual_cost_usd",
}

// columnIndexes maps the SELECT list onto the table's column order.
func columnIndexes(columns []string) ([]int, bool) {
	indexOf := func(name string) (int, bool) {
		for index, candidate := range columns {
			if candidate == name {
				return index, true
			}
		}
		return 0, false
	}
	indexes := make([]int, len(sessionColumns))
	for i, name := range sessionColumns {
		index, ok := indexOf(name)
		if !ok {
			return nil, false
		}
		indexes[i] = index
	}
	return indexes, true
}

// readSessionRow converts one decoded row; ok=false drops the row.
func readSessionRow(record []sqliteValue, indexes []int) (*hermesEntry, bool) {
	column := func(index int) *sqliteValue {
		if index < len(record) {
			return &record[index]
		}
		return nil
	}
	idValue := column(indexes[0])
	if idValue == nil || !idValue.isText {
		return nil, false
	}
	sessionID := string(idValue.text)
	modelValue := column(indexes[1])
	if modelValue == nil || !modelValue.isText {
		return nil, false
	}
	model := strings.TrimSpace(string(modelValue.text))
	if sessionID == "" || model == "" {
		return nil, false
	}
	providerRaw := ""
	if providerValue := column(indexes[2]); providerValue != nil && providerValue.isText {
		providerRaw = string(providerValue.text)
	}
	startedAt, ok := readF64(column(indexes[3]))
	if !ok {
		return nil, false
	}
	timestamp, ok := timestampFromNumber(startedAt)
	if !ok {
		return nil, false
	}
	messageCount := readU64(column(indexes[4]))
	inputTokens := readU64(column(indexes[5]))
	outputTokens := readU64(column(indexes[6]))
	cacheReadTokens := readU64(column(indexes[7]))
	cacheWriteTokens := readU64(column(indexes[8]))
	reasoningTokens := readU64(column(indexes[9]))
	estimatedCost := readNonNegativeF64(column(indexes[10]))
	actualCost := readNonNegativeF64(column(indexes[11]))
	var costUSD *float64
	if actualCost != nil {
		costUSD = actualCost
	} else if estimatedCost != nil {
		costUSD = estimatedCost
	}
	costValue := 0.0
	if costUSD != nil {
		costValue = *costUSD
	}
	if inputTokens == 0 && outputTokens == 0 && cacheReadTokens == 0 &&
		cacheWriteTokens == 0 && reasoningTokens == 0 && costValue == 0 {
		return nil, false
	}
	return &hermesEntry{
		timestamp:     timestamp,
		timestampText: core.FormatRFC3339Millis(timestamp),
		sessionID:     sessionID,
		provider:      normalizeProvider(providerRaw, model),
		model:         model,
		usage: core.TokenUsageRaw{
			InputTokens:              inputTokens,
			OutputTokens:             outputTokens,
			CacheCreationInputTokens: cacheWriteTokens,
			CacheReadInputTokens:     cacheReadTokens,
		},
		reasoningTokens: reasoningTokens,
		messageCount:    messageCount,
		costUSD:         costUSD,
	}, true
}

func readU64(value *sqliteValue) uint64 {
	if value == nil {
		return 0
	}
	if value.isInt {
		if value.int > 0 {
			return uint64(value.int)
		}
		return 0
	}
	if value.isFloat && !math.IsNaN(value.float) && !math.IsInf(value.float, 0) && value.float > 0 {
		return uint64(math.Trunc(value.float))
	}
	return 0
}

func readF64(value *sqliteValue) (float64, bool) {
	if value == nil {
		return 0, false
	}
	if value.isFloat {
		if math.IsNaN(value.float) || math.IsInf(value.float, 0) {
			return 0, false
		}
		return value.float, true
	}
	if value.isInt {
		return float64(value.int), true
	}
	return 0, false
}

func readNonNegativeF64(value *sqliteValue) *float64 {
	parsed, ok := readF64(value)
	if !ok {
		return nil
	}
	if parsed < 0 {
		parsed = 0
	}
	return &parsed
}

func timestampFromNumber(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	millis := value
	if value <= 1e12 {
		millis = value * 1000
	}
	if millis <= 0 {
		return 0, false
	}
	return int64(math.Trunc(millis)), true
}

func normalizeProvider(value, model string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return inferProviderFromModel(model)
	}
	normalized := strings.ReplaceAll(strings.ToLower(trimmed), "-", "_")
	switch normalized {
	case "anthropic", "claude":
		return "anthropic"
	case "openai", "openai_codex":
		return "openai"
	case "google", "google_ai", "gemini", "vertex", "vertex_ai":
		return "google"
	case "openrouter":
		return "openrouter"
	case "xai":
		return "xai"
	case "groq":
		return "groq"
	default:
		return normalized
	}
}

func inferProviderFromModel(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "claude-") || strings.HasPrefix(lower, "claude/"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt") || strings.HasPrefix(lower, "chatgpt") || startsWithOSeries(lower):
		return "openai"
	case strings.HasPrefix(lower, "gemini-") || strings.HasPrefix(lower, "gemini/"):
		return "google"
	default:
		return "hermes"
	}
}

// startsWithOSeries matches o1/o3/o4-style OpenAI model ids.
func startsWithOSeries(model string) bool {
	if len(model) < 2 || model[0] != 'o' {
		return false
	}
	return model[1] >= '0' && model[1] <= '9'
}

func toLoadedEntry(entry *hermesEntry, tz *time.Location, pricing *core.PricingMap) core.LoadedEntry {
	cost := calculateHermesCost(entry, pricing)
	missingPricingModel := missingHermesPricing(entry, pricing)
	sessionID := entry.sessionID
	model := entry.model
	messageID := "hermes:" + entry.sessionID
	usage := entry.usage
	messageCount := entry.messageCount
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: entry.timestampText,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &model,
				ID:    &messageID,
			},
			CostUSD: entry.costUSD,
		},
		Timestamp:        entry.timestamp,
		Date:             core.FormatDateTZ(entry.timestamp, tz),
		Project:          "hermes",
		SessionID:        entry.sessionID,
		ProjectPath:      "Hermes",
		Cost:             cost,
		ExtraTotalTokens: entry.reasoningTokens,
		MessageCount:     &messageCount,
		Model:            &model,
		MissingPricingModel: missingPricingModel,
	}
}

// billableUsage folds reasoning tokens into the output count for pricing.
func billableUsage(entry *hermesEntry) core.TokenUsageRaw {
	usage := entry.usage
	usage.OutputTokens = entry.usage.OutputTokens + entry.reasoningTokens
	return usage
}

func calculateHermesCost(entry *hermesEntry, pricing *core.PricingMap) float64 {
	if entry.costUSD != nil && *entry.costUSD > 0 {
		return *entry.costUSD
	}
	usage := billableUsage(entry)
	for _, candidate := range modelCandidates(entry) {
		model := candidate
		cost := core.CalculateCostForUsage(&model, usage, nil, core.ModeCalculate, pricing)
		if !math.IsNaN(cost) && !math.IsInf(cost, 0) && cost > 0 {
			return cost
		}
	}
	return 0
}

func missingHermesPricing(entry *hermesEntry, pricing *core.PricingMap) *string {
	if entry.costUSD != nil && *entry.costUSD > 0 {
		return nil
	}
	usage := billableUsage(entry)
	return missingPricingModelForCandidates(entry.model, modelCandidates(entry), core.TotalUsageTokens(usage), pricing)
}

func modelCandidates(entry *hermesEntry) []string {
	var candidates []string
	if entry.provider != "hermes" {
		candidates = append(candidates, entry.provider+"/"+entry.model)
	}
	candidates = append(candidates, entry.model)
	seen := map[string]bool{}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	return unique
}

// missingPricingModelForCandidates ports ccusage-core
// missing_pricing_model_for_candidates.
func missingPricingModelForCandidates(model string, candidates []string, totalTokens uint64, pricing *core.PricingMap) *string {
	if totalTokens == 0 || pricing == nil {
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
