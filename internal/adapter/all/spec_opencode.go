// OpenCode's participation in the unified (all) report. Roster index 2,
// after claude (0) and codex (1).

package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/opencode"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	RegisterSpec(2, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 2,
			Agent: "opencode",
			Load: func(kind ReportKind) (AgentRows, error) {
				loaderShared := *shared
				loaderShared.JSON = true
				rows, err := loadOpenCodeRows(kind, &loaderShared, shared)
				if err != nil {
					return AgentRows{}, err
				}
				// The OpenCode loader narrows to the date window as it
				// reads, so an out-of-range query yields no entries and the
				// usual "entries are non-empty" test would drop OpenCode
				// from the report's detected agents. Ask the source instead.
				rows.Detected = rows.Detected || opencode.HasData()
				return rows, nil
			},
		}
	})
}

func loadOpenCodeRows(kind ReportKind, loaderShared, shared *core.SharedArgs) (AgentRows, error) {
	entries, err := opencode.LoadEntries(loaderShared)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(entries) > 0
	entries = filterLoadedEntriesByDate(entries, shared)
	summaries := opencode.SummarizeEntries(entries, openCodeKind(kind))
	return AgentRows{
		Rows:     SummaryRows("opencode", summaries, false),
		Detected: detected,
	}, nil
}

// openCodeKind maps the unified report kind onto the opencode adapter's.
func openCodeKind(kind ReportKind) opencode.ReportKind {
	switch kind {
	case KindWeekly:
		return opencode.KindWeekly
	case KindMonthly:
		return opencode.KindMonthly
	case KindSession:
		return opencode.KindSession
	default:
		return opencode.KindDaily
	}
}

// filterLoadedEntriesByDate keeps entries whose local date is inside the
// --since/--until window (the authoritative check shared by every report).
func filterLoadedEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	kept := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			kept = append(kept, entries[i])
		}
	}
	return kept
}
