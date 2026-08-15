// Ticket-10 hook: token-usage config application for commands that register the
// shared flags. This is the interim wiring that keeps golden config-* cases
// green; the lead's final wiring calls config.ApplyConfig directly from each
// RunE (see internal/config/apply.go for the documented call).
package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/config"
	"github.com/wujunwei928/token-usage/internal/core"
)

// WireCommandConfig installs a PreRunE hook on a command that registers the
// shared flags. It runs after cobra parsed the flags and before RunE resolves
// them: discovered config values are pushed through the parsed flag set with
// flags.Set, so flag-bound variables observe them exactly like CLI input.
// Flags the CLI set explicitly are never touched (the Changed snapshot is
// taken before any config Set), which yields CLI > config > defaults; later
// config sections override earlier ones, matching the reference.
func WireCommandConfig(cmd *cobra.Command, shared *core.SharedArgs) {
	previous := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if previous != nil {
			if err := previous(c, args); err != nil {
				return err
			}
		}
		agent := ""
		if parent := c.Parent(); parent != nil && parent.Name() != "token-usage" {
			agent = parent.Name()
		}
		if err := config.ApplyToFlags(agent, c.Name(), c.Flags(), shared); err != nil {
			return parseErr("%s", err.Error())
		}
		return nil
	}
}
