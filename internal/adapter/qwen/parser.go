package qwen

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

const defaultQwenModel = "unknown"

// qwenLine is a lenient view of one Qwen chat record. usageMetadata must be an
// object when present, mirroring the reference's strict Option<T>.
type qwenLine struct {
	fields        map[string]json.RawMessage
	usageMetadata *qwenUsageMetadata
}

// qwenUsageMetadata carries the Gemini-style token counts of a Qwen assistant
// record.
type qwenUsageMetadata struct {
	promptTokenCount        uint64
	candidatesTokenCount    uint64
	thoughtsTokenCount      uint64
	cachedContentTokenCount uint64
	totalTokenCount         uint64
}

func parseQwenLine(content []byte) (qwenLine, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return qwenLine{}, false
	}
	line := qwenLine{fields: fields}
	if raw := rawField(fields, "usageMetadata"); raw != nil {
		var usageFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &usageFields); err != nil || usageFields == nil {
			return qwenLine{}, false
		}
		line.usageMetadata = &qwenUsageMetadata{
			promptTokenCount:        jsonU64(usageFields["promptTokenCount"]),
			candidatesTokenCount:    jsonU64(usageFields["candidatesTokenCount"]),
			thoughtsTokenCount:      jsonU64(usageFields["thoughtsTokenCount"]),
			cachedContentTokenCount: jsonU64(usageFields["cachedContentTokenCount"]),
			totalTokenCount:         jsonU64(usageFields["totalTokenCount"]),
		}
	}
	return line, true
}

func (l qwenLine) typeName() (string, bool) {
	return nonEmptyStringAt(l.fields, "type")
}

// LoadEntries discovers Qwen chat files, parses them (in parallel unless
// disabled), applies the first-wins dedup in discovery order, and sorts the
// surviving entries by timestamp. Pricing is only loaded outside display mode.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	var pricing *core.PricingMap
	if shared.Mode != core.ModeDisplay {
		refreshLog := true
		if level := core.LogLevel(); level != nil && *level == 0 {
			refreshLog = false
		}
		pricing = core.LoadWithOverrides(shared.Offline, refreshLog, shared.PricingOverrides)
	}
	tz := core.ParseTZ(shared.Timezone)
	files := DiscoverChatFiles()
	// Read chat files in parallel; the first-wins dedup runs sequentially over
	// the original discovery order so the surviving record per id matches the
	// single-threaded read.
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []core.LoadedEntry {
		entries, err := readChatFile(file, tz, shared.Mode, pricing, shared)
		if err != nil {
			core.DebugLog(shared, "Failed to read Qwen chat file "+file+": "+err.Error())
			return nil
		}
		return entries
	})
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, fileEntries := range loaded {
		for i := range fileEntries {
			key := qwenEntryID(&fileEntries[i])
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, fileEntries[i])
		}
	}
	sortEntriesByTimestamp(entries)
	return entries, nil
}

// HasData reports whether any Qwen chat file exists, even when date filters
// leave no entries.
func HasData() bool {
	return len(DiscoverChatFiles()) > 0
}

func readChatFile(file string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap, shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	fallback := fileTimestamp(file, shared)
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	// Every usable Qwen line carries token counts under the usageMetadata
	// key, so lines without it are skipped before JSON parsing.
	var entries []core.LoadedEntry
	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, []byte(`"usageMetadata"`)) {
			continue
		}
		record, ok := parseQwenLine(line)
		if !ok {
			continue
		}
		if entry, ok := parseLine(file, fallback, record, tz, mode, pricing); ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func parseLine(file string, fallback int64, record qwenLine, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) (core.LoadedEntry, bool) {
	if typeName, ok := record.typeName(); !ok || typeName != "assistant" {
		return core.LoadedEntry{}, false
	}
	usage := record.usageMetadata
	if usage == nil {
		return core.LoadedEntry{}, false
	}
	displayUsage := core.TokenUsageRaw{
		InputTokens:          usage.promptTokenCount,
		OutputTokens:         usage.candidatesTokenCount,
		CacheReadInputTokens: usage.cachedContentTokenCount,
	}
	displayUsage, extraTotalTokens := applyTotalTokenFallback(displayUsage, usage.thoughtsTokenCount, usage.totalTokenCount)
	if displayUsage.InputTokens == 0 && displayUsage.OutputTokens == 0 &&
		displayUsage.CacheReadInputTokens == 0 && extraTotalTokens == 0 {
		return core.LoadedEntry{}, false
	}

	timestampText := core.FormatRFC3339Millis(fallback)
	if value, ok := nonEmptyStringAt(record.fields, "timestamp"); ok {
		if _, parses := core.ParseTSTimestamp(value); parses {
			timestampText = value
		}
	}
	timestamp, ok := core.ParseTSTimestamp(timestampText)
	if !ok {
		timestamp = fallback
	}
	project, hasProject := ProjectFromFile(file)
	if !hasProject {
		project = "unknown"
	}
	sessionID := ""
	if value, ok := nonEmptyStringAt(record.fields, "sessionId"); ok {
		sessionID = value
	} else {
		stem := filepath.Base(file)
		if ext := filepath.Ext(stem); ext != "" {
			stem = strings.TrimSuffix(stem, ext)
		}
		sessionID = project + "-" + stem
	}
	model := defaultQwenModel
	if value, ok := nonEmptyStringAt(record.fields, "model"); ok {
		model = value
	}
	billableUsage := displayUsage
	billableUsage.OutputTokens += extraTotalTokens
	cost := qwenCalculateCost(model, billableUsage, mode, pricing)
	missingPricingModel := qwenMissingPricing(model, billableUsage, mode, pricing)
	modelPtr := model
	sessionIDPtr := sessionID
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionIDPtr,
			Timestamp: timestampText,
			Message: core.UsageMessage{
				Usage: displayUsage,
				Model: &modelPtr,
			},
		},
		Timestamp:           timestamp,
		Date:                core.FormatDateTZ(timestamp, tz),
		Project:             "qwen",
		SessionID:           sessionID,
		ProjectPath:         project,
		Cost:                cost,
		ExtraTotalTokens:    extraTotalTokens,
		Model:               &modelPtr,
		MissingPricingModel: missingPricingModel,
	}, true
}

// qwenCalculateCost prices the first candidate that resolves; display mode
// prices as zero even when an explicit zero-price entry exists.
func qwenCalculateCost(model string, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) float64 {
	for _, candidate := range qwenModelCandidates(model) {
		if mode == core.ModeDisplay || pricing.Find(candidate) != nil {
			name := candidate
			return core.CalculateCostForUsage(&name, usage, nil, mode, pricing)
		}
	}
	return 0
}

func qwenMissingPricing(model string, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay {
		return nil
	}
	total := core.TotalUsageTokens(usage)
	if total == 0 || pricing == nil {
		return nil
	}
	for _, candidate := range qwenModelCandidates(model) {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

func qwenModelCandidates(model string) []string {
	return []string{model, "qwen/" + model, "alibaba/" + model}
}

func fileTimestamp(file string, shared *core.SharedArgs) int64 {
	info, err := os.Stat(file)
	if err != nil {
		core.DebugLog(shared, "Failed to read Qwen chat file timestamp for "+file+": "+err.Error())
		return time.Now().UnixMilli()
	}
	return info.ModTime().UnixMilli()
}

// qwenEntryID mirrors the reference JSON-array identity: compact JSON of
// [sessionId, timestamp, model, input, output, cacheRead, extra].
func qwenEntryID(entry *core.LoadedEntry) string {
	model := ""
	if entry.Model != nil {
		model = *entry.Model
	}
	sessionID := ""
	if entry.Data.SessionID != nil {
		sessionID = *entry.Data.SessionID
	}
	usage := entry.Data.Message.Usage
	value := []any{
		sessionID,
		entry.Data.Timestamp,
		model,
		usage.InputTokens,
		usage.OutputTokens,
		usage.CacheReadInputTokens,
		entry.ExtraTotalTokens,
	}
	encoded, err := marshalCompactJSON(value)
	if err != nil {
		return ""
	}
	return encoded
}

// marshalCompactJSON renders a value like serde_json::to_string: compact,
// no HTML escaping.
func marshalCompactJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return string(bytes.TrimRight(buffer.Bytes(), "\n")), nil
}

func sortEntriesByTimestamp(entries []core.LoadedEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
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
