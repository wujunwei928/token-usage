package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/adapter/grok"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// grok landed upstream after the 20.0.19 release the installed reference
// binary ships; keep the adapter but do not register the command until the
// port tracks a newer version.
func disabledInit() {
	registerAgentCommand(newGrokCommand)
}

func newGrokCommand() *cobra.Command {
	run := func(f *sharedFlags, kind grok.ReportKind) error {
		rows, err := grok.LoadSummaries(f.shared, kind)
		if err != nil {
			return err
		}
		rows = core.SortSummaries(rows, f.shared.Order, grok.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(grok.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("Grok Token Usage Report", kind.FirstColumn(), rows, f.shared, false, nil)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:     "grok",
		Display: "Grok",
		Short:   "Show Grok usage commands",
		About:   "Usage reports for grok.",
		RunDaily: func(f *sharedFlags) error {
			return run(f, grok.KindDaily)
		},
		RunMonthly: func(f *sharedFlags) error {
			return run(f, grok.KindMonthly)
		},
		RunSession: func(f *sharedFlags) error {
			return run(f, grok.KindSession)
		},
	})
}
