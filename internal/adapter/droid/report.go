package droid

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind selects the Droid report granularity.
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// String returns the CLI name of the kind.
func (k ReportKind) String() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "session"
	}
}

// FirstColumn returns the table's first column header for the kind.
func (k ReportKind) FirstColumn() string {
	switch k {
	case KindDaily:
		return "Date"
	case KindWeekly:
		return "Week"
	case KindMonthly:
		return "Month"
	default:
		return "Session"
	}
}

// RowsKey returns the JSON key holding the rows for the kind.
func (k ReportKind) RowsKey() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "sessions"
	}
}

func (k ReportKind) periodKey() string {
	switch k {
	case KindDaily:
		return "date"
	case KindWeekly:
		return "week"
	case KindMonthly:
		return "month"
	default:
		return "sessionId"
	}
}

// SummarizeEntries groups entries into report rows for the kind.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(key string) (string, *string) { return key, nil })
	case KindMonthly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		rows := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.SessionID },
			func(key string) (string, *string) { return key, nil })
		for i := range rows {
			if rows[i].Date != nil {
				sessionID := *rows[i].Date
				rows[i].SessionID = &sessionID
				rows[i].Date = nil
			}
		}
		return rows
	default: // KindWeekly
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
}

// SummaryPeriod picks the sort key of a row.
func SummaryPeriod(row *core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Week != nil:
		return *row.Week
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	default:
		return ""
	}
}

// ReportFromRows builds the JSON report object for the kind.
func ReportFromRows(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = agentSummaryJSON(&rows[i], kind)
	}
	return core.JObjV(
		kind.RowsKey(), core.JArrV(items...),
		"totals", core.TotalsJSON(rows),
	)
}

func agentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		kind.periodKey(), core.JStrV(SummaryPeriod(row)),
		"inputTokens", core.JUintV(row.InputTokens),
		"outputTokens", core.JUintV(row.OutputTokens),
		"cacheCreationTokens", core.JUintV(row.CacheCreationTokens),
		"cacheReadTokens", core.JUintV(row.CacheReadTokens),
		"totalTokens", core.JUintV(row.TotalTokens()),
		"totalCost", core.JFloatV(row.TotalCost),
		"modelsUsed", modelsUsedJ(row.ModelsUsed),
		"modelBreakdowns", modelBreakdownsJ(row.ModelBreakdowns),
	}
	if row.Credits != nil {
		pairs = append(pairs, "credits", core.JFloatV(*row.Credits))
	}
	if row.MessageCount != nil {
		pairs = append(pairs, "messageCount", core.JUintV(*row.MessageCount))
	}
	return core.JObjV(pairs...)
}

func modelsUsedJ(models []string) core.J {
	items := make([]core.J, 0, len(models))
	for _, m := range models {
		items = append(items, core.JStrV(m))
	}
	return core.JArrV(items...)
}

func modelBreakdownsJ(breakdowns []core.ModelBreakdown) core.J {
	items := make([]core.J, 0, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items = append(items, core.JObjV(
			"modelName", core.JStrV(b.ModelName),
			"inputTokens", core.JUintV(b.InputTokens),
			"outputTokens", core.JUintV(b.OutputTokens),
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			"cost", core.JFloatV(b.Cost),
		))
	}
	return core.JArrV(items...)
}

// FilterLoadedEntriesByDate applies the since/until window to entry dates.
func FilterLoadedEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	filtered := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			filtered = append(filtered, entries[i])
		}
	}
	return filtered
}

// LoadSummaries loads entries and summarizes them for the kind, applying the
// shared date window first (the reference agent run order).
func LoadSummaries(shared *core.SharedArgs, kind ReportKind) ([]core.UsageSummary, error) {
	entries, err := LoadEntries(shared)
	if err != nil {
		return nil, err
	}
	entries = FilterLoadedEntriesByDate(entries, shared)
	return SummarizeEntries(entries, kind), nil
}
