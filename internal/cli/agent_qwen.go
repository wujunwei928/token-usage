package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/qwen"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newQwenCommand)
}

// Qwen sessions carry the activity metadata and filter by last activity after
// summarizing; empty reports render a null totals object (the profile and
// render flags express the reference's session semantics).
func newQwenCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:           "qwen",
		display:         "Qwen",
		short:           "Show Qwen usage commands",
		title:           "Qwen Token Usage Report",
		profile:         qwen.Profile,
		sessionMeta:     true,
		totalsNullEmpty: true,
		load: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error) {
			return qwen.LoadEntries(f.shared)
		},
	})
}
