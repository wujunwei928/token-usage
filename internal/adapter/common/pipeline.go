package common

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportProfile declares the per-agent report behaviors the shared pipeline
// consumes (ADR 0009): the differences that used to live in sixteen copies of
// SummarizeEntries. The zero value is the plain profile — Sunday weeks,
// key-swap sessions, filter-before-summarize.
type ReportProfile struct {
	// WeekStart is the weekly bucket start day. Zero value means Sunday.
	WeekStart core.WeekDay

	// SessionByActivity: session rows carry activity bounds via the session
	// accumulator. False groups by session key and swaps the date slot to the
	// session id (no activity metadata).
	SessionByActivity bool

	// SessionFilterAfter: session rows filter by last activity AFTER
	// summarizing, so a session whose activity reaches into the window
	// survives whole. False filters entries by date before summarizing.
	SessionFilterAfter bool
}

// SummarizeReport groups entries into report rows for the kind, applying the
// profile's knobs. One implementation replaces the per-adapter copies.
func SummarizeReport(entries []core.LoadedEntry, kind core.ReportKind, profile ReportProfile) []core.UsageSummary {
	switch kind {
	case core.KindDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(key string) (string, *string) { return key, nil })
	case core.KindMonthly:
		daily := SummarizeReport(entries, core.KindDaily, profile)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, profile.WeekStart)
	case core.KindSession:
		if profile.SessionByActivity {
			return summarizeSessionsByActivity(entries)
		}
		return summarizeSessionsByKeySwap(entries)
	default: // KindWeekly
		daily := SummarizeReport(entries, core.KindDaily, profile)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, profile.WeekStart)
	}
}

// summarizeSessionsByActivity groups by session key and emits rows in sorted
// key order. HEAD's opencode emitted insertion order instead; every current
// consumer re-sorts rows by period before display, so the two agree on
// output — but anyone consuming SummarizeReport directly gets key order
// (qwen/zcode/claude semantics), not opencode's historical insertion order.
func summarizeSessionsByActivity(entries []core.LoadedEntry) []core.UsageSummary {
	groups := map[string]*core.SessionAccumulator{}
	var keys []string
	for i := range entries {
		key := entries[i].SessionID
		if _, ok := groups[key]; !ok {
			groups[key] = &core.SessionAccumulator{}
			keys = append(keys, key)
		}
		groups[key].AddEntry(&entries[i])
	}
	sort.Strings(keys)
	rows := make([]core.UsageSummary, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, groups[key].IntoSummary())
	}
	return rows
}

func summarizeSessionsByKeySwap(entries []core.LoadedEntry) []core.UsageSummary {
	rows := core.SummarizeByKey(entries,
		func(e *core.LoadedEntry) string { return e.SessionID },
		func(key string) (string, *string) { return key, nil })
	for i := range rows {
		if rows[i].Date != nil {
			sessionID := *rows[i].Date
			rows[i].SessionID = &sessionID
			rows[i].Date = nil
		}
	}
	return rows
}

// ReportRows runs the full per-agent pipeline: the shared date window in the
// profile's order (sessions with SessionFilterAfter summarize first, then
// filter rows by last activity), then summarize for the kind.
func ReportRows(entries []core.LoadedEntry, kind core.ReportKind, shared *core.SharedArgs, profile ReportProfile) []core.UsageSummary {
	if kind == core.KindSession && profile.SessionFilterAfter {
		rows := SummarizeReport(entries, core.KindSession, profile)
		return filterRowsByLastActivity(rows, shared)
	}
	entries = FilterLoadedEntriesByDate(entries, shared)
	return SummarizeReport(entries, kind, profile)
}

func filterRowsByLastActivity(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
	if shared.Since == nil && shared.Until == nil {
		return rows
	}
	out := make([]core.UsageSummary, 0, len(rows))
	for i := range rows {
		date := ""
		if rows[i].LastActivity != nil {
			date = *rows[i].LastActivity
		}
		if core.DateWithinRange(date, shared.Since, shared.Until) {
			out = append(out, rows[i])
		}
	}
	return out
}
