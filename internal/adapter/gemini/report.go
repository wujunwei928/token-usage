package gemini

import (
	"github.com/wujunwei/ccusage-go/internal/core"
)

// ReportKind selects the report granularity for the Gemini adapter.
type ReportKind int

// Report kinds (mirrors AgentReportKind).
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// SummarizeEntries groups loaded entries into report rows: sessions group by
// session id (plain token totals, no activity bounds), weekly and monthly
// roll up the daily rows.
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
		rows := core.SummarizeByKey(entries,
			func(entry *core.LoadedEntry) string { return entry.SessionID },
			func(sessionID string) (string, *string) { return sessionID, nil })
		for i := range rows {
			rows[i].SessionID = rows[i].Date
			rows[i].Date = nil
		}
		return rows
	default: // KindWeekly
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
}
