package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/kimi"
)

func init() {
	registerAgentCommand(newKimiCommand)
}

func newKimiCommand() *cobra.Command {
	spec := &agentCommandSpec{
		agent:   "kimi",
		display: "Kimi",
		short:   "Show Kimi usage commands",
		run:     runKimiReport,
	}
	return newAgentCommandTree(spec)
}

// runKimiReport loads Kimi entries, filters by date, summarizes, prints.
func runKimiReport(f *sharedFlags, kind agentKind, st *agentFlagState) error {
	shared := f.shared
	pricing := agentPricing(shared)
	entries, err := kimi.LoadEntries(shared, pricing)
	if err != nil {
		return err
	}
	entries = filterAgentEntriesByDate(entries, shared)
	rows := kimi.SummarizeEntries(entries, mapKimiReportKind(kind))
	return printAgentReport(f, rows, kind, "Kimi Token Usage Report", false, false)
}

func mapKimiReportKind(kind agentKind) kimi.ReportKind {
	switch kind {
	case agentKindSession:
		return kimi.KindSession
	case agentKindMonthly:
		return kimi.KindMonthly
	case agentKindWeekly:
		return kimi.KindWeekly
	default:
		return kimi.KindDaily
	}
}
