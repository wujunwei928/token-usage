package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/kimi"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func init() {
	RegisterSpec(13, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 13,
			Agent: "kimi",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadKimiRows(kind, shared)
			},
		}
	})
}

// loadKimiRows follows load_priced_summary_agent_rows: entries load with the
// unified pricing, detection predates the date filter, and rows summarize
// after filtering.
func loadKimiRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := kimi.LoadEntries(shared, unifiedPricing(shared))
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = filterAgentLoadedEntriesByDate(entries, shared)
	summaries := kimi.SummarizeEntries(entries, mapKimiKind(kind))
	return AgentRows{Rows: SummaryRows("kimi", summaries, false), Detected: detected}, nil
}

func mapKimiKind(kind ReportKind) kimi.ReportKind {
	switch kind {
	case KindSession:
		return kimi.KindSession
	case KindMonthly:
		return kimi.KindMonthly
	case KindWeekly:
		return kimi.KindWeekly
	default:
		return kimi.KindDaily
	}
}
