package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/qwen"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	RegisterSpec(14, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 14,
			Agent: "qwen",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadQwenRows(kind, shared)
			},
		}
	})
}

// loadQwenRows follows load_qwen_rows: detection also honors has_data so an
// empty date window still reports Qwen as detected; session rows summarize
// unfiltered and then filter by last activity.
func loadQwenRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := qwen.LoadEntries(shared)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0 || qwen.HasData()
	if kind == KindSession {
		summaries := qwen.SummarizeEntries(entries, qwen.KindSession)
		summaries = filterSessionSummaries(summaries, shared)
		return AgentRows{Rows: SummaryRows("qwen", summaries, false), Detected: detected}, nil
	}
	entries = filterAgentLoadedEntriesByDate(entries, shared)
	summaries := qwen.SummarizeEntries(entries, mapQwenKind(kind))
	return AgentRows{Rows: SummaryRows("qwen", summaries, false), Detected: detected}, nil
}

func mapQwenKind(kind ReportKind) qwen.ReportKind {
	switch kind {
	case KindMonthly:
		return qwen.KindMonthly
	case KindWeekly:
		return qwen.KindWeekly
	default:
		return qwen.KindDaily
	}
}
