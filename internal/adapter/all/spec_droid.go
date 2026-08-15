package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/droid"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Droid participates in the unified report at roster index 4.
func init() {
	RegisterSpec(4, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 4,
			Agent: "droid",
			Load: func(kind ReportKind) (AgentRows, error) {
				entries, err := droid.LoadEntries(shared)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = droid.FilterLoadedEntriesByDate(entries, shared)
				summaries := droid.SummarizeEntries(entries, toDroidKind(kind))
				return AgentRows{
					Rows:     SummaryRows("droid", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toDroidKind(kind ReportKind) droid.ReportKind {
	switch kind {
	case KindWeekly:
		return droid.KindWeekly
	case KindMonthly:
		return droid.KindMonthly
	case KindSession:
		return droid.KindSession
	default:
		return droid.KindDaily
	}
}
