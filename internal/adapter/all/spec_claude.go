package all

import (
	"sort"
	"strings"

	"github.com/wujunwei928/token-usage/internal/adapter/claude"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Claude participates in the unified report with a hand-written spec: its
// daily family runs the dedicated daily pipeline and its sessions group by
// (project, session) — the permanent exception of ADR 0009.
func init() {
	RegisterSpec("claude", func(shared *core.SharedArgs) Spec {
		return Spec{
			Agent: "claude",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadClaudeRows(kind, shared)
			},
		}
	})
}

func loadClaudeRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	if kind == KindSession {
		entries, err := claude.LoadEntries(claude.LoadOptions{Shared: shared})
		if err != nil {
			return AgentRows{}, err
		}
		detected := len(entries) > 0
		summaries := summarizeEntrySessions(entries)
		summaries = filterSessionSummaries(summaries, shared)
		return AgentRows{Rows: SummaryRows("claude", summaries, false), Detected: detected}, nil
	}
	summaries, err := claude.LoadDailySummaries(shared, nil, false)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(summaries) > 0
	summaries = filterDailySummariesByDate(summaries, shared)
	return AgentRows{Rows: SummaryRows("claude", summaries, false), Detected: detected}, nil
}

func summarizeEntrySessions(entries []core.LoadedEntry) []core.UsageSummary {
	type key struct{ projectPath, sessionID string }
	groups := map[key]*core.SessionAccumulator{}
	for i := range entries {
		k := key{entries[i].ProjectPath, entries[i].SessionID}
		if _, ok := groups[k]; !ok {
			groups[k] = &core.SessionAccumulator{}
		}
		groups[k].AddEntry(&entries[i])
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].projectPath != keys[j].projectPath {
			return keys[i].projectPath < keys[j].projectPath
		}
		return keys[i].sessionID < keys[j].sessionID
	})
	out := make([]core.UsageSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, groups[k].IntoSummary())
	}
	return out
}

func filterSessionSummaries(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
	if shared.Since == nil && shared.Until == nil {
		return rows
	}
	out := make([]core.UsageSummary, 0, len(rows))
	for i := range rows {
		date := ""
		if rows[i].LastActivity != nil {
			date = strings.ReplaceAll(*rows[i].LastActivity, "-", "")
		}
		if core.DateWithinRange(date, shared.Since, shared.Until) {
			out = append(out, rows[i])
		}
	}
	return out
}

func filterDailySummariesByDate(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
	if shared.Since == nil && shared.Until == nil {
		return rows
	}
	out := make([]core.UsageSummary, 0, len(rows))
	for i := range rows {
		date := ""
		if rows[i].Date != nil {
			date = strings.ReplaceAll(*rows[i].Date, "-", "")
		}
		if core.DateWithinRange(date, shared.Since, shared.Until) {
			out = append(out, rows[i])
		}
	}
	return out
}
