package common

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// SummaryPeriod picks the sort key of a row: date, else week, else month,
// else session id, else the empty string.
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

// AgentReportJSON is the single agent-report JSON shape (ADR 0009/0010):
// rows under the kind's array key plus totals. With sessionMeta, session rows
// append the activity metadata (lastActivity/firstActivity/projectPath); with
// totalsNullEmpty an empty report renders a null totals object.
func AgentReportJSON(rows []core.UsageSummary, kind core.ReportKind, sessionMeta, totalsNullEmpty bool) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = agentSummaryJSON(&rows[i], kind, sessionMeta)
	}
	totals := core.TotalsJSON(rows)
	if totalsNullEmpty && len(rows) == 0 {
		totals = core.JNullV
	}
	return core.JObjV(
		kind.RowsKey(), core.JArrV(items...),
		"totals", totals,
	)
}

func agentSummaryJSON(row *core.UsageSummary, kind core.ReportKind, sessionMeta bool) core.J {
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
	if sessionMeta {
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
