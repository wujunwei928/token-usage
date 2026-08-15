package grok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// costUSDTicksPerUSD: Grok's costUsdTicks are fixed-point USD (1 tick = 1e-10 USD).
const costUSDTicksPerUSD = 1e10

// grokUsage is one turn_completed usage payload.
type grokUsage struct {
	inputTokens       uint64
	outputTokens      uint64
	cachedReadTokens  uint64
	cacheCreationToks uint64
	reasoningTokens   uint64
	totalTokens       uint64
	costUSDTicks      uint64
	modelUsage        map[string]grokModelUsage
}

type grokModelUsage struct {
	inputTokens       uint64
	outputTokens      uint64
	cachedReadTokens  uint64
	cacheCreationToks uint64
	reasoningTokens   uint64
	totalTokens       uint64
	costUSDTicks      uint64
}

// sessionMeta carries the identity resolved from summary.json and the path.
type sessionMeta struct {
	sessionID    string
	projectPath  string
	defaultModel *string
}

// ParseSessionFiles parses one session's updates.jsonl into loaded entries.
func ParseSessionFiles(files *SessionFiles, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	meta := loadSessionMeta(files)
	content, err := os.ReadFile(files.Updates)
	if err != nil {
		return nil, err
	}
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, []byte(`"turn_completed"`)) {
			continue
		}
		record, ok := parseJSONLine(line)
		if !ok {
			continue
		}
		params, ok := record["params"].(map[string]any)
		if !ok {
			continue
		}
		update, ok := params["update"].(map[string]any)
		if !ok {
			continue
		}
		// A present-but-malformed _meta fails the whole line's decode.
		var metaMap map[string]any
		if rawMeta, present := params["_meta"]; present {
			metaMap, ok = rawMeta.(map[string]any)
			if !ok {
				continue
			}
		}
		if nonEmptyString(update["sessionUpdate"]) != "turn_completed" {
			continue
		}
		rawUsage, hasUsage := update["usage"]
		if !hasUsage {
			continue
		}
		usageRecord, ok := rawUsage.(map[string]any)
		if !ok {
			continue
		}
		usage, ok := parseGrokUsage(usageRecord)
		if !ok {
			continue
		}

		var eventID *string
		if metaMap != nil {
			if id := nonEmptyString(metaMap["eventId"]); id != "" {
				eventID = &id
			}
		}
		timestampMS := resolveTimestampMS(record, metaMap)
		sessionID := nonEmptyString(params["sessionId"])
		if sessionID == "" {
			sessionID = meta.sessionID
		}

		for _, row := range modelUsageRows(&usage, meta.defaultModel) {
			rawModel := row.model
			modelUsage := row.usage
			uncached, cacheRead, cacheCreation := splitInputTokens(
				modelUsage.inputTokens, modelUsage.cachedReadTokens, modelUsage.cacheCreationToks)
			outputTokens := modelUsage.outputTokens
			reasoningTokens := modelUsage.reasoningTokens
			if uncached == 0 && cacheRead == 0 && cacheCreation == 0 && outputTokens == 0 && reasoningTokens == 0 {
				continue
			}
			usageTokens := core.TokenUsageRaw{
				InputTokens:              uncached,
				OutputTokens:             outputTokens,
				CacheCreationInputTokens: cacheCreation,
				CacheReadInputTokens:     cacheRead,
			}
			key := dedupeKey(eventID, sessionID, timestampMS, rawModel, usageTokens, reasoningTokens)
			if seen[key] {
				continue
			}
			seen[key] = true
			// The raw modelUsage key is displayed as-is (e.g. grok-4.5-build).
			displayModel := rawModel
			costUSD := costUSDFromTicks(modelUsage.costUSDTicks)
			cost := calculateGrokCost(rawModel, usageTokens, costUSD, mode, pricing)
			missingPricingModel := missingGrokPricing(rawModel, usageTokens, costUSD, mode, pricing)
			timestampText := core.FormatRFC3339Millis(timestampMS)
			usageCopy := usageTokens
			session := sessionID
			entries = append(entries, core.LoadedEntry{
				Data: core.UsageEntry{
					SessionID: &session,
					Timestamp: timestampText,
					Message: core.UsageMessage{
						Usage: usageCopy,
						Model: &displayModel,
						ID:    eventID,
					},
					CostUSD:   costUSD,
					RequestID: eventID,
				},
				Timestamp:           timestampMS,
				Date:                core.FormatDateTZ(timestampMS, tz),
				Project:             "grok",
				SessionID:           sessionID,
				ProjectPath:         meta.projectPath,
				Cost:                cost,
				Model:               &displayModel,
				MissingPricingModel: missingPricingModel,
				// Grok reports totalTokens == inputTokens + outputTokens, so its
				// reasoning tokens are already a subset of the output count.
			})
		}
	}
	return entries, nil
}

type modelRow struct {
	model string
	usage grokModelUsage
}

func modelUsageRows(usage *grokUsage, defaultModel *string) []modelRow {
	if len(usage.modelUsage) > 0 {
		rows := make([]modelRow, 0, len(usage.modelUsage))
		for model, modelUsage := range usage.modelUsage {
			rows = append(rows, modelRow{model, modelUsage})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].model < rows[j].model })
		return rows
	}
	model := "unknown"
	if defaultModel != nil {
		model = *defaultModel
	}
	return []modelRow{{model, grokModelUsage{
		inputTokens:       usage.inputTokens,
		outputTokens:      usage.outputTokens,
		cachedReadTokens:  usage.cachedReadTokens,
		cacheCreationToks: usage.cacheCreationToks,
		reasoningTokens:   usage.reasoningTokens,
		totalTokens:       usage.totalTokens,
		costUSDTicks:      usage.costUSDTicks,
	}}}
}

func loadSessionMeta(files *SessionFiles) sessionMeta {
	sessionDirName := "unknown"
	if base := filepath.Base(filepath.Dir(files.Updates)); base != "" && base != "." && base != string(filepath.Separator) {
		sessionDirName = base
	}
	projectFromPath := "unknown"
	if projectDir := filepath.Dir(filepath.Dir(files.Updates)); projectDir != "" && projectDir != "." && projectDir != string(filepath.Separator) {
		if base := filepath.Base(projectDir); base != "" && base != "." && base != string(filepath.Separator) {
			projectFromPath = urlDecodeLightweight(base)
		}
	}
	sessionID := sessionDirName
	projectPath := projectFromPath
	var defaultModel *string
	if files.Summary != nil {
		if content, err := os.ReadFile(*files.Summary); err == nil {
			var summary any
			decoder := json.NewDecoder(bytes.NewReader(content))
			decoder.UseNumber()
			if err := decoder.Decode(&summary); err == nil {
				if record, ok := summary.(map[string]any); ok {
					if info, ok := record["info"].(map[string]any); ok {
						if id := nonEmptyString(info["id"]); id != "" {
							sessionID = id
						}
					}
					var info map[string]any
					if i, ok := record["info"].(map[string]any); ok {
						info = i
					}
					if cwd := nonEmptyString(info["cwd"]); cwd != "" {
						projectPath = cwd
					} else if gitRoot := nonEmptyString(record["git_root_dir"]); gitRoot != "" {
						projectPath = gitRoot
					}
					if model := nonEmptyString(record["current_model_id"]); model != "" {
						defaultModel = &model
					}
				}
			}
		}
	}
	return sessionMeta{sessionID: sessionID, projectPath: projectPath, defaultModel: defaultModel}
}

func resolveTimestampMS(record map[string]any, metaMap map[string]any) int64 {
	if metaMap != nil {
		if ms, ok := valueAsI64(metaMap["agentTimestampMs"]); ok && ms > 0 {
			return ms
		}
	}
	if seconds, ok := valueAsI64(record["timestamp"]); ok && seconds > 0 {
		// Grok writes Unix seconds on the envelope timestamp field.
		if seconds > math.MaxInt64/1000 {
			return math.MaxInt64
		}
		return seconds * 1000
	}
	return 0
}

func valueAsI64(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed, true
		}
		if parsed, err := typed.Float64(); err == nil {
			return int64(parsed), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func dedupeKey(eventID *string, sessionID string, timestamp int64, model string, usage core.TokenUsageRaw, reasoning uint64) string {
	if eventID != nil {
		return *eventID + "|" + model
	}
	return fmt.Sprintf("%s|%d|%s|%d|%d|%d|%d|%d",
		sessionID, timestamp, model,
		usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens,
		usage.CacheCreationInputTokens, reasoning)
}

// costUSDFromTicks converts Grok's fixed-point ticks into USD, if any.
func costUSDFromTicks(ticks uint64) *float64 {
	if ticks == 0 {
		return nil
	}
	cost := float64(ticks) / costUSDTicksPerUSD
	return &cost
}

// splitTokens splits OpenAI-style input that includes cache.
func splitTokens(input, cached uint64) (uncached, cache uint64) {
	cache = min64(cached, input)
	uncached = input - cache
	return uncached, cache
}

// splitInputTokens splits inputTokens into uncached, cache-read and
// cache-write parts.
func splitInputTokens(input, cachedRead, cacheCreation uint64) (uncached, cacheRead, creation uint64) {
	uncached, cacheRead = splitTokens(input, cachedRead)
	creation = min64(cacheCreation, uncached)
	uncached -= creation
	return uncached, cacheRead, creation
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// pricingCandidates returns lookup candidates for a raw Grok model id.
func pricingCandidates(rawModel string) []string {
	var candidates []string
	push := func(value string) {
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}
	stripped := strings.TrimSpace(strings.TrimPrefix(rawModel, "[grok] "))
	if stripped == "" {
		return candidates
	}
	normalized := strings.TrimSuffix(stripped, "-build")
	push(stripped)
	push("xai/" + stripped)
	push("x-ai/" + stripped)
	push(normalized)
	push("xai/" + normalized)
	push("x-ai/" + normalized)
	return candidates
}

func calculateGrokCost(rawModel string, usage core.TokenUsageRaw, costUSD *float64, mode core.CostMode, pricing *core.PricingMap) float64 {
	switch {
	case mode == core.ModeDisplay:
		if costUSD != nil {
			return *costUSD
		}
		return 0
	case mode == core.ModeAuto && costUSD != nil:
		return *costUSD
	default:
		for _, candidate := range pricingCandidates(rawModel) {
			if pricing.Find(candidate) != nil {
				return core.CalculateCostForUsage(&candidate, usage, nil, core.ModeCalculate, pricing)
			}
		}
		return 0
	}
}

func missingGrokPricing(rawModel string, usage core.TokenUsageRaw, costUSD *float64, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay || (mode == core.ModeAuto && costUSD != nil) {
		return nil
	}
	return missingPricingModelForCandidates(rawModel, pricingCandidates(rawModel), core.TotalUsageTokens(usage), pricing)
}

// missingPricingModelForCandidates ports ccusage-core
// missing_pricing_model_for_candidates.
func missingPricingModelForCandidates(model string, candidates []string, totalTokens uint64, pricing *core.PricingMap) *string {
	if totalTokens == 0 || pricing == nil {
		return nil
	}
	for _, candidate := range candidates {
		if pricing.Find(candidate) != nil {
			return nil
		}
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

func parseGrokUsage(record map[string]any) (grokUsage, bool) {
	usage := grokUsage{
		inputTokens:       lenientU64(record["inputTokens"]),
		outputTokens:      lenientU64(record["outputTokens"]),
		cachedReadTokens:  lenientU64(record["cachedReadTokens"]),
		cacheCreationToks: lenientU64(record["cacheCreationTokens"]),
		reasoningTokens:   lenientU64(record["reasoningTokens"]),
		totalTokens:       lenientU64(record["totalTokens"]),
		costUSDTicks:      lenientU64(record["costUsdTicks"]),
	}
	if rawModelUsage, present := record["modelUsage"]; present {
		modelUsage, ok := rawModelUsage.(map[string]any)
		if !ok {
			return grokUsage{}, false
		}
		usage.modelUsage = make(map[string]grokModelUsage, len(modelUsage))
		for model, value := range modelUsage {
			fields, ok := value.(map[string]any)
			if !ok {
				return grokUsage{}, false
			}
			usage.modelUsage[model] = grokModelUsage{
				inputTokens:       lenientU64(fields["inputTokens"]),
				outputTokens:      lenientU64(fields["outputTokens"]),
				cachedReadTokens:  lenientU64(fields["cachedReadTokens"]),
				cacheCreationToks: lenientU64(fields["cacheCreationTokens"]),
				reasoningTokens:   lenientU64(fields["reasoningTokens"]),
				totalTokens:       lenientU64(fields["totalTokens"]),
				costUSDTicks:      lenientU64(fields["costUsdTicks"]),
			}
		}
	}
	return usage, true
}

// lenientU64 mirrors the jsonl::lenient_u64 deserializer: anything that is
// not a JSON unsigned integer becomes 0.
func lenientU64(value any) uint64 {
	number, ok := value.(json.Number)
	if !ok {
		return 0
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 {
		return 0
	}
	return uint64(parsed)
}

func nonEmptyString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(text)
	return trimmed
}

func parseJSONLine(line []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	record, ok := value.(map[string]any)
	return record, ok
}

// urlDecodeLightweight decodes percent triplets into bytes, degrading
// invalid UTF-8 to the replacement character.
func urlDecodeLightweight(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			if hi, ok := fromHex(value[i+1]); ok {
				if lo, ok := fromHex(value[i+2]); ok {
					out = append(out, hi*16+lo)
					i += 2
					continue
				}
			}
		}
		out = append(out, value[i])
	}
	return strings.ToValidUTF8(string(out), "\uFFFD")
}

func fromHex(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}
