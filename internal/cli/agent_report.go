package cli

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// printAgentReport sorts the rows and renders the JSON or table form shared by
// the standard agent path. The JSON shape is common.AgentReportJSON — the one
// agent-report shape (ADR 0009/0010).
func printAgentReport(f *sharedFlags, rows []core.UsageSummary, kind core.ReportKind, title string, sessionMeta, totalsNullEmpty bool) error {
	shared := f.shared
	rows = core.SortSummaries(rows, shared.Order, common.SummaryPeriod)
	if core.WantsJSON(shared) {
		return core.PrintJSONOrJQ(
			common.AgentReportJSON(rows, kind, sessionMeta, totalsNullEmpty),
			shared.JQ, shared.NoCost)
	}
	return core.PrintUsageTable(title, kind.FirstColumn(), rows, shared, false, nil)
}

// printCustomAgentReport is the custom-render twin of printAgentReport: the
// JSON path is the shared agent shape, the table render is the agent's own
// (codebuff/goose titles, amp's Credits columns, copilot's empty-data hint).
func printCustomAgentReport(f *sharedFlags, rows []core.UsageSummary, kind core.ReportKind, table func() error) error {
	shared := f.shared
	rows = core.SortSummaries(rows, shared.Order, common.SummaryPeriod)
	if core.WantsJSON(shared) {
		return core.PrintJSONOrJQ(
			common.AgentReportJSON(rows, kind, false, false),
			shared.JQ, shared.NoCost)
	}
	return table()
}
