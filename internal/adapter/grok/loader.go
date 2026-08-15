package grok

import (
	"sort"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// LoadEntries parses every Grok session file, globally deduplicating by
// event id + model.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	sessions, err := DiscoverSessionFiles()
	if err != nil {
		return nil, err
	}
	updatesPaths := make([]string, len(sessions))
	for i := range sessions {
		updatesPaths[i] = sessions[i].Updates
	}
	mode := shared.Mode
	loaded := common.ReadFilesParallel(updatesPaths, shared.SingleThread, func(updates string) []core.LoadedEntry {
		session := SessionFiles{Updates: updates}
		if summary := siblingSummary(updates); summary != nil {
			session.Summary = summary
		}
		entries, err := ParseSessionFiles(&session, tz, mode, pricing)
		if err != nil {
			core.DebugLog(shared, "Failed to read Grok session file "+updates+": "+err.Error())
			return nil
		}
		return entries
	})
	var entries []core.LoadedEntry
	for _, fileEntries := range loaded {
		entries = append(entries, fileEntries...)
	}
	// Global dedupe across files: the same server event can be exported into
	// more than one session, and eventId is what identifies it.
	seen := map[string]bool{}
	kept := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		if entry.Data.Message.ID == nil {
			kept = append(kept, entries[i])
			continue
		}
		model := ""
		if entry.Model != nil {
			model = *entry.Model
		}
		key := *entry.Data.Message.ID + "|" + model
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, entries[i])
	}
	entries = kept
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

// HasData reports whether any Grok session tree exists.
func HasData() bool {
	files, err := DiscoverSessionFiles()
	return err == nil && len(files) > 0
}

// loadPricing builds the pricing table like the reference adapters.
func loadPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}
