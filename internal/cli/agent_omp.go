package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/omp"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newOmpCommand)
}

// omp accepts --omp-path to point at its sessions directory; empty reports
// render a null totals object (the pi-family JSON shape).
func newOmpCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:           "omp",
		display:         "oh-my-pi",
		short:           "Usage reports for oh-my-pi.",
		title:           "oh-my-pi Token Usage Report",
		profile:         omp.Profile,
		totalsNullEmpty: true,
		extraOptions: []agentExtraOption{
			{
				long:  "--omp-path",
				help:  "Path to oh-my-pi sessions directory (default: auto-discovery)",
				store: func(st *agentFlagState, value string) { st.set("--omp-path", value) },
			},
		},
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			var customPath *string
			if st.has("--omp-path") {
				path := st.get("--omp-path")
				customPath = &path
			}
			return omp.LoadEntries(omp.LoadOptions{Shared: f.shared, CustomPath: customPath, Pricing: agentPricing(f.shared)})
		},
	})
}
