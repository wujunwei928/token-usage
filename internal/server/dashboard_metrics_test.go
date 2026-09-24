package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Ticket 01/02/03/04 data-layer seams: the dashboard aggregates cache metrics
// under the B definition (read / (read+write+input), the claude-usage-dashboard
// and vLLM convention — see .scratch/web-dashboard-cache-metrics/spec.md).

func TestDashboardCacheHitRateTodayOnly(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "hit")
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	postReport(t, srv, token, samplePayload("h1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5",
			Input: 1000, Output: 100, CacheRead: 8900, Cache5m: 100},
	))
	postReport(t, srv, token, samplePayload("h2", yesterday,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5",
			Input: 500, Output: 50, CacheRead: 4500, Cache5m: 500},
	))

	user, _ := store.UserByName(context.Background(), "hit")
	data := store.Dashboard(context.Background(), user, pricing)

	// Today: 8900 / (8900 + 100 + 1000) = 0.89 — yesterday must not leak in.
	if !data.HasRate || diff(data.CacheHitRate, 0.89) {
		t.Fatalf("today hit rate = %v (hasRate %v), want 0.89", data.CacheHitRate, data.HasRate)
	}
	// Three-way split sums to 1 with the right proportions.
	if diff(data.CacheSplit.Read, 0.89) || diff(data.CacheSplit.Write, 0.01) || diff(data.CacheSplit.Fresh, 0.10) {
		t.Fatalf("split = %+v, want {0.89 0.01 0.10}", data.CacheSplit)
	}
	// Daily points carry their own rate: yesterday 4500/5500 ≈ 0.8182.
	var yest *DayPoint
	for i := range data.Daily30 {
		if data.Daily30[i].Date == yesterday {
			yest = &data.Daily30[i]
		}
	}
	if yest == nil || !yest.HasRate || diff(yest.HitRate, 4500.0/5500.0) {
		t.Fatalf("yesterday daily rate wrong: %+v", yest)
	}
	// Payback: 8900 reads / 100 writes = 89.
	if diff(data.ReadPerWrite, 89) {
		t.Fatalf("read-per-write = %v, want 89", data.ReadPerWrite)
	}
}

func TestDashboardCacheHitRateNoData(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "empty")
	postReport(t, srv, token, samplePayload("e1", time.Now().Format("2006-01-02"),
		HourRow{Hour: 9, Tool: "codex", Model: "gpt-5", Input: 300, Output: 200},
	))
	user, _ := store.UserByName(context.Background(), "empty")
	data := store.Dashboard(context.Background(), user, pricing)
	// Codex-style rows: no cache at all → rate 0 but defined (fresh input > 0).
	if !data.HasRate || data.CacheHitRate != 0 {
		t.Fatalf("no-cache day rate = %v hasRate %v, want 0/true", data.CacheHitRate, data.HasRate)
	}
	if diff(data.CacheSplit.Fresh, 1) {
		t.Fatalf("split %+v, want fresh=1", data.CacheSplit)
	}
	if data.ReadPerWrite != 0 {
		t.Fatalf("read-per-write = %v, want 0 (no writes)", data.ReadPerWrite)
	}
}

func TestDashboardToolModelRows(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "cross")
	today := time.Now().Format("2006-01-02")
	postReport(t, srv, token, samplePayload("c1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5",
			Input: 1000, Output: 100, CacheRead: 8900, Cache5m: 100},
		HourRow{Hour: 10, Tool: "codex", Model: "gpt-5", Input: 300, Output: 200},
		// Pure-output row: no input-side tokens at all → no definable rate.
		HourRow{Hour: 11, Tool: "amp", Model: "amp-1", Output: 10},
	))
	user, _ := store.UserByName(context.Background(), "cross")
	data := store.Dashboard(context.Background(), user, pricing)

	if len(data.ToolModels) != 3 {
		t.Fatalf("tool×model rows = %d, want 3: %+v", len(data.ToolModels), data.ToolModels)
	}
	// Sorted by tokens desc: claude 10100 > codex 500.
	first := data.ToolModels[0]
	if first.Tool != "claude" || first.Model != "claude-sonnet-4-5" {
		t.Fatalf("top row = %+v, want claude/claude-sonnet-4-5", first)
	}
	if !first.HasRate || diff(first.HitRate, 0.89) || first.Cost <= 0 {
		t.Fatalf("claude row rate/cost wrong: %+v", first)
	}
	second := data.ToolModels[1]
	if second.Tool != "codex" || !second.HasRate || second.HitRate != 0 || second.Cost <= 0 {
		t.Fatalf("codex row wrong: %+v", second)
	}
	third := data.ToolModels[2]
	if third.Tool != "amp" || third.HasRate || third.HitRate != 0 {
		t.Fatalf("pure-output row must have HasRate=false: %+v", third)
	}
	// ByTool/ByModel now carry rate+cost under NameStat.
	if len(data.ByTool) != 3 || data.ByTool[0].Name != "claude" || !data.ByTool[0].HasRate ||
		diff(data.ByTool[0].HitRate, 0.89) || data.ByTool[0].Cost <= 0 {
		t.Fatalf("ByTool wrong: %+v", data.ByTool)
	}
	if len(data.ByModel) != 3 || data.ByModel[0].Name != "claude-sonnet-4-5" {
		t.Fatalf("ByModel wrong: %+v", data.ByModel)
	}
}

func TestCacheSavingsMath(t *testing.T) {
	table, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	model := "claude-sonnet-4-5"
	card, ok := table.Card(model)
	if !ok {
		t.Fatalf("seed card missing for %s", model)
	}
	row := HourRow{Tool: "claude", Model: model, Input: 1_000_000,
		CacheRead: 2_000_000, Cache5m: 1_000_000, Cache1h: 500_000}
	want := (float64(row.CacheRead)*(card.Input-card.CacheRead) -
		float64(row.Cache5m)*(card.CacheWrite5m-card.Input) -
		float64(row.Cache1h)*(card.CacheWrite1h-card.Input)) / 1e6
	got := table.CacheSavingsForHourRow(&row)
	if d := got - want; d > 0.01 || d < -0.01 {
		t.Fatalf("savings: got %v want %v (card %+v)", got, want, card)
	}
	// Unknown model: no card, no estimate.
	if got := table.CacheSavingsForHourRow(&HourRow{Model: "nope", Input: 100, CacheRead: 100}); got != 0 {
		t.Fatalf("unknown model savings = %v, want 0", got)
	}
}

func TestDashboardCacheSavingsTodayOnly(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "save")
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	postReport(t, srv, token, samplePayload("s1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5",
			Input: 1000, Output: 100, CacheRead: 8900, Cache5m: 100},
	))
	postReport(t, srv, token, samplePayload("s2", yesterday,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5",
			Input: 100, Output: 10, CacheRead: 100000, Cache5m: 1000},
	))
	user, _ := store.UserByName(context.Background(), "save")
	data := store.Dashboard(context.Background(), user, pricing)

	card, _ := pricing.Card("claude-sonnet-4-5")
	want := (8900*(card.Input-card.CacheRead) - 100*(card.CacheWrite5m-card.Input)) / 1e6
	if d := data.CacheSavings - want; d > 1e-9 || d < -1e-9 {
		t.Fatalf("today savings = %v, want %v (yesterday must not leak)", data.CacheSavings, want)
	}
}

func TestDashboardHeatmap(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "heat")
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	postReport(t, srv, token, samplePayload("t1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "m", Input: 100, Output: 50},
	))
	postReport(t, srv, token, samplePayload("t2", yesterday,
		HourRow{Hour: 10, Tool: "claude", Model: "m", Input: 70, Output: 30},
	))
	user, _ := store.UserByName(context.Background(), "heat")
	data := store.Dashboard(context.Background(), user, pricing)

	if len(data.Heatmap) != 7*24 {
		t.Fatalf("heatmap cells = %d, want 168", len(data.Heatmap))
	}
	wdToday := int(time.Now().Weekday()+6) % 7
	wdYest := int(time.Now().AddDate(0, 0, -1).Weekday()+6) % 7
	find := func(wd, hour int) uint64 {
		for _, c := range data.Heatmap {
			if c.Weekday == wd && c.Hour == hour {
				return c.Tokens
			}
		}
		return 0
	}
	if got := find(wdToday, 9); got != 150 {
		t.Fatalf("today 9:00 cell = %d, want 150", got)
	}
	if got := find(wdYest, 10); got != 100 {
		t.Fatalf("yesterday 10:00 cell = %d, want 100", got)
	}
	if got := find(wdToday, 10); got != 0 {
		t.Fatalf("empty cell leaked: %d", got)
	}
}

// Ticket 05: Chinese number units (万/亿) replace the western K/M/B across the
// web UI — FormatTokens and its JS twin in app.js.
func TestFormatTokensCNUnits(t *testing.T) {
	cases := []struct {
		v    uint64
		want string
	}{
		{0, "0"},
		{42, "42"},
		{9999, "9999"},
		{10_000, "1.0万"},
		{55_500, "5.5万"},
		{7_100, "7100"}, // e2e fixture: stays raw under 1万
		{12_345_678, "1234.6万"},
		{123_456_789, "1.23亿"},
		{9_999_999_999, "100.00亿"},
	}
	for _, tc := range cases {
		if got := FormatTokens(tc.v); got != tc.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

// diff reports whether a and b differ by more than 1e-9 (float helper).
func diff(a, b float64) bool {
	d := a - b
	return d > 1e-9 || d < -1e-9
}

// Ticket 01/02/03/04 HTML surface: labels, the split meter, the breakdown
// table and the heatmap box all render on /me.
func TestDashboardMetricsHTML(t *testing.T) {
	_, mux := newLocalWeb(t)
	body := getFrom(t, mux, "127.0.0.1:53812", "/me").Body.String()
	for _, want := range []string{
		"当日缓存命中率",
		"当日缓存净节省",
		"工具 × 模型",
		"meter-split",
		"chart-heatmap",
		"工具 × 模型明细",
		"命中率",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}
