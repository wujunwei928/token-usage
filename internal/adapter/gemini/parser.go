package gemini

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
)

const defaultModel = "unknown"

var providerPrefixes = [4]string{"google", "gemini", "vertex_ai", "openrouter/google"}

// geminiUsageEvent is one extracted token-usage record.
type geminiUsageEvent struct {
	timestamp      int64 // Unix millis
	timestampText  string
	sessionID      string
	model          string
	inputTokens    uint64
	outputTokens   uint64
	cacheReadToken uint64
	reasoningToken uint64
	totalTokens    uint64
	messageID      *string
}

// geminiTokens is the alias-resolved token block of a Gemini record.
type geminiTokens struct {
	input    uint64
	output   uint64
	cached   uint64
	thoughts uint64
	tool     uint64
	total    *uint64
}

// geminiRecord is a lenient view of one Gemini log record: a whole-file JSON
// document or a single JSONL line. Fields are read from a raw object map so
// unexpected shapes degrade to "absent" instead of failing the record.
type geminiRecord struct {
	fields map[string]json.RawMessage
}

func parseGeminiRecord(content []byte) (geminiRecord, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return geminiRecord{}, false
	}
	return geminiRecord{fields: fields}, true
}

func (r geminiRecord) raw(key string) json.RawMessage {
	raw, ok := r.fields[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	return raw
}

// lenientStr mirrors Value::as_str: JSON strings verbatim (no trim), anything
// else absent.
func (r geminiRecord) lenientStr(key string) (string, bool) {
	return lenientStringAt(r.fields, key)
}

// nonEmptyStr mirrors non_empty_json_string: strings trimmed to non-empty,
// anything else absent.
func (r geminiRecord) nonEmptyStr(key string) (string, bool) {
	return nonEmptyStringAt(r.fields, key)
}

// sessionID prefers the camelCase key, matching the reference lookup order.
func (r geminiRecord) sessionID() (string, bool) {
	if value, ok := r.nonEmptyStr("sessionId"); ok {
		return value, true
	}
	return r.nonEmptyStr("session_id")
}

// statsValue prefers the top-level stats, then result.stats.
func (r geminiRecord) statsValue() json.RawMessage {
	if raw := r.raw("stats"); raw != nil {
		return raw
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(r.raw("result"), &result); err == nil && result != nil {
		if raw, ok := result["stats"]; ok && string(raw) != "null" {
			return raw
		}
	}
	return nil
}

func fileModifiedTimestamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

// parseJSONFile parses a whole-file JSON Gemini log document.
func parseJSONFile(path string) ([]geminiUsageEvent, error) {
	fallback := fileModifiedTimestamp(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	record, ok := parseGeminiRecord(content)
	if !ok {
		return nil, nil
	}
	sessionID := "unknown"
	if value, ok := record.sessionID(); ok {
		sessionID = value
	} else {
		stem := filepath.Base(path)
		if ext := filepath.Ext(stem); ext != "" {
			stem = strings.TrimSuffix(stem, ext)
		}
		if stem != "" {
			sessionID = stem
		}
	}
	sessionTimestamp := firstParsedTimestamp(record, "startTime", "lastUpdated", fallback)

	if messages, ok := arrayValue(record.raw("messages")); ok {
		var events []geminiUsageEvent
		for _, message := range messages {
			fields, ok := objectFields(message)
			if !ok {
				continue
			}
			if !isGeminiType(fields) {
				continue
			}
			if event, ok := parseDirectEvent(fields, "", sessionID, sessionTimestamp); ok {
				events = append(events, event)
			}
		}
		return events, nil
	}
	if isGeminiType(record.fields) {
		if event, ok := parseDirectEventRecord(record, "", sessionID, fallback); ok {
			return []geminiUsageEvent{event}, nil
		}
		return nil, nil
	}
	model, _ := record.nonEmptyStr("model")
	return parseStatsEvents(record.statsValue(), model, sessionID,
		firstParsedTimestamp(record, "timestamp", "", fallback)), nil
}

// firstParsedTimestamp tries each field in order and falls back to the mtime.
func firstParsedTimestamp(record geminiRecord, key, fallbackKey string, fallback int64) int64 {
	for _, candidate := range []string{key, fallbackKey} {
		if candidate == "" {
			continue
		}
		if value, ok := record.lenientStr(candidate); ok {
			if ts, ok := core.ParseTSTimestamp(value); ok {
				return ts
			}
		}
	}
	return fallback
}

// parseJSONLFile parses a JSONL Gemini chat log, carrying the session id and
// model forward line to line and replacing direct events that repeat an id.
func parseJSONLFile(path string) ([]geminiUsageEvent, error) {
	fallback := fileModifiedTimestamp(path)
	stem := filepath.Base(path)
	if ext := filepath.Ext(stem); ext != "" {
		stem = strings.TrimSuffix(stem, ext)
	}
	if stem == "" {
		stem = "unknown"
	}
	sessionID := stem
	currentModel := ""
	var events []geminiUsageEvent
	directEventIndexes := map[string]int{}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for _, line := range splitJSONL(content) {
		record, ok := parseGeminiRecord(line)
		if !ok {
			continue
		}
		if value, ok := record.sessionID(); ok {
			sessionID = value
		}
		if model, ok := record.nonEmptyStr("model"); ok {
			currentModel = model
		}
		if isGeminiType(record.fields) {
			event, ok := parseDirectEventRecord(record, currentModel, sessionID, fallback)
			if !ok {
				continue
			}
			if id, ok := record.nonEmptyStr("id"); ok {
				if index, seen := directEventIndexes[id]; seen {
					events[index] = event
				} else {
					directEventIndexes[id] = len(events)
					events = append(events, event)
				}
			} else {
				events = append(events, event)
			}
			continue
		}
		stats := record.statsValue()
		if stats != nil {
			events = append(events, parseStatsEvents(stats, currentModel, sessionID,
				firstParsedTimestamp(record, "timestamp", "", fallback))...)
		}
	}
	return events, nil
}

// isGeminiType compares the raw type field against "gemini" without trimming.
func isGeminiType(fields map[string]json.RawMessage) bool {
	value, ok := lenientStringAt(fields, "type")
	return ok && value == "gemini"
}

// parseDirectEvent builds an event from a messages[] element.
func parseDirectEvent(fields map[string]json.RawMessage, modelHint, sessionID string, fallback int64) (geminiUsageEvent, bool) {
	tokens, ok := parseTokens(fields["tokens"])
	if !ok {
		return geminiUsageEvent{}, false
	}
	model, _ := nonEmptyStringAt(fields, "model")
	if model == "" {
		model = modelHint
	}
	timestamp := fallback
	for _, key := range []string{"timestamp", "created_at"} {
		if value, ok := lenientStringAt(fields, key); ok {
			if ts, ok := core.ParseTSTimestamp(value); ok {
				timestamp = ts
				break
			}
		}
	}
	var id *string
	if value, ok := nonEmptyStringAt(fields, "id"); ok {
		id = &value
	}
	return buildEvent(model, sessionID, timestamp, tokens, normalizeSessionInput, id)
}

// parseDirectEventRecord builds an event from a typed top-level record.
func parseDirectEventRecord(record geminiRecord, modelHint, sessionID string, fallback int64) (geminiUsageEvent, bool) {
	tokens, ok := parseTokens(record.raw("tokens"))
	if !ok {
		return geminiUsageEvent{}, false
	}
	model, _ := record.nonEmptyStr("model")
	if model == "" {
		model = modelHint
	}
	timestamp := fallback
	for _, key := range []string{"timestamp", "created_at"} {
		if value, ok := record.lenientStr(key); ok {
			if ts, ok := core.ParseTSTimestamp(value); ok {
				timestamp = ts
				break
			}
		}
	}
	var id *string
	if value, ok := record.nonEmptyStr("id"); ok {
		id = &value
	}
	return buildEvent(model, sessionID, timestamp, tokens, normalizeSessionInput, id)
}

// parseStatsEvents handles the cumulative stats layout: per-model stats first,
// then a flat token block. stats.models iterates in sorted key order, matching
// serde_json's BTreeMap objects.
func parseStatsEvents(stats json.RawMessage, modelHint, sessionID string, timestamp int64) []geminiUsageEvent {
	fields, ok := objectFields(stats)
	if !ok {
		return nil
	}
	if models, ok := objectFields(fields["models"]); ok {
		keys := sortedKeys(models)
		var events []geminiUsageEvent
		for _, model := range keys {
			dataFields, ok := objectFields(models[model])
			if !ok {
				continue
			}
			tokens, ok := parseTokens(dataFields["tokens"])
			if !ok {
				continue
			}
			if event, ok := buildEvent(model, sessionID, timestamp, tokens, subtractCachedOverlapTokens, nil); ok {
				events = append(events, event)
			}
		}
		if len(events) > 0 {
			return events
		}
	}
	tokens, ok := parseTokens(stats)
	if !ok {
		return nil
	}
	model := modelHint
	if model == "" {
		model = defaultModel
	}
	if event, ok := buildEvent(model, sessionID, timestamp, tokens, subtractCachedOverlapTokens, nil); ok {
		return []geminiUsageEvent{event}
	}
	return nil
}

func parseTokens(value json.RawMessage) (geminiTokens, bool) {
	fields, ok := objectFields(value)
	if !ok {
		return geminiTokens{}, false
	}
	tokens := geminiTokens{
		input:    tokenNumber(fields, []string{"input", "prompt", "input_tokens", "prompt_tokens"}),
		output:   tokenNumber(fields, []string{"output", "candidates", "output_tokens", "candidates_tokens"}),
		cached:   tokenNumber(fields, []string{"cached", "cached_tokens"}),
		thoughts: tokenNumber(fields, []string{"thoughts", "reasoning", "thoughts_tokens", "reasoning_tokens"}),
		tool:     tokenNumber(fields, []string{"tool", "tool_tokens"}),
	}
	for _, key := range []string{"total", "total_tokens"} {
		if total, ok := valueU64(fields[key]); ok {
			tokens.total = &total
			break
		}
	}
	return tokens, true
}

func tokenNumber(fields map[string]json.RawMessage, keys []string) uint64 {
	for _, key := range keys {
		if value, ok := valueU64(fields[key]); ok {
			return value
		}
	}
	return 0
}

// valueU64 accepts any JSON number, truncating toward zero and clamping
// negatives to 0, mirroring the reference as_f64-based coercion.
func valueU64(raw json.RawMessage) (uint64, bool) {
	value, ok := numberF64(raw)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value < 0 {
		return 0, true
	}
	if value >= 18446744073709551616.0 {
		return math.MaxUint64, true
	}
	return uint64(math.Trunc(value)), true
}

// numberF64 parses a JSON number literal; strings/null/bools fail.
func numberF64(raw json.RawMessage) (float64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, false
	}
	if c := text[0]; !(c == '-' || (c >= '0' && c <= '9')) {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func subtractCachedOverlapTokens(tokens geminiTokens) (uint64, uint64) {
	cachedPortion := tokens.input
	if tokens.cached < cachedPortion {
		cachedPortion = tokens.cached
	}
	return tokens.input - cachedPortion, tokens.cached
}

func normalizeSessionInput(tokens geminiTokens) (uint64, uint64) {
	inclusiveTotal := tokens.input + tokens.output + tokens.thoughts + tokens.tool
	exclusiveTotal := inclusiveTotal + tokens.cached
	if tokens.cached > 0 && tokens.total != nil &&
		*tokens.total == inclusiveTotal && *tokens.total != exclusiveTotal {
		return subtractCachedOverlapTokens(tokens)
	}
	return tokens.input, tokens.cached
}

func buildEvent(model, sessionID string, timestamp int64, tokens geminiTokens, normalizeInput func(geminiTokens) (uint64, uint64), messageID *string) (geminiUsageEvent, bool) {
	if strings.TrimSpace(model) == "" {
		return geminiUsageEvent{}, false
	}
	inputWithoutCache, cacheReadTokens := normalizeInput(tokens)
	inputTokens := inputWithoutCache + tokens.tool
	totalTokens := inputTokens + tokens.output + cacheReadTokens + tokens.thoughts
	if tokens.total != nil {
		totalTokens = *tokens.total
	}
	usage, extraTotalTokens := applyTotalTokenFallback(core.TokenUsageRaw{
		InputTokens:          inputTokens,
		OutputTokens:         tokens.output,
		CacheReadInputTokens: cacheReadTokens,
	}, tokens.thoughts, totalTokens)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheReadInputTokens == 0 && extraTotalTokens == 0 {
		return geminiUsageEvent{}, false
	}
	return geminiUsageEvent{
		timestamp:      timestamp,
		timestampText:  core.FormatRFC3339Millis(timestamp),
		sessionID:      sessionID,
		model:          model,
		inputTokens:    usage.InputTokens,
		outputTokens:   usage.OutputTokens,
		cacheReadToken: usage.CacheReadInputTokens,
		reasoningToken: extraTotalTokens,
		totalTokens:    totalTokens,
		messageID:      messageID,
	}, true
}

// eventToLoaded converts one event into a loaded entry; reasoning tokens bill
// as output but display separately.
func eventToLoaded(event geminiUsageEvent, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	usage := core.TokenUsageRaw{
		InputTokens:          event.inputTokens,
		OutputTokens:         event.outputTokens,
		CacheReadInputTokens: event.cacheReadToken,
	}
	costUsage := usage
	costUsage.OutputTokens = event.outputTokens + event.reasoningToken
	known := event.inputTokens + event.outputTokens + event.cacheReadToken
	extraTotalTokens := uint64(0)
	if event.totalTokens > known {
		extraTotalTokens = event.totalTokens - known
	}
	cost := 0.0
	if mode != core.ModeDisplay {
		cost = candidateCost(event.model, costUsage, pricing)
	}
	missingPricingModel := missingPricingForCandidates(event.model, costUsage, mode, pricing)
	model := event.model
	sessionID := event.sessionID
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: event.timestampText,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &model,
				ID:    event.messageID,
			},
		},
		Timestamp:           event.timestamp,
		Date:                core.FormatDateTZ(event.timestamp, tz),
		Project:             "gemini",
		SessionID:           event.sessionID,
		ProjectPath:         "Gemini",
		Cost:                cost,
		ExtraTotalTokens:    extraTotalTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

// candidateCost prices the usage with the first candidate that has pricing.
func candidateCost(model string, usage core.TokenUsageRaw, pricing *core.PricingMap) float64 {
	for _, candidate := range modelCandidates(model) {
		if pricing.Find(candidate) != nil {
			name := candidate
			return core.CalculateCostForUsage(&name, usage, nil, core.ModeCalculate, pricing)
		}
	}
	return 0
}

func missingPricingForCandidates(model string, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay {
		return nil
	}
	total := core.TotalUsageTokens(usage)
	if total == 0 || pricing == nil {
		return nil
	}
	for _, candidate := range modelCandidates(model) {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

func modelCandidates(model string) []string {
	var candidates []string
	seen := map[string]bool{}
	add := func(candidate string) {
		if !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	for _, prefix := range providerPrefixes {
		add(prefix + "/" + model)
	}
	add(model)
	return candidates
}

// applyTotalTokenFallback ports the shared fallback: missing tokens land in
// output when it is zero, otherwise they become extra total tokens.
func applyTotalTokenFallback(usage core.TokenUsageRaw, extraTotalTokens, totalTokens uint64) (core.TokenUsageRaw, uint64) {
	knownTokens := core.TotalUsageTokens(usage) + extraTotalTokens
	missingTokens := uint64(0)
	if totalTokens > knownTokens {
		missingTokens = totalTokens - knownTokens
	}
	if missingTokens == 0 {
		return usage, extraTotalTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = missingTokens
		return usage, extraTotalTokens
	}
	return usage, extraTotalTokens + missingTokens
}

func lenientStringAt(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func nonEmptyStringAt(fields map[string]json.RawMessage, key string) (string, bool) {
	value, ok := lenientStringAt(fields, key)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func objectFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

func arrayValue(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, false
	}
	return items, true
}

func sortedKeys(fields map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// splitJSONL splits content on newlines, tolerating CRLF and a trailing
// newline, mirroring the shared byte_lines helper.
func splitJSONL(content []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(content) {
		line := content[start:]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		lines = append(lines, line)
	}
	return lines
}
