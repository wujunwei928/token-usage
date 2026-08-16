package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/codebuff"
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newCodebuffCommand)
}

// Codebuff renders through its own table (the "<agent> Token Usage Report -
// <period>" title shape); JSON uses the shared agent shape.
func newCodebuffCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "codebuff",
		display: "Codebuff",
		short:   "Usage reports for codebuff.",
		profile: codebuff.Profile,
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			entries, err := codebuff.LoadEntries(f.shared)
			if err != nil {
				return err
			}
			rows := common.ReportRows(entries, kind, f.shared, codebuff.Profile)
			return printCustomAgentReport(f, rows, kind,
				func() error { return codebuff.PrintTableForAgent("Codebuff", kind, rows, f.shared) })
		},
	})
}
