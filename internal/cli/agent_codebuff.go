package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/adapter/codebuff"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func init() {
	registerAgentCommand(newCodebuffCommand)
}

func newCodebuffCommand() *cobra.Command {
	run := func(f *sharedFlags, kind codebuff.ReportKind) error {
		rows, err := codebuff.LoadSummaries(f.shared, kind)
		if err != nil {
			return err
		}
		rows = core.SortSummaries(rows, f.shared.Order, codebuff.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(codebuff.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return codebuff.PrintTableForAgent("Codebuff", kind, rows, f.shared)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:     "codebuff",
		Display: "Codebuff",
		Short:   "Show Codebuff usage commands",
		About:   "Usage reports for codebuff.",
		RunDaily: func(f *sharedFlags) error {
			return run(f, codebuff.KindDaily)
		},
		RunMonthly: func(f *sharedFlags) error {
			return run(f, codebuff.KindMonthly)
		},
		RunSession: func(f *sharedFlags) error {
			return run(f, codebuff.KindSession)
		},
	})
}
