package pi

import (
	"sort"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// ReportKind selects the pi report granularity.
type ReportKind int

// Report kinds.
const (
	ReportDaily ReportKind = iota
	ReportWeekly
	ReportMonthly
	ReportSession
)

// RowsKey returns the JSON rows key for the kind.
func (k ReportKind) RowsKey() string {
	switch k {
	case ReportDaily:
		return "daily"
	case ReportWeekly:
		return "weekly"
	case ReportMonthly:
		return "monthly"
	default:
		return "sessions"
	}
}

// PeriodKey returns the JSON period field for the kind.
func (k ReportKind) PeriodKey() string {
	switch k {
	case ReportDaily:
		return "date"
	case ReportWeekly:
		return "week"
	case ReportMonthly:
		return "month"
	default:
		return "sessionId"
	}
}

// FirstColumn returns the table's first column header for the kind.
func (k ReportKind) FirstColumn() string {
	switch k {
	case ReportDaily:
		return "Date"
	case ReportWeekly:
		return "Week"
	case ReportMonthly:
		return "Month"
	default:
		return "Session"
	}
}

// SummaryPeriod picks the row's display/sort period: date, week, month, then
// session id.
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

// SummarizeEntries groups loaded pi entries into report rows.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case ReportDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(date string) (string, *string) { return date, nil })
	case ReportMonthly:
		daily := SummarizeEntries(entries, ReportDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case ReportSession:
		groups := map[string]*core.SessionAccumulator{}
		for i := range entries {
			key := entries[i].SessionID
			if _, ok := groups[key]; !ok {
				groups[key] = &core.SessionAccumulator{}
			}
			groups[key].AddEntry(&entries[i])
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		rows := make([]core.UsageSummary, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, groups[key].IntoSummary())
		}
		return rows
	case ReportWeekly:
		daily := SummarizeEntries(entries, ReportDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
	return nil
}

// ReportFromRows builds {<kind>: rows, totals} for JSON output. The pi report
// renders totals as null when there are no rows.
func ReportFromRows(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = AgentSummaryJSON(&rows[i], kind, kind == ReportSession)
	}
	totals := core.JNullV
	if len(rows) > 0 {
		totals = core.TotalsJSON(rows)
	}
	return core.JObjV(kind.RowsKey(), core.JArrV(items...), "totals", totals)
}

// AgentSummaryJSON renders one row in the agent report JSON shape: the period
// key is always present, credits/messageCount appear when set, and session
// rows may carry lastActivity/firstActivity/projectPath metadata.
func AgentSummaryJSON(row *core.UsageSummary, kind ReportKind, includeSessionMetadata bool) core.J {
	pairs := []any{
		kind.PeriodKey(), core.JStrV(SummaryPeriod(row)),
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
	if includeSessionMetadata {
		pairs = append(pairs,
			"lastActivity", core.JOptStrV(row.LastActivity),
			"firstActivity", core.JOptStrV(row.FirstActivity),
			"projectPath", core.JOptStrV(row.ProjectPath),
		)
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
