package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/codex"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The codex agent command: daily/monthly/session reports over ~/.codex
// session logs (weekly is not a supported codex report).
func init() {
	registerAgentCommand(newCodexCommand)
}

const codexSpeedFlagHelp = "Cost speed tier: auto uses recorded settings, then Codex config.toml; use standard or fast to override (default: auto, choices: auto | standard | fast)"

func newCodexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codex",
		Short: "Usage reports for codex.",
	}
	f := registerSharedFlags(cmd)
	cmd.Flags().String("speed", "auto", codexSpeedFlagHelp)
	cmd.Flags().Lookup("speed").NoOptDefVal = "auto"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Bare `token-usage codex` (or `token-usage codex --flags`) is the daily
		// report; any positional token names a (possibly unsupported) report.
		if len(args) > 0 {
			if agentReportSupported("codex", args[0]) {
				return parseErr("Expected option, got '%s'", args[0])
			}
			if args[0] == "blocks" || args[0] == "statusline" {
				return parseErr("The \"%s\" report is only available for Claude Code usage.\nUse \"token-usage codex daily\" for Codex usage reports.", args[0])
			}
			return parseErr("The \"%s\" report is not available for Codex usage.\nUse \"token-usage codex daily\" for Codex usage reports.", args[0])
		}
		return runCodexReport(codex.KindDaily, f, cmd)
	}
	cmd.AddCommand(
		newCodexReportCommand(codex.KindDaily),
		newCodexReportCommand(codex.KindMonthly),
		newCodexSessionReportCommand(),
	)
	return cmd
}

func newCodexReportCommand(kind codex.Kind) *cobra.Command {
	use, short := codexReportMeta(kind)
	cmd := &cobra.Command{Use: use, Short: short}
	f := registerSharedFlags(cmd)
	cmd.Flags().String("speed", "auto", codexSpeedFlagHelp)
	cmd.Flags().Lookup("speed").NoOptDefVal = "auto"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.validateLast(true); err != nil {
			return err
		}
		resolveCodexLastSince(kind, f)
		return runCodexReport(kind, f, cmd)
	}
	return cmd
}

func newCodexSessionReportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Show Codex token usage grouped by session",
	}
	f := registerSharedFlags(cmd)
	cmd.Flags().String("speed", "auto", codexSpeedFlagHelp)
	cmd.Flags().Lookup("speed").NoOptDefVal = "auto"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.validateLast(false); err != nil {
			return err
		}
		return runCodexReport(codex.KindSession, f, cmd)
	}
	return cmd
}

func codexReportMeta(kind codex.Kind) (use, short string) {
	switch kind {
	case codex.KindMonthly:
		return "monthly", "Show Codex token usage grouped by month"
	default:
		return "daily", "Show Codex token usage grouped by day"
	}
}

// resolveCodexLastSince turns --last into a since bound anchored on the
// report's own period unit (Monday weeks, matching the unified reports).
func resolveCodexLastSince(kind codex.Kind, f *sharedFlags) {
	unit := core.PeriodDay
	if kind == codex.KindMonthly {
		unit = core.PeriodMonth
	} else if kind == codex.KindWeekly {
		unit = core.PeriodWeek
	}
	f.resolveLastSince(unit, core.Monday)
}

func runCodexReport(kind codex.Kind, f *sharedFlags, cmd *cobra.Command) error {
	if err := f.resolve(); err != nil {
		return err
	}
	speedRaw := cmd.Flags().Lookup("speed").Value.String()
	speedChoice, ok := codex.ParseSpeed(speedRaw)
	if !ok {
		return parseErr("Invalid speed option '%s'", speedRaw)
	}
	pricing := core.LoadWithOverrides(f.shared.Offline, codexPricingLog(), f.shared.PricingOverrides)
	groups, err := codex.LoadGroups(f.shared, kind)
	if err != nil {
		return err
	}
	speed := codex.ResolveSpeed(speedChoice)
	if core.WantsJSON(f.shared) {
		return core.PrintJSONOrJQ(codex.ReportJSON(groups, kind, pricing, speed), f.shared.JQ, f.shared.NoCost)
	}
	return codex.PrintTable(groups, kind, pricing, speed, f.shared)
}

func codexPricingLog() bool {
	level := core.LogLevel()
	return level == nil || *level != 0
}
