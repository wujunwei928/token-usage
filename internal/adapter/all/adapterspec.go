package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// SpecOptions declares an adapter's unified-report participation beyond the
// shared pipeline: its report profile and whether session rows carry the
// project-path metadata (pi does, like the reference's load_pi_format_rows).
type SpecOptions struct {
	Profile            common.ReportProfile
	IncludeProjectPath bool
}

// AdapterSpec builds a unified-report Spec from an adapter registered in the
// common registry: load → detect → shared pipeline → summary rows. The
// per-agent spec pipelines collapse into this one wrapper (ADR 0009). The
// adapter is built once per spec construction — its factory (including any
// pricing load) runs once per run, not once per report kind.
func AdapterSpec(agent string, opts SpecOptions) func(shared *core.SharedArgs) Spec {
	return func(shared *core.SharedArgs) Spec {
		adapter, ok := common.BuildAdapter(agent, shared)
		if !ok {
			return Spec{Agent: agent, Load: notImplemented}
		}
		return Spec{
			Agent: agent,
			Load: func(kind ReportKind) (AgentRows, error) {
				result, err := adapter.LoadEntries(common.LoadRequest{Shared: shared})
				if err != nil {
					return AgentRows{}, err
				}
				summaries := common.ReportRows(result.Entries, kind, shared, opts.Profile)
				return AgentRows{
					Rows:     SummaryRows(agent, summaries, opts.IncludeProjectPath),
					Detected: result.Detected,
				}, nil
			},
		}
	}
}
