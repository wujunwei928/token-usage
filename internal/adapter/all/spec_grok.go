package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/grok"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// Grok participates in the unified report at roster index 15. It is detected
// whenever a session tree exists, even when no rows survive the window.
// grok landed upstream after 20.0.19; see internal/cli/agent_grok.go.
func disabledInit() {
	RegisterSpec(15, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 15,
			Agent: "grok",
			Load: func(kind ReportKind) (AgentRows, error) {
				entries, err := grok.LoadEntries(shared)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0 || grok.HasData()
				entries = grok.FilterLoadedEntriesByDate(entries, shared)
				summaries := grok.SummarizeEntries(entries, toGrokKind(kind))
				return AgentRows{
					Rows:     SummaryRows("grok", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toGrokKind(kind ReportKind) grok.ReportKind {
	switch kind {
	case KindWeekly:
		return grok.KindWeekly
	case KindMonthly:
		return grok.KindMonthly
	case KindSession:
		return grok.KindSession
	default:
		return grok.KindDaily
	}
}
