package kimi

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
)

const (
	defaultModel    = "kimi-for-coding"
	defaultProvider = "moonshot"
	// kimiForCodingK26CutoffMs switches the default model's pricing between
	// kimi-k2.5 and kimi-k2.6.
	kimiForCodingK26CutoffMs int64 = 1_776_698_890_072
)

// kimiUsageEntry is one deduplicated Kimi usage record.
type kimiUsageEntry struct {
	timestamp           int64
	timestampText       string
	sessionID           string
	model               string
	messageID           *string
	inputTokens         uint64
	outputTokens        uint64
	cacheCreationTokens uint64
	cacheReadTokens     uint64
	extraTotalTokens    uint64
}

// kimiWireLine is a lenient view of one wire.jsonl line. The nested message,
// payload, token_usage, and usage fields must be objects when present,
// mirroring the reference's strict Option<T> deserialization: a malformed
// nested object rejects the whole line.
type kimiWireLine struct {
	fields    map[string]json.RawMessage
	message   *kimiWireMessage
	timestamp *float64
	usage     *kimiCodeUsage
	time      *int64
}

type kimiWireMessage struct {
	tokenUsage *kimiTokenUsage
	messageID  *string
}

// kimiTokenUsage carries the old-format message.payload.token_usage counts.
type kimiTokenUsage struct {
	inputOther         uint64
	output             uint64
	inputCacheCreation uint64
	inputCacheRead     uint64
	total              uint64
}

// kimiCodeUsage carries the new-format usage.record counts.
type kimiCodeUsage struct {
	inputOther         uint64
	output             uint64
	inputCacheCreation uint64
	inputCacheRead     uint64
}

func parseKimiWireLine(content []byte) (kimiWireLine, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return kimiWireLine{}, false
	}
	line := kimiWireLine{fields: fields}
	if raw := rawField(fields, "timestamp"); raw != nil {
		if value, ok := jsonNumberF64(raw); ok {
			line.timestamp = &value
		}
	}
	if raw := rawField(fields, "time"); raw != nil {
		if value, ok := jsonNumberI64(raw); ok {
			line.time = &value
		}
	}
	if raw := rawField(fields, "message"); raw != nil {
		var messageFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &messageFields); err != nil || messageFields == nil {
			return kimiWireLine{}, false
		}
		message := kimiWireMessage{}
		if raw := rawField(messageFields, "payload"); raw != nil {
			var payloadFields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &payloadFields); err != nil || payloadFields == nil {
				return kimiWireLine{}, false
			}
			if raw := rawField(payloadFields, "token_usage"); raw != nil {
				var usageFields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &usageFields); err != nil || usageFields == nil {
					return kimiWireLine{}, false
				}
				usage := kimiTokenUsage{
					inputOther:         jsonU64(usageFields["input_other"]),
					output:             jsonU64(usageFields["output"]),
					inputCacheCreation: jsonU64(usageFields["input_cache_creation"]),
					inputCacheRead:     jsonU64(usageFields["input_cache_read"]),
					total:              jsonU64(usageFields["total"]),
				}
				message.tokenUsage = &usage
			}
			if value, ok := nonEmptyStringAt(payloadFields, "message_id"); ok {
				message.messageID = &value
			}
		}
		line.message = &message
	}
	if raw := rawField(fields, "usage"); raw != nil {
		var usageFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &usageFields); err != nil || usageFields == nil {
			return kimiWireLine{}, false
		}
		usage := kimiCodeUsage{
			inputOther:         jsonU64(usageFields["inputOther"]),
			output:             jsonU64(usageFields["output"]),
			inputCacheCreation: jsonU64(usageFields["inputCacheCreation"]),
			inputCacheRead:     jsonU64(usageFields["inputCacheRead"]),
		}
		line.usage = &usage
	}
	return line, true
}

func (l kimiWireLine) typeName() (string, bool) {
	return nonEmptyStringAt(l.fields, "type")
}

func (l kimiWireLine) model() (string, bool) {
	return nonEmptyStringAt(l.fields, "model")
}

func (l kimiWireLine) usageScope() (string, bool) {
	return nonEmptyStringAt(l.fields, "usageScope")
}

// readWireFile parses one wire.jsonl file. Usable lines carry either
// token_usage (old format) or usage.record (new format), so lines without
// either marker are skipped before parsing.
func readWireFile(path string) ([]kimiUsageEntry, error) {
	model := readModelFromConfig(path)
	fallbackTimestamp := fileModifiedTimestamp(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []kimiUsageEntry
	for _, line := range splitJSONL(content) {
		if !bytesContainsAny(line, `"token_usage"`, `"usage.record"`) {
			continue
		}
		parsed, ok := parseKimiWireLine(line)
		if !ok {
			continue
		}
		if entry, ok := wireLineToEntry(parsed, path, model, fallbackTimestamp); ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// readModelFromConfig loads the display model from config.json at the Kimi
// root, falling back to the default.
func readModelFromConfig(filePath string) string {
	root, ok := kimiRootFromWirePath(filePath)
	if !ok {
		return defaultModel
	}
	content, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		return defaultModel
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return defaultModel
	}
	if model, ok := nonEmptyStringAt(fields, "model"); ok {
		return model
	}
	return defaultModel
}

// kimiRootFromWirePath resolves the Kimi root for either wire layout.
func kimiRootFromWirePath(filePath string) (string, bool) {
	agentDir := filepath.Dir(filePath)
	if filepath.Base(filepath.Dir(agentDir)) == "agents" {
		dir := agentDir
		for i := 0; i < 5; i++ {
			parent := filepath.Dir(dir)
			if parent == dir {
				return "", false
			}
			dir = parent
		}
		return dir, true
	}
	dir := filePath
	for i := 0; i < 4; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return dir, true
}

func fileModifiedTimestamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

func wireLineToEntry(line kimiWireLine, filePath, model string, fallbackTimestamp int64) (kimiUsageEntry, bool) {
	typeName, _ := line.typeName()
	switch typeName {
	case "usage.record":
		return wireLineToEntryNew(line, filePath, fallbackTimestamp)
	case "metadata":
		return kimiUsageEntry{}, false
	default:
		return wireLineToEntryOld(line, filePath, model, fallbackTimestamp)
	}
}

// wireLineToEntryNew parses a Kimi Code usage.record line; only turn-scoped
// records count because session records hold cumulative totals.
func wireLineToEntryNew(line kimiWireLine, filePath string, fallbackTimestamp int64) (kimiUsageEntry, bool) {
	if scope, ok := line.usageScope(); !ok || scope != "turn" {
		return kimiUsageEntry{}, false
	}
	usageCounts := line.usage
	if usageCounts == nil {
		return kimiUsageEntry{}, false
	}
	usage := core.TokenUsageRaw{
		InputTokens:          usageCounts.inputOther,
		OutputTokens:         usageCounts.output,
		CacheCreationInputTokens: usageCounts.inputCacheCreation,
		CacheReadInputTokens:     usageCounts.inputCacheRead,
	}
	usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, 0)
	if core.TotalUsageTokens(usage)+extraTotalTokens == 0 {
		return kimiUsageEntry{}, false
	}
	timestamp := fallbackTimestamp
	if line.time != nil {
		timestamp = *line.time
	}
	model := defaultModel
	if value, ok := line.model(); ok {
		model = strings.TrimPrefix(value, "kimi-code/")
	}
	return kimiUsageEntry{
		timestamp:           timestamp,
		timestampText:       core.FormatRFC3339Millis(timestamp),
		sessionID:           extractSessionID(filePath),
		model:               model,
		inputTokens:         usage.InputTokens,
		outputTokens:        usage.OutputTokens,
		cacheCreationTokens: usage.CacheCreationInputTokens,
		cacheReadTokens:     usage.CacheReadInputTokens,
		extraTotalTokens:    extraTotalTokens,
	}, true
}

// wireLineToEntryOld parses the StatusUpdate token_usage wire format.
func wireLineToEntryOld(line kimiWireLine, filePath, model string, fallbackTimestamp int64) (kimiUsageEntry, bool) {
	message := line.message
	if message == nil {
		return kimiUsageEntry{}, false
	}
	if typeName, ok := nonEmptyStringAt(messageFields(line), "type"); !ok || typeName != "StatusUpdate" {
		return kimiUsageEntry{}, false
	}
	tokenUsage := message.tokenUsage
	if tokenUsage == nil {
		return kimiUsageEntry{}, false
	}
	usage := core.TokenUsageRaw{
		InputTokens:              tokenUsage.inputOther,
		OutputTokens:             tokenUsage.output,
		CacheCreationInputTokens: tokenUsage.inputCacheCreation,
		CacheReadInputTokens:     tokenUsage.inputCacheRead,
	}
	usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, tokenUsage.total)
	if core.TotalUsageTokens(usage)+extraTotalTokens == 0 {
		return kimiUsageEntry{}, false
	}
	timestamp := fallbackTimestamp
	if line.timestamp != nil {
		if ts, ok := timestampFromSeconds(*line.timestamp); ok {
			timestamp = ts
		}
	}
	return kimiUsageEntry{
		timestamp:           timestamp,
		timestampText:       core.FormatRFC3339Millis(timestamp),
		sessionID:           extractSessionID(filePath),
		model:               model,
		messageID:           message.messageID,
		inputTokens:         usage.InputTokens,
		outputTokens:        usage.OutputTokens,
		cacheCreationTokens: usage.CacheCreationInputTokens,
		cacheReadTokens:     usage.CacheReadInputTokens,
		extraTotalTokens:    extraTotalTokens,
	}, true
}

// messageFields re-extracts the message object for its type lookup.
func messageFields(line kimiWireLine) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawField(line.fields, "message"), &fields); err != nil {
		return nil
	}
	return fields
}

func timestampFromSeconds(seconds float64) (int64, bool) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, false
	}
	millis := math.Trunc(seconds * 1000.0)
	if millis < math.MinInt64 || millis > math.MaxInt64 {
		return 0, false
	}
	return int64(millis), true
}

// extractSessionID reads the session directory name for either wire layout.
func extractSessionID(filePath string) string {
	parent := filepath.Dir(filePath)
	var sessionDir string
	if filepath.Base(filepath.Dir(parent)) == "agents" {
		sessionDir = filepath.Dir(filepath.Dir(parent))
	} else {
		sessionDir = parent
	}
	name := filepath.Base(sessionDir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "unknown"
	}
	return name
}

func kimiEntryKey(entry *kimiUsageEntry) string {
	messageID := ""
	if entry.messageID != nil {
		messageID = *entry.messageID
	}
	return strings.Join([]string{
		entry.sessionID,
		messageID,
		entry.timestampText,
		entry.model,
		strconv.FormatUint(entry.inputTokens, 10),
		strconv.FormatUint(entry.outputTokens, 10),
		strconv.FormatUint(entry.cacheCreationTokens, 10),
		strconv.FormatUint(entry.cacheReadTokens, 10),
		strconv.FormatUint(entry.extraTotalTokens, 10),
	}, ":")
}

// kimiEntryToLoaded prices one entry and stamps the fixed Kimi identity.
func kimiEntryToLoaded(entry kimiUsageEntry, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	usage := core.TokenUsageRaw{
		InputTokens:              entry.inputTokens,
		OutputTokens:             entry.outputTokens,
		CacheCreationInputTokens: entry.cacheCreationTokens,
		CacheReadInputTokens:     entry.cacheReadTokens,
	}
	cost := 0.0
	if mode != core.ModeDisplay {
		cost = kimiCandidateCost(entry, usage, pricing)
	}
	missingPricingModel := kimiMissingPricing(entry, usage, mode, pricing)
	model := entry.model
	sessionID := entry.sessionID
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: entry.timestampText,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &model,
				ID:    entry.messageID,
			},
		},
		Timestamp:           entry.timestamp,
		Date:                core.FormatDateTZ(entry.timestamp, tz),
		Project:             "kimi",
		SessionID:           entry.sessionID,
		ProjectPath:         "Kimi",
		Cost:                cost,
		ExtraTotalTokens:    entry.extraTotalTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

func kimiCandidateCost(entry kimiUsageEntry, usage core.TokenUsageRaw, pricing *core.PricingMap) float64 {
	for _, candidate := range kimiModelCandidates(entry) {
		if pricing.Find(candidate) != nil {
			name := candidate
			return core.CalculateCostForUsage(&name, usage, nil, core.ModeCalculate, pricing)
		}
	}
	return 0
}

func kimiMissingPricing(entry kimiUsageEntry, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay {
		return nil
	}
	total := core.TotalUsageTokens(usage) + entry.extraTotalTokens
	if total == 0 || pricing == nil {
		return nil
	}
	for _, candidate := range kimiModelCandidates(entry) {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(entry.model)
	return &resolved
}

// kimiModelCandidates prices the default model by timestamp while keeping the
// display model unchanged.
func kimiModelCandidates(entry kimiUsageEntry) []string {
	var candidates []string
	seen := map[string]bool{}
	add := func(candidate string) {
		if !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	if entry.model == defaultModel {
		add(kimiForCodingPricingModel(entry.timestamp))
	}
	add(defaultProvider + "/" + entry.model)
	add("kimi/" + entry.model)
	add(entry.model)
	return candidates
}

func kimiForCodingPricingModel(timestamp int64) string {
	if timestamp < kimiForCodingK26CutoffMs {
		return "moonshot/kimi-k2.5"
	}
	return "moonshot/kimi-k2.6"
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

func rawField(fields map[string]json.RawMessage, key string) json.RawMessage {
	raw, ok := fields[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	return raw
}

func lenientStringAt(fields map[string]json.RawMessage, key string) (string, bool) {
	raw := rawField(fields, key)
	if raw == nil {
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

// jsonU64 mirrors Value::as_u64: only non-negative integers that fit u64.
func jsonU64(raw json.RawMessage) uint64 {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// jsonNumberF64 mirrors Value::as_f64: any JSON number.
func jsonNumberF64(raw json.RawMessage) (float64, bool) {
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

// jsonNumberI64 mirrors Value::as_i64: only integers that fit i64.
func jsonNumberI64(raw json.RawMessage) (int64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func bytesContainsAny(line []byte, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(string(line), marker) {
			return true
		}
	}
	return false
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
