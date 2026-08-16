package kimi

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers Kimi wire files, parses them (in parallel unless
// disabled), applies the first-wins dedup in discovery order, and sorts the
// surviving entries by timestamp.
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	return core.TrackUsageLoad("Kimi", shared, func() ([]core.LoadedEntry, error) {
		return loadEntries(shared, pricing)
	})
}

func loadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	files := DiscoverWireFiles()
	// Read wire files in parallel, then apply the first-wins dedup
	// sequentially over the original discovery order so the surviving entry
	// per key matches the single-threaded read.
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []kimiUsageEntry {
		entries, err := readWireFile(file)
		if err != nil {
			core.DebugLog(shared, "Failed to read Kimi wire file "+file+": "+err.Error())
			return nil
		}
		return entries
	})
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, fileEntries := range loaded {
		for i := range fileEntries {
			key := kimiEntryKey(&fileEntries[i])
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, kimiEntryToLoaded(fileEntries[i], tz, shared.Mode, pricing))
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}
