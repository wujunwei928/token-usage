package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/gemini"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	RegisterSpec(12, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 12,
			Agent: "gemini",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadGeminiRows(kind, shared)
			},
		}
	})
}

// loadGeminiRows follows load_priced_summary_agent_rows: entries load with the
// unified pricing, detection predates the date filter, and rows summarize
// after filtering.
func loadGeminiRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := gemini.LoadEntries(shared, unifiedPricing(shared))
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = filterAgentLoadedEntriesByDate(entries, shared)
	summaries := gemini.SummarizeEntries(entries, mapGeminiKind(kind))
	return AgentRows{Rows: SummaryRows("gemini", summaries, false), Detected: detected}, nil
}

func mapGeminiKind(kind ReportKind) gemini.ReportKind {
	switch kind {
	case KindSession:
		return gemini.KindSession
	case KindMonthly:
		return gemini.KindMonthly
	case KindWeekly:
		return gemini.KindWeekly
	default:
		return gemini.KindDaily
	}
}

// unifiedPricing mirrors the unified loader's pricing: always a full map,
// never suppressed by display mode.
func unifiedPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.Offline, refreshLog, shared.PricingOverrides)
}

// filterAgentLoadedEntriesByDate applies the since/until window to loaded
// entries (the unified loaders filter before summarizing).
func filterAgentLoadedEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	out := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			out = append(out, entries[i])
		}
	}
	return out
}
