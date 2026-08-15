package opencode

// Report aggregation for the OpenCode agent: daily/weekly/monthly buckets and
// session grouping, plus the JSON rendering shared by the CLI and unified
// report.

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind selects the report granularity. It mirrors all.ReportKind for
// adapter-local use.
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// SummarizeEntries aggregates loaded entries into report rows for one kind:
// daily rows by entry date (weekly/monthly roll those up, weekly buckets
// starting Monday), or one row per session.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(date string) (string, *string) { return date, nil })
	case KindWeekly:
		daily := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(date string) (string, *string) { return date, nil })
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Monday)
	case KindMonthly:
		daily := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(date string) (string, *string) { return date, nil })
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		var grouped []*core.SessionAccumulator
		indexes := map[string]int{}
		for i := range entries {
			key := entries[i].SessionID
			index, ok := indexes[key]
			if !ok {
				index = len(grouped)
				indexes[key] = index
				grouped = append(grouped, &core.SessionAccumulator{})
			}
			grouped[index].AddEntry(&entries[i])
		}
		rows := make([]core.UsageSummary, 0, len(grouped))
		for _, group := range grouped {
			rows = append(rows, group.IntoSummary())
		}
		return rows
	}
	return nil
}

// summaryPeriod picks the row's period label: date, week, month, or session id.
func summaryPeriod(row *core.UsageSummary) string {
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

// ReportJSON builds the JSON report for one kind: sorted rows plus totals.
func ReportJSON(entries []core.LoadedEntry, kind ReportKind, order core.SortOrder) core.J {
	rows := SummarizeEntries(entries, kind)
	rows = core.SortSummaries(rows, order, summaryPeriod)
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = AgentSummaryJSON(&rows[i], kind)
	}
	return core.JObjV(
		rowsKey(kind), core.JArrV(items...),
		"totals", core.TotalsJSON(rows),
	)
}

func rowsKey(kind ReportKind) string {
	switch kind {
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

// AgentSummaryJSON renders one row for the agent JSON report. Keys render
// alphabetically, matching the reference's serde_json::Value output.
func AgentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		periodKey(kind), core.JStrV(summaryPeriod(row)),
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

func periodKey(kind ReportKind) string {
	switch kind {
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
