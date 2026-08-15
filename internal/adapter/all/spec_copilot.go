package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/copilot"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// The GitHub Copilot CLI adapter joins the unified load at roster index 11
// (see all.go).
func init() {
	RegisterSpec(11, newCopilotSpec)
}

func newCopilotSpec(shared *core.SharedArgs) Spec {
	pricing := nonJsonlAgentSpecPricing(shared)
	return Spec{
		Index: 11,
		Agent: "copilot",
		Load: func(kind ReportKind) (AgentRows, error) {
			return loadCopilotRows(kind, shared, pricing)
		},
	}
}

// loadCopilotRows mirrors load_priced_summary_agent_rows for copilot.
func loadCopilotRows(kind ReportKind, shared *core.SharedArgs, pricing *core.PricingMap) (AgentRows, error) {
	entries, err := copilot.LoadEntries(shared, pricing)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = nonJsonlAgentSpecFilterEntries(entries, shared)
	summaries := copilot.SummarizeEntries(entries, copilotReportKind(kind))
	return AgentRows{Rows: SummaryRows("copilot", summaries, false), Detected: detected}, nil
}

func copilotReportKind(kind ReportKind) copilot.ReportKind {
	switch kind {
	case KindWeekly:
		return copilot.KindWeekly
	case KindMonthly:
		return copilot.KindMonthly
	case KindSession:
		return copilot.KindSession
	default:
		return copilot.KindDaily
	}
}
