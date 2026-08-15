package cli

// The `ccusage opencode` command tree: daily/weekly/monthly/session reports
// over the OpenCode adapter.

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/adapter/opencode"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func init() {
	registerAgentCommand(newOpenCodeCommand)
}

// openCodeKindMeta carries the per-subcommand presentation strings.
type openCodeKindMeta struct {
	use         string
	short       string
	firstColumn string
	jsonKey     string
}

func openCodeMeta(kind opencode.ReportKind) openCodeKindMeta {
	switch kind {
	case opencode.KindDaily:
		return openCodeKindMeta{"daily", "Show OpenCode token usage grouped by day", "Date", "daily"}
	case opencode.KindWeekly:
		return openCodeKindMeta{"weekly", "Show OpenCode token usage grouped by week", "Week", "weekly"}
	case opencode.KindMonthly:
		return openCodeKindMeta{"monthly", "Show OpenCode token usage grouped by month", "Month", "monthly"}
	default:
		return openCodeKindMeta{"session", "Show OpenCode token usage grouped by session", "Session", "sessions"}
	}
}

func newOpenCodeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "opencode",
		Short: "Usage reports for opencode.",
	}
	cmd.AddCommand(
		newOpenCodeReportCommand(opencode.KindDaily),
		newOpenCodeReportCommand(opencode.KindWeekly),
		newOpenCodeReportCommand(opencode.KindMonthly),
		newOpenCodeReportCommand(opencode.KindSession),
	)
	return cmd
}

// openCodePeriod resolves the --last window parameters per kind: sessions do
// not support --last, and weekly windows start on Monday like every unified
// report.
func openCodePeriod(kind opencode.ReportKind) (core.PeriodUnit, bool) {
	switch kind {
	case opencode.KindDaily:
		return core.PeriodDay, true
	case opencode.KindWeekly:
		return core.PeriodWeek, true
	case opencode.KindMonthly:
		return core.PeriodMonth, true
	default:
		return core.PeriodDay, false
	}
}

func newOpenCodeReportCommand(kind opencode.ReportKind) *cobra.Command {
	meta := openCodeMeta(kind)
	cmd := &cobra.Command{Use: meta.use, Short: meta.short}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.resolve(); err != nil {
			return err
		}
		unit, lastSupported := openCodePeriod(kind)
		if err := f.validateLast(lastSupported); err != nil {
			return err
		}
		f.resolveLastSince(unit, core.Monday)

		entries, err := opencode.LoadEntries(f.shared)
		if err != nil {
			return err
		}
		entries = filterOpenCodeEntriesByDate(entries, f.shared)

		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(
				opencode.ReportJSON(entries, kind, f.shared.Order),
				f.shared.JQ, f.shared.NoCost)
		}
		rows := opencode.SummarizeEntries(entries, kind)
		rows = core.SortSummaries(rows, f.shared.Order, openCodeSummaryPeriod)
		return core.PrintUsageTable("OpenCode Token Usage Report", meta.firstColumn,
			rows, f.shared, false, nil)
	}
	return cmd
}

// filterOpenCodeEntriesByDate keeps entries whose local date is inside the
// --since/--until window.
func filterOpenCodeEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	kept := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			kept = append(kept, entries[i])
		}
	}
	return kept
}

func openCodeSummaryPeriod(row *core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Week != nil:
		return *row.Week
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	default:
		return ""
	}
}
