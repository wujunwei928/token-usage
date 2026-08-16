package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/gemini"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newGeminiCommand)
}

func newGeminiCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "gemini",
		display: "Gemini CLI",
		short:   "Show Gemini CLI usage commands",
		title:   "Gemini CLI Token Usage Report",
		profile: gemini.Profile,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return gemini.LoadEntries(f.shared, agentPricing(f.shared))
		},
	})
}
