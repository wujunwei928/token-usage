package all

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

func agentRow(period, agent string, input uint64) Row {
	return Row{Period: period, Agent: agent, InputTokens: input, TotalTokens: input,
		MetadataAgents: []string{agent}}
}

// Weekly aggregation buckets periods on Monday starts — the unified report's
// fixed convention, independent of any agent profile.
func TestAggregateRowsWeeklyMonday(t *testing.T) {
	rows := []Row{
		agentRow("2026-08-14", "a", 1), // Friday
		agentRow("2026-08-15", "a", 2), // Saturday
		agentRow("2026-08-16", "b", 4), // Sunday
		agentRow("2026-08-17", "b", 8), // Monday
	}
	out := AggregateRows(rows, KindWeekly)
	want := []struct {
		period string
		input  uint64
	}{{"2026-08-10", 7}, {"2026-08-17", 8}}
	if len(out) != 2 {
		t.Fatalf("weekly buckets = %d, want 2", len(out))
	}
	for i, w := range want {
		if out[i].Period != w.period || out[i].InputTokens != w.input {
			t.Errorf("bucket %d = %s/%d, want %s/%d", i, out[i].Period, out[i].InputTokens, w.period, w.input)
		}
	}
}

func TestAggregateRowsMonthlyPrefix(t *testing.T) {
	rows := []Row{
		agentRow("2026-07-31", "a", 1),
		agentRow("2026-08-01", "a", 2),
	}
	out := AggregateRows(rows, KindMonthly)
	if len(out) != 2 || out[0].Period != "2026-07" || out[1].Period != "2026-08" {
		t.Fatalf("monthly buckets = %+v", out)
	}
}

// SummaryRows drops zero-token rows and keeps metadata agents for the merge.
func TestSummaryRowsDropsZeroTokens(t *testing.T) {
	date := "2026-08-14"
	session := "s1"
	summaries := []core.UsageSummary{
		{Date: &date, InputTokens: 10},
		{Date: &date, InputTokens: 0},
		{SessionID: &session},
	}
	rows := SummaryRows("a", summaries, false)
	if len(rows) != 1 || rows[0].InputTokens != 10 {
		t.Fatalf("rows = %+v, want only the non-zero row", rows)
	}
	if rows[0].MetadataAgents[0] != "a" {
		t.Errorf("metadata agents = %v", rows[0].MetadataAgents)
	}
}

// SortRows orders by period then agent; descending reverses the whole slice.
func TestSortRowsOrder(t *testing.T) {
	rows := []Row{
		agentRow("2026-08-15", "b", 1),
		agentRow("2026-08-14", "z", 1),
		agentRow("2026-08-15", "a", 1),
	}
	SortRows(rows, core.OrderAsc)
	if rows[0].Period != "2026-08-14" || rows[1].Agent != "a" || rows[2].Agent != "b" {
		t.Fatalf("asc order = %+v", rows)
	}
	SortRows(rows, core.OrderDesc)
	if rows[0].Agent != "b" || rows[2].Period != "2026-08-14" {
		t.Fatalf("desc order = %+v", rows)
	}
}
