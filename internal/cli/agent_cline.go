package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/cline"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newClineCommand)
}

func newClineCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:           "cline",
		display:         "Cline",
		short:           "Show Cline usage commands",
		title:           "Cline Token Usage Report",
		profile:         cline.Profile,
		sessionMeta:     true,
		totalsNullEmpty: true,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return cline.LoadEntries(f.shared, agentPricing(f.shared))
		},
	})
}
