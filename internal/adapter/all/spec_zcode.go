package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/zcode"
	"github.com/wujunwei928/token-usage/internal/core"
)

// zcode participates in the unified report at roster index 16, appended after
// the reference roster: an adapter beyond upstream ccusage (ADR 0006).
func init() {
	RegisterSpec(16, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 16,
			Agent: "zcode",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadZcodeRows(kind, shared)
			},
		}
	})
}

// loadZcodeRows mirrors loadQwenRows: detection also honors HasData so an
// empty date window still reports zcode as detected; session rows summarize
// unfiltered and then filter by last activity.
func loadZcodeRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := zcode.LoadEntries(shared)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0 || zcode.HasData()
	if kind == KindSession {
		summaries := zcode.SummarizeEntries(entries, zcode.KindSession)
		summaries = filterSessionSummaries(summaries, shared)
		return AgentRows{Rows: SummaryRows("zcode", summaries, false), Detected: detected}, nil
	}
	entries = filterAgentLoadedEntriesByDate(entries, shared)
	summaries := zcode.SummarizeEntries(entries, mapZcodeKind(kind))
	return AgentRows{Rows: SummaryRows("zcode", summaries, false), Detected: detected}, nil
}

func mapZcodeKind(kind ReportKind) zcode.ReportKind {
	switch kind {
	case KindMonthly:
		return zcode.KindMonthly
	case KindWeekly:
		return zcode.KindWeekly
	default:
		return zcode.KindDaily
	}
}
