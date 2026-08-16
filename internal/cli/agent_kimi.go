package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/kimi"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newKimiCommand)
}

func newKimiCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "kimi",
		display: "Kimi",
		short:   "Show Kimi usage commands",
		title:   "Kimi Token Usage Report",
		profile: kimi.Profile,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return kimi.LoadEntries(f.shared, agentPricing(f.shared))
		},
	})
}
