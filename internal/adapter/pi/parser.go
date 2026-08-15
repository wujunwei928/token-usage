package pi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// PiLine is one parsed pi session record; fields token-usage does not consume are
// skipped, and unexpected field types degrade leniently like the reference.
type piLine struct {
	Type      *string
	Timestamp *string
	Message   *piMessage
}

type piMessage struct {
	Role  *string
	Model *string
	Usage *piUsage
}

type piUsage struct {
	Input       uint64
	Output      uint64
	CacheRead   uint64
	CacheWrite  uint64
	TotalTokens uint64
	CostTotal   *float64
}

// parsePiLine decodes one JSONL record; ok=false mirrors the reference's
// silently-skipped unparsable lines.
func parsePiLine(line []byte) (piLine, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return piLine{}, false
	}
	parsed := piLine{
		Type:      nonEmptyString(field(fields, "type")),
		Timestamp: nonEmptyString(field(fields, "timestamp")),
	}
	if raw := field(fields, "message"); raw != nil {
		message, ok := parsePiMessage(raw)
		if !ok {
			return piLine{}, false
		}
		parsed.Message = message
	}
	return parsed, true
}

// parsePiMessage decodes the pi `message` block. A non-object message fails
// the whole line (plain Option<PiMessage> in the reference), as does a
// malformed nested `usage` value; `cost` stays lenient.
func parsePiMessage(raw json.RawMessage) (*piMessage, bool) {
	fields, ok := rawFields(raw)
	if !ok {
		return nil, false
	}
	message := &piMessage{
		Role:  nonEmptyString(field(fields, "role")),
		Model: nonEmptyString(field(fields, "model")),
	}
	if usageRaw := field(fields, "usage"); usageRaw != nil {
		usageFields, ok := rawFields(usageRaw)
		if !ok {
			return nil, false
		}
		usage := &piUsage{
			Input:       lenientU64(field(usageFields, "input")),
			Output:      lenientU64(field(usageFields, "output")),
			CacheRead:   lenientU64(field(usageFields, "cacheRead")),
			CacheWrite:  lenientU64(field(usageFields, "cacheWrite")),
			TotalTokens: lenientU64(field(usageFields, "totalTokens")),
		}
		// A non-object `cost` previously left display cost absent without
		// dropping the record, so treat it leniently.
		if costRaw := field(usageFields, "cost"); costRaw != nil && isJSONObject(costRaw) {
			var costFields map[string]json.RawMessage
			if err := json.Unmarshal(costRaw, &costFields); err == nil {
				usage.CostTotal = lenientF64(field(costFields, "total"))
			}
		}
		message.Usage = usage
	}
	return message, true
}

// isPiMessageUsage mirrors the record gate: type is absent or "message", the
// role is assistant, and usage is present.
func isPiMessageUsage(record *piLine) bool {
	if record.Type != nil && *record.Type != "message" {
		return false
	}
	if record.Message == nil {
		return false
	}
	return record.Message.Role != nil && *record.Message.Role == "assistant" && record.Message.Usage != nil
}

// Usable pi lines carry token counts under a `usage` key nested in a
// `message` object, so both substrings are required before JSON parsing.
var (
	piUsageMarker   = []byte(`"usage"`)
	piMessageMarker = []byte(`"message"`)
)

// ReadSessionFile parses one pi session file into loaded entries.
func ReadSessionFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	project := ExtractProject(path)
	sessionID := ExtractSessionID(path)
	var entries []core.LoadedEntry

	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, piUsageMarker) || !bytes.Contains(line, piMessageMarker) {
			continue
		}
		record, ok := parsePiLine(line)
		if !ok || !isPiMessageUsage(&record) {
			continue
		}
		if record.Timestamp == nil {
			continue
		}
		timestamp, ok := core.ParseTSTimestamp(*record.Timestamp)
		if !ok {
			continue
		}
		usageValue := record.Message.Usage
		usage := core.TokenUsageRaw{
			InputTokens:              usageValue.Input,
			OutputTokens:             usageValue.Output,
			CacheCreationInputTokens: usageValue.CacheWrite,
			CacheReadInputTokens:     usageValue.CacheRead,
		}
		usage, extraTotalTokens := applyTotalTokenFallback(usage, 0, usageValue.TotalTokens)
		if core.TotalUsageTokens(usage)+extraTotalTokens == 0 {
			continue
		}
		rawModel := record.Message.Model
		var model *string
		if rawModel != nil {
			display := "[pi] " + *rawModel
			model = &display
		}
		displayCost := usageValue.CostTotal
		cost := core.CalculateCostForUsage(model, usage, displayCost, mode, pricing)
		missingPricingModel := core.MissingPricingModelForUsage(model, usage, displayCost, mode, pricing)
		timestampText := *record.Timestamp
		entries = append(entries, core.LoadedEntry{
			Date:                core.FormatDateTZ(timestamp, tz),
			Timestamp:           timestamp,
			Project:             project,
			SessionID:           sessionID,
			ProjectPath:         project,
			Cost:                cost,
			ExtraTotalTokens:    extraTotalTokens,
			Model:               model,
			MissingPricingModel: missingPricingModel,
			Data: core.UsageEntry{
				SessionID: &sessionID,
				Timestamp: timestampText,
				Message: core.UsageMessage{
					Usage: usage,
					Model: model,
				},
				CostUSD: displayCost,
			},
		})
	}
	return entries, nil
}

// ExtractSessionID returns the part of the file stem after the first '_' (the
// pi filename layout `<started-at>_<session-id>.jsonl`).
func ExtractSessionID(path string) string {
	stem := filepath.Base(path)
	if dot := strings.LastIndex(stem, "."); dot > 0 {
		stem = stem[:dot]
	}
	if underscore := strings.Index(stem, "_"); underscore >= 0 {
		return stem[underscore+1:]
	}
	return stem
}

// ExtractProject returns the path component right after the first "sessions"
// component anywhere in the file path.
func ExtractProject(path string) string {
	previousWasSessions := false
	for _, segment := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if previousWasSessions {
			return segment
		}
		previousWasSessions = segment == "sessions"
	}
	return "unknown"
}

// entryID builds the first-wins dedupe identity for a pi entry.
func entryID(entry *core.LoadedEntry) string {
	model := ""
	if entry.Model != nil {
		model = *entry.Model
	}
	return strings.Join([]string{
		"pi",
		entry.Project,
		entry.SessionID,
		entry.Data.Timestamp,
		model,
		strconv.FormatUint(entry.Data.Message.Usage.InputTokens, 10),
		strconv.FormatUint(entry.Data.Message.Usage.OutputTokens, 10),
		strconv.FormatUint(entry.Data.Message.Usage.CacheCreationInputTokens, 10),
		strconv.FormatUint(entry.Data.Message.Usage.CacheReadInputTokens, 10),
		strconv.FormatUint(entry.ExtraTotalTokens, 10),
		rustDisplayF64(entry.Cost),
	}, ":")
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

// rustDisplayF64 formats like Rust's f64 Display (shortest round-trip digits,
// no exponent, "0" for zero), used only inside dedupe identities.
func rustDisplayF64(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// sortEntriesByTimestamp stable-sorts entries by their millisecond timestamp.
func sortEntriesByTimestamp(entries []core.LoadedEntry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
}
