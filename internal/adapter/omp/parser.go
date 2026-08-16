package omp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// ompLine is one parsed omp session record; fields token-usage does not
// consume are skipped, and unexpected field types degrade leniently like the
// reference (omp's extra fields, e.g. usage.reasoningTokens, are ignored —
// reasoning output is already inside usage.output).
type ompLine struct {
	Type      *string
	Timestamp *string
	Message   *ompMessage
}

type ompMessage struct {
	Role  *string
	Model *string
	Usage *ompUsage
}

type ompUsage struct {
	Input       uint64
	Output      uint64
	CacheRead   uint64
	CacheWrite  uint64
	TotalTokens uint64
	CostTotal   *float64
}

// parseOmpLine decodes one JSONL record; ok=false mirrors the reference's
// silently-skipped unparsable lines.
func parseOmpLine(line []byte) (ompLine, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return ompLine{}, false
	}
	parsed := ompLine{
		Type:      nonEmptyString(field(fields, "type")),
		Timestamp: nonEmptyString(field(fields, "timestamp")),
	}
	if raw := field(fields, "message"); raw != nil {
		message, ok := parseOmpMessage(raw)
		if !ok {
			return ompLine{}, false
		}
		parsed.Message = message
	}
	return parsed, true
}

// parseOmpMessage decodes the `message` block. A non-object message fails
// the whole line, as does a malformed nested `usage` value; `cost` stays
// lenient.
func parseOmpMessage(raw json.RawMessage) (*ompMessage, bool) {
	fields, ok := rawFields(raw)
	if !ok {
		return nil, false
	}
	message := &ompMessage{
		Role:  nonEmptyString(field(fields, "role")),
		Model: nonEmptyString(field(fields, "model")),
	}
	if usageRaw := field(fields, "usage"); usageRaw != nil {
		usageFields, ok := rawFields(usageRaw)
		if !ok {
			return nil, false
		}
		usage := &ompUsage{
			Input:       lenientU64(field(usageFields, "input")),
			Output:      lenientU64(field(usageFields, "output")),
			CacheRead:   lenientU64(field(usageFields, "cacheRead")),
			CacheWrite:  lenientU64(field(usageFields, "cacheWrite")),
			TotalTokens: lenientU64(field(usageFields, "totalTokens")),
		}
		// A non-object `cost` leaves display cost absent without dropping
		// the record, so treat it leniently.
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

// isOmpMessageUsage mirrors the record gate: type is absent or "message", the
// role is assistant, and usage is present.
func isOmpMessageUsage(record *ompLine) bool {
	if record.Type != nil && *record.Type != "message" {
		return false
	}
	if record.Message == nil {
		return false
	}
	return record.Message.Role != nil && *record.Message.Role == "assistant" && record.Message.Usage != nil
}

// Usable omp lines carry token counts under a `usage` key nested in a
// `message` object, so both substrings are required before JSON parsing.
var (
	ompUsageMarker   = []byte(`"usage"`)
	ompMessageMarker = []byte(`"message"`)
)

// ReadSessionFile parses one omp session file into loaded entries.
func ReadSessionFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	project := ExtractProject(path)
	sessionID := ExtractSessionID(path)
	var entries []core.LoadedEntry

	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, ompUsageMarker) || !bytes.Contains(line, ompMessageMarker) {
			continue
		}
		record, ok := parseOmpLine(line)
		if !ok || !isOmpMessageUsage(&record) {
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
			display := "[omp] " + *rawModel
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

// ExtractSessionID returns the owning session id of a session file. Top-level
// files follow the pi filename layout `<started-at>_<session-id>.jsonl` (the
// stem part after the first '_'). Files nested inside a `<started-at>_<session-id>`
// sidecar directory are omp sub-sessions (extension/design subagents); their
// usage attributes to that parent session, mirroring the Claude adapter's
// sidechain semantics. Anything unparseable falls back to the file stem.
func ExtractSessionID(path string) string {
	if parent := sidecarParentSessionID(path); parent != "" {
		return parent
	}
	stem := fileStem(path)
	if underscore := strings.Index(stem, "_"); underscore >= 0 {
		return stem[underscore+1:]
	}
	return stem
}

// sidecarParentSessionID resolves the parent session id when the file sits
// directly inside a session sidecar directory named `<started-at>_<session-id>`;
// "" when the enclosing directory is not a session stem (a project directory).
func sidecarParentSessionID(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	if !isSessionStem(dir) {
		return ""
	}
	if underscore := strings.Index(dir, "_"); underscore >= 0 {
		return dir[underscore+1:]
	}
	return dir
}

// isSessionStem reports whether a name starts with the `<YYYY-MM-DD>T…`
// timestamp prefix every session file stem (and sidecar directory) begins
// with; project directory slugs never match it.
func isSessionStem(name string) bool {
	if len(name) < 11 || name[4] != '-' || name[7] != '-' || name[10] != 'T' {
		return false
	}
	return allDigits(name[:4]) && allDigits(name[5:7]) && allDigits(name[8:10])
}

func fileStem(path string) string {
	stem := filepath.Base(path)
	if dot := strings.LastIndex(stem, "."); dot > 0 {
		return stem[:dot]
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

// entryID builds the first-wins dedupe identity for an omp entry.
func entryID(entry *core.LoadedEntry) string {
	model := ""
	if entry.Model != nil {
		model = *entry.Model
	}
	return strings.Join([]string{
		"omp",
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

// applyTotalTokenFallback ports the pi helper: a totalTokens value larger
// than the known counters fills a missing output count first, then becomes
// extra tokens.
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
