package codebuff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// defaultCodebuffModel labels messages that carry no model information.
const defaultCodebuffModel = "codebuff-unknown"

// assistantUsage is the merged usage payload of one assistant message.
type assistantUsage struct {
	Model                    *string
	Credits                  float64
	InputTokens              uint64
	OutputTokens             uint64
	CacheCreationInputTokens uint64
	CacheReadInputTokens     uint64
	ExtraTotalTokens         uint64
}

// codebuffEntry mirrors the Rust CodebuffEntry.
type codebuffEntry struct {
	Timestamp        int64
	TimestampText    string
	SessionID        string
	Model            string
	Provider         string
	Credits          float64
	Usage            core.TokenUsageRaw
	ExtraTotalTokens uint64
	DedupKey         string
}

type codebuffContext struct {
	chatID    string
	sessionID string
}

// LoadChatFile parses one chat-messages.json transcript.
func LoadChatFile(path string) ([]codebuffEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var messages []any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&messages); err != nil {
		return []codebuffEntry{}, nil
	}
	context := deriveContext(path)
	chatTimestamp := parseCodebuffChatIDTimestamp(context.chatID)
	fileTimestamp := int64(0)
	if ts, ok := fileModifiedTimestamp(path); ok {
		fileTimestamp = ts
	}
	var entries []codebuffEntry
	for ordinal, message := range messages {
		record, ok := message.(map[string]any)
		if !ok {
			continue
		}
		if !isAssistantMessage(record) {
			continue
		}
		usage := extractAssistantUsage(record)
		if !hasSignal(&usage) {
			continue
		}
		timestamp := fileTimestamp
		if ts, ok := messageTimestamp(record); ok {
			timestamp = ts
		} else if chatTimestamp != nil {
			timestamp = *chatTimestamp
		}
		model := defaultCodebuffModel
		if usage.Model != nil {
			model = *usage.Model
		}
		dedupKey := dedupKey(record, context.sessionID, timestamp, model, &usage, ordinal)
		entries = append(entries, codebuffEntry{
			Timestamp:     timestamp,
			TimestampText: core.FormatRFC3339Millis(timestamp),
			SessionID:     context.sessionID,
			Provider:      inferProvider(model),
			Model:         model,
			Credits:       usage.Credits,
			Usage: core.TokenUsageRaw{
				InputTokens:              usage.InputTokens,
				OutputTokens:             usage.OutputTokens,
				CacheCreationInputTokens: usage.CacheCreationInputTokens,
				CacheReadInputTokens:     usage.CacheReadInputTokens,
			},
			ExtraTotalTokens: usage.ExtraTotalTokens,
			DedupKey:         dedupKey,
		})
	}
	return entries, nil
}

func deriveContext(path string) codebuffContext {
	dirBase := func(dir string) string {
		base := filepath.Base(dir)
		if base == "" || base == "." || base == string(filepath.Separator) {
			return "unknown"
		}
		return base
	}
	chatDir := filepath.Dir(path)
	chatID := dirBase(chatDir)
	chatsDir := filepath.Dir(chatDir)
	projectDir := filepath.Dir(chatsDir)
	project := dirBase(projectDir)
	channel := "manicode"
	// project_dir.parent().parent().file_name(): the data root directory name.
	channelBase := filepath.Base(filepath.Dir(filepath.Dir(projectDir)))
	if channelBase != "" && channelBase != "." && channelBase != string(filepath.Separator) {
		channel = channelBase
	}
	return codebuffContext{
		sessionID: channel + "/" + project + "/" + chatID,
		chatID:    chatID,
	}
}

func isAssistantMessage(message map[string]any) bool {
	value := stringField(message, "variant")
	if value == "" {
		value = stringField(message, "role")
	}
	switch value {
	case "ai", "agent", "assistant":
		return true
	}
	return false
}

func extractAssistantUsage(message map[string]any) assistantUsage {
	usage := assistantUsage{}
	if metadata, ok := message["metadata"].(map[string]any); ok {
		usage.Model = stringFieldPtr(metadata, "model")
		mergeFallback(&usage, parseUsageObject(metadata["usage"]))
		if codebuff, ok := metadata["codebuff"].(map[string]any); ok {
			mergeFallback(&usage, parseUsageObject(codebuff["usage"]))
		}
		if runStateUsage, ok := extractUsageFromRunState(metadata); ok {
			mergeFallback(&usage, runStateUsage)
		}
	}
	credits := numberField(message, "credits")
	if credits > 0 && usage.Credits <= 0 {
		usage.Credits = credits
	}
	return usage
}

func extractUsageFromRunState(metadata map[string]any) (assistantUsage, bool) {
	runState, ok := metadata["runState"].(map[string]any)
	if !ok {
		return assistantUsage{}, false
	}
	sessionState, ok := runState["sessionState"].(map[string]any)
	if !ok {
		return assistantUsage{}, false
	}
	mainAgentState, ok := sessionState["mainAgentState"].(map[string]any)
	if !ok {
		return assistantUsage{}, false
	}
	history, ok := mainAgentState["messageHistory"].([]any)
	if !ok {
		return assistantUsage{}, false
	}
	usage := assistantUsage{}
	found := false
	for i := len(history) - 1; i >= 0; i-- {
		entry, ok := history[i].(map[string]any)
		if !ok {
			continue
		}
		if stringField(entry, "role") != "assistant" {
			continue
		}
		providerOptions, ok := entry["providerOptions"].(map[string]any)
		if !ok {
			continue
		}
		entryUsage := assistantUsage{}
		mergeFallback(&entryUsage, parseUsageObject(providerOptions["usage"]))
		if codebuff, ok := providerOptions["codebuff"].(map[string]any); ok {
			mergeFallback(&entryUsage, parseUsageObject(codebuff["usage"]))
			if model := stringFieldPtr(codebuff, "model"); model != nil {
				entryUsage.Model = model
			}
		}
		if hasSignal(&entryUsage) || entryUsage.Model != nil {
			found = true
		}
		mergeFallback(&usage, entryUsage)
	}
	return usage, found
}

// ParseUsageObject reads one usage object across its known field spellings.
func ParseUsageObject(value any) assistantUsage {
	var usage assistantUsage
	record, ok := value.(map[string]any)
	if !ok {
		return usage
	}
	usage.InputTokens = pickU64(record, "inputTokens", "input_tokens", "promptTokens", "prompt_tokens")
	usage.OutputTokens = pickU64(record, "outputTokens", "output_tokens", "completionTokens", "completion_tokens")
	usage.CacheReadInputTokens = maxU64(
		pickU64(record, "cacheReadInputTokens", "cache_read_input_tokens"),
		pickNestedU64(record, "promptTokensDetails", "cachedTokens"),
		pickNestedU64(record, "prompt_tokens_details", "cached_tokens"),
	)
	usage.CacheCreationInputTokens = pickU64(record,
		"cacheCreationInputTokens", "cache_creation_input_tokens",
		"cacheCreationTokens", "cache_creation_tokens",
		"cachedTokensCreated", "cached_tokens_created")
	totalTokens := pickU64(record, "totalTokens", "total_tokens", "total")
	rawUsage := core.TokenUsageRaw{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}
	rawUsage, extraTotalTokens := applyTotalTokenFallback(rawUsage, usage.ExtraTotalTokens, totalTokens)
	usage.InputTokens = rawUsage.InputTokens
	usage.OutputTokens = rawUsage.OutputTokens
	usage.CacheCreationInputTokens = rawUsage.CacheCreationInputTokens
	usage.CacheReadInputTokens = rawUsage.CacheReadInputTokens
	usage.ExtraTotalTokens = extraTotalTokens
	usage.Credits = numberField(record, "credits")
	usage.Model = stringFieldPtr(record, "model")
	return usage
}

// parseUsageObject is the internal alias used above.
func parseUsageObject(value any) assistantUsage { return ParseUsageObject(value) }

func mergeFallback(target *assistantUsage, fallback assistantUsage) {
	if target.InputTokens == 0 {
		target.InputTokens = fallback.InputTokens
	}
	if target.OutputTokens == 0 {
		target.OutputTokens = fallback.OutputTokens
	}
	if target.CacheCreationInputTokens == 0 {
		target.CacheCreationInputTokens = fallback.CacheCreationInputTokens
	}
	if target.CacheReadInputTokens == 0 {
		target.CacheReadInputTokens = fallback.CacheReadInputTokens
	}
	if target.ExtraTotalTokens == 0 {
		target.ExtraTotalTokens = fallback.ExtraTotalTokens
	}
	if target.Credits <= 0 {
		target.Credits = fallback.Credits
	}
	if target.Model == nil {
		target.Model = fallback.Model
	}
}

func hasSignal(usage *assistantUsage) bool {
	return usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0 ||
		usage.ExtraTotalTokens > 0 || usage.Credits > 0
}

func messageTimestamp(message map[string]any) (int64, bool) {
	if ts, ok := timestampValue(message["timestamp"]); ok {
		return ts, true
	}
	if ts, ok := timestampValue(message["createdAt"]); ok {
		return ts, true
	}
	if metadata, ok := message["metadata"].(map[string]any); ok {
		if ts, ok := timestampValue(metadata["timestamp"]); ok {
			return ts, true
		}
	}
	return 0, false
}

func parseCodebuffChatIDTimestamp(chatID string) *int64 {
	date, timePart, ok := strings.Cut(chatID, "T")
	if !ok {
		return nil
	}
	for i := 0; i < 2; i++ {
		if index := strings.Index(timePart, "-"); index >= 0 {
			timePart = timePart[:index] + ":" + timePart[index+1:]
		}
	}
	if timestamp, ok := core.ParseTSTimestamp(date + "T" + timePart); ok {
		return &timestamp
	}
	return nil
}

func timestampValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case string:
		if ts, ok := core.ParseTSTimestamp(typed); ok {
			return ts, true
		}
		return 0, false
	case json.Number:
		raw, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		millis := raw
		if raw < 10_000_000_000 {
			millis = raw * 1000
		}
		if millis > 0 {
			return millis, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func fileModifiedTimestamp(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.ModTime().UnixNano() / int64(1e6), true
}

func dedupKey(message map[string]any, sessionID string, timestamp int64, model string, usage *assistantUsage, ordinal int) string {
	if id := stringField(message, "id"); id != "" {
		return "codebuff:" + sessionID + ":" + id
	}
	return fmt.Sprintf("codebuff:%s:%s:%s:%d:%d:%d:%d:%d:%d",
		sessionID, core.FormatRFC3339Millis(timestamp), model, ordinal,
		usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens,
		usage.CacheCreationInputTokens, usage.ExtraTotalTokens)
}

func inferProvider(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "claude-") || strings.HasPrefix(lower, "anthropic/") || strings.HasPrefix(lower, "anthropic."):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt-") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") || strings.HasPrefix(lower, "openai/"):
		return "openai"
	case strings.HasPrefix(lower, "gemini") || strings.HasPrefix(lower, "google/"):
		return "google"
	case strings.HasPrefix(lower, "grok") || strings.HasPrefix(lower, "xai/"):
		return "xai"
	case strings.HasPrefix(lower, "openrouter/"):
		return "openrouter"
	default:
		return "unknown"
	}
}

func calculateCodebuffCost(entry *codebuffEntry, pricing *core.PricingMap) float64 {
	usage := entry.Usage
	usage.OutputTokens = saturatingAdd(entry.Usage.OutputTokens, entry.ExtraTotalTokens)
	model := entry.Model
	raw := core.CalculateCostForUsage(&model, usage, nil, core.ModeCalculate, pricing)
	if raw > 0 || entry.Provider == "unknown" || strings.HasPrefix(entry.Model, entry.Provider+"/") {
		return raw
	}
	qualified := entry.Provider + "/" + entry.Model
	return core.CalculateCostForUsage(&qualified, usage, nil, core.ModeCalculate, pricing)
}

func missingCodebuffPricing(entry *codebuffEntry, pricing *core.PricingMap) *string {
	usage := entry.Usage
	usage.OutputTokens = saturatingAdd(entry.Usage.OutputTokens, entry.ExtraTotalTokens)
	candidates := []string{entry.Model}
	if entry.Provider != "unknown" && !strings.HasPrefix(entry.Model, entry.Provider+"/") {
		candidates = append(candidates, entry.Provider+"/"+entry.Model)
	}
	return missingPricingModelForCandidates(entry.Model, candidates, core.TotalUsageTokens(usage), pricing)
}

func objectField(record map[string]any, key string) (map[string]any, bool) {
	object, ok := record[key].(map[string]any)
	return object, ok
}

func stringField(record map[string]any, key string) string {
	if value := stringFieldPtr(record, key); value != nil {
		return *value
	}
	return ""
}

func stringFieldPtr(record map[string]any, key string) *string {
	value, ok := record[key].(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func numberField(record map[string]any, key string) float64 {
	value, ok := record[key].(json.Number)
	if !ok {
		return 0
	}
	parsed, err := value.Float64()
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 {
		return 0
	}
	return parsed
}

func pickU64(record map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		if number, ok := record[key].(json.Number); ok {
			if parsed, err := number.Int64(); err == nil && parsed > 0 {
				return uint64(parsed)
			}
		}
	}
	return 0
}

func pickNestedU64(record map[string]any, key string, keys ...string) uint64 {
	nested, ok := objectField(record, key)
	if !ok {
		return 0
	}
	return pickU64(nested, keys...)
}

func maxU64(values ...uint64) uint64 {
	var best uint64
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func saturatingAdd(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return math.MaxUint64
	}
	return sum
}

// applyTotalTokenFallback ports ccusage-core apply_total_token_fallback.
func applyTotalTokenFallback(usage core.TokenUsageRaw, extraTotalTokens, totalTokens uint64) (core.TokenUsageRaw, uint64) {
	knownTokens := core.TotalUsageTokens(usage) + extraTotalTokens
	var missingTokens uint64
	if totalTokens > knownTokens {
		missingTokens = totalTokens - knownTokens
	}
	if missingTokens == 0 {
		return usage, extraTotalTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = missingTokens
	} else {
		extraTotalTokens += missingTokens
	}
	return usage, extraTotalTokens
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
