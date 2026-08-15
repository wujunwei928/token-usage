package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/openclaw"
)

func init() {
	registerAgentCommand(newOpenClawCommand)
}

func newOpenClawCommand() *cobra.Command {
	spec := &agentCommandSpec{
		agent:       "openclaw",
		display:     "OpenClaw",
		short:       "Show OpenClaw usage commands",
		openClawArg: true,
		run:         runOpenClawReport,
	}
	return newAgentCommandTree(spec)
}

// runOpenClawReport loads OpenClaw entries (honoring --open-claw-path),
// filters by date, summarizes, prints.
func runOpenClawReport(f *sharedFlags, kind agentKind, st *agentFlagState) error {
	shared := f.shared
	pricing := agentPricing(shared)
	var customPath *string
	if st.openClawPathGiven {
		customPath = &st.openClawPath
	}
	entries, err := openclaw.LoadEntries(shared, customPath, pricing)
	if err != nil {
		return err
	}
	entries = filterAgentEntriesByDate(entries, shared)
	rows := openclaw.SummarizeEntries(entries, mapOpenClawReportKind(kind))
	return printAgentReport(f, rows, kind, "OpenClaw Token Usage Report", kind == agentKindSession, false)
}

func mapOpenClawReportKind(kind agentKind) openclaw.ReportKind {
	switch kind {
	case agentKindSession:
		return openclaw.KindSession
	case agentKindMonthly:
		return openclaw.KindMonthly
	case agentKindWeekly:
		return openclaw.KindWeekly
	default:
		return openclaw.KindDaily
	}
}
