package openclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// openClawLine is a lenient view of one OpenClaw session line. The type and
// customType fields must be strings when present (anything else rejects the
// line), while message and data degrade to absent on malformed shapes.
type openClawLine struct {
	typeValue  *string
	customType *string
	data       *openClawModelSource
	modelID    *string
	model      *string
	provider   *string
	message    *openClawMessage
	timestamp  json.RawMessage
}

// openClawModelSource carries model/provider fields from a model-change
// record, at the root or nested under data.
type openClawModelSource struct {
	modelID  *string
	model    *string
	provider *string
}

// openClawMessage is the assistant message payload.
type openClawMessage struct {
	role      *string
	usage     *openClawUsage
	timestamp json.RawMessage
	modelID   *string
	model     *string
	provider  *string
}

// openClawUsage is the token usage block of an assistant message.
type openClawUsage struct {
	input       uint64
	output      uint64
	cacheRead   uint64
	cacheWrite  uint64
	totalTokens uint64
	cost        *float64
}

// openClawEntry is one parsed assistant usage record.
type openClawEntry struct {
	timestamp           int64
	timestampText       string
	sessionID           string
	model               string
	provider            *string
	inputTokens         uint64
	outputTokens        uint64
	cacheCreationTokens uint64
	cacheReadTokens     uint64
	totalTokens         uint64
	cost                *float64
}

func parseOpenClawLine(content []byte) (openClawLine, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return openClawLine{}, false
	}
	line := openClawLine{timestamp: rawField(fields, "timestamp")}
	if raw := rawField(fields, "type"); raw != nil {
		value, ok := strictString(raw)
		if !ok {
			return openClawLine{}, false
		}
		line.typeValue = &value
	}
	if raw := rawField(fields, "customType"); raw != nil {
		value, ok := strictString(raw)
		if !ok {
			return openClawLine{}, false
		}
		line.customType = &value
	}
	line.modelID = optionalNonEmptyString(fields, "modelId")
	line.model = optionalNonEmptyString(fields, "model")
	line.provider = optionalNonEmptyString(fields, "provider")
	if raw := rawField(fields, "data"); raw != nil {
		if dataFields, ok := objectFields(raw); ok {
			line.data = &openClawModelSource{
				modelID:  optionalNonEmptyString(dataFields, "modelId"),
				model:    optionalNonEmptyString(dataFields, "model"),
				provider: optionalNonEmptyString(dataFields, "provider"),
			}
		}
	}
	if raw := rawField(fields, "message"); raw != nil {
		line.message = parseOpenClawMessage(raw)
	}
	return line, true
}

// parseOpenClawMessage deserializes the message block; ok=false marks an
// object whose strict fields (role, usage) are malformed, which the reference
// treats as an absent message rather than a rejected line.
func parseOpenClawMessage(raw json.RawMessage) *openClawMessage {
	messageFields, ok := objectFields(raw)
	if !ok {
		return nil
	}
	message := openClawMessage{
		modelID:   optionalNonEmptyString(messageFields, "modelId"),
		model:     optionalNonEmptyString(messageFields, "model"),
		provider:  optionalNonEmptyString(messageFields, "provider"),
		timestamp: rawField(messageFields, "timestamp"),
	}
	if raw := rawField(messageFields, "role"); raw != nil {
		value, ok := strictString(raw)
		if !ok {
			return nil
		}
		message.role = &value
	}
	if raw := rawField(messageFields, "usage"); raw != nil {
		usageFields, ok := objectFields(raw)
		if !ok {
			return nil
		}
		usage := openClawUsage{
			input:       jsonU64(usageFields["input"]),
			output:      jsonU64(usageFields["output"]),
			cacheRead:   jsonU64(usageFields["cacheRead"]),
			cacheWrite:  jsonU64(usageFields["cacheWrite"]),
			totalTokens: jsonU64(usageFields["totalTokens"]),
		}
		if raw := rawField(usageFields, "cost"); raw != nil {
			if costFields, ok := objectFields(raw); ok {
				if value, ok := jsonNumberF64(costFields["total"]); ok {
					usage.cost = &value
				}
			}
		}
		message.usage = &usage
	}
	return &message
}

// parseSessionFile walks one session file, tracking the current model and
// provider across model_change / model-snapshot records.
func parseSessionFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	sessionID := extractSessionID(path)
	fallbackTimestamp := fileModifiedTimestamp(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Lines that matter are model-tracking records or assistant records
	// carrying usage, so admit lines containing any of those markers first.
	var currentModel *string
	var currentProvider *string
	var entries []core.LoadedEntry
	for _, line := range common.SplitBytesLines(content) {
		if !isOpenClawMarkerLine(line) {
			continue
		}
		record, ok := parseOpenClawLine(line)
		if !ok {
			continue
		}
		if isModelChange(record) {
			source := openClawModelSource{modelID: record.modelID, model: record.model, provider: record.provider}
			if record.data != nil {
				source = *record.data
			}
			if source.modelID != nil {
				currentModel = source.modelID
			} else if source.model != nil {
				currentModel = source.model
			}
			if source.provider != nil {
				currentProvider = source.provider
			}
			continue
		}
		if entry, ok := parseMessageEntry(record, sessionID, currentModel, currentProvider, fallbackTimestamp); ok {
			entries = append(entries, openClawEntryToLoaded(entry, tz, mode, pricing))
		}
	}
	return entries, nil
}

// isOpenClawMarkerLine mirrors the LinePrefilter: the line must contain one of
// the model-tracking markers or the usage key before parsing.
func isOpenClawMarkerLine(line []byte) bool {
	for _, marker := range []string{`"model_change"`, `"model-snapshot"`, `"usage"`} {
		if strings.Contains(string(line), marker) {
			return true
		}
	}
	return false
}

func isModelChange(record openClawLine) bool {
	if record.typeValue == nil {
		return false
	}
	if *record.typeValue == "model_change" {
		return true
	}
	return *record.typeValue == "custom" && record.customType != nil && *record.customType == "model-snapshot"
}

func parseMessageEntry(record openClawLine, sessionID string, currentModel, currentProvider *string, fallbackTimestamp int64) (openClawEntry, bool) {
	if record.typeValue == nil || *record.typeValue != "message" {
		return openClawEntry{}, false
	}
	message := record.message
	if message == nil {
		return openClawEntry{}, false
	}
	if message.role == nil || *message.role != "assistant" {
		return openClawEntry{}, false
	}
	usage := message.usage
	if usage == nil {
		return openClawEntry{}, false
	}
	rawUsage := core.TokenUsageRaw{
		InputTokens:              usage.input,
		OutputTokens:             usage.output,
		CacheCreationInputTokens: usage.cacheWrite,
		CacheReadInputTokens:     usage.cacheRead,
	}
	rawUsage, extraTotalTokens := applyTotalTokenFallback(rawUsage, 0, usage.totalTokens)
	if core.TotalUsageTokens(rawUsage)+extraTotalTokens == 0 {
		return openClawEntry{}, false
	}
	totalTokens := core.TotalUsageTokens(rawUsage) + extraTotalTokens
	if usage.totalTokens > totalTokens {
		totalTokens = usage.totalTokens
	}
	timestampValue := message.timestamp
	if timestampValue == nil {
		timestampValue = record.timestamp
	}
	timestamp, ok := timestampFromValue(timestampValue)
	if !ok {
		timestamp = fallbackTimestamp
	}
	model := "unknown"
	switch {
	case message.modelID != nil:
		model = *message.modelID
	case message.model != nil:
		model = *message.model
	case currentModel != nil:
		model = *currentModel
	}
	var provider *string
	switch {
	case message.provider != nil:
		provider = message.provider
	case currentProvider != nil:
		provider = currentProvider
	}
	return openClawEntry{
		timestamp:           timestamp,
		timestampText:       core.FormatRFC3339Millis(timestamp),
		sessionID:           sessionID,
		model:               "[openclaw] " + model,
		provider:            provider,
		inputTokens:         rawUsage.InputTokens,
		outputTokens:        rawUsage.OutputTokens,
		cacheCreationTokens: rawUsage.CacheCreationInputTokens,
		cacheReadTokens:     rawUsage.CacheReadInputTokens,
		totalTokens:         totalTokens,
		cost:                usage.cost,
	}, true
}

func timestampFromValue(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	return core.ParseTSTimestamp(text)
}

// openClawEntryToLoaded prices the entry (preferring the reported cost in auto
// mode) and stamps the fixed OpenClaw identity.
func openClawEntryToLoaded(entry openClawEntry, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	usage := core.TokenUsageRaw{
		InputTokens:              entry.inputTokens,
		OutputTokens:             entry.outputTokens,
		CacheCreationInputTokens: entry.cacheCreationTokens,
		CacheReadInputTokens:     entry.cacheReadTokens,
	}
	model := entry.model
	sessionID := entry.sessionID
	cost := core.CalculateCostForUsage(&model, usage, entry.cost, mode, pricing)
	missingPricingModel := core.MissingPricingModelForUsage(&model, usage, entry.cost, mode, pricing)
	known := entry.inputTokens + entry.outputTokens + entry.cacheCreationTokens + entry.cacheReadTokens
	extraTotalTokens := uint64(0)
	if entry.totalTokens > known {
		extraTotalTokens = entry.totalTokens - known
	}
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: entry.timestampText,
			Version:   entry.provider,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &model,
			},
			CostUSD: entry.cost,
		},
		Timestamp:           entry.timestamp,
		Date:                core.FormatDateTZ(entry.timestamp, tz),
		Project:             "openclaw",
		SessionID:           entry.sessionID,
		ProjectPath:         "OpenClaw",
		Cost:                cost,
		ExtraTotalTokens:    extraTotalTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

// extractSessionID strips the .jsonl* suffix family from the file name.
func extractSessionID(path string) string {
	filename := filepath.Base(path)
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		return "unknown"
	}
	index := strings.Index(filename, ".jsonl")
	if index < 0 {
		return filename
	}
	if index == 0 {
		return filename
	}
	return filename[:index]
}

func fileModifiedTimestamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

// entryID mirrors the reference dedup identity.
func entryID(entry *core.LoadedEntry) string {
	model := ""
	if entry.Model != nil {
		model = *entry.Model
	}
	usage := entry.Data.Message.Usage
	return strings.Join([]string{
		"openclaw",
		entry.SessionID,
		entry.Data.Timestamp,
		model,
		strconv.FormatUint(usage.InputTokens, 10),
		strconv.FormatUint(usage.OutputTokens, 10),
		strconv.FormatUint(usage.CacheCreationTokenCount(), 10),
		strconv.FormatUint(usage.CacheReadInputTokens, 10),
		strconv.FormatUint(entry.ExtraTotalTokens, 10),
		strconv.FormatFloat(entry.Cost, 'f', -1, 64),
	}, ":")
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

// strictString accepts only JSON strings, untrimmed.
func strictString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func optionalNonEmptyString(fields map[string]json.RawMessage, key string) *string {
	if value, ok := nonEmptyStringAt(fields, key); ok {
		return &value
	}
	return nil
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
