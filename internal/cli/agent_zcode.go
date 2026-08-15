package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/zcode"
)

func init() {
	registerAgentCommand(newZcodeCommand)
}

func newZcodeCommand() *cobra.Command {
	spec := &agentCommandSpec{
		agent:   "zcode",
		display: "ZCode",
		short:   "Show ZCode usage commands",
		run:     runZcodeReport,
	}
	return newAgentCommandTree(spec)
}

// runZcodeReport follows the reference agent run(): session rows summarize
// the unfiltered entries and then filter by last activity; the other reports
// filter entries by date first.
func runZcodeReport(f *sharedFlags, kind agentKind, st *agentFlagState) error {
	shared := f.shared
	entries, err := zcode.LoadEntries(shared)
	if err != nil {
		return err
	}
	if kind == agentKindSession {
		rows := zcode.SummarizeEntries(entries, zcode.KindSession)
		rows = filterAgentSessionSummaries(rows, shared)
		return printAgentReport(f, rows, kind, "ZCode Token Usage Report", true, true)
	}
	entries = filterAgentEntriesByDate(entries, shared)
	rows := zcode.SummarizeEntries(entries, mapZcodeReportKind(kind))
	return printAgentReport(f, rows, kind, "ZCode Token Usage Report", false, true)
}

func mapZcodeReportKind(kind agentKind) zcode.ReportKind {
	switch kind {
	case agentKindMonthly:
		return zcode.KindMonthly
	case agentKindWeekly:
		return zcode.KindWeekly
	default:
		return zcode.KindDaily
	}
}
