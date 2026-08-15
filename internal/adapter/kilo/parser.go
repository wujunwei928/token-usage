package kilo

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// KiloMessage is one parsed Kilo message row payload; fields ccusage does not
// consume are skipped, and unexpected field types degrade leniently.
type KiloMessage struct {
	Role       *string
	Tokens     *KiloTokens
	ModelID    *string
	Time       *KiloTime
	SessionID  *string
	ID         *string
	Cost       *float64
	ProviderID *string
}

// KiloTokens is the token usage block carried by assistant messages.
type KiloTokens struct {
	Input     uint64
	Output    uint64
	Cache     *KiloCache
	Reasoning uint64
	Total     uint64
}

// KiloCache holds the read/write counts nested under token usage.
type KiloCache struct {
	Read  uint64
	Write uint64
}

// KiloTime carries the creation timestamp block.
type KiloTime struct {
	Created *int64
}

// parseKiloMessage decodes one message payload; ok=false mirrors silently
// skipped unparsable rows.
func parseKiloMessage(data string) (*KiloMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		return nil, false
	}
	message := &KiloMessage{
		ModelID:    nonEmptyString(field(fields, "modelID")),
		Time:       parseKiloTime(field(fields, "time")),
		SessionID:  nonEmptyString(field(fields, "session_id")),
		ID:         nonEmptyString(field(fields, "id")),
		ProviderID: nonEmptyString(field(fields, "providerID")),
	}
	// role is a plain Option<String>: a non-string value fails the record.
	if roleRaw := field(fields, "role"); roleRaw != nil {
		var role string
		if err := json.Unmarshal(roleRaw, &role); err != nil {
			return nil, false
		}
		message.Role = &role
	}
	if cost := lenientF64(field(fields, "cost")); cost != nil {
		message.Cost = cost
	}
	if tokensRaw := field(fields, "tokens"); tokensRaw != nil {
		tokens, ok := parseKiloTokens(tokensRaw)
		if !ok {
			return nil, false
		}
		message.Tokens = tokens
	}
	return message, true
}

// parseKiloTokens decodes the tokens block leniently (non-object payloads
// become absent rather than failing the record).
func parseKiloTokens(raw json.RawMessage) (*KiloTokens, bool) {
	fields, ok := rawFields(raw)
	if !ok {
		return nil, true
	}
	tokens := &KiloTokens{
		Input:     lenientU64(field(fields, "input")),
		Output:    lenientU64(field(fields, "output")),
		Reasoning: lenientU64(field(fields, "reasoning")),
		Total:     lenientU64(field(fields, "total")),
	}
	if cacheRaw := field(fields, "cache"); cacheRaw != nil && isJSONObject(cacheRaw) {
		var cacheFields map[string]json.RawMessage
		if err := json.Unmarshal(cacheRaw, &cacheFields); err == nil {
			tokens.Cache = &KiloCache{
				Read:  lenientU64(field(cacheFields, "read")),
				Write: lenientU64(field(cacheFields, "write")),
			}
		}
	}
	return tokens, true
}

func parseKiloTime(raw json.RawMessage) *KiloTime {
	if raw == nil || !isJSONObject(raw) {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return &KiloTime{Created: lenientI64(field(fields, "created"))}
}

// MessageValueToEntry turns one parsed payload into a loaded entry; nil rows
// are skipped like the reference's early returns.
func MessageValueToEntry(value *KiloMessage, rowID, rowSessionID, dbPath string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) *core.LoadedEntry {
	if value.Role == nil || *value.Role != "assistant" {
		return nil
	}
	if value.Tokens == nil {
		return nil
	}
	usage := core.TokenUsageRaw{
		InputTokens:  value.Tokens.Input,
		OutputTokens: value.Tokens.Output,
	}
	if value.Tokens.Cache != nil {
		usage.CacheCreationInputTokens = value.Tokens.Cache.Write
		usage.CacheReadInputTokens = value.Tokens.Cache.Read
	}
	usage, extraTotalTokens := applyTotalTokenFallback(usage, value.Tokens.Reasoning, value.Tokens.Total)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 &&
		usage.CacheCreationInputTokens == 0 && usage.CacheReadInputTokens == 0 &&
		extraTotalTokens == 0 {
		return nil
	}
	if value.ModelID == nil {
		return nil
	}
	created := int64(-1)
	if value.Time != nil && value.Time.Created != nil {
		created = *value.Time.Created
	}
	timestamp, ok := normalizeTimestamp(created)
	if !ok {
		return nil
	}
	timestampText := core.FormatRFC3339Millis(timestamp)
	sessionID := rowSessionID
	if value.SessionID != nil {
		sessionID = *value.SessionID
	}
	messageID := dbPath + ":" + rowID
	if value.ID != nil {
		messageID = *value.ID
	}
	costUSD := value.Cost
	model := *value.ModelID
	messageIDCopy := messageID
	sessionIDCopy := sessionID
	data := core.UsageEntry{
		SessionID: &sessionIDCopy,
		Timestamp: timestampText,
		Message: core.UsageMessage{
			Usage: usage,
			Model: &model,
			ID:    &messageIDCopy,
		},
		CostUSD: costUSD,
	}
	costData := data
	costData.Message.Usage.OutputTokens = saturatingAddU64(usage.OutputTokens, extraTotalTokens)
	cost := calculateKiloCost(&costData, value.ProviderID, mode, pricing)
	missingPricingModel := missingKiloPricing(&costData, value.ProviderID, mode, pricing)
	return &core.LoadedEntry{
		Date:                core.FormatDateTZ(timestamp, tz),
		Timestamp:           timestamp,
		Project:             "kilo",
		SessionID:           sessionID,
		ProjectPath:         "Kilo",
		Cost:                cost,
		ExtraTotalTokens:    extraTotalTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
		Data:                data,
	}
}

// normalizeTimestamp converts unix seconds or millis into milliseconds.
func normalizeTimestamp(value int64) (int64, bool) {
	if value <= 0 {
		return 0, false
	}
	millis := value
	if value < 1_000_000_000_000 {
		millis = value * 1000
	}
	return millis, true
}

func calculateKiloCost(data *core.UsageEntry, provider *string, mode core.CostMode, pricing *core.PricingMap) float64 {
	switch mode {
	case core.ModeDisplay:
		if data.CostUSD != nil {
			return *data.CostUSD
		}
		return 0
	case core.ModeAuto:
		if data.CostUSD != nil {
			return *data.CostUSD
		}
		return calculateKiloCostFromTokens(data, provider, pricing)
	default:
		return calculateKiloCostFromTokens(data, provider, pricing)
	}
}

func calculateKiloCostFromTokens(data *core.UsageEntry, provider *string, pricing *core.PricingMap) float64 {
	if data.Message.Model == nil {
		return 0
	}
	model := *data.Message.Model
	for _, candidate := range modelCandidates(model, provider) {
		if pricing.Find(candidate) != nil {
			candidateCopy := candidate
			return core.CalculateCostForUsage(&candidateCopy, data.Message.Usage, nil, core.ModeCalculate, pricing)
		}
	}
	return 0
}

// missingKiloPricing flags models with no pricing across all candidates;
// display mode and a positive recorded cost never consult pricing.
func missingKiloPricing(data *core.UsageEntry, provider *string, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay {
		return nil
	}
	if data.CostUSD != nil && *data.CostUSD > 0 {
		return nil
	}
	if data.Message.Model == nil {
		return nil
	}
	model := *data.Message.Model
	total := core.TotalUsageTokens(data.Message.Usage)
	if total == 0 {
		return nil
	}
	if pricing == nil {
		return nil
	}
	for _, candidate := range modelCandidates(model, provider) {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

// modelCandidates lists the pricing lookup order: a real provider's
// `provider/model` form first, then the bare model.
func modelCandidates(model string, provider *string) []string {
	var candidates []string
	if provider != nil {
		normalized := strings.ReplaceAll(*provider, "-", "_")
		if normalized != "unknown" && normalized != "kilo" {
			candidates = append(candidates, normalized+"/"+model)
		}
	}
	candidates = append(candidates, model)
	seen := map[string]struct{}{}
	unique := candidates[:0]
	for _, candidate := range candidates {
		if _, dup := seen[candidate]; dup {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

// applyTotalTokenFallback ports the reference helper: a totalTokens value
// larger than the known counters fills a missing output count first, then
// becomes extra tokens.
func applyTotalTokenFallback(usage core.TokenUsageRaw, extraTotalTokens, totalTokens uint64) (core.TokenUsageRaw, uint64) {
	knownTokens := saturatingAddU64(core.TotalUsageTokens(usage), extraTotalTokens)
	missingTokens := saturatingSubU64(totalTokens, knownTokens)
	if missingTokens == 0 {
		return usage, extraTotalTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = missingTokens
	} else {
		extraTotalTokens = saturatingAddU64(extraTotalTokens, missingTokens)
	}
	return usage, extraTotalTokens
}

func saturatingAddU64(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return ^uint64(0)
	}
	return sum
}

func saturatingSubU64(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// ---------------------------------------------------------------------------
// Lenient JSON field helpers (rust adapters/common/src/jsonl.rs semantics)
// ---------------------------------------------------------------------------

// nonEmptyString trims a string value and drops empty/non-string values.
func nonEmptyString(raw json.RawMessage) *string {
	if !isJSONString(raw) {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// lenientU64 mirrors serde_json Value::as_u64: only non-negative integers
// that fit u64 count; floats, strings, nulls, and negatives become 0.
func lenientU64(raw json.RawMessage) uint64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || !allDigits(text) {
		return 0
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// lenientI64 mirrors Value::as_i64: any integer that fits i64; floats,
// strings, nulls become absent.
func lenientI64(raw json.RawMessage) *int64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil
	}
	digits := strings.TrimPrefix(text, "-")
	if digits == "" || !allDigits(digits) {
		return nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// lenientF64 mirrors Value::as_f64: any JSON number yields a value; strings
// and nulls become absent.
func lenientF64(raw json.RawMessage) *float64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || !isJSONNumber(text) {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return &value
}

func isJSONObject(raw json.RawMessage) bool {
	return strings.HasPrefix(strings.TrimSpace(string(raw)), "{")
}

func isJSONString(raw json.RawMessage) bool {
	return strings.HasPrefix(strings.TrimSpace(string(raw)), "\"")
}

// isJSONNumber accepts the JSON number grammar (no leading zeros beyond "0").
func isJSONNumber(text string) bool {
	if text == "" {
		return false
	}
	start := 0
	if text[0] == '-' {
		start = 1
		if len(text) == 1 {
			return false
		}
	}
	digits := text[start:]
	i := 0
	if digits[0] == '0' {
		i = 1
	} else {
		for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
			i++
		}
	}
	if i == 0 {
		return false
	}
	rest := digits[i:]
	if rest != "" && rest[0] == '.' {
		rest = rest[1:]
		frac := 0
		for frac < len(rest) && rest[frac] >= '0' && rest[frac] <= '9' {
			frac++
		}
		if frac == 0 {
			return false
		}
		rest = rest[frac:]
	}
	if rest != "" && (rest[0] == 'e' || rest[0] == 'E') {
		rest = rest[1:]
		if rest != "" && (rest[0] == '+' || rest[0] == '-') {
			rest = rest[1:]
		}
		exp := 0
		for exp < len(rest) && rest[exp] >= '0' && rest[exp] <= '9' {
			exp++
		}
		if exp == 0 {
			return false
		}
		rest = rest[exp:]
	}
	return rest == ""
}

func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func rawFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if !isJSONObject(raw) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

func field(fields map[string]json.RawMessage, name string) json.RawMessage {
	raw, ok := fields[name]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	return raw
}
