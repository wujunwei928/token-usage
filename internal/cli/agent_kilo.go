package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/kilo"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newKiloCommand)
}

func newKiloCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "kilo",
		display: "Kilo",
		short:   "Usage reports for kilo.",
		title:   "Kilo Token Usage Report",
		profile: kilo.Profile,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return kilo.LoadEntries(f.shared, agentPricing(f.shared))
		},
	})
}
