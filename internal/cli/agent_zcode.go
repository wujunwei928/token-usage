package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/zcode"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newZcodeCommand)
}

// ZCode sessions carry the activity metadata and filter by last activity
// after summarizing; empty reports render a null totals object.
func newZcodeCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:           "zcode",
		display:         "ZCode",
		short:           "Show ZCode usage commands",
		title:           "ZCode Token Usage Report",
		profile:         zcode.Profile,
		sessionMeta:     true,
		totalsNullEmpty: true,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return zcode.LoadEntries(f.shared)
		},
	})
}
