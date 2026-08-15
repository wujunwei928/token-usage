package openclaw

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind selects the report granularity for the OpenClaw adapter.
type ReportKind int

// Report kinds (mirrors AgentReportKind).
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// SummarizeEntries groups loaded entries into report rows. Sessions use the
// session accumulator (activity bounds), weekly and monthly roll up the daily
// rows.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindDaily:
		return core.SummarizeByKey(entries,
			func(entry *core.LoadedEntry) string { return entry.Date },
			func(date string) (string, *string) { return date, nil })
	case KindMonthly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		groups := map[string]*core.SessionAccumulator{}
		var keys []string
		for i := range entries {
			key := entries[i].SessionID
			if _, ok := groups[key]; !ok {
				groups[key] = &core.SessionAccumulator{}
				keys = append(keys, key)
			}
			groups[key].AddEntry(&entries[i])
		}
		sort.Strings(keys)
		rows := make([]core.UsageSummary, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, groups[key].IntoSummary())
		}
		return rows
	default: // KindWeekly
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
}
