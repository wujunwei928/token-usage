package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadOptions carries what the loader needs from the CLI layer.
type LoadOptions struct {
	Shared        *core.SharedArgs
	ProjectFilter *string
}

// loadPricing builds the pricing table for token-priced runs; display mode
// skips pricing entirely, matching the reference adapters.
func loadPricing(shared *core.SharedArgs) *core.PricingMap {
	if shared.Mode == core.ModeDisplay {
		return nil
	}
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	// Ticket 10: token-usage config pricingOverrides ride on SharedArgs into the
	// pricing map load.
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}

// LoadEntries discovers, parses, and dedups Claude usage entries.
func LoadEntries(opts LoadOptions) ([]core.LoadedEntry, error) {
	shared := opts.Shared
	paths, err := ClaudePaths()
	if err != nil {
		return nil, err
	}
	core.DebugLog(shared, "Scanning Claude data directories: "+strings.Join(paths, ", "))
	files := UsageFiles(paths, opts.ProjectFilter)
	core.DebugLog(shared, "Found "+strconv.Itoa(len(files))+" JSONL usage files")
	if len(files) == 0 {
		return []core.LoadedEntry{}, nil
	}

	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	mode := shared.Mode
	loadedFiles := common.ReadFilesParallel(files, shared.SingleThread, func(file string) loadedFile {
		return readUsageFile(file, tz, mode, pricing)
	})

	dedup := newDeduper()
	for _, lf := range loadedFiles {
		for _, entry := range lf.entries {
			if opts.ProjectFilter != nil && entry.Project != *opts.ProjectFilter {
				continue
			}
			dedup.Push(entry)
		}
	}
	core.DebugLog(shared, "Kept "+strconv.Itoa(len(dedup.entries))+" usage entries after deduplication")
	return dedup.entries, nil
}

type loadedFile struct {
	timestamp *int64
	entries   []core.LoadedEntry
}

// dedupKey identifies a dedup bucket: exact includes the request ID.
type dedupKey struct {
	messageID string
	requestID string
	exact     bool
}

type deduper struct {
	exact   map[dedupKey][]int
	message map[string][]int
	entries []core.LoadedEntry
}

func newDeduper() *deduper {
	return &deduper{
		exact:   map[dedupKey][]int{},
		message: map[string][]int{},
	}
}

// Push inserts one entry, replacing an existing duplicate when the candidate
// wins (non-sidechain > larger token total > carries speed).
func (d *deduper) Push(entry core.LoadedEntry) {
	if entry.Data.Message.ID == nil {
		d.entries = append(d.entries, entry)
		return
	}
	messageID := *entry.Data.Message.ID
	exactKey := dedupKey{messageID: messageID, exact: true}
	if entry.Data.RequestID != nil {
		exactKey.requestID = *entry.Data.RequestID
	}

	if index, ok := d.findExact(exactKey); ok {
		if shouldReplaceDedupedEntry(&entry.Data, &d.entries[index].Data) {
			d.entries[index] = entry
			// Refresh the index buckets so later exact-key lookups find the
			// replacement under its new request ID (pushDedupedEntry in the
			// reference re-registers both hashes after a replacement).
			d.pushIndex(exactKey, index)
			d.pushMessageIndex(messageID, index)
		}
		return
	}

	// Sidechain logs can replay parent messages with new request IDs.
	candidateIsSidechain := isSidechainUsageEntry(&entry.Data)
	if index, ok := d.findMessageSidechain(messageID, candidateIsSidechain); ok {
		if shouldReplaceDedupedEntry(&entry.Data, &d.entries[index].Data) {
			d.entries[index] = entry
			d.pushIndex(exactKey, index)
			d.pushMessageIndex(messageID, index)
		}
		return
	}

	index := len(d.entries)
	d.entries = append(d.entries, entry)
	d.pushIndex(exactKey, index)
	d.pushMessageIndex(messageID, index)
}

func (d *deduper) findExact(key dedupKey) (int, bool) {
	for _, index := range d.exact[key] {
		existing := &d.entries[index]
		if existing.Data.Message.ID == nil || *existing.Data.Message.ID != key.messageID {
			continue
		}
		if key.requestID == "" {
			if existing.Data.RequestID == nil {
				return index, true
			}
		} else if existing.Data.RequestID != nil && *existing.Data.RequestID == key.requestID {
			return index, true
		}
	}
	return 0, false
}

func (d *deduper) findMessageSidechain(messageID string, candidateIsSidechain bool) (int, bool) {
	for _, index := range d.message[messageID] {
		existing := &d.entries[index]
		if existing.Data.Message.ID == nil || *existing.Data.Message.ID != messageID {
			continue
		}
		if candidateIsSidechain || isSidechainUsageEntry(&existing.Data) {
			return index, true
		}
	}
	return 0, false
}

func (d *deduper) pushIndex(key dedupKey, index int) {
	for _, existing := range d.exact[key] {
		if existing == index {
			return
		}
	}
	d.exact[key] = append(d.exact[key], index)
}

func (d *deduper) pushMessageIndex(messageID string, index int) {
	for _, existing := range d.message[messageID] {
		if existing == index {
			return
		}
	}
	d.message[messageID] = append(d.message[messageID], index)
}

func usageTokenTotal(data *core.UsageEntry) uint64 {
	u := data.Message.Usage
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokenCount() + u.CacheReadInputTokens
}

func isSidechainUsageEntry(entry *core.UsageEntry) bool {
	return entry.IsSidechain != nil && *entry.IsSidechain
}

func shouldReplaceDedupedEntry(candidate, existing *core.UsageEntry) bool {
	candidateSidechain := isSidechainUsageEntry(candidate)
	existingSidechain := isSidechainUsageEntry(existing)
	if candidateSidechain != existingSidechain {
		return existingSidechain
	}
	candidateTotal := usageTokenTotal(candidate)
	existingTotal := usageTokenTotal(existing)
	if candidateTotal != existingTotal {
		return candidateTotal > existingTotal
	}
	return candidate.Message.Usage.Speed != nil && existing.Message.Usage.Speed == nil
}

func readUsageFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) loadedFile {
	project := ExtractProject(path)
	sessionID, projectPath := ExtractSessionParts(path)
	lf := loadedFile{}
	content, err := os.ReadFile(path)
	if err != nil {
		return lf
	}

	usageMarker := []byte(`"usage":{`)
	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, usageMarker) {
			continue
		}
		if hasUnsupportedNullField(line) {
			continue
		}
		var data core.UsageEntry
		if !decodeUsageEntry(line, &data) {
			continue
		}
		timestamp, ok := core.ParseTSTimestamp(data.Timestamp)
		if !ok {
			continue
		}
		if lf.timestamp == nil || timestamp < *lf.timestamp {
			ts := timestamp
			lf.timestamp = &ts
		}
		if !isValidUsageEntry(&data) {
			continue
		}
		date := core.FormatDateTZ(timestamp, tz)
		cost := core.CalculateCost(&data, mode, pricing)
		missingPricingModel := core.MissingPricingModelForUsage(
			data.Message.Model, data.Message.Usage, data.CostUSD, mode, pricing)
		usageLimitResetTime := usageLimitResetTimeFromLine(line, data.IsAPIErrorMessage)
		var model *string
		if data.Message.Model != nil {
			if *data.Message.Model == "<synthetic>" {
				model = nil
			} else if data.Message.Usage.Speed != nil && *data.Message.Usage.Speed == core.SpeedFast {
				suffixed := *data.Message.Model + "-fast"
				model = &suffixed
			} else {
				dup := *data.Message.Model
				model = &dup
			}
		}
		entry := core.LoadedEntry{
			Data:                data,
			Timestamp:           timestamp,
			Date:                date,
			Project:             project,
			SessionID:           sessionID,
			ProjectPath:         projectPath,
			Cost:                cost,
			Model:               model,
			UsageLimitResetTime: usageLimitResetTime,
			MissingPricingModel: missingPricingModel,
		}
		lf.entries = append(lf.entries, entry)
		for advisorIndex, advisor := range advisorUsagesFromLine(line) {
			advisorData := data
			if advisorData.Message.ID != nil {
				suffixed := *advisorData.Message.ID + ":advisor:" + strconv.Itoa(advisorIndex)
				advisorData.Message.ID = &suffixed
			}
			advisorData.Message.Model = &advisor.Model
			advisorData.Message.Usage = advisor.Usage
			advisorData.CostUSD = nil
			missing := core.MissingPricingModelForUsage(&advisor.Model, advisor.Usage, nil, mode, pricing)
			lf.entries = append(lf.entries, core.LoadedEntry{
				Data:                advisorData,
				Timestamp:           timestamp,
				Date:                date,
				Project:             project,
				SessionID:           sessionID,
				ProjectPath:         projectPath,
				Cost:                core.CalculateCostForUsage(&advisor.Model, advisor.Usage, nil, mode, pricing),
				Model:               &advisor.Model,
				UsageLimitResetTime: usageLimitResetTime,
				MissingPricingModel: missing,
			})
		}
	}
	return lf
}

func isValidUsageEntry(data *core.UsageEntry) bool {
	if data.Version != nil && !isSemverPrefix(*data.Version) {
		return false
	}
	if data.SessionID != nil && *data.SessionID == "" {
		return false
	}
	if data.RequestID != nil && *data.RequestID == "" {
		return false
	}
	if data.Message.ID != nil && *data.Message.ID == "" {
		return false
	}
	if data.Message.Model != nil && *data.Message.Model == "" {
		return false
	}
	return true
}

func isSemverPrefix(value string) bool {
	b := []byte(value)
	index := 0
	if !consumeASCIIDigits(b, &index) || index >= len(b) || b[index] != '.' {
		return false
	}
	index++
	if !consumeASCIIDigits(b, &index) || index >= len(b) || b[index] != '.' {
		return false
	}
	index++
	return index < len(b) && b[index] >= '0' && b[index] <= '9'
}

func consumeASCIIDigits(b []byte, index *int) bool {
	start := *index
	for *index < len(b) && b[*index] >= '0' && b[*index] <= '9' {
		*index++
	}
	return *index > start
}

var unsupportedNullableFields = map[string]bool{
	"id": true, "cwd": true, "model": true, "speed": true, "costUSD": true,
	"version": true, "sessionId": true, "requestId": true, "isApiErrorMessage": true,
	"cache_read_input_tokens": true, "cache_creation_input_tokens": true,
}

func hasUnsupportedNullField(line []byte) bool {
	offset := 0
	nullMarker := []byte(":null")
	for {
		relative := bytes.Index(line[offset:], nullMarker)
		if relative < 0 {
			return false
		}
		nullIndex := offset + relative
		fieldEnd := nullIndex - 1
		if fieldEnd < 0 {
			fieldEnd = 0
		}
		if fieldEnd < len(line) && line[fieldEnd] != '"' {
			for fieldEnd > 0 && line[fieldEnd] != '"' {
				fieldEnd--
			}
		}
		if fieldEnd < len(line) && line[fieldEnd] == '"' && fieldEnd > 0 {
			fieldStart := fieldEnd - 1
			for fieldStart > 0 && line[fieldStart] != '"' {
				fieldStart--
			}
			if line[fieldStart] == '"' {
				if unsupportedNullableFields[string(line[fieldStart+1:fieldEnd])] {
					return true
				}
			}
		}
		offset = nullIndex + len(nullMarker)
		if offset > len(line) {
			return false
		}
	}
}

func usageLimitResetTimeFromLine(line []byte, isAPIErrorMessage *bool) *int64 {
	if isAPIErrorMessage == nil || !*isAPIErrorMessage {
		return nil
	}
	marker := []byte("Claude AI usage limit reached")
	markerStart := bytes.Index(line, marker)
	if markerStart < 0 {
		return nil
	}
	pipeRelative := bytes.IndexByte(line[markerStart:], '|')
	if pipeRelative < 0 {
		return nil
	}
	timestampStart := markerStart + pipeRelative + 1
	timestampEnd := timestampStart
	for timestampEnd < len(line) && line[timestampEnd] >= '0' && line[timestampEnd] <= '9' {
		timestampEnd++
	}
	if timestampStart == timestampEnd {
		return nil
	}
	seconds, err := strconv.ParseInt(string(line[timestampStart:timestampEnd]), 10, 64)
	if err != nil || seconds <= 0 {
		return nil
	}
	millis := seconds * 1000
	return &millis
}

// rawTokenUsage mirrors the serde shape of TokenUsageRaw: input_tokens and
// output_tokens are required (no serde default), the remaining counters
// default to 0, and speed must be "standard" or "fast" when present.
type rawTokenUsage struct {
	InputTokens              *uint64                `json:"input_tokens"`
	OutputTokens             *uint64                `json:"output_tokens"`
	CacheCreationInputTokens uint64                 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64                 `json:"cache_read_input_tokens"`
	Speed                    *string                `json:"speed"`
	CacheCreation            *core.CacheCreationRaw `json:"cache_creation"`
}

func (u *rawTokenUsage) valid() bool {
	return u.InputTokens != nil && u.OutputTokens != nil && validUsageSpeed(u.Speed)
}

func (u *rawTokenUsage) toCore() core.TokenUsageRaw {
	return core.TokenUsageRaw{
		InputTokens:              *u.InputTokens,
		OutputTokens:             *u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
		Speed:                    u.Speed,
		CacheCreation:            u.CacheCreation,
	}
}

func validUsageSpeed(speed *string) bool {
	if speed == nil {
		return true
	}
	return *speed == core.SpeedStandard || *speed == core.SpeedFast
}

// rawUsageMessage mirrors UsageMessage: usage is a required object.
type rawUsageMessage struct {
	Usage *rawTokenUsage `json:"usage"`
	Model *string        `json:"model"`
	ID    *string        `json:"id"`
}

// rawUsageEntry mirrors UsageEntry: timestamp and message are required.
type rawUsageEntry struct {
	SessionID         *string          `json:"sessionId"`
	Timestamp         *string          `json:"timestamp"`
	Version           *string          `json:"version"`
	Message           *rawUsageMessage `json:"message"`
	CostUSD           *float64         `json:"costUSD"`
	RequestID         *string          `json:"requestId"`
	IsAPIErrorMessage *bool            `json:"isApiErrorMessage"`
	IsSidechain       *bool            `json:"isSidechain"`
}

// decodeUsageEntry parses one JSONL line with the same acceptance rules as
// serde_json::from_slice::<UsageEntry>: type mismatches and missing required
// members (timestamp, message, message.usage, input/output tokens) reject the
// line, as do unknown speed variants.
func decodeUsageEntry(line []byte, out *core.UsageEntry) bool {
	var raw rawUsageEntry
	if err := json.Unmarshal(line, &raw); err != nil {
		return false
	}
	if raw.Timestamp == nil || raw.Message == nil || raw.Message.Usage == nil || !raw.Message.Usage.valid() {
		return false
	}
	*out = core.UsageEntry{
		SessionID:         raw.SessionID,
		Timestamp:         *raw.Timestamp,
		Version:           raw.Version,
		Message:           core.UsageMessage{Usage: raw.Message.Usage.toCore(), Model: raw.Message.Model, ID: raw.Message.ID},
		CostUSD:           raw.CostUSD,
		RequestID:         raw.RequestID,
		IsAPIErrorMessage: raw.IsAPIErrorMessage,
		IsSidechain:       raw.IsSidechain,
	}
	return true
}

type iterationsEnvelope struct {
	Message struct {
		Usage struct {
			Iterations []json.RawMessage `json:"iterations"`
		} `json:"usage"`
	} `json:"message"`
}

type advisorUsage struct {
	Model string
	Usage core.TokenUsageRaw
}

// advisorUsagesFromLine extracts advisor_message iterations. The reference
// deserializes the whole envelope strictly, so any malformed iteration (wrong
// types, missing input/output tokens, unknown speed) discards ALL iterations.
func advisorUsagesFromLine(line []byte) []advisorUsage {
	if !bytes.Contains(line, []byte(`"advisor_message"`)) {
		return nil
	}
	var envelope iterationsEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}
	type rawIteration struct {
		Kind  *string `json:"type"`
		Model *string `json:"model"`
	}
	var parsed []advisorUsage
	for _, raw := range envelope.Message.Usage.Iterations {
		var head rawIteration
		var usage rawTokenUsage
		if json.Unmarshal(raw, &head) != nil || json.Unmarshal(raw, &usage) != nil ||
			head.Kind == nil || !usage.valid() {
			return nil
		}
		if *head.Kind != "advisor_message" || head.Model == nil || *head.Model == "" {
			continue
		}
		parsed = append(parsed, advisorUsage{Model: *head.Model, Usage: usage.toCore()})
	}
	return parsed
}
