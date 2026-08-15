package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// ServiceTier classifies a recorded Codex service tier.
type ServiceTier int

// Service tiers.
const (
	TierStandard ServiceTier = iota
	TierFast
)

// RawUsage is one token-usage record with Codex/OpenAI field spellings folded
// into a canonical shape.
type RawUsage struct {
	InputTokens           uint64
	CachedInputTokens     uint64
	OutputTokens          uint64
	ReasoningOutputTokens uint64
	TotalTokens           uint64
}

// TokenUsageEvent is one parsed usage event from a Codex log.
type TokenUsageEvent struct {
	SessionID             string
	Timestamp             string
	Model                 *string
	InputTokens           uint64
	CachedInputTokens     uint64
	OutputTokens          uint64
	ReasoningOutputTokens uint64
	TotalTokens           uint64
	IsFallbackModel       bool
	ServiceTier           *ServiceTier
}

const (
	eventMsgFinder            = `"type":"event_msg"`
	turnContextFinder         = `"type":"turn_context"`
	tokenCountFinder          = `"type":"token_count"`
	threadSettingsFinder      = `"type":"thread_settings_applied"`
	threadSettingsPlain       = `thread_settings_applied`
	compactTypeFinder         = `"type":`
	typeKeyFinder             = `"type"`
	usageFinder               = `"usage":`
	inputTokensFinder         = `"input_tokens":`
	promptTokensFinder        = `"prompt_tokens":`
	maxNestedLineLen          = 64 * 1024
	rewrittenBurstPauseMillis = int64(1000)
	codexAutoReviewModel      = "codex-auto-review"
)

// codexAutoReviewFallbacks is the embedded release ladder for the synthetic
// codex-auto-review model; copied verbatim from the reference crate.
const codexAutoReviewFallbacksJSON = `[
	{"releasedOn": "2026-04-23", "model": "gpt-5.5"},
	{"releasedOn": "2026-03-05", "model": "gpt-5.4"},
	{"releasedOn": "2026-02-05", "model": "gpt-5.3-codex"},
	{"releasedOn": "2025-12-11", "model": "gpt-5.2-codex"},
	{"releasedOn": "2025-11-13", "model": "gpt-5.1-codex"},
	{"releasedOn": "2025-09-15", "model": "gpt-5-codex"},
	{"releasedOn": "2025-08-07", "model": "gpt-5"}
]`

type autoReviewFallback struct {
	ReleasedOn string `json:"releasedOn"`
	Model      string `json:"model"`
}

var autoReviewFallbacks = func() []autoReviewFallback {
	var fallbacks []autoReviewFallback
	if err := json.Unmarshal([]byte(codexAutoReviewFallbacksJSON), &fallbacks); err != nil {
		panic("embedded codex-auto-review fallback snapshot must parse: " + err.Error())
	}
	return fallbacks
}()

type lineKind int

const (
	lineNone lineKind = iota
	lineSession
	lineHeadless
)

// codexLineUsageKind classifies a log line by cheap byte scans first, falling
// back to a whitespace-tolerant "type" key scan.
func codexLineUsageKind(line []byte) lineKind {
	hasEventMsg := bytes.Contains(line, []byte(eventMsgFinder))
	hasTokenCount := hasEventMsg && bytes.Contains(line, []byte(tokenCountFinder))
	hasThreadSettings := hasEventMsg && bytes.Contains(line, []byte(threadSettingsFinder))
	if bytes.Contains(line, []byte(turnContextFinder)) || hasTokenCount || hasThreadSettings {
		return lineSession
	}
	hasCompactType := bytes.Contains(line, []byte(compactTypeFinder))
	hasNestedTokenCount := !hasEventMsg && hasCompactType && len(line) < maxNestedLineLen &&
		bytes.Contains(line, []byte(tokenCountFinder))
	hasNestedThreadSettings := !hasEventMsg && hasCompactType && len(line) < maxNestedLineLen &&
		bytes.Contains(line, []byte(threadSettingsPlain))
	if hasEventMsg || hasNestedTokenCount || hasNestedThreadSettings || !hasCompactType {
		hasTurnContext, hasEventMsg2, hasTokenCount2, hasThreadSettings2 := codexLineTypeFlags(line)
		if hasTurnContext || (hasEventMsg2 && (hasTokenCount2 || hasThreadSettings2)) {
			return lineSession
		}
	}
	if bytes.Contains(line, []byte(usageFinder)) ||
		bytes.Contains(line, []byte(inputTokensFinder)) ||
		bytes.Contains(line, []byte(promptTokensFinder)) {
		return lineHeadless
	}
	return lineNone
}

func codexLineTypeFlags(line []byte) (hasTurnContext, hasEventMsg, hasTokenCount, hasThreadSettings bool) {
	start := 0
	for {
		index := bytes.Index(line[start:], []byte(typeKeyFinder))
		if index < 0 {
			return
		}
		keyStart := start + index
		cursor := skipJSONWhitespace(line, keyStart+len(typeKeyFinder))
		if cursor >= len(line) || line[cursor] != ':' {
			start = keyStart + len(typeKeyFinder)
			continue
		}
		cursor = skipJSONWhitespace(line, cursor+1)
		if cursor >= len(line) || line[cursor] != '"' {
			start = cursor + 1
			continue
		}
		cursor++
		hasTurnContext = hasTurnContext || jsonStringValueMatches(line, cursor, "turn_context")
		hasEventMsg = hasEventMsg || jsonStringValueMatches(line, cursor, "event_msg")
		hasTokenCount = hasTokenCount || jsonStringValueMatches(line, cursor, "token_count")
		hasThreadSettings = hasThreadSettings || jsonStringValueMatches(line, cursor, "thread_settings_applied")
		if hasTurnContext || (hasEventMsg && (hasTokenCount || hasThreadSettings)) {
			return
		}
		start = cursor + 1
	}
}

func jsonStringValueMatches(line []byte, start int, value string) bool {
	if start+len(value) > len(line) {
		return false
	}
	if string(line[start:start+len(value)]) != value {
		return false
	}
	return start+len(value) < len(line) && line[start+len(value)] == '"'
}

func skipJSONWhitespace(line []byte, index int) int {
	for index < len(line) {
		switch line[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

// ---------------------------------------------------------------------------
// Lossy JSON field helpers mirroring the reference's serde visitors
// ---------------------------------------------------------------------------

// objectFields returns the map for a JSON object; ok=false for absent, null,
// and any non-object value.
func objectFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

// stringField extracts a JSON string field. hardFail marks values that are
// neither strings nor null (the reference's typed deserializer rejects them).
func stringField(fields map[string]json.RawMessage, key string) (value *string, hardFail bool) {
	raw, ok := fields[key]
	if !ok || len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, true
	}
	return &s, false
}

// u64Lossy parses a token counter: integers and numeric strings are accepted,
// other scalars become absent, and objects/arrays are hard failures (the
// reference's lossy u64 visitor).
func u64Lossy(raw json.RawMessage) (value *uint64, hardFail bool) {
	if len(raw) == 0 {
		return nil, false
	}
	switch raw[0] {
	case 'n': // null
		return nil, false
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, true
		}
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return nil, false
		}
		parsed, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return nil, false
		}
		return &parsed, false
	case '{', '[':
		return nil, true
	}
	c := raw[0]
	isNumber := c == '-' || (c >= '0' && c <= '9')
	if !isNumber {
		// Booleans drop the field without failing.
		return nil, false
	}
	if bytes.ContainsAny(raw, ".eE") {
		// Floats drop the field without failing.
		return nil, false
	}
	parsed, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		// Negative or overflowing integer literals parse as floats in
		// serde_json, which the lossy visitor drops.
		return nil, false
	}
	return &parsed, false
}

// parseRawUsage implements CodexRawUsage::deserialize: field spelling
// fallbacks and a derived total that never double counts reasoning.
func parseRawUsage(raw json.RawMessage) (usage *RawUsage, hardFail bool) {
	fields, ok := objectFields(raw)
	if !ok {
		return nil, false
	}
	pick := func(names ...string) (uint64, bool, bool) {
		for _, name := range names {
			field, ok := fields[name]
			if !ok {
				continue
			}
			value, hard := u64Lossy(field)
			if hard {
				return 0, false, true
			}
			if value != nil {
				return *value, true, false
			}
		}
		return 0, false, false
	}
	input, _, hard := pick("input_tokens", "prompt_tokens", "input")
	if hard {
		return nil, true
	}
	cached, _, hard := pick("cached_input_tokens", "cache_read_input_tokens", "cached_tokens")
	if hard {
		return nil, true
	}
	output, _, hard := pick("output_tokens", "completion_tokens", "output")
	if hard {
		return nil, true
	}
	reasoning, _, hard := pick("reasoning_output_tokens", "reasoning_tokens")
	if hard {
		return nil, true
	}
	total, hasTotal, hard := pick("total_tokens")
	if hard {
		return nil, true
	}
	if !hasTotal || total == 0 {
		// Codex reports reasoning as a subset of output, and a recorded zero
		// total means the field is unusable, so derive input+output.
		total = saturatingAdd(input, output)
	}
	return &RawUsage{
		InputTokens:           input,
		CachedInputTokens:     cached,
		OutputTokens:          output,
		ReasoningOutputTokens: reasoning,
		TotalTokens:           total,
	}, false
}

func saturatingAdd(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return ^uint64(0)
	}
	return sum
}

// timestampField is a string-or-number timestamp.
type timestampField struct {
	set   bool
	str   string
	num   uint64
	isNum bool
}

// timestampValue applies the strict CodexTimestamp untagged rules: absent,
// null, string, or u64 integer are valid and anything else is a hard failure.
func timestampValue(raw json.RawMessage) (field timestampField, hardFail bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return timestampField{}, false
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return timestampField{}, true
		}
		return timestampField{set: true, str: s}, false
	case '{', '[', 't', 'f':
		return timestampField{}, true
	}
	c := raw[0]
	if c != '-' && (c < '0' || c > '9') {
		return timestampField{}, true
	}
	if bytes.ContainsAny(raw, ".eE") || c == '-' {
		return timestampField{}, true
	}
	parsed, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		return timestampField{}, true
	}
	return timestampField{set: true, num: parsed, isNum: true}, false
}

// normalize renders a string via strict parse+reformat and a number as epoch
// millis; nil means unusable.
func (t timestampField) normalize() *string {
	if !t.set {
		return nil
	}
	if t.isNum {
		return normalizeEpochMillis(t.num)
	}
	trimmed := strings.TrimSpace(t.str)
	if trimmed == "" {
		return nil
	}
	if millis, ok := core.ParseTSTimestamp(trimmed); ok {
		formatted := core.FormatRFC3339Millis(millis)
		return &formatted
	}
	return nil
}

// rawOrNormalized keeps a well-formed date string verbatim but rewrites
// anything else through the parser.
func (t timestampField) rawOrNormalized() *string {
	if !t.set {
		return nil
	}
	if t.isNum {
		return normalizeEpochMillis(t.num)
	}
	trimmed := strings.TrimSpace(t.str)
	if trimmed == "" {
		return nil
	}
	if codexTimestampDate(trimmed) != "" {
		return &trimmed
	}
	return t.normalize()
}

func normalizeEpochMillis(raw uint64) *string {
	millis := raw
	if raw <= 10_000_000_000 {
		product := raw * 1000
		if product/1000 != raw {
			return nil
		}
		millis = product
	}
	const i64max = uint64(^uint64(0) >> 1)
	if millis > i64max {
		millis = i64max
	}
	formatted := core.FormatRFC3339Millis(int64(millis))
	return &formatted
}

// codexTimestampDate validates the leading YYYY-MM-DD of a timestamp string.
func codexTimestampDate(timestamp string) string {
	if len(timestamp) < 10 {
		return ""
	}
	date := timestamp[:10]
	b := []byte(date)
	if !(len(b) == 10 && isDigits(b[0:4]) && b[4] == '-' && isDigits(b[5:7]) && b[7] == '-' && isDigits(b[8:10])) {
		return ""
	}
	year := digitRun(b[0:4])
	month := digitRun(b[5:7])
	day := digitRun(b[8:10])
	if year < 0 || month < 0 || day < 0 {
		return ""
	}
	maxDay := daysInMonth(year, month)
	if maxDay == 0 || day < 1 || day > maxDay {
		return ""
	}
	return date
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func digitRun(b []byte) int {
	value := 0
	for _, c := range b {
		digit := int(c - '0')
		if digit > 9 {
			return -1
		}
		value = value*10 + digit
	}
	return value
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(year) {
			return 29
		}
		return 28
	}
	return 0
}

func isLeapYear(year int) bool {
	return year%4 == 0 && year%100 != 0 || year%400 == 0
}

// ---------------------------------------------------------------------------
// Shared payload pieces
// ---------------------------------------------------------------------------

type modelMetadata struct {
	model *string
}

func parseModelMetadata(raw json.RawMessage) (metadata *modelMetadata, hardFail bool) {
	fields, ok := objectFields(raw)
	if !ok {
		return nil, false
	}
	model, hard := stringField(fields, "model")
	if hard {
		return nil, true
	}
	return &modelMetadata{model: model}, false
}

func modelFromParts(model, modelName *string, metadata *modelMetadata) *string {
	if s := nonEmptyString(model); s != nil {
		return s
	}
	if s := nonEmptyString(modelName); s != nil {
		return s
	}
	if metadata != nil {
		return nonEmptyString(metadata.model)
	}
	return nil
}

func nonEmptyString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ---------------------------------------------------------------------------
// Session log entries (strict parse: hard field failures skip the line)
// ---------------------------------------------------------------------------

type threadSettings struct {
	serviceTier *string
}

func parseThreadSettings(raw json.RawMessage) (settings *threadSettings, hardFail bool) {
	fields, ok := objectFields(raw)
	if !ok {
		return nil, false
	}
	tier, hard := stringField(fields, "service_tier")
	if hard {
		return nil, true
	}
	return &threadSettings{serviceTier: tier}, false
}

type codexInfo struct {
	lastUsage  *RawUsage
	totalUsage *RawUsage
	model      *string
	modelName  *string
	metadata   *modelMetadata
}

func parseCodexInfo(raw json.RawMessage) (info *codexInfo, hardFail bool) {
	fields, ok := objectFields(raw)
	if !ok {
		return nil, false
	}
	info = &codexInfo{}
	if raw, ok := fields["last_token_usage"]; ok {
		usage, hard := parseRawUsage(raw)
		if hard {
			return nil, true
		}
		info.lastUsage = usage
	}
	if raw, ok := fields["total_token_usage"]; ok {
		usage, hard := parseRawUsage(raw)
		if hard {
			return nil, true
		}
		info.totalUsage = usage
	}
	model, hard := stringField(fields, "model")
	if hard {
		return nil, true
	}
	modelName, hard := stringField(fields, "model_name")
	if hard {
		return nil, true
	}
	metadata, hard := parseModelMetadata(fields["metadata"])
	if hard {
		return nil, true
	}
	info.model = model
	info.modelName = modelName
	info.metadata = metadata
	return info, false
}

type sessionPayload struct {
	payloadType    *string
	info           *codexInfo
	model          *string
	modelName      *string
	metadata       *modelMetadata
	threadSettings *threadSettings
}

func parseSessionPayload(raw json.RawMessage) (payload *sessionPayload, hardFail bool) {
	fields, ok := objectFields(raw)
	if !ok {
		return nil, false
	}
	payloadType, hard := stringField(fields, "type")
	if hard {
		return nil, true
	}
	info, hard := parseCodexInfo(fields["info"])
	if hard {
		return nil, true
	}
	model, hard := stringField(fields, "model")
	if hard {
		return nil, true
	}
	modelName, hard := stringField(fields, "model_name")
	if hard {
		return nil, true
	}
	metadata, hard := parseModelMetadata(fields["metadata"])
	if hard {
		return nil, true
	}
	settings, hard := parseThreadSettings(fields["thread_settings"])
	if hard {
		return nil, true
	}
	return &sessionPayload{
		payloadType:    payloadType,
		info:           info,
		model:          model,
		modelName:      modelName,
		metadata:       metadata,
		threadSettings: settings,
	}, false
}

type sessionEntry struct {
	entryType *string
	timestamp timestampField
	payload   *sessionPayload
}

func parseSessionEntry(line []byte) (entry sessionEntry, ok bool) {
	fields, valid := objectFields(line)
	if !valid {
		return entry, false
	}
	entryType, hard := stringField(fields, "type")
	if hard {
		return entry, false
	}
	timestamp, hard := timestampValue(fields["timestamp"])
	if hard {
		return entry, false
	}
	payload, hard := parseSessionPayload(fields["payload"])
	if hard {
		return entry, false
	}
	return sessionEntry{
		entryType: entryType,
		timestamp: timestamp,
		payload:   payload,
	}, true
}

// sessionTimestamp mirrors codex_session_timestamp: strings pass through raw,
// numbers normalize to epoch millis.
func (e sessionEntry) sessionTimestamp() *string {
	if !e.timestamp.set {
		return nil
	}
	if e.timestamp.isNum {
		return e.timestamp.normalize()
	}
	trimmed := strings.TrimSpace(e.timestamp.str)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (e sessionEntry) timestampMillis() (int64, bool) {
	value := e.sessionTimestamp()
	if value == nil {
		return 0, false
	}
	return core.ParseTSTimestamp(*value)
}

// ---------------------------------------------------------------------------
// Headless (codex exec) entries: value-mode semantics — a field with an
// incompatible type is dropped, not fatal (the reference falls back from its
// typed struct to a generic Value parse).
// ---------------------------------------------------------------------------

type resultFields struct {
	timestamp      timestampField
	createdAt      timestampField
	createdAtCamel timestampField
	usage          *RawUsage
	model          *string
	modelName      *string
	metadata       *modelMetadata
}

func parseResultFields(raw json.RawMessage) *resultFields {
	obj, ok := objectFields(raw)
	if !ok {
		return nil
	}
	out := &resultFields{}
	if field, ok := obj["timestamp"]; ok {
		ts, _ := timestampValue(field)
		out.timestamp = ts
	}
	if field, ok := obj["created_at"]; ok {
		ts, _ := timestampValue(field)
		out.createdAt = ts
	}
	if field, ok := obj["createdAt"]; ok {
		ts, _ := timestampValue(field)
		out.createdAtCamel = ts
	}
	if field, ok := obj["usage"]; ok {
		usage, hard := parseRawUsage(field)
		if !hard {
			out.usage = usage
		}
	}
	out.model, _ = stringField(obj, "model")
	out.modelName, _ = stringField(obj, "model_name")
	out.metadata, _ = parseModelMetadata(obj["metadata"])
	return out
}

type headlessEntry struct {
	timestamp      timestampField
	createdAt      timestampField
	createdAtCamel timestampField
	data           *resultFields
	result         *resultFields
	response       *resultFields
	usage          *RawUsage
	model          *string
	modelName      *string
	metadata       *modelMetadata
}

func parseHeadlessEntry(line []byte) (entry headlessEntry, ok bool) {
	fields, valid := objectFields(line)
	if !valid {
		return entry, false
	}
	if field, ok := fields["timestamp"]; ok {
		ts, _ := timestampValue(field)
		entry.timestamp = ts
	}
	if field, ok := fields["created_at"]; ok {
		ts, _ := timestampValue(field)
		entry.createdAt = ts
	}
	if field, ok := fields["createdAt"]; ok {
		ts, _ := timestampValue(field)
		entry.createdAtCamel = ts
	}
	if field, ok := fields["data"]; ok {
		entry.data = parseResultFields(field)
	}
	if field, ok := fields["result"]; ok {
		entry.result = parseResultFields(field)
	}
	if field, ok := fields["response"]; ok {
		entry.response = parseResultFields(field)
	}
	if field, ok := fields["usage"]; ok {
		usage, hard := parseRawUsage(field)
		if !hard {
			entry.usage = usage
		}
	}
	entry.model, _ = stringField(fields, "model")
	entry.modelName, _ = stringField(fields, "model_name")
	entry.metadata, _ = parseModelMetadata(fields["metadata"])
	return entry, true
}

func (e *headlessEntry) usageChain() *RawUsage {
	if e.usage != nil {
		return e.usage
	}
	for _, fields := range []*resultFields{e.data, e.result, e.response} {
		if fields != nil && fields.usage != nil {
			return fields.usage
		}
	}
	return nil
}

func (e *headlessEntry) modelChain() *string {
	if model := modelFromParts(e.model, e.modelName, e.metadata); model != nil {
		return model
	}
	for _, fields := range []*resultFields{e.data, e.result, e.response} {
		if fields == nil {
			continue
		}
		if model := modelFromParts(fields.model, fields.modelName, fields.metadata); model != nil {
			return model
		}
	}
	return nil
}

func (e *headlessEntry) eventTimestamp() *string {
	for _, ts := range []timestampField{e.timestamp, e.createdAt, e.createdAtCamel} {
		if normalized := ts.normalize(); normalized != nil {
			return normalized
		}
	}
	for _, fields := range []*resultFields{e.data, e.result, e.response} {
		if fields == nil {
			continue
		}
		for _, ts := range []timestampField{fields.timestamp, fields.createdAt, fields.createdAtCamel} {
			if normalized := ts.normalize(); normalized != nil {
				return normalized
			}
		}
	}
	return nil
}

func (e *headlessEntry) modelTimestamp() *string {
	for _, ts := range []timestampField{e.timestamp, e.createdAt, e.createdAtCamel} {
		if value := ts.rawOrNormalized(); value != nil {
			return value
		}
	}
	for _, fields := range []*resultFields{e.data, e.result, e.response} {
		if fields == nil {
			continue
		}
		for _, ts := range []timestampField{fields.timestamp, fields.createdAt, fields.createdAtCamel} {
			if value := ts.rawOrNormalized(); value != nil {
				return value
			}
		}
	}
	return nil
}

func (e *headlessEntry) normalizeUsage() *RawUsage {
	usage := e.usageChain()
	if usage == nil {
		return nil
	}
	if usage.InputTokens == 0 && usage.CachedInputTokens == 0 &&
		usage.OutputTokens == 0 && usage.ReasoningOutputTokens == 0 &&
		usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

// ---------------------------------------------------------------------------
// Model resolution and fallbacks
// ---------------------------------------------------------------------------

type modelResolver struct {
	current           *string
	currentIsFallback bool
}

func resolveCodexUsageModel(parsedModel *string, timestamp string, r *modelResolver) (model *string, isFallback bool) {
	if parsedModel != nil {
		copied := *parsedModel
		r.current = &copied
		r.currentIsFallback = false
	}
	if model = parsedModel; model == nil && r.current != nil {
		copied := *r.current
		model = &copied
	}
	if model == nil {
		fallbackModel := "gpt-5"
		r.current = &fallbackModel
		r.currentIsFallback = true
		isFallback = true
		model = &fallbackModel
	}
	if r.current != nil && r.currentIsFallback {
		isFallback = true
	}
	if fallback := codexLogModelFallback(*model, timestamp); fallback != "" {
		isFallback = true
		model = &fallback
	}
	return model, isFallback
}

func codexLogModelFallback(model, timestamp string) string {
	if model != codexAutoReviewModel {
		return ""
	}
	date := codexTimestampDate(timestamp)
	if date == "" {
		return "gpt-5"
	}
	for _, fallback := range autoReviewFallbacks {
		if date >= fallback.ReleasedOn {
			return fallback.Model
		}
	}
	return "gpt-5"
}

func codexServiceTier(value string) *ServiceTier {
	switch value {
	// Both spellings mean non-priority pricing and occur in the same Codex
	// version on the same day; which one is written depends on the client.
	case "default", "standard":
		tier := TierStandard
		return &tier
	case "fast", "priority":
		tier := TierFast
		return &tier
	}
	return nil
}

// mergeServiceTiers preserves explicit metadata over an unclassified copy and
// resolves conflicts toward Standard so results are order-independent.
func mergeServiceTiers(current, incoming *ServiceTier) *ServiceTier {
	if (current != nil && *current == TierStandard) || (incoming != nil && *incoming == TierStandard) {
		tier := TierStandard
		return &tier
	}
	if (current != nil && *current == TierFast) || (incoming != nil && *incoming == TierFast) {
		tier := TierFast
		return &tier
	}
	return nil
}

func fileModifiedTimestamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return core.FormatRFC3339Millis(0)
	}
	return core.FormatRFC3339Millis(info.ModTime().UnixMilli())
}

func subtractRawUsage(current RawUsage, previous *RawUsage) RawUsage {
	if previous == nil {
		return current
	}
	sub := func(a, b uint64) uint64 {
		if b >= a {
			return 0
		}
		return a - b
	}
	return RawUsage{
		InputTokens:           sub(current.InputTokens, previous.InputTokens),
		CachedInputTokens:     sub(current.CachedInputTokens, previous.CachedInputTokens),
		OutputTokens:          sub(current.OutputTokens, previous.OutputTokens),
		ReasoningOutputTokens: sub(current.ReasoningOutputTokens, previous.ReasoningOutputTokens),
		TotalTokens:           sub(current.TotalTokens, previous.TotalTokens),
	}
}

// SessionID derives the event session id: the file path relative to the
// sessions directory without its extension ("unknown" when empty).
func SessionID(sessionsDir, path string) string {
	relative := stripPathPrefix(sessionsDir, path)
	if ext := pathExt(relative); ext != "" {
		relative = strings.TrimSuffix(relative, ext)
	}
	var parts []string
	for _, part := range strings.Split(relative, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	sessionID := strings.Join(parts, "/")
	if sessionID == "" {
		return "unknown"
	}
	return sessionID
}

func stripPathPrefix(base, target string) string {
	if base == "" || !strings.HasPrefix(target, base) {
		return target
	}
	rest := target[len(base):]
	if rest == "" {
		return ""
	}
	if rest[0] == '/' {
		return rest[1:]
	}
	// Only a full component boundary counts as a prefix.
	if base[len(base)-1] == '/' {
		return rest
	}
	return target
}

func pathExt(p string) string {
	lastSlash := strings.LastIndexAny(p, "/")
	name := p
	if lastSlash >= 0 {
		name = p[lastSlash+1:]
	}
	dot := strings.LastIndex(name, ".")
	if dot <= 0 {
		return ""
	}
	return name[dot:]
}

// ---------------------------------------------------------------------------
// Replay filtering
// ---------------------------------------------------------------------------

const (
	replayMatching = iota
	replaySkipping
	replayDone
)

// replayFilter tracks how far a forked session's leading events still match
// the history it replayed from its parent.
type replayFilter struct {
	phase     int
	prefix    []RawUsage
	index     int
	skipStart int64
	path      string
}

func newReplayFilter(prefix []RawUsage, path string) *replayFilter {
	if prefix == nil {
		return &replayFilter{phase: replayDone, path: path}
	}
	return &replayFilter{phase: replayMatching, prefix: prefix, path: path}
}

// filter reports whether the event is replayed history and must be skipped.
func (f *replayFilter) filter(event *TokenUsageEvent) bool {
	for {
		switch f.phase {
		case replayMatching:
			usage := RawUsage{
				InputTokens:           event.InputTokens,
				CachedInputTokens:     event.CachedInputTokens,
				OutputTokens:          event.OutputTokens,
				ReasoningOutputTokens: event.ReasoningOutputTokens,
				TotalTokens:           event.TotalTokens,
			}
			if f.index < len(f.prefix) && f.prefix[f.index] == usage {
				f.index++
				return true
			}
			// Nothing matched, so the parent stream cannot anchor this
			// replay: fall back to skipping the rewritten burst.
			if f.index == 0 {
				if start, ok := detectRewrittenBurst(f.path); ok {
					f.phase = replaySkipping
					f.skipStart = start
					continue
				}
			}
			f.phase = replayDone
		case replaySkipping:
			if timestamp, ok := core.ParseTSTimestamp(event.Timestamp); ok {
				delta := timestamp - f.skipStart
				if delta >= 0 && delta <= rewrittenBurstPauseMillis {
					f.skipStart = timestamp
					return true
				}
			}
			f.phase = replayDone
		case replayDone:
			return false
		}
	}
}

// detectRewrittenBurst finds a burst of replayed usage at the head of path: a
// session that opens with two usage events written within a second replayed a
// history it did not spend.
func detectRewrittenBurst(path string) (int64, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 128*1024)
	var first int64
	hasFirst := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			return 0, false
		}
		if codexLineUsageKind(line) == lineSession {
			if entry, ok := parseSessionEntry(line); ok {
				if isUsageEntry(entry) {
					if timestamp, ok := entry.timestampMillis(); ok {
						if !hasFirst {
							first = timestamp
							hasFirst = true
						} else {
							delta := timestamp - first
							if delta >= 0 && delta <= rewrittenBurstPauseMillis {
								return first, true
							}
							return 0, false
						}
					}
				}
			}
		}
		if readErr != nil {
			return 0, false
		}
	}
}

func isUsageEntry(entry sessionEntry) bool {
	return entry.entryType != nil && *entry.entryType == "event_msg" &&
		entry.payload != nil && entry.payload.payloadType != nil &&
		*entry.payload.payloadType == "token_count" &&
		entry.payload.info != nil &&
		(entry.payload.info.lastUsage != nil || entry.payload.info.totalUsage != nil)
}

// ---------------------------------------------------------------------------
// File visiting
// ---------------------------------------------------------------------------

// VisitSessionFile visits every usage event in a Codex session log.
// replayedPrefix is nil for sessions that are not forks and non-nil (possibly
// empty) for forks whose copied history must not be counted twice.
func VisitSessionFile(sessionsDir, path string, replayedPrefix []RawUsage, visit func(TokenUsageEvent)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 128*1024)
	sessionID := SessionID(sessionsDir, path)
	var previousTotals *RawUsage
	resolver := &modelResolver{}
	var currentServiceTier *ServiceTier
	fallbackTimestamp := fileModifiedTimestamp(path)
	filter := newReplayFilter(replayedPrefix, path)

	emit := func(event TokenUsageEvent) {
		if !filter.filter(&event) {
			visit(event)
		}
	}

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			return
		}
		switch codexLineUsageKind(line) {
		case lineSession:
			if entry, ok := parseSessionEntry(line); ok {
				visitSessionEntry(sessionID, entry, &previousTotals, resolver, &currentServiceTier, emit)
			}
		case lineHeadless:
			if entry, ok := parseHeadlessEntry(line); ok {
				addHeadlessEvent(sessionID, entry, fallbackTimestamp, resolver, emit)
			}
		}
		if readErr != nil {
			return
		}
	}
}

func visitSessionEntry(sessionID string, entry sessionEntry, previousTotals **RawUsage, resolver *modelResolver, currentServiceTier **ServiceTier, visit func(TokenUsageEvent)) {
	entryType := ""
	if entry.entryType != nil {
		entryType = *entry.entryType
	}
	if entryType == "turn_context" {
		if entry.payload != nil {
			if model := modelFromParts(entry.payload.model, entry.payload.modelName, entry.payload.metadata); model != nil {
				resolver.current = model
				resolver.currentIsFallback = false
			}
		}
		return
	}
	if entryType != "event_msg" {
		return
	}
	timestamp := entry.sessionTimestamp()
	if timestamp == nil {
		return
	}
	payload := entry.payload
	if payload == nil {
		return
	}
	payloadType := ""
	if payload.payloadType != nil {
		payloadType = *payload.payloadType
	}
	if payloadType == "thread_settings_applied" {
		// A settings event that carries no service_tier at all says nothing
		// about the tier, so the previous one stands; a tier that is present
		// but unrecognized clears the stale value.
		if payload.threadSettings != nil && payload.threadSettings.serviceTier != nil {
			*currentServiceTier = codexServiceTier(*payload.threadSettings.serviceTier)
		}
		return
	}
	if payloadType != "token_count" {
		return
	}
	var info *codexInfo
	if payload.info != nil {
		info = payload.info
	}
	var totalUsage *RawUsage
	if info != nil {
		totalUsage = info.totalUsage
	}
	cumulativeAdvanced := totalUsage == nil ||
		!(*previousTotals != nil && **previousTotals == *totalUsage)
	var rawUsage *RawUsage
	if info != nil && info.lastUsage != nil && cumulativeAdvanced {
		usage := *info.lastUsage
		rawUsage = &usage
	} else if totalUsage != nil {
		usage := subtractRawUsage(*totalUsage, *previousTotals)
		rawUsage = &usage
	}
	if totalUsage != nil {
		saved := *totalUsage
		*previousTotals = &saved
	}
	if rawUsage == nil {
		return
	}
	if rawUsage.InputTokens == 0 && rawUsage.CachedInputTokens == 0 &&
		rawUsage.OutputTokens == 0 && rawUsage.ReasoningOutputTokens == 0 {
		return
	}
	var parsedModel *string
	if model := modelFromParts(payload.model, payload.modelName, payload.metadata); model != nil {
		parsedModel = model
	} else if info != nil {
		parsedModel = modelFromParts(info.model, info.modelName, info.metadata)
	}
	model, isFallback := resolveCodexUsageModel(parsedModel, *timestamp, resolver)
	cached := rawUsage.CachedInputTokens
	if cached > rawUsage.InputTokens {
		cached = rawUsage.InputTokens
	}
	var tier *ServiceTier
	if *currentServiceTier != nil {
		t := **currentServiceTier
		tier = &t
	}
	visit(TokenUsageEvent{
		SessionID:             sessionID,
		Timestamp:             *timestamp,
		Model:                 model,
		InputTokens:           rawUsage.InputTokens,
		CachedInputTokens:     cached,
		OutputTokens:          rawUsage.OutputTokens,
		ReasoningOutputTokens: rawUsage.ReasoningOutputTokens,
		TotalTokens:           rawUsage.TotalTokens,
		IsFallbackModel:       isFallback,
		ServiceTier:           tier,
	})
}

func addHeadlessEvent(sessionID string, entry headlessEntry, fallbackTimestamp string, resolver *modelResolver, visit func(TokenUsageEvent)) {
	rawUsage := entry.normalizeUsage()
	if rawUsage == nil {
		return
	}
	parsedModel := entry.modelChain()
	eventTimestamp := fallbackTimestamp
	if value := entry.eventTimestamp(); value != nil {
		eventTimestamp = *value
	}
	modelTimestamp := fallbackTimestamp
	if value := entry.modelTimestamp(); value != nil {
		modelTimestamp = *value
	}
	model, isFallback := resolveCodexUsageModel(parsedModel, modelTimestamp, resolver)
	cached := rawUsage.CachedInputTokens
	if cached > rawUsage.InputTokens {
		cached = rawUsage.InputTokens
	}
	visit(TokenUsageEvent{
		SessionID:             sessionID,
		Timestamp:             eventTimestamp,
		Model:                 model,
		InputTokens:           rawUsage.InputTokens,
		CachedInputTokens:     cached,
		OutputTokens:          rawUsage.OutputTokens,
		ReasoningOutputTokens: rawUsage.ReasoningOutputTokens,
		TotalTokens:           rawUsage.TotalTokens,
		IsFallbackModel:       isFallback,
		ServiceTier:           nil,
	})
}
