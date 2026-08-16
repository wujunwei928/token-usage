package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/codex"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newCodexCommand)
}

// codex keeps its Groups pipeline (ADR 0009's permanent exception): the run
// closure loads Groups, not entries; --speed (bare = auto, the reference's
// NoOptDefVal) reaches it as an extra option.
const codexSpeedFlagHelp = "Cost speed tier: auto uses recorded settings, then Codex config.toml; use standard or fast to override (default: auto, choices: auto | standard | fast)"

func newCodexCommand() *cobra.Command {
	return newAgentCommandTree(&agentCommandSpec{
		agent:    "codex",
		display:  "Codex",
		short:    "Usage reports for codex.",
		subShort: func(kind core.ReportKind) string { return shortTokenUsageGrouped("Codex", kind) },
		extraOptions: []agentExtraOption{
			{
				long:        "--speed",
				bareDefault: "auto",
				help:        codexSpeedFlagHelp,
				helpSubs:    true,
				store:       func(st *agentFlagState, value string) { st.set("--speed", value) },
			},
		},
		run: func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error {
			speedRaw := st.get("--speed")
			if speedRaw == "" {
				speedRaw = "auto"
			}
			speedChoice, ok := codex.ParseSpeed(speedRaw)
			if !ok {
				return parseErr("Invalid speed option '%s'", speedRaw)
			}
			pricing := agentPricing(f.shared)
			groups, err := codex.LoadGroups(f.shared, kind)
			if err != nil {
				return err
			}
			speed := codex.ResolveSpeed(speedChoice)
			if core.WantsJSON(f.shared) {
				return core.PrintJSONOrJQ(codex.ReportJSON(groups, kind, pricing, speed), f.shared.JQ, f.shared.NoCost)
			}
			return codex.PrintTable(groups, kind, pricing, speed, f.shared)
		},
	})
}
