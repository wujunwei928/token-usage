package droid

import (
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers and parses Droid settings snapshots, keeping the
// latest snapshot per session.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	return core.TrackUsageLoad("Droid", shared, func() ([]core.LoadedEntry, error) {
		return loadEntries(shared)
	})
}

func loadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	files, err := DiscoverSettingsFiles()
	if err != nil {
		return nil, err
	}
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) *droidEntry {
		entry, err := loadSettingsFile(file)
		if err != nil {
			core.DebugLog(shared, "Failed to read Droid settings file "+file+": "+err.Error())
			return nil
		}
		return entry
	})
	var parsed []*droidEntry
	for _, entry := range loaded {
		if entry != nil {
			parsed = append(parsed, entry)
		}
	}
	sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].Timestamp < parsed[j].Timestamp })
	seenSessions := map[string]bool{}
	entries := make([]core.LoadedEntry, 0, len(parsed))
	// Reverse iteration keeps the latest snapshot per session, matching the
	// reference loader; entries end up newest-first.
	for i := len(parsed) - 1; i >= 0; i-- {
		entry := parsed[i]
		if seenSessions[entry.SessionID] {
			continue
		}
		seenSessions[entry.SessionID] = true
		entries = append(entries, toLoadedEntry(entry, tz, pricing))
	}
	return entries, nil
}

func toLoadedEntry(entry *droidEntry, tz *time.Location, pricing *core.PricingMap) core.LoadedEntry {
	cost := calculateDroidCost(entry, pricing)
	missingPricingModel := missingDroidPricing(entry, pricing)
	sessionID := entry.SessionID
	model := entry.Model
	messageID := "droid:" + entry.SessionID
	usage := entry.Usage
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
		Project:             "droid",
		SessionID:           entry.SessionID,
		ProjectPath:         "Droid",
		Cost:                cost,
		ExtraTotalTokens:    entry.ReasoningToks,
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
