package pi

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadOptions carries what the pi loader needs from the CLI layer.
type LoadOptions struct {
	Shared *core.SharedArgs
	// CustomPath is the --pi-path value (comma-separated session roots).
	CustomPath *string
	// Pricing supplies token pricing; nil skips pricing entirely (display
	// mode), matching the reference's unused-pricing behavior.
	Pricing *core.PricingMap
}

// LoadEntries discovers, parses, dedupes, and time-orders pi session entries.
func LoadEntries(opts LoadOptions) ([]core.LoadedEntry, error) {
	shared := opts.Shared
	paths, err := Paths(opts.CustomPath)
	if err != nil {
		return nil, err
	}
	tz := core.ParseTZ(shared.Timezone)
	var entries []core.LoadedEntry
	seen := map[string]struct{}{}
	for _, path := range paths {
		var files []string
		common.CollectUsageFiles(path, &files)
		// Read session files in parallel; the first-wins dedup runs
		// sequentially over the original file order so the surviving record
		// per id matches the single-threaded read.
		loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []core.LoadedEntry {
			fileEntries, err := ReadSessionFile(file, tz, shared.Mode, opts.Pricing)
			if err != nil {
				core.DebugLog(shared, "Failed to read pi session file "+file+": "+err.Error())
				return nil
			}
			return fileEntries
		})
		for _, fileEntries := range loaded {
			for i := range fileEntries {
				id := entryID(&fileEntries[i])
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					entries = append(entries, fileEntries[i])
				}
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

// FilterEntriesByDate keeps entries whose local date falls in the inclusive
// since/until window.
func FilterEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	out := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			out = append(out, entries[i])
		}
	}
	return out
}
