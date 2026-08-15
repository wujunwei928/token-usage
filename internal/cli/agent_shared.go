package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/core"
)

// simpleAgentKind is the report granularity shared by the simple agent
// commands (daily/monthly/session).
type simpleAgentKind int

const (
	simpleAgentDaily simpleAgentKind = iota
	simpleAgentMonthly
	simpleAgentSession
)

// simpleAgentConfig describes one agent's command tree.
type simpleAgentConfig struct {
	Use         string // "droid"
	Display     string // "Droid"
	Short       string // "Show Droid usage commands"
	About       string // "Usage reports for droid."
	RunDaily    func(f *sharedFlags) error
	RunMonthly  func(f *sharedFlags) error
	RunSession  func(f *sharedFlags) error
}

// newSimpleAgentCommand builds the `<agent>` command with its daily/monthly/
// session subcommands; a bare `<agent>` runs the daily report.
func newSimpleAgentCommand(cfg simpleAgentConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   cfg.Use,
		Short: cfg.Short,
		Long:  cfg.About,
	}
	newReportCommand := func(kind simpleAgentKind, use, short string, run func(f *sharedFlags) error) *cobra.Command {
		sub := &cobra.Command{Use: use, Short: short}
		f := registerSharedFlags(sub)
		sub.RunE = func(command *cobra.Command, args []string) error {
			if len(args) > 0 {
				return parseErr("Expected option, got '%s'", args[0])
			}
			if err := f.resolve(); err != nil {
				return err
			}
			switch kind {
			case simpleAgentSession:
				if err := f.validateLast(false); err != nil {
					return err
				}
			default:
				if err := f.validateLast(true); err != nil {
					return err
				}
				unit := core.PeriodDay
				if kind == simpleAgentMonthly {
					unit = core.PeriodMonth
				}
				f.resolveLastSince(unit, core.Monday)
			}
			return run(f)
		}
		return sub
	}
	daily := newReportCommand(simpleAgentDaily, "daily",
		"Show "+cfg.Display+" usage grouped by date", cfg.RunDaily)
	monthly := newReportCommand(simpleAgentMonthly, "monthly",
		"Show "+cfg.Display+" usage grouped by month", cfg.RunMonthly)
	session := newReportCommand(simpleAgentSession, "session",
		"Show "+cfg.Display+" usage grouped by session", cfg.RunSession)

	// The bare `<agent>` form accepts shared flags directly and runs daily.
	parentFlags := registerSharedFlags(cmd)
	cmd.RunE = func(command *cobra.Command, args []string) error {
		if len(args) > 0 {
			if strings.HasPrefix(args[0], "-") {
				return unknownAgentCommandError()
			}
			// An unsupported report name is reported like the reference parser.
			return unsupportedAgentReportError(cfg.Use, cfg.Display, args[0])
		}
		if err := parentFlags.resolve(); err != nil {
			return err
		}
		if err := parentFlags.validateLast(true); err != nil {
			return err
		}
		parentFlags.resolveLastSince(core.PeriodDay, core.Monday)
		return cfg.RunDaily(parentFlags)
	}
	cmd.AddCommand(daily, monthly, session)
	return cmd
}

// unsupportedAgentReportError mirrors the reference parser's message for a
// report name the agent does not offer.
func unsupportedAgentReportError(use, display, report string) error {
	if report == "blocks" || report == "statusline" {
		return parseErr("The %q report is only available for Claude Code usage.\nUse \"token-usage %s daily\" for %s usage reports.", report, use, display)
	}
	return parseErr("The %q report is not available for %s usage.\nUse \"token-usage %s daily\" for %s usage reports.", report, display, use, display)
}

// unknownAgentCommandError reproduces the reference's catch-all parse error.
func unknownAgentCommandError() error {
	return parseErr("Unknown command '%s'", strings.Join(os.Args[1:], " "))
}
