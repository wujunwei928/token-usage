package gemini

import (
	"sort"
	"strings"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers Gemini log files, parses them (in parallel unless
// disabled), sorts events by timestamp, and prices each entry.
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	files := DiscoverLogFiles()
	// Read each log file in parallel; events keep their original file order
	// before the stable sort, so output matches the sequential read.
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []geminiUsageEvent {
		var (
			events []geminiUsageEvent
			err    error
		)
		if filepathJSONL(file) {
			events, err = parseJSONLFile(file)
		} else {
			events, err = parseJSONFile(file)
		}
		if err != nil {
			core.DebugLog(shared, "Failed to read Gemini log file "+file+": "+err.Error())
			return nil
		}
		return events
	})
	var events []geminiUsageEvent
	for _, fileEvents := range loaded {
		events = append(events, fileEvents...)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].timestamp < events[j].timestamp })
	entries := make([]core.LoadedEntry, 0, len(events))
	for _, event := range events {
		entries = append(entries, eventToLoaded(event, tz, shared.Mode, pricing))
	}
	return entries, nil
}

// filepathJSONL mirrors the reference extension check: only files whose
// extension is exactly "jsonl" parse as JSONL; everything else is JSON.
func filepathJSONL(file string) bool {
	return strings.HasSuffix(file, ".jsonl")
}
