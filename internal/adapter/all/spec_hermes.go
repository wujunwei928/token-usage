package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/hermes"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Hermes participates in the unified report at roster index 6.
func init() {
	RegisterSpec(6, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 6,
			Agent: "hermes",
			Load: func(kind ReportKind) (AgentRows, error) {
				entries, err := hermes.LoadEntries(shared)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = hermes.FilterLoadedEntriesByDate(entries, shared)
				summaries := hermes.SummarizeEntries(entries, toHermesKind(kind))
				return AgentRows{
					Rows:     SummaryRows("hermes", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toHermesKind(kind ReportKind) hermes.ReportKind {
	switch kind {
	case KindWeekly:
		return hermes.KindWeekly
	case KindMonthly:
		return hermes.KindMonthly
	case KindSession:
		return hermes.KindSession
	default:
		return hermes.KindDaily
	}
}
