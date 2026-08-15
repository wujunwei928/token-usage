package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/copilot"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newCopilotCommand)
}

// newCopilotCommand builds the `token-usage copilot` command tree: daily (the
// default when no report is named), monthly, and session.
func newCopilotCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "copilot", Short: "Show GitHub Copilot CLI usage commands"}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := copilotReportArgsError(args); err != nil {
			return err
		}
		return runCopilotReport(copilot.KindDaily, f)
	}
	cmd.AddCommand(
		newCopilotReportCommand(copilot.KindDaily, "daily", "Show GitHub Copilot CLI usage grouped by date"),
		newCopilotReportCommand(copilot.KindMonthly, "monthly", "Show GitHub Copilot CLI usage grouped by month"),
		newCopilotReportCommand(copilot.KindSession, "session", "Show GitHub Copilot CLI usage grouped by session"),
	)
	return cmd
}

func newCopilotReportCommand(kind copilot.ReportKind, use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return copilotReportArgsError(args)
		}
		return runCopilotReport(kind, f)
	}
	return cmd
}

// copilotReportArgsError rejects positional arguments with the reference
// message for unsupported reports.
func copilotReportArgsError(args []string) error {
	for _, arg := range args {
		if !agentReportSupported("copilot", arg) {
			return parseErr(
				"The %q report is not available for GitHub Copilot CLI usage.\nUse \"token-usage copilot daily\" for GitHub Copilot CLI usage reports.",
				arg)
		}
	}
	return nil
}

// runCopilotReport is the copilot pipeline: load, window, summarize, sort,
// render (rust/adapters/copilot/src/lib.rs run()).
func runCopilotReport(kind copilot.ReportKind, f *sharedFlags) error {
	if err := f.resolve(); err != nil {
		return err
	}
	if err := f.validateLast(kind != copilot.KindSession); err != nil {
		return err
	}
	unit := core.PeriodDay
	if kind == copilot.KindMonthly {
		unit = core.PeriodMonth
	}
	f.resolveLastSince(unit, core.Monday)

	shared := f.shared
	pricing := core.LoadWithOverrides(shared.Offline, ampCopilotPricingLog(), shared.PricingOverrides)
	entries, err := copilot.LoadEntries(shared, pricing)
	if err != nil {
		return err
	}
	entries = ampCopilotEntriesInWindow(entries, shared)
	rows := copilot.SummarizeEntries(entries, kind)
	rows = core.SortSummaries(rows, shared.Order, func(r *core.UsageSummary) string {
		return copilot.SummaryPeriod(r)
	})
	if core.WantsJSON(shared) {
		return core.PrintJSONOrJQ(copilot.ReportJSON(rows, kind), shared.JQ, shared.NoCost)
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, copilotEmptyUsageMessage)
		return nil
	}
	return core.PrintUsageTable("GitHub Copilot CLI Token Usage Report", kind.FirstColumn(),
		rows, shared, false, nil)
}

const copilotEmptyUsageMessage = "No GitHub Copilot CLI usage data found.\nEnable Copilot OpenTelemetry file export before starting or resuming Copilot sessions.\nSee https://ccusage.com/guide/copilot/#data-source"
