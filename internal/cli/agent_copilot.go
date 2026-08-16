package cli

import (
	"github.com/spf13/cobra"

	"fmt"
	"os"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/adapter/copilot"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newCopilotCommand)
}

// Copilot's table render carries the empty-data hint on stderr (the
// OpenTelemetry export pointer); JSON uses the shared agent shape.
func newCopilotCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:   "copilot",
		display: "GitHub Copilot CLI",
		short:   "Show GitHub Copilot CLI usage commands",
		profile: copilot.Profile,
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			entries, err := copilot.LoadEntries(f.shared, agentPricing(f.shared))
			if err != nil {
				return err
			}
			rows := common.ReportRows(entries, kind, f.shared, copilot.Profile)
			return printCustomAgentReport(f, rows, kind, func() error {
				if len(rows) == 0 {
					fmt.Fprintln(os.Stderr, copilotEmptyUsageMessage)
					return nil
				}
				return core.PrintUsageTable("GitHub Copilot CLI Token Usage Report",
					kind.FirstColumn(), rows, f.shared, false, nil)
			})
		},
	})
}

const copilotEmptyUsageMessage = "No GitHub Copilot CLI usage data found.\nEnable Copilot OpenTelemetry file export before starting or resuming Copilot sessions.\nSee https://ccusage.com/guide/copilot/#data-source"
