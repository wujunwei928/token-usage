package droid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/wujunwei928/token-usage/internal/core"
)

// droidEntry mirrors the Rust DroidEntry: one parsed settings snapshot.
type droidEntry struct {
	Timestamp     int64
	TimestampText string
	SessionID     string
	Model         string
	Provider      string
	Usage         core.TokenUsageRaw
	ReasoningToks uint64
}

// droidTokenUsage carries the Droid-specific token buckets.
type droidTokenUsage struct {
	inputTokens         uint64
	outputTokens        uint64
	cacheCreationTokens uint64
	cacheReadTokens     uint64
	thinkingTokens      uint64
}

// loadSettingsFile parses one <session>.settings.json file; a nil entry means
// the file carries no usage signal.
func loadSettingsFile(path string) (*droidEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("failed to parse Droid settings %s: %w", path, err)
	}
	settings, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	usage, ok := parseTokenUsage(settings["tokenUsage"])
	if !ok {
		return nil, nil
	}
	provider := normalizeDroidProvider(stringField(settings, "providerLock"))
	model := stringField(settings, "model")
	if model != "" {
		model = NormalizeDroidModelName(model)
	} else if sidecar, err := extractModelFromSidecarJSONL(path); err == nil && sidecar != "" {
		model = sidecar
	} else {
		model = defaultModelFromProvider(provider)
	}
	if model == "" {
		model = defaultModelFromProvider(provider)
	}
	if provider == "unknown" {
		provider = inferDroidProviderFromModel(model)
	}
	timestamp, timestampText, ok := settingsTimestamp(settings, path)
	if !ok {
		return nil, nil
	}
	sessionID := "unknown"
	if base := filepath.Base(path); strings.HasSuffix(base, ".settings.json") {
		sessionID = strings.TrimSuffix(base, ".settings.json")
	}
	return &droidEntry{
		Timestamp:     timestamp,
		TimestampText: timestampText,
		SessionID:     sessionID,
		Model:         model,
		Provider:      provider,
		Usage: core.TokenUsageRaw{
			InputTokens:              usage.inputTokens,
			OutputTokens:             usage.outputTokens,
			CacheCreationInputTokens: usage.cacheCreationTokens,
			CacheReadInputTokens:     usage.cacheReadTokens,
		},
		ReasoningToks: usage.thinkingTokens,
	}, nil
}

// parseTokenUsage reads the tokenUsage object; ok=false when every bucket is
// zero (no signal).
func parseTokenUsage(value any) (droidTokenUsage, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return droidTokenUsage{}, false
	}
	rawUsage := core.TokenUsageRaw{
		InputTokens:              jsonValueU64(record["inputTokens"]),
		OutputTokens:             jsonValueU64(record["outputTokens"]),
		CacheCreationInputTokens: jsonValueU64(record["cacheCreationTokens"]),
		CacheReadInputTokens:     jsonValueU64(record["cacheReadTokens"]),
	}
	thinkingTokens := jsonValueU64(record["thinkingTokens"])
	totalTokens := jsonValueU64(record["totalTokens"])
	rawUsage, extraTokens := applyTotalTokenFallback(rawUsage, thinkingTokens, totalTokens)
	usage := droidTokenUsage{
		inputTokens:         rawUsage.InputTokens,
		outputTokens:        rawUsage.OutputTokens,
		cacheCreationTokens: rawUsage.CacheCreationInputTokens,
		cacheReadTokens:     rawUsage.CacheReadInputTokens,
		thinkingTokens:      extraTokens,
	}
	hasSignal := usage.inputTokens+usage.outputTokens+usage.cacheCreationTokens+
		usage.cacheReadTokens+usage.thinkingTokens > 0
	return usage, hasSignal
}

func settingsTimestamp(settings map[string]any, path string) (int64, string, bool) {
	if text := stringField(settings, "providerLockTimestamp"); text != "" {
		if timestamp, ok := core.ParseTSTimestamp(text); ok {
			return timestamp, core.FormatRFC3339Millis(timestamp), true
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", false
	}
	timestamp := info.ModTime().UnixNano() / int64(time.Millisecond)
	return timestamp, core.FormatRFC3339Millis(timestamp), true
}

func calculateDroidCost(entry *droidEntry, pricing *core.PricingMap) float64 {
	usage := entry.Usage
	usage.OutputTokens = entry.Usage.OutputTokens + entry.ReasoningToks
	for _, candidate := range droidModelCandidates(entry) {
		model := candidate
		cost := core.CalculateCostForUsage(&model, usage, nil, core.ModeCalculate, pricing)
		if cost > 0 {
			return cost
		}
	}
	return 0
}

func missingDroidPricing(entry *droidEntry, pricing *core.PricingMap) *string {
	usage := entry.Usage
	usage.OutputTokens = entry.Usage.OutputTokens + entry.ReasoningToks
	return missingPricingModelForCandidates(entry.Model, droidModelCandidates(entry), core.TotalUsageTokens(usage), pricing)
}

func droidModelCandidates(entry *droidEntry) []string {
	var candidates []string
	candidates = append(candidates, entry.Model)
	for _, prefix := range providerPrefixes(entry.Provider) {
		candidates = append(candidates, prefix+entry.Model)
	}
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

func providerPrefixes(provider string) []string {
	switch provider {
	case "anthropic":
		return []string{"anthropic/", "openrouter/anthropic/"}
	case "openai":
		return []string{"openai/", "openrouter/openai/"}
	case "google":
		return []string{"google/", "vertex_ai/", "openrouter/google/"}
	case "xai":
		return []string{"xai/", "openrouter/x-ai/"}
	case "unknown":
		return nil
	default:
		return []string{provider + "/", "openrouter/" + provider + "/"}
	}
}

func stringField(record map[string]any, key string) string {
	value, ok := record[key].(string)
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	return trimmed
}

// NormalizeDroidModelName canonicalizes Droid's model labels.
func NormalizeDroidModelName(model string) string {
	raw := strings.TrimPrefix(model, "custom:")
	var withoutBrackets strings.Builder
	bracketDepth := 0
	for _, ch := range raw {
		switch ch {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		default:
			if bracketDepth == 0 {
				withoutBrackets.WriteRune(ch)
			}
		}
	}
	lower := asciiLower(strings.TrimRight(strings.TrimSpace(withoutBrackets.String()), "-"))
	var normalized strings.Builder
	previousDash := false
	for _, ch := range lower {
		next := ch
		if ch == '.' || unicode.IsSpace(ch) || ch == '-' {
			next = '-'
		}
		if next == '-' {
			if !previousDash {
				normalized.WriteRune('-')
				previousDash = true
			}
		} else {
			normalized.WriteRune(next)
			previousDash = false
		}
	}
	return strings.Trim(normalized.String(), "-")
}

// asciiLower mirrors Rust to_ascii_lowercase: non-ASCII bytes are untouched.
func asciiLower(value string) string {
	out := []byte(value)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + ('a' - 'A')
		}
	}
	return string(out)
}

func normalizeDroidProvider(value string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	switch normalized {
	case "":
		return "unknown"
	case "claude", "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "google", "google_ai", "gemini", "vertex", "vertex_ai":
		return "google"
	case "xai", "x_ai", "grok":
		return "xai"
	default:
		return normalized
	}
}

func inferDroidProviderFromModel(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "claude") || strings.Contains(lower, "opus") ||
		strings.Contains(lower, "sonnet") || strings.Contains(lower, "haiku"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt-") || strings.Contains(lower, "-gpt-") ||
		strings.Contains(lower, "chatgpt") || startsWithOSeries(lower):
		return "openai"
	case strings.Contains(lower, "gemini"):
		return "google"
	case strings.Contains(lower, "grok"):
		return "xai"
	default:
		return "unknown"
	}
}

// startsWithOSeries matches o1/o3/o4-style OpenAI model ids.
func startsWithOSeries(model string) bool {
	if len(model) < 2 || model[0] != 'o' {
		return false
	}
	return model[1] >= '0' && model[1] <= '9'
}

func defaultModelFromProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-unknown"
	case "openai":
		return "gpt-unknown"
	case "google":
		return "gemini-unknown"
	case "xai":
		return "grok-unknown"
	default:
		return "unknown"
	}
}

func extractModelFromSidecarJSONL(settingsPath string) (string, error) {
	base := filepath.Base(settingsPath)
	prefix, ok := strings.CutSuffix(base, ".settings.json")
	if !ok {
		return "", nil
	}
	sidecar := filepath.Join(filepath.Dir(settingsPath), prefix+".jsonl")
	content, err := os.ReadFile(sidecar)
	if err != nil {
		return "", nil
	}
	lines := bytes.Split(content, []byte("\n"))
	limit := len(lines)
	if limit > 500 {
		limit = 500
	}
	for i := 0; i < limit; i++ {
		if model, ok := extractDroidModelFromLine(string(lines[i])); ok {
			return model, nil
		}
	}
	return "", nil
}

func extractDroidModelFromLine(line string) (string, bool) {
	index := strings.Index(line, "Model:")
	if index < 0 {
		return "", false
	}
	tail := line[index+len("Model:"):]
	cutset := func(r rune) bool { return r == '"' || r == '\\' || r == '[' }
	segment := tail
	if end := strings.IndexFunc(tail, cutset); end >= 0 {
		segment = tail[:end]
	}
	raw := strings.TrimRight(segment, " \t\n\r")
	if raw == "" {
		return "", false
	}
	normalized := NormalizeDroidModelName(raw)
	if normalized == "" {
		return "", false
	}
	return normalized, true
}

// jsonValueU64 mirrors Rust json_value_u64: only JSON integers map to u64.
func jsonValueU64(value any) uint64 {
	number, ok := value.(json.Number)
	if !ok {
		return 0
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 {
		return 0
	}
	return uint64(parsed)
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
