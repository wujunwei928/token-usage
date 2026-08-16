package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/pi"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newPiCommand)
}

// pi accepts --pi-path to point at its sessions directory; empty reports
// render a null totals object (the pi JSON shape).
func newPiCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:           "pi",
		display:         "pi-agent",
		short:           "Usage reports for pi.",
		title:           "pi-agent Token Usage Report",
		profile:         pi.Profile,
		totalsNullEmpty: true,
		extraOptions: []agentExtraOption{
			{
				long:  "--pi-path",
				help:  "Path to pi agent sessions directory (default: auto-discovery)",
				store: func(st *agentFlagState, value string) { st.set("--pi-path", value) },
			},
		},
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			var customPath *string
			if st.has("--pi-path") {
				path := st.get("--pi-path")
				customPath = &path
			}
			return pi.LoadEntries(pi.LoadOptions{Shared: f.shared, CustomPath: customPath, Pricing: agentPricing(f.shared)})
		},
	})
}
