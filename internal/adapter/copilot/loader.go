package copilot

import (
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers and parses every Copilot OTel export, returning
// entries sorted by timestamp (stable; same-timestamp entries keep file
// order, and files are processed in sorted path order).
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	files := Paths()
	// Entries keep their original file order before the stable sort, so the
	// output is identical to a sequential read.
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(path string) []core.LoadedEntry {
		entries, err := readOtelFile(path, tz, shared.Mode, pricing)
		if err != nil {
			core.DebugLog(shared, "Failed to read Copilot OTEL file "+path+": "+err.Error())
			return nil
		}
		return entries
	})
	var entries []core.LoadedEntry
	for _, fileEntries := range loaded {
		entries = append(entries, fileEntries...)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

func readOtelFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	parsed, err := parseOtelFile(path)
	if err != nil {
		return nil, err
	}
	entries := make([]core.LoadedEntry, 0, len(parsed))
	for i := range parsed {
		entries = append(entries, usageEntryToLoaded(&parsed[i], tz, mode, pricing))
	}
	return entries, nil
}

// usageEntryToLoaded converts one parsed OTel record into a LoadedEntry. The
// reasoning tokens ride along as extra total tokens and are folded into the
// cost pass's output tokens only.
func usageEntryToLoaded(entry *copilotUsageEntry, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	usage := core.TokenUsageRaw{
		InputTokens:              entry.inputTokens,
		OutputTokens:             entry.outputTokens,
		CacheCreationInputTokens: entry.cacheCreationTokens,
		CacheReadInputTokens:     entry.cacheReadTokens,
	}
	costUsage := usage
	costUsage.OutputTokens = saturatingAddU64(costUsage.OutputTokens, entry.reasoningOutputTokens)
	sessionID := entry.sessionID
	model := entry.model
	dedupKey := entry.dedupKey
	data := core.UsageEntry{
		SessionID: &sessionID,
		Timestamp: entry.timestampText,
		Message: core.UsageMessage{
			Usage: usage,
			Model: &model,
			ID:    &dedupKey,
		},
	}
	cost := core.CalculateCostForUsage(&model, costUsage, nil, mode, pricing)
	missingPricingModel := core.MissingPricingModelForUsage(&model, costUsage, nil, mode, pricing)
	return core.LoadedEntry{
		Data:                data,
		Timestamp:           entry.timestamp,
		Date:                core.FormatDateTZ(entry.timestamp, tz),
		Project:             "copilot",
		SessionID:           entry.sessionID,
		ProjectPath:         "GitHub Copilot CLI",
		Cost:                cost,
		ExtraTotalTokens:    entry.reasoningOutputTokens,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

func saturatingAddU64(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return ^uint64(0)
	}
	return sum
}
