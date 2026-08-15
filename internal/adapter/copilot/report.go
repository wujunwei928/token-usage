package copilot

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind selects the report granularity (mirrors AgentReportKind).
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// FirstColumn renders the table's first column header for the kind.
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

// SummarizeEntries groups loaded entries into report rows exactly like
// rust/adapters/copilot/src/report.rs.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindMonthly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		rows := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.SessionID },
			func(key string) (string, *string) { return key, nil })
		for i := range rows {
			rows[i].SessionID = rows[i].Date
			rows[i].Date = nil
		}
		return rows
	case KindWeekly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	default:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(key string) (string, *string) { return key, nil })
	}
}

// SummaryPeriod returns the row's period label (date, week, month, or session).
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
	}
	return ""
}

// ReportJSON builds {<kind>: [...], totals: {...}} for JSON output; rows use
// the agent summary shape (period key varies by kind).
func ReportJSON(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = agentSummaryJSON(&rows[i], kind)
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

// agentSummaryJSON mirrors ccusage-core agent_report.rs (no session metadata).
func agentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		periodKey(kind), core.JStrV(SummaryPeriod(row)),
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
	items := make([]core.J, len(models))
	for i, m := range models {
		items[i] = core.JStrV(m)
	}
	return core.JArrV(items...)
}

func modelBreakdownsJ(breakdowns []core.ModelBreakdown) core.J {
	items := make([]core.J, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items[i] = core.JObjV(
			"modelName", core.JStrV(b.ModelName),
			"inputTokens", core.JUintV(b.InputTokens),
			"outputTokens", core.JUintV(b.OutputTokens),
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			"cost", core.JFloatV(b.Cost),
		)
	}
	return core.JArrV(items...)
}
