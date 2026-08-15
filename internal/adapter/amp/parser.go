package amp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// readThreadFile parses one Amp thread file (a single JSON object). The file is
// navigated leniently like the reference: a malformed file, a missing thread
// id, or unexpectedly typed fields yield no entries rather than an error.
func readThreadFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, ok := decodeObject(content)
	if !ok {
		return []core.LoadedEntry{}, nil
	}
	threadID, ok := nonEmptyString(root["id"])
	if !ok {
		return []core.LoadedEntry{}, nil
	}
	messages := objectElements(root["messages"])

	// A usageLedger object with a usable events array takes precedence over
	// per-message usage; anything else (missing, non-object, events not an
	// array) falls back to the messages path.
	if ledger, isObject := root["usageLedger"].(map[string]any); isObject {
		if events, isArray := ledger["events"].([]any); isArray {
			cacheTokens := cacheTokensByMessageID(messages)
			return parseLedgerEvents(events, cacheTokens, threadID, tz, mode, pricing), nil
		}
	}

	return parseMessageUsage(messages, threadID, tz, mode, pricing), nil
}

// parseLedgerEvents converts usage-ledger events into entries. Cache counters
// live on the chat messages, keyed by the event's toMessageId.
func parseLedgerEvents(events []any, cacheTokens map[int64][2]uint64, threadID string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) []core.LoadedEntry {
	var entries []core.LoadedEntry
	for _, rawEvent := range events {
		event, ok := rawEvent.(map[string]any)
		if !ok {
			continue
		}
		timestampText, ok := nonEmptyString(event["timestamp"])
		if !ok {
			continue
		}
		timestamp, ok := core.ParseTSTimestamp(timestampText)
		if !ok {
			continue
		}
		model, ok := nonEmptyString(event["model"])
		if !ok {
			continue
		}
		tokens, hasTokens := event["tokens"]
		if !hasTokens || tokens == nil {
			continue
		}
		var cache [2]uint64
		if id, ok := asInt64(event["toMessageId"]); ok {
			cache = cacheTokens[id]
		}
		usage := core.TokenUsageRaw{
			InputTokens:              valueU64(tokens, "input"),
			OutputTokens:             valueU64(tokens, "output"),
			CacheCreationInputTokens: cache[0],
			CacheReadInputTokens:     cache[1],
		}
		totalTokens := valueU64(tokens, "total")
		usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, totalTokens)
		if usageIsZero(usage, extraTotalTokens) {
			continue
		}
		entries = append(entries, buildEntry(entryParts{
			threadID:      threadID,
			timestampText: timestampText,
			timestamp:     timestamp,
			model:         model,
			usage:         usage,
			extraTotal:    extraTotalTokens,
			messageID:     nonEmptyStringValue(event["id"]),
			credits:       asFloat64(event["credits"]),
			tz:            tz,
			mode:          mode,
			pricing:       pricing,
		}))
	}
	return entries
}

// parseMessageUsage converts assistant-message usage blocks into entries.
func parseMessageUsage(messages []any, threadID string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) []core.LoadedEntry {
	var entries []core.LoadedEntry
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := message["role"].(string); role != "assistant" {
			continue
		}
		usageRaw, hasUsage := message["usage"]
		if !hasUsage || usageRaw == nil {
			continue
		}
		timestampText, ok := objectNonEmptyString(usageRaw, "timestamp")
		if !ok {
			timestampText, ok = nonEmptyString(message["timestamp"])
			if !ok {
				continue
			}
		}
		timestamp, ok := core.ParseTSTimestamp(timestampText)
		if !ok {
			continue
		}
		model, ok := objectNonEmptyString(usageRaw, "model")
		if !ok {
			model, ok = nonEmptyString(message["model"])
			if !ok {
				continue
			}
		}
		usage := core.TokenUsageRaw{
			InputTokens:              valueU64(usageRaw, "inputTokens"),
			OutputTokens:             valueU64(usageRaw, "outputTokens"),
			CacheCreationInputTokens: valueU64(usageRaw, "cacheCreationInputTokens"),
			CacheReadInputTokens:     valueU64(usageRaw, "cacheReadInputTokens"),
		}
		totalTokens := valueU64(usageRaw, "totalTokens")
		usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, totalTokens)
		if usageIsZero(usage, extraTotalTokens) {
			continue
		}
		entries = append(entries, buildEntry(entryParts{
			threadID:      threadID,
			timestampText: timestampText,
			timestamp:     timestamp,
			model:         model,
			usage:         usage,
			extraTotal:    extraTotalTokens,
			messageID:     messageIDValue(message["messageId"]),
			credits:       asFloat64Object(usageRaw, "credits"),
			tz:            tz,
			mode:          mode,
			pricing:       pricing,
		}))
	}
	return entries
}

// entryParts collects the fields shared by both parsing paths.
type entryParts struct {
	threadID      string
	timestampText string
	timestamp     int64
	model         string
	usage         core.TokenUsageRaw
	extraTotal    uint64
	messageID     *string
	credits       *float64
	tz            *time.Location
	mode          core.CostMode
	pricing       *core.PricingMap
}

// buildEntry assembles a LoadedEntry exactly like the reference: the raw usage
// lands in data.message.usage, while the cost pass folds extra total tokens
// into output tokens (cache creation split dropped for pricing).
func buildEntry(p entryParts) core.LoadedEntry {
	sessionID := p.threadID
	model := p.model
	data := core.UsageEntry{
		SessionID: &sessionID,
		Timestamp: p.timestampText,
		Message: core.UsageMessage{
			Usage: p.usage,
			Model: &model,
			ID:    p.messageID,
		},
	}
	costUsage := p.usage
	costUsage.OutputTokens = saturatingAdd(costUsage.OutputTokens, p.extraTotal)
	costUsage.CacheCreation = nil
	cost := core.CalculateCostForUsage(&model, costUsage, nil, p.mode, p.pricing)
	missingPricingModel := core.MissingPricingModelForUsage(&model, costUsage, nil, p.mode, p.pricing)
	return core.LoadedEntry{
		Data:                data,
		Timestamp:           p.timestamp,
		Date:                core.FormatDateTZ(p.timestamp, p.tz),
		Project:             "amp",
		SessionID:           p.threadID,
		ProjectPath:         "Amp",
		Cost:                cost,
		ExtraTotalTokens:    p.extraTotal,
		Credits:             p.credits,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

// usageIsZero mirrors the reference all-zero skip (cache_creation is a plain
// counter in amp data, so the split accessor equals the raw field).
func usageIsZero(usage core.TokenUsageRaw, extraTotalTokens uint64) bool {
	return usage.InputTokens == 0 && usage.OutputTokens == 0 &&
		usage.CacheCreationTokenCount() == 0 && usage.CacheReadInputTokens == 0 &&
		extraTotalTokens == 0
}

// cacheTokensByMessageID maps assistant message ids to their
// (cacheCreationInputTokens, cacheReadInputTokens) pair.
func cacheTokensByMessageID(messages []any) map[int64][2]uint64 {
	cacheTokens := map[int64][2]uint64{}
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := message["role"].(string); role != "assistant" {
			continue
		}
		id, ok := asInt64(message["messageId"])
		if !ok {
			continue
		}
		cacheTokens[id] = [2]uint64{
			valueU64(message["usage"], "cacheCreationInputTokens"),
			valueU64(message["usage"], "cacheReadInputTokens"),
		}
	}
	return cacheTokens
}

// applyTotalTokenFallback mirrors ccusage-core utils.rs: tokens reported only
// in a total counter are folded into output tokens (or extra when output is
// already non-zero).
func applyTotalTokenFallback(usage core.TokenUsageRaw, extraTotalTokens, totalTokens uint64) (core.TokenUsageRaw, uint64) {
	knownTokens := saturatingAdd(core.TotalUsageTokens(usage), extraTotalTokens)
	missingTokens := totalTokens - minU64(totalTokens, knownTokens)
	if missingTokens == 0 {
		return usage, extraTotalTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = missingTokens
	} else {
		extraTotalTokens = saturatingAdd(extraTotalTokens, missingTokens)
	}
	return usage, extraTotalTokens
}

func saturatingAdd(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return ^uint64(0)
	}
	return sum
}

func minU64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Lenient JSON navigation (serde_json Value semantics)
// ---------------------------------------------------------------------------

// decodeObject parses a whole JSON document into an object; false for
// non-objects and invalid JSON (including trailing data, like serde_json).
func decodeObject(content []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false
	}
	obj, ok := value.(map[string]any)
	return obj, ok
}

// objectElements returns the object elements of an array value (lenient_vec:
// non-array values and non-object elements are dropped).
func objectElements(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []any
	for _, item := range items {
		if _, ok := item.(map[string]any); ok {
			out = append(out, item)
		}
	}
	return out
}

// nonEmptyString mirrors non_empty_json_string: trimmed non-empty strings only.
func nonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// nonEmptyStringValue is nonEmptyString returning a *string (nil when absent).
func nonEmptyStringValue(value any) *string {
	if s, ok := nonEmptyString(value); ok {
		return &s
	}
	return nil
}

// objectNonEmptyString reads a trimmed non-empty string field from an object.
func objectNonEmptyString(value any, key string) (string, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	return nonEmptyString(obj[key])
}

// valueU64 mirrors json_value_u64(Value::get(key)): 0 unless the field is a
// non-negative integer that fits u64.
func valueU64(value any, key string) uint64 {
	obj, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	return asUint64(obj[key])
}

func asUint64(value any) uint64 {
	number, ok := value.(json.Number)
	if !ok {
		return 0
	}
	u, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return 0
	}
	return u
}

func asInt64(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

// asFloat64 mirrors Value::as_f64: any JSON number yields a value.
func asFloat64(value any) *float64 {
	if f, ok := asFloat64Value(value); ok {
		return &f
	}
	return nil
}

func asFloat64Value(value any) (float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := number.Float64()
	if err != nil {
		return 0, false
	}
	return f, true
}

func asFloat64Object(value any, key string) *float64 {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return asFloat64(obj[key])
}

// messageIDValue renders an amp message id the way the reference does:
// integers become decimal strings, strings are kept verbatim (no trim).
func messageIDValue(value any) *string {
	if id, ok := asInt64(value); ok {
		text := strconv.FormatInt(id, 10)
		return &text
	}
	if text, ok := value.(string); ok {
		return &text
	}
	return nil
}
