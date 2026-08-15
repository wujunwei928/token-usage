package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/openclaw"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func init() {
	RegisterSpec(9, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 9,
			Agent: "openclaw",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadOpenClawRows(kind, shared)
			},
		}
	})
}

// loadOpenClawRows follows load_summary_agent_rows with the unified pricing:
// detection predates the date filter, which applies to every report kind.
func loadOpenClawRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := openclaw.LoadEntries(shared, nil, unifiedPricing(shared))
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = filterAgentLoadedEntriesByDate(entries, shared)
	summaries := openclaw.SummarizeEntries(entries, mapOpenClawKind(kind))
	return AgentRows{Rows: SummaryRows("openclaw", summaries, false), Detected: detected}, nil
}

func mapOpenClawKind(kind ReportKind) openclaw.ReportKind {
	switch kind {
	case KindSession:
		return openclaw.KindSession
	case KindMonthly:
		return openclaw.KindMonthly
	case KindWeekly:
		return openclaw.KindWeekly
	default:
		return openclaw.KindDaily
	}
}
