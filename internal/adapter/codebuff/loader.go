package codebuff

import (
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers and parses Codebuff chat transcripts, deduplicating
// repeated messages by their dedup key (last file wins).
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	return core.TrackUsageLoad("Codebuff", shared, func() ([]core.LoadedEntry, error) {
		return loadEntries(shared)
	})
}

func loadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	files, err := DiscoverChatFiles()
	if err != nil {
		return nil, err
	}
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []codebuffEntry {
		entries, err := LoadChatFile(file)
		if err != nil {
			core.DebugLog(shared, "Failed to read Codebuff chat file "+file+": "+err.Error())
			return nil
		}
		return entries
	})
	deduped := map[string]codebuffEntry{}
	for _, fileEntries := range loaded {
		for _, entry := range fileEntries {
			deduped[entry.DedupKey] = entry
		}
	}
	entries := make([]core.LoadedEntry, 0, len(deduped))
	for _, entry := range deduped {
		entries = append(entries, toLoadedEntry(entry, tz, pricing))
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

func toLoadedEntry(entry codebuffEntry, tz *time.Location, pricing *core.PricingMap) core.LoadedEntry {
	cost := calculateCodebuffCost(&entry, pricing)
	missingPricingModel := missingCodebuffPricing(&entry, pricing)
	sessionID := entry.SessionID
	model := entry.Model
	messageID := entry.DedupKey
	usage := entry.Usage
	var credits *float64
	if entry.Credits > 0 {
		credits = &entry.Credits
	}
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: entry.TimestampText,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &model,
				ID:    &messageID,
			},
		},
		Timestamp:           entry.Timestamp,
		Date:                core.FormatDateTZ(entry.Timestamp, tz),
		Project:             "codebuff",
		SessionID:           entry.SessionID,
		ProjectPath:         "Codebuff",
		Cost:                cost,
		ExtraTotalTokens:    entry.ExtraTotalTokens,
		Credits:             credits,
		Model:               &model,
		MissingPricingModel: missingPricingModel,
	}
}

// loadPricing builds the pricing table like the reference adapters.
func loadPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}
