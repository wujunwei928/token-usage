package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/amp"
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newAmpCommand)
}

// Amp renders through its own table (the Credits column shape); JSON uses the
// shared agent shape.
func newAmpCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:    "amp",
		display:  "Amp",
		short:    "Show Amp token usage commands",
		subShort: func(kind core.ReportKind) string { return shortTokenUsageGrouped("Amp", kind) },
		profile:  amp.Profile,
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			entries, err := amp.LoadEntries(f.shared, agentPricing(f.shared))
			if err != nil {
				return err
			}
			rows := common.ReportRows(entries, kind, f.shared, amp.Profile)
			return printCustomAgentReport(f, rows, kind,
				func() error { return amp.PrintTable(kind, rows, f.shared) })
		},
	})
}
