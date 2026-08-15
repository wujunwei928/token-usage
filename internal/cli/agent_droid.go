package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/droid"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newDroidCommand)
}

func newDroidCommand() *cobra.Command {
	run := func(f *sharedFlags, kind droid.ReportKind) error {
		rows, err := droid.LoadSummaries(f.shared, kind)
		if err != nil {
			return err
		}
		rows = core.SortSummaries(rows, f.shared.Order, droid.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(droid.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("Droid Token Usage Report", kind.FirstColumn(), rows, f.shared, false, nil)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:     "droid",
		Display: "Droid",
		Short:   "Show Droid usage commands",
		About:   "Usage reports for droid.",
		RunDaily: func(f *sharedFlags) error {
			return run(f, droid.KindDaily)
		},
		RunMonthly: func(f *sharedFlags) error {
			return run(f, droid.KindMonthly)
		},
		RunSession: func(f *sharedFlags) error {
			return run(f, droid.KindSession)
		},
	})
}
