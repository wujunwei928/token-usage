package opencode

// Report aggregation for the OpenCode agent: daily/weekly/monthly buckets and
// session grouping, plus the JSON rendering shared by the CLI and unified
// report.

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind is the shared report vocabulary (ADR 0009): an alias of
// core.ReportKind kept for this file's local signatures.
type ReportKind = core.ReportKind

// SummarizeEntries aggregates loaded entries via the shared pipeline under
// OpenCode's profile (Monday weeks, activity-bounded sessions).
func summarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	return common.SummarizeReport(entries, kind, Profile)
}

// ReportJSON builds the JSON report for one kind: sorted rows plus totals.
func ReportJSON(entries []core.LoadedEntry, kind ReportKind, order core.SortOrder) core.J {
	rows := summarizeEntries(entries, kind)
	rows = core.SortSummaries(rows, order, common.SummaryPeriod)
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = AgentSummaryJSON(&rows[i], kind)
	}
	return core.JObjV(
		kind.RowsKey(), core.JArrV(items...),
		"totals", core.TotalsJSON(rows),
	)
}

// AgentSummaryJSON renders one row for the agent JSON report. Keys render
// alphabetically, matching the reference's serde_json::Value output.
func AgentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		kind.PeriodKey(), core.JStrV(common.SummaryPeriod(row)),
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
