package cli

// The `token-usage opencode` command: daily/weekly/monthly/session reports
// over the OpenCode adapter (the weekly kind comes from the support matrix).

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/adapter/opencode"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newOpenCodeCommand)
}

// OpenCode's JSON is entries-shaped (opencode.ReportJSON summarizes with its
// own order handling); the table render is the standard shape.
func newOpenCodeCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:    "opencode",
		display:  "OpenCode",
		short:    "Usage reports for opencode.",
		subShort: func(kind core.ReportKind) string { return shortTokenUsageGrouped("OpenCode", kind) },
		profile:  opencode.Profile,
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			entries, err := opencode.LoadEntries(f.shared)
			if err != nil {
				return err
			}
			entries = common.FilterLoadedEntriesByDate(entries, f.shared)
			if core.WantsJSON(f.shared) {
				return core.PrintJSONOrJQ(
					opencode.ReportJSON(entries, kind, f.shared.Order),
					f.shared.JQ, f.shared.NoCost)
			}
			rows := common.ReportRows(entries, kind, f.shared, opencode.Profile)
			return printCustomAgentReport(f, rows, kind, func() error {
				return core.PrintUsageTable("OpenCode Token Usage Report",
					kind.FirstColumn(), rows, f.shared, false, nil)
			})
		},
	})
}
