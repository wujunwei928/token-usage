package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/codebuff"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Codebuff participates in the unified report at roster index 5.
func init() {
	RegisterSpec(5, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 5,
			Agent: "codebuff",
			Load: func(kind ReportKind) (AgentRows, error) {
				entries, err := codebuff.LoadEntries(shared)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = codebuff.FilterLoadedEntriesByDate(entries, shared)
				summaries := codebuff.SummarizeEntries(entries, toCodebuffKind(kind))
				return AgentRows{
					Rows:     SummaryRows("codebuff", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toCodebuffKind(kind ReportKind) codebuff.ReportKind {
	switch kind {
	case KindWeekly:
		return codebuff.KindWeekly
	case KindMonthly:
		return codebuff.KindMonthly
	case KindSession:
		return codebuff.KindSession
	default:
		return codebuff.KindDaily
	}
}
