package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/amp"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newAmpCommand)
}

// newAmpCommand builds the `token-usage amp` command tree: daily (the default when
// no report is named), monthly, and session.
func newAmpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "amp", Short: "Show Amp token usage commands"}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := ampReportArgsError(args); err != nil {
			return err
		}
		return runAmpReport(amp.KindDaily, f)
	}
	cmd.AddCommand(
		newAmpReportCommand(amp.KindDaily, "daily", "Show Amp token usage grouped by day"),
		newAmpReportCommand(amp.KindMonthly, "monthly", "Show Amp token usage grouped by month"),
		newAmpReportCommand(amp.KindSession, "session", "Show Amp token usage grouped by session"),
	)
	return cmd
}

func newAmpReportCommand(kind amp.ReportKind, use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return ampReportArgsError(args)
		}
		return runAmpReport(kind, f)
	}
	return cmd
}

// ampReportArgsError rejects positional arguments with the reference message
// for unsupported reports (unknown subcommands land here).
func ampReportArgsError(args []string) error {
	for _, arg := range args {
		if !agentReportSupported("amp", arg) {
			return parseErr(
				"The %q report is not available for Amp usage.\nUse \"token-usage amp daily\" for Amp usage reports.",
				arg)
		}
	}
	return nil
}

// runAmpReport is the amp pipeline: load, window, summarize, sort, render
// (rust/adapters/amp/src/lib.rs run()).
func runAmpReport(kind amp.ReportKind, f *sharedFlags) error {
	if err := f.resolve(); err != nil {
		return err
	}
	if err := f.validateLast(kind != amp.KindSession); err != nil {
		return err
	}
	unit := core.PeriodDay
	if kind == amp.KindMonthly {
		unit = core.PeriodMonth
	}
	f.resolveLastSince(unit, core.Monday)

	shared := f.shared
	pricing := core.LoadWithOverrides(shared.Offline, ampCopilotPricingLog(), shared.PricingOverrides)
	entries, err := amp.LoadEntries(shared, pricing)
	if err != nil {
		return err
	}
	entries = ampCopilotEntriesInWindow(entries, shared)
	rows := amp.SummarizeEntries(entries, kind)
	rows = core.SortSummaries(rows, shared.Order, func(r *core.UsageSummary) string {
		return amp.SummaryPeriod(r)
	})
	if core.WantsJSON(shared) {
		return core.PrintJSONOrJQ(amp.ReportJSON(rows, kind), shared.JQ, shared.NoCost)
	}
	return amp.PrintTable(kind, rows, shared)
}

// ampCopilotPricingLog mirrors the reference log gate (`log_level() != Some(0)`)
// for the amp/copilot pricing load.
func ampCopilotPricingLog() bool {
	if level := core.LogLevel(); level != nil && *level == 0 {
		return false
	}
	return true
}

// ampCopilotEntriesInWindow applies the since/until window to loaded entries
// (filter_loaded_entries_by_date).
func ampCopilotEntriesInWindow(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	filtered := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			filtered = append(filtered, entries[i])
		}
	}
	return filtered
}
