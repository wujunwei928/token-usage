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
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "droid",
		display: "Droid",
		short:   "Show Droid usage commands",
		title:   "Droid Token Usage Report",
		profile: droid.Profile,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return droid.LoadEntries(f.shared)
		},
	})
}
