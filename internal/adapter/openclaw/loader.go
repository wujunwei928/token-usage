package openclaw

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers OpenClaw session files across all roots, parses them
// (in parallel unless disabled), applies the first-wins dedup across roots in
// path order, and sorts the surviving entries by timestamp.
func LoadEntries(shared *core.SharedArgs, customPath *string, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, root := range DataPaths(customPath) {
		files, err := collectSessionFiles(root)
		if err != nil {
			return nil, err
		}
		// Read session files in parallel; the first-wins dedup runs
		// sequentially over the original file order so the surviving record
		// per id is the same as the single-threaded read.
		loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []core.LoadedEntry {
			entries, err := parseSessionFile(file, tz, shared.Mode, pricing)
			if err != nil {
				core.DebugLog(shared, "Failed to read OpenClaw session file "+file+": "+err.Error())
				return nil
			}
			return entries
		})
		for _, fileEntries := range loaded {
			for i := range fileEntries {
				key := entryID(&fileEntries[i])
				if seen[key] {
					continue
				}
				seen[key] = true
				entries = append(entries, fileEntries[i])
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}
