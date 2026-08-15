// Package cli defines the cobra command tree. CLI surface is behaviorally
// identical to the reference Rust implementation (ADR-0002).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/adapter/all"
)

// Version is the baseline reference version this port tracks; overridable at
// link time for release builds.
var Version = "20.0.19"

// NewRootCommand builds the full command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "ccusage",
		Short:         "ccusage - Analyze Claude Code and AI agent usage from local JSONL logs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Register the unified-report flags so bare `ccusage` behaves like
	// `ccusage daily`.
	rootFlags := registerAllFlags(root)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return runAllReport(all.KindDaily, rootFlags)
	}

	// Version flags: reference accepts -v, -V and --version at every level.
	// Cobra's built-in version machinery only binds -v, so we handle both
	// shorthands ourselves as persistent flags.
	root.PersistentFlags().BoolP("version", "v", false, "Display this version")
	root.PersistentFlags().BoolP("version-cap", "V", false, "Display this version")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		v1, _ := cmd.Flags().GetBool("version")
		v2, _ := cmd.Flags().GetBool("version-cap")
		if v1 || v2 {
			fmt.Fprintf(cmd.OutOrStdout(), "ccusage %s\n", Version)
			// Neutralize the command so only the version line is printed.
			cmd.RunE = func(*cobra.Command, []string) error { return nil }
		}
		return nil
	}

	root.AddCommand(newClaudeCommand())
	for _, factory := range agentCommandFactories {
		root.AddCommand(factory())
	}
	root.AddCommand(
		newAllReportCommand(all.KindDaily),
		newAllReportCommand(all.KindMonthly),
		newAllReportCommand(all.KindWeekly),
		newAllReportCommand(all.KindSession),
		newClaudeBlocksCommand(),
		newClaudeStatuslineCommand(),
	)
	return root
}

// Execute runs the CLI and returns the terminal error (already printed).
func Execute() error {
	rewriteOSArgs()
	raw := os.Args[1:]
	if err := reportFlagAliasError(raw); err != nil {
		return err
	}
	if err := agentFilterOptionError(raw); err != nil {
		return err
	}
	root := NewRootCommand()
	err := root.Execute()
	if err != nil {
		if reformatted := reformatCobraFlagError(root, raw, err); reformatted != nil {
			return reformatted
		}
	}
	return err
}
