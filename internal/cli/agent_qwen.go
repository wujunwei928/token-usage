package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/qwen"
)

func init() {
	registerAgentCommand(newQwenCommand)
}

func newQwenCommand() *cobra.Command {
	spec := &agentCommandSpec{
		agent:   "qwen",
		display: "Qwen",
		short:   "Show Qwen usage commands",
		run:     runQwenReport,
	}
	return newAgentCommandTree(spec)
}

// runQwenReport follows the reference qwen run(): session rows summarize the
// unfiltered entries and then filter by last activity; the other reports
// filter entries by date first. Empty reports render a null totals object.
func runQwenReport(f *sharedFlags, kind agentKind, st *agentFlagState) error {
	shared := f.shared
	entries, err := qwen.LoadEntries(shared)
	if err != nil {
		return err
	}
	if kind == agentKindSession {
		rows := qwen.SummarizeEntries(entries, qwen.KindSession)
		rows = filterAgentSessionSummaries(rows, shared)
		return printAgentReport(f, rows, kind, "Qwen Token Usage Report", true, true)
	}
	entries = filterAgentEntriesByDate(entries, shared)
	rows := qwen.SummarizeEntries(entries, mapQwenReportKind(kind))
	return printAgentReport(f, rows, kind, "Qwen Token Usage Report", false, true)
}

func mapQwenReportKind(kind agentKind) qwen.ReportKind {
	switch kind {
	case agentKindMonthly:
		return qwen.KindMonthly
	case agentKindWeekly:
		return qwen.KindWeekly
	default:
		return qwen.KindDaily
	}
}
