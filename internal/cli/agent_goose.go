package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/adapter/goose"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newGooseCommand)
}

// Goose renders through its own table (the "<agent> Token Usage Report -
// <period>" title shape); JSON uses the shared agent shape.
func newGooseCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "goose",
		display: "Goose",
		short:   "Show Goose usage commands",
		profile: goose.Profile,
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			entries, err := goose.LoadEntries(f.shared, agentPricing(f.shared))
			if err != nil {
				return err
			}
			rows := common.ReportRows(entries, kind, f.shared, goose.Profile)
			return printCustomAgentReport(f, rows, kind,
				func() error { return goose.PrintTableForAgent("Goose", kind, rows, f.shared) })
		},
	})
}
