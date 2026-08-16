package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/openclaw"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newOpenClawCommand)
}

func newOpenClawCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "openclaw",
		display: "OpenClaw",
		short:   "Show OpenClaw usage commands",
		title:   "OpenClaw Token Usage Report",
		profile: openclaw.Profile,
		extraOptions: []agentExtraOption{
			{long: "--open-claw-path", store: func(st *agentFlagState, value string) { st.set("--open-claw-path", value) }},
		},
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			var customPath *string
			if st.has("--open-claw-path") {
				path := st.get("--open-claw-path")
				customPath = &path
			}
			return openclaw.LoadEntries(f.shared, customPath, agentPricing(f.shared))
		},
	})
}
