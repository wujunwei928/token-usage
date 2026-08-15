package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/core"
)

// agentCommandFactories collects agent subcommands registered by adapter
// files; each adapter owns its own agent_<name>.go registration file.
var agentCommandFactories []func() *cobra.Command

// registerAgentCommand adds an agent command factory (called from init()).
func registerAgentCommand(factory func() *cobra.Command) {
	agentCommandFactories = append(agentCommandFactories, factory)
}

// newAgentReportCommand builds a report subcommand shared by agent adapters:
// it loads rows via the supplied loader and renders the single-agent report.
func newAgentReportCommand(use, short, title, firstColumn, jsonKey string, load func(shared *core.SharedArgs) ([]core.UsageSummary, error), weekStart core.WeekDay) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.resolve(); err != nil {
			return err
		}
		rows, err := load(f.shared)
		if err != nil {
			return err
		}
		rows = core.FilterAndSortSummaries(rows, f.shared, func(r *core.UsageSummary) string {
			if r.Date != nil {
				return *r.Date
			}
			return ""
		})
		if core.WantsJSON(f.shared) {
			items := make([]core.J, len(rows))
			for i := range rows {
				items[i] = core.SummaryJSON(&rows[i])
			}
			return core.PrintJSONOrJQ(core.JObjV(
				jsonKey, core.JArrV(items...),
				"totals", core.TotalsJSON(rows),
			), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable(title, firstColumn, rows, f.shared, false, nil)
	}
	return cmd
}
