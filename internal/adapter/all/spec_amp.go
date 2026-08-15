package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/amp"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// The Amp adapter joins the unified load at roster index 3 (see all.go).
func init() {
	RegisterSpec(3, newAmpSpec)
}

func newAmpSpec(shared *core.SharedArgs) Spec {
	pricing := nonJsonlAgentSpecPricing(shared)
	return Spec{
		Index: 3,
		Agent: "amp",
		Load: func(kind ReportKind) (AgentRows, error) {
			return loadAmpRows(kind, shared, pricing)
		},
	}
}

// loadAmpRows mirrors load_priced_summary_agent_rows for amp: load, detect on
// the unfiltered entries, filter to the date window, summarize, convert.
func loadAmpRows(kind ReportKind, shared *core.SharedArgs, pricing *core.PricingMap) (AgentRows, error) {
	entries, err := amp.LoadEntries(shared, pricing)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = nonJsonlAgentSpecFilterEntries(entries, shared)
	summaries := amp.SummarizeEntries(entries, ampReportKind(kind))
	return AgentRows{Rows: SummaryRows("amp", summaries, false), Detected: detected}, nil
}

func ampReportKind(kind ReportKind) amp.ReportKind {
	switch kind {
	case KindWeekly:
		return amp.KindWeekly
	case KindMonthly:
		return amp.KindMonthly
	case KindSession:
		return amp.KindSession
	default:
		return amp.KindDaily
	}
}

// nonJsonlAgentSpecPricing mirrors the all-report loader's load_pricing: the
// raw --offline flag (no --no-offline fold) and the LOG_LEVEL spinner gate.
func nonJsonlAgentSpecPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.Offline, refreshLog, shared.PricingOverrides)
}

// nonJsonlAgentSpecFilterEntries applies the since/until window to loaded
// entries (filter_loaded_entries_by_date in the reference).
func nonJsonlAgentSpecFilterEntries(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	filtered := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			filtered = append(filtered, entries[i])
		}
	}
	return filtered
}
