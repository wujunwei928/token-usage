package copilot

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// copilotUsageEntry is one parsed Copilot OTel usage record.
type copilotUsageEntry struct {
	timestamp             int64
	timestampText         string
	sessionID             string
	model                 string
	inputTokens           uint64
	outputTokens          uint64
	cacheCreationTokens   uint64
	cacheReadTokens       uint64
	reasoningOutputTokens uint64
	dedupKey              string
}

type copilotUsageSource int

const (
	sourceChatSpan copilotUsageSource = iota
	sourceInferenceLog
	sourceAgentTurnLog
	sourceAgentSummarySpan
)

type traceContext struct {
	model             *string
	sessionID         *string
	sessionIDPriority uint8
}

type copilotUsageCandidate struct {
	source                copilotUsageSource
	traceID               *string
	responseID            *string
	model                 string
	sessionID             string
	timestamp             int64
	inputTokens           uint64
	outputTokens          uint64
	cacheCreationTokens   uint64
	cacheReadTokens       uint64
	reasoningOutputTokens uint64
	dedupKey              string
}

var modelAttrs = []string{"gen_ai.response.model", "gen_ai.request.model"}

// sessionAttrs lists the session-id attribute candidates with their
// priorities; ties resolve to the LAST maximal entry (Rust max_by_key).
var sessionAttrs = []struct {
	key      string
	priority uint8
}{
	{"gen_ai.conversation.id", 3},
	{"copilot_chat.session_id", 3},
	{"copilot_chat.chat_session_id", 3},
	{"session.id", 3},
	{"github.copilot.interaction_id", 2},
	{"gen_ai.response.id", 1},
}

// parseOtelFile reads one OTel JSONL export. Lines without an "attributes"
// object are prefiltered; each surviving record is classified as one of four
// usage sources, cross-checked for duplicates across traces/response ids.
func parseOtelFile(path string) ([]copilotUsageEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []map[string]any
	for _, line := range common.SplitBytesLines(content) {
		// Every usable Copilot OTel record carries the `attributes` object.
		if !bytes.Contains(line, []byte(`"attributes"`)) {
			continue
		}
		record, ok := decodeRecord(line)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	traceContexts := collectTraceContexts(records)
	fallbackTimestamp := fileModifiedTimestamp(path)
	var candidates []copilotUsageCandidate
	for index, record := range records {
		if candidate, ok := toCandidate(record, index, fallbackTimestamp, traceContexts); ok {
			candidates = append(candidates, candidate)
		}
	}
	sets := newCandidateSets(candidates)
	entries := make([]copilotUsageEntry, 0, len(candidates))
	for i := range candidates {
		candidate := &candidates[i]
		if !shouldEmitCandidate(candidate, sets) {
			continue
		}
		entries = append(entries, copilotUsageEntry{
			timestamp:             candidate.timestamp,
			timestampText:         core.FormatRFC3339Millis(candidate.timestamp),
			sessionID:             candidate.sessionID,
			model:                 candidate.model,
			inputTokens:           candidate.inputTokens,
			outputTokens:          candidate.outputTokens,
			cacheCreationTokens:   candidate.cacheCreationTokens,
			cacheReadTokens:       candidate.cacheReadTokens,
			reasoningOutputTokens: candidate.reasoningOutputTokens,
			dedupKey:              candidate.dedupKey,
		})
	}
	return entries, nil
}

// decodeRecord parses one OTel line; a record whose attributes field exists
// but is not an object fails deserialization in the reference (Option<Map>),
// so those lines are skipped entirely, as are non-object lines.
func decodeRecord(line []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	record, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if attributes, present := record["attributes"]; present && attributes != nil {
		if _, isObject := attributes.(map[string]any); !isObject {
			return nil, false
		}
	}
	return record, true
}

func collectTraceContexts(records []map[string]any) map[string]*traceContext {
	contexts := map[string]*traceContext{}
	for _, record := range records {
		traceID, ok := traceIDFromRecord(record)
		if !ok {
			continue
		}
		attributes, ok := record["attributes"].(map[string]any)
		if !ok {
			continue
		}
		context, exists := contexts[traceID]
		if !exists {
			context = &traceContext{}
			contexts[traceID] = context
		}
		if context.model == nil {
			if model := firstNonEmptyAttr(attributes, modelAttrs); model != nil {
				context.model = model
			}
		}
		if sessionID, priority, ok := bestSessionAttr(attributes); ok && priority > context.sessionIDPriority {
			context.sessionID = &sessionID
			context.sessionIDPriority = priority
		}
	}
	return contexts
}

func toCandidate(record map[string]any, index int, fallbackTimestamp int64, traceContexts map[string]*traceContext) (copilotUsageCandidate, bool) {
	attributes, ok := record["attributes"].(map[string]any)
	if !ok {
		return copilotUsageCandidate{}, false
	}
	var source copilotUsageSource
	switch {
	case isChatSpanRecord(record, attributes):
		source = sourceChatSpan
	case isInferenceLogRecord(record, attributes):
		source = sourceInferenceLog
	case isAgentTurnLogRecord(record, attributes):
		source = sourceAgentTurnLog
	case isAgentSummarySpanRecord(record, attributes):
		source = sourceAgentSummarySpan
	default:
		return copilotUsageCandidate{}, false
	}
	input := attrNumber(attributes, "gen_ai.usage.input_tokens")
	output := attrNumber(attributes, "gen_ai.usage.output_tokens")
	cacheRead := attrNumber(attributes, "gen_ai.usage.cache_read.input_tokens")
	cacheCreation := attrNumberFirst(attributes, "gen_ai.usage.cache_write.input_tokens", "gen_ai.usage.cache_creation.input_tokens")
	reasoning := attrNumberFirst(attributes, "gen_ai.usage.reasoning.output_tokens", "gen_ai.usage.reasoning_tokens")
	total := attrNumberFirst(attributes, "gen_ai.usage.total_tokens", "gen_ai.usage.total.token_count")
	usage := core.TokenUsageRaw{
		InputTokens:              input - minU64(input, cacheRead),
		OutputTokens:             output,
		CacheCreationInputTokens: cacheCreation,
		CacheReadInputTokens:     cacheRead,
	}
	usage, reasoning = applyTotalTokenFallback(usage, reasoning, total)
	if core.TotalUsageTokens(usage)+reasoning == 0 {
		return copilotUsageCandidate{}, false
	}
	traceID, hasTraceID := traceIDFromRecord(record)
	var context *traceContext
	if hasTraceID {
		context = traceContexts[traceID]
	}
	responseID := attrStringPtr(attributes, "gen_ai.response.id")
	model := "unknown"
	if found := firstNonEmptyAttr(attributes, modelAttrs); found != nil {
		model = *found
	} else if context != nil && context.model != nil {
		model = *context.model
	}
	sessionID := "unknown-session"
	if found, _, ok := bestSessionAttr(attributes); ok {
		sessionID = found
	} else if context != nil && context.sessionID != nil {
		sessionID = *context.sessionID
	} else if hasTraceID {
		sessionID = traceID
	}
	timestamp, hasTimestamp := timestampFromRecord(record)
	if !hasTimestamp {
		timestamp = fallbackTimestamp
	}
	dedupKey := dedupKeyForRecord(source, record, attributes, traceID, sessionID, timestamp, index)
	return copilotUsageCandidate{
		source:                source,
		traceID:               stringPtrOrNil(traceID, hasTraceID),
		responseID:            responseID,
		model:                 model,
		sessionID:             sessionID,
		timestamp:             timestamp,
		inputTokens:           usage.InputTokens,
		outputTokens:          usage.OutputTokens,
		cacheCreationTokens:   usage.CacheCreationInputTokens,
		cacheReadTokens:       usage.CacheReadInputTokens,
		reasoningOutputTokens: reasoning,
		dedupKey:              dedupKey,
	}, true
}

type candidateSets struct {
	chatTraces           map[string]struct{}
	inferenceTraces      map[string]struct{}
	agentTurnTraces      map[string]struct{}
	chatResponseIDs      map[string]struct{}
	inferenceResponseIDs map[string]struct{}
	agentTurnResponseIDs map[string]struct{}
}

func newCandidateSets(candidates []copilotUsageCandidate) *candidateSets {
	sets := &candidateSets{
		chatTraces:           map[string]struct{}{},
		inferenceTraces:      map[string]struct{}{},
		agentTurnTraces:      map[string]struct{}{},
		chatResponseIDs:      map[string]struct{}{},
		inferenceResponseIDs: map[string]struct{}{},
		agentTurnResponseIDs: map[string]struct{}{},
	}
	for i := range candidates {
		candidate := &candidates[i]
		var traces, responses map[string]struct{}
		switch candidate.source {
		case sourceChatSpan:
			traces, responses = sets.chatTraces, sets.chatResponseIDs
		case sourceInferenceLog:
			traces, responses = sets.inferenceTraces, sets.inferenceResponseIDs
		case sourceAgentTurnLog:
			traces, responses = sets.agentTurnTraces, sets.agentTurnResponseIDs
		default:
			continue
		}
		if candidate.traceID != nil {
			traces[*candidate.traceID] = struct{}{}
		}
		if candidate.responseID != nil {
			responses[*candidate.responseID] = struct{}{}
		}
	}
	return sets
}

func shouldEmitCandidate(candidate *copilotUsageCandidate, sets *candidateSets) bool {
	traceMatch := func(values map[string]struct{}) bool {
		return candidate.traceID != nil && containsKey(values, *candidate.traceID)
	}
	responseMatch := func(values map[string]struct{}) bool {
		return candidate.responseID != nil && containsKey(values, *candidate.responseID)
	}
	switch candidate.source {
	case sourceChatSpan:
		return true
	case sourceInferenceLog:
		return !traceMatch(sets.chatTraces) && !responseMatch(sets.chatResponseIDs)
	case sourceAgentTurnLog:
		return !traceMatch(sets.chatTraces) && !traceMatch(sets.inferenceTraces) &&
			!responseMatch(sets.chatResponseIDs) && !responseMatch(sets.inferenceResponseIDs)
	default: // sourceAgentSummarySpan
		return !traceMatch(sets.chatTraces) && !traceMatch(sets.inferenceTraces) &&
			!traceMatch(sets.agentTurnTraces) &&
			!responseMatch(sets.chatResponseIDs) && !responseMatch(sets.inferenceResponseIDs) &&
			!responseMatch(sets.agentTurnResponseIDs)
	}
}

func containsKey(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func isSpanRecord(record map[string]any) bool {
	if recordType, ok := stringValueOf(record["type"]); ok && recordType == "span" {
		return true
	}
	if _, ok := stringValueOf(record["name"]); !ok {
		return false
	}
	_, spanID := stringValueOf(record["spanId"])
	_, traceID := stringValueOf(record["traceId"])
	return spanID || traceID ||
		record["startTime"] != nil || record["endTime"] != nil ||
		record["duration"] != nil || record["kind"] != nil
}

func isChatSpanRecord(record map[string]any, attributes map[string]any) bool {
	if !isSpanRecord(record) {
		return false
	}
	if operation, ok := attrString(attributes, "gen_ai.operation.name"); ok && operation == "chat" {
		return true
	}
	name, ok := stringValueOf(record["name"])
	return ok && strings.HasPrefix(name, "chat ")
}

func isAgentSummarySpanRecord(record map[string]any, attributes map[string]any) bool {
	if !isSpanRecord(record) {
		return false
	}
	if operation, ok := attrString(attributes, "gen_ai.operation.name"); ok && operation == "invoke_agent" {
		return true
	}
	name, ok := stringValueOf(record["name"])
	return ok && strings.HasPrefix(name, "invoke_agent ")
}

func isInferenceLogRecord(record map[string]any, attributes map[string]any) bool {
	if isSpanRecord(record) {
		return false
	}
	if event, ok := attrString(attributes, "event.name"); ok && event == "gen_ai.client.inference.operation.details" {
		return true
	}
	body, ok := recordBody(record)
	return ok && strings.HasPrefix(body, "GenAI inference:")
}

func isAgentTurnLogRecord(record map[string]any, attributes map[string]any) bool {
	if isSpanRecord(record) {
		return false
	}
	if event, ok := attrString(attributes, "event.name"); ok && event == "copilot_chat.agent.turn" {
		return true
	}
	body, ok := recordBody(record)
	return ok && strings.HasPrefix(body, "copilot_chat.agent.turn")
}

func dedupKeyForRecord(source copilotUsageSource, record map[string]any, attributes map[string]any, traceID string, sessionID string, timestamp int64, index int) string {
	spanID, hasSpanID := spanIDFromRecord(record)
	millis := strconv.FormatInt(timestamp, 10)
	indexText := strconv.Itoa(index)
	switch source {
	case sourceChatSpan, sourceAgentSummarySpan:
		if traceID != "" && hasSpanID {
			return traceID + ":" + spanID
		}
		return "span:" + sessionID + ":" + millis + ":" + indexText
	case sourceInferenceLog:
		if traceID != "" && hasSpanID {
			return "log:" + traceID + ":" + spanID
		}
		return "log:" + sessionID + ":" + millis + ":" + indexText
	default: // sourceAgentTurnLog
		turnIndex := "idx-" + indexText
		if value, ok := numberValue(attributes["turn.index"]); ok {
			turnIndex = strconv.FormatUint(value, 10)
		} else if value, ok := numberValue(attributes["copilot_chat.turn.index"]); ok {
			turnIndex = strconv.FormatUint(value, 10)
		}
		if traceID != "" {
			return "agent-turn:" + traceID + ":" + turnIndex
		}
		return "agent-turn:" + sessionID + ":" + turnIndex + ":" + indexText
	}
}

func traceIDFromRecord(record map[string]any) (string, bool) {
	if traceID, ok := stringValueOf(record["traceId"]); ok {
		return traceID, true
	}
	return nestedString(record["spanContext"], "traceId")
}

func spanIDFromRecord(record map[string]any) (string, bool) {
	if spanID, ok := stringValueOf(record["spanId"]); ok {
		return spanID, true
	}
	return nestedString(record["spanContext"], "spanId")
}

func nestedString(value any, key string) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	return stringValueOf(object[key])
}

func recordBody(record map[string]any) (string, bool) {
	if body, ok := stringValueOf(record["body"]); ok {
		return body, true
	}
	return stringValueOf(record["_body"])
}

// stringValueOf mirrors string_value: trimmed non-empty strings only.
func stringValueOf(value any) (string, bool) {
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

// numberValue mirrors number_value: JSON integers (u64, or i64 >= 0) and
// numeric strings parse; floats, negatives, and anything else do not.
func numberValue(value any) (uint64, bool) {
	switch v := value.(type) {
	case json.Number:
		if u, err := strconv.ParseUint(v.String(), 10, 64); err == nil {
			return u, true
		}
		if i, err := strconv.ParseInt(v.String(), 10, 64); err == nil && i >= 0 {
			return uint64(i), true
		}
		return 0, false
	case string:
		u, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return u, true
	default:
		return 0, false
	}
}

func attrString(attributes map[string]any, key string) (string, bool) {
	return stringValueOf(attributes[key])
}

// attrStringPtr is attr_string's Option<String> shape.
func attrStringPtr(attributes map[string]any, key string) *string {
	if value, ok := attrString(attributes, key); ok {
		return &value
	}
	return nil
}

func attrNumber(attributes map[string]any, key string) uint64 {
	if value, ok := numberValue(attributes[key]); ok {
		return value
	}
	return 0
}

func attrNumberFirst(attributes map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		if value := attrNumber(attributes, key); value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyAttr(attributes map[string]any, keys []string) *string {
	for _, key := range keys {
		if value, ok := attrString(attributes, key); ok {
			return &value
		}
	}
	return nil
}

// bestSessionAttr picks the highest-priority session attribute; ties resolve
// to the last maximal key in SESSION_ATTRS order (Rust max_by_key semantics).
func bestSessionAttr(attributes map[string]any) (string, uint8, bool) {
	found := false
	var best string
	var bestPriority uint8
	for _, attr := range sessionAttrs {
		if value, ok := attrString(attributes, attr.key); ok && attr.priority >= bestPriority {
			best = value
			bestPriority = attr.priority
			found = true
		}
	}
	return best, bestPriority, found
}

func timestampFromRecord(record map[string]any) (int64, bool) {
	for _, key := range []string{"endTime", "startTime", "hrTime", "_hrTime", "time"} {
		if ts, ok := timestampFromParts(record[key]); ok {
			return ts, true
		}
	}
	if ts, ok := timestampFromScalar(record["timestamp"]); ok {
		return ts, true
	}
	if ts, ok := timestampFromScalar(record["observedTimestamp"]); ok {
		return ts, true
	}
	if ts, ok := timestampFromUnixNanos(record["timeUnixNano"]); ok {
		return ts, true
	}
	return 0, false
}

func timestampFromParts(value any) (int64, bool) {
	items, ok := value.([]any)
	if !ok || len(items) < 2 {
		return 0, false
	}
	seconds, ok := numberValue(items[0])
	if !ok {
		return 0, false
	}
	nanos, ok := numberValue(items[1])
	if !ok {
		return 0, false
	}
	secondsMillis, ok := mulU64(seconds, 1000)
	if !ok {
		return 0, false
	}
	millis, ok := addU64(secondsMillis, nanos/1_000_000)
	if !ok {
		return 0, false
	}
	return clampI64(millis), true
}

func timestampFromScalar(value any) (int64, bool) {
	raw, ok := numberValue(value)
	if !ok {
		return 0, false
	}
	var millis uint64
	switch {
	case raw >= 100_000_000_000_000_000:
		millis = raw / 1_000_000
	case raw >= 100_000_000_000_000:
		millis = raw / 1_000
	case raw >= 100_000_000_000:
		millis = raw
	default:
		product, ok := mulU64(raw, 1_000)
		if !ok {
			product = ^uint64(0)
		}
		millis = product
	}
	return clampI64(millis), true
}

func timestampFromUnixNanos(value any) (int64, bool) {
	raw, ok := numberValue(value)
	if !ok || raw == 0 {
		return 0, false
	}
	return clampI64(raw / 1_000_000), true
}

const i64Max = uint64(math.MaxInt64)

func clampI64(v uint64) int64 {
	if v > i64Max {
		return math.MaxInt64
	}
	return int64(v)
}

func mulU64(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
}

func addU64(a, b uint64) (uint64, bool) {
	sum := a + b
	if sum < a {
		return 0, false
	}
	return sum, true
}

func fileModifiedTimestamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return core.UTCNow()
	}
	modified := info.ModTime()
	millis := modified.UnixMilli()
	if millis < 0 {
		return core.UTCNow()
	}
	return millis
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

func stringPtrOrNil(value string, ok bool) *string {
	if !ok {
		return nil
	}
	return &value
}
