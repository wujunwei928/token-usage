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
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "hermes",
		display: "Hermes",
		short:   "Show Hermes usage commands",
		title:   "Hermes Token Usage Report",
		profile: hermes.Profile,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return hermes.LoadEntries(f.shared)
		},
	})
}
