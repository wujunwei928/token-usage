package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/hermes"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newHermesCommand)
}

func newHermesCommand() *cobra.Command {
	run := func(f *sharedFlags, kind hermes.ReportKind) error {
		rows, err := hermes.LoadSummaries(f.shared, kind)
		if err != nil {
			return err
		}
		rows = core.SortSummaries(rows, f.shared.Order, hermes.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(hermes.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("Hermes Token Usage Report", kind.FirstColumn(), rows, f.shared, false, nil)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:     "hermes",
		Display: "Hermes",
		Short:   "Show Hermes usage commands",
		About:   "Usage reports for hermes.",
		RunDaily: func(f *sharedFlags) error {
			return run(f, hermes.KindDaily)
		},
		RunMonthly: func(f *sharedFlags) error {
			return run(f, hermes.KindMonthly)
		},
		RunSession: func(f *sharedFlags) error {
			return run(f, hermes.KindSession)
		},
	})
}
