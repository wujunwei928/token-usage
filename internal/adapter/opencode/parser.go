package opencode

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/wujunwei928/token-usage/internal/core"
)

// OpenCodeMessage is one parsed OpenCode message. Every field uses the
// reference's lenient navigation rules: a wrongly typed field is treated as
// absent instead of failing the record.
type OpenCodeMessage struct {
	Tokens     *OpenCodeTokens
	ModelID    *string
	ProviderID *string
	Time       *OpenCodeTime
	ID         *string
	SessionID  *string
	Cost       *float64
}

// OpenCodeTokens is the token usage block carried by OpenCode messages.
type OpenCodeTokens struct {
	Input  uint64
	Output uint64
	Cache  *OpenCodeCache
	Total  uint64
}

// OpenCodeCache holds the cache read/write counts nested under tokens.
type OpenCodeCache struct {
	Read  uint64
	Write uint64
}

// OpenCodeTime carries the creation timestamp block.
type OpenCodeTime struct {
	Created *int64
}

// ParseOpenCodeMessage decodes one message payload. It returns nil for
// anything that is not a JSON object, mirroring
// serde_json::from_str::<OpenCodeMessage>.
func ParseOpenCodeMessage(data []byte) *OpenCodeMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil
	}
	msg := &OpenCodeMessage{}
	if raw, ok := fields["tokens"]; ok {
		msg.Tokens = parseTokens(raw)
	}
	msg.ModelID = nonEmptyString(fields["modelID"])
	msg.ProviderID = nonEmptyString(fields["providerID"])
	if raw, ok := fields["time"]; ok {
		msg.Time = parseTime(raw)
	}
	msg.ID = nonEmptyString(fields["id"])
	msg.SessionID = nonEmptyString(fields["sessionID"])
	if raw, ok := fields["cost"]; ok {
		msg.Cost = lenientF64(raw)
	}
	return msg
}

// lenientObject decodes a nested object; nil for anything that is not a JSON
// object (mirrors jsonl::lenient_object).
func lenientObject(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil
	}
	return fields
}

func parseTokens(raw json.RawMessage) *OpenCodeTokens {
	fields := lenientObject(raw)
	if fields == nil {
		return nil
	}
	return &OpenCodeTokens{
		Input:  lenientU64(fields["input"]),
		Output: lenientU64(fields["output"]),
		Cache:  parseCache(fields["cache"]),
		Total:  lenientU64(fields["total"]),
	}
}

func parseCache(raw json.RawMessage) *OpenCodeCache {
	fields := lenientObject(raw)
	if fields == nil {
		return nil
	}
	return &OpenCodeCache{
		Read:  lenientU64(fields["read"]),
		Write: lenientU64(fields["write"]),
	}
}

func parseTime(raw json.RawMessage) *OpenCodeTime {
	fields := lenientObject(raw)
	if fields == nil {
		return nil
	}
	return &OpenCodeTime{Created: lenientI64(fields["created"])}
}

// nonEmptyString mirrors jsonl::non_empty_string: only JSON strings, trimmed,
// non-empty.
func nonEmptyString(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	trimmed := strings.TrimFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// lenientU64 mirrors jsonl::lenient_u64: only unsigned integers in u64 range;
// everything else is 0.
func lenientU64(raw json.RawMessage) uint64 {
	if len(raw) == 0 || raw[0] == '-' || raw[0] == '+' {
		return 0
	}
	for _, b := range raw {
		if b < '0' || b > '9' {
			return 0
		}
	}
	value, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// lenientI64 mirrors jsonl::lenient_i64: only integers in i64 range.
func lenientI64(raw json.RawMessage) *int64 {
	if len(raw) == 0 {
		return nil
	}
	body := raw
	negative := false
	if body[0] == '-' {
		negative = true
		body = body[1:]
	}
	if len(body) == 0 {
		return nil
	}
	for _, b := range body {
		if b < '0' || b > '9' {
			return nil
		}
	}
	if !negative && len(body) > 1 && body[0] == '0' {
		return nil
	}
	if negative && len(body) > 1 && body[0] == '0' {
		return nil
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// lenientF64 mirrors jsonl::lenient_f64: any JSON number.
func lenientF64(raw json.RawMessage) *float64 {
	if len(raw) == 0 {
		return nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return &value
}

// applyTotalTokenFallback ports ccusage-core's apply_total_token_fallback:
// tokens missing from the parts but present in the total land in output when
// output is unknown, otherwise they accumulate as extras.
func applyTotalTokenFallback(usage core.TokenUsageRaw, extraTotalTokens, totalTokens uint64) (core.TokenUsageRaw, uint64) {
	known := core.TotalUsageTokens(usage) + extraTotalTokens
	if totalTokens <= known {
		return usage, extraTotalTokens
	}
	missing := totalTokens - known
	if usage.OutputTokens == 0 {
		usage.OutputTokens = missing
		return usage, extraTotalTokens
	}
	extraTotalTokens += missing
	return usage, extraTotalTokens
}

// MessageValueToEntry converts one parsed message into a LoadedEntry; nil when
// the message carries no usable usage, model, or provider.
func MessageValueToEntry(
	value *OpenCodeMessage,
	id, sessionID *string,
	tz *time.Location,
	mode core.CostMode,
	pricing *core.PricingMap,
) *core.LoadedEntry {
	if value.Tokens == nil {
		return nil
	}
	tokens := value.Tokens
	usage := core.TokenUsageRaw{
		InputTokens:              tokens.Input,
		OutputTokens:             tokens.Output,
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
	}
	if tokens.Cache != nil {
		usage.CacheCreationInputTokens = tokens.Cache.Write
		usage.CacheReadInputTokens = tokens.Cache.Read
	}
	usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, tokens.Total)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 &&
		usage.CacheCreationInputTokens == 0 && usage.CacheReadInputTokens == 0 &&
		extraTotalTokens == 0 {
		return nil
	}
	if value.ModelID == nil {
		return nil
	}
	if value.ProviderID == nil {
		return nil
	}
	model := *value.ModelID
	provider := *value.ProviderID

	var millis int64
	if value.Time != nil && value.Time.Created != nil {
		millis = *value.Time.Created
	}
	timestampText := core.FormatRFC3339Millis(millis)

	messageID := id
	if messageID == nil {
		messageID = value.ID
	}
	session := sessionID
	if session == nil {
		session = value.SessionID
	}
	data := core.UsageEntry{
		SessionID: session,
		Timestamp: timestampText,
		Message: core.UsageMessage{
			Usage: usage,
			Model: &model,
			ID:    messageID,
		},
		CostUSD: value.Cost,
	}

	costUsage := usage
	costUsage.OutputTokens += extraTotalTokens
	cost := calculateOpenCodeCost(model, provider, costUsage, data.CostUSD, pricing)
	// A usable positive cost means the candidates already resolved, so the
	// redundant missing-pricing scan cannot find anything new.
	var missingPricingModel *string
	if !(cost > 0) {
		missingPricingModel = missingOpenCodePricing(model, provider, costUsage, data.CostUSD, mode, pricing)
	}

	loadedSessionID := "unknown"
	if session != nil {
		loadedSessionID = *session
	}
	return &core.LoadedEntry{
		Data:                data,
		Timestamp:           millis,
		Date:                core.FormatDateTZ(millis, tz),
		Project:             "opencode",
		SessionID:           loadedSessionID,
		ProjectPath:         "OpenCode",
		Cost:                cost,
		ExtraTotalTokens:    extraTotalTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

// calculateOpenCodeCost prices one message: a stored positive cost wins, then
// the model candidates are priced from tokens (mode is irrelevant here — the
// loader passes no pricing in display mode, which already zeroes this path).
func calculateOpenCodeCost(model, provider string, usage core.TokenUsageRaw, costUSD *float64, pricing *core.PricingMap) float64 {
	if costUSD != nil && *costUSD > 0 {
		return *costUSD
	}
	for _, candidate := range openCodeModelCandidates(model, provider) {
		cost := core.CalculateCostForUsage(&candidate, usage, nil, core.ModeCalculate, pricing)
		if cost > 0 {
			return cost
		}
	}
	return 0
}

// missingOpenCodePricing reports the model name when every candidate lacked
// pricing and the cost had to be computed from tokens.
func missingOpenCodePricing(model, provider string, usage core.TokenUsageRaw, costUSD *float64, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay || (costUSD != nil && *costUSD > 0) {
		return nil
	}
	if pricing == nil {
		return nil
	}
	total := core.TotalUsageTokens(usage)
	if total == 0 {
		return nil
	}
	for _, candidate := range openCodeModelCandidates(model, provider) {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

// openCodeModelCandidates lists the pricing keys tried for one model: the
// resolved name, its dash-normalized claude variant, then each under
// provider/ with '-' turned into '_'.
func openCodeModelCandidates(model, provider string) []string {
	resolved := resolveOpenCodeModelName(model)
	normalized := normalizeOpenCodeModelName(resolved)
	base := []string{resolved}
	if normalized != resolved {
		base = append(base, normalized)
	}
	candidates := append([]string{}, base...)
	if provider != "unknown" {
		withUnderscores := strings.ReplaceAll(provider, "-", "_")
		for _, m := range base {
			candidates = append(candidates, withUnderscores+"/"+m)
		}
	}
	// Rust's Vec::dedup removes consecutive duplicates only.
	deduped := candidates[:0]
	for i, candidate := range candidates {
		if i == 0 || candidate != candidates[i-1] {
			deduped = append(deduped, candidate)
		}
	}
	return deduped
}

// resolveOpenCodeModelName maps OpenCode model aliases onto their pricing
// names.
func resolveOpenCodeModelName(model string) string {
	switch model {
	case "gemini-3-pro-high":
		return "gemini-3-pro-preview"
	case "k2p6":
		return "kimi-k2.6"
	default:
		return model
	}
}

// normalizeOpenCodeModelName rewrites claude family versions like
// "claude-sonnet-4.5" (or "claude-sonnet-45...") into the dashed form pricing
// keys use ("claude-sonnet-4-5").
func normalizeOpenCodeModelName(model string) string {
	for _, family := range []string{"claude-haiku-", "claude-opus-", "claude-sonnet-"} {
		rest, ok := strings.CutPrefix(model, family)
		if !ok {
			continue
		}
		// A "major.minor..." split with numeric major and digit-leading minor
		// becomes "major-minor...".
		if major, minorAndSuffix, found := strings.Cut(rest, "."); found &&
			allASCIIDigits(major) && minorAndSuffix != "" && isASCIIDigit(minorAndSuffix[0]) {
			return family + major + "-" + minorAndSuffix
		}
		// Without a usable dot split, two leading digits still split:
		// "45x" -> "4-5x".
		if len(rest) >= 2 && isASCIIDigit(rest[0]) && isASCIIDigit(rest[1]) {
			return family + string(rest[0]) + "-" + rest[1:]
		}
		return model
	}
	return model
}

func allASCIIDigits(s string) bool {
	// Rust's chars().all() is vacuously true for "" (e.g. "claude-sonnet-.5"
	// normalizes to "claude-sonnet--5"), so no empty rejection here.
	for _, b := range []byte(s) {
		if !isASCIIDigit(b) {
			return false
		}
	}
	return true
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
