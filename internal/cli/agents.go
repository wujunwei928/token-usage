package cli

import (
	"github.com/spf13/cobra"
)

// agentCommandFactories collects agent subcommands registered by adapter
// files; each adapter owns its own agent_<name>.go registration file.
var agentCommandFactories []func() *cobra.Command

// registerAgentCommand adds an agent command factory (called from init()).
func registerAgentCommand(factory func() *cobra.Command) {
	agentCommandFactories = append(agentCommandFactories, factory)
}
