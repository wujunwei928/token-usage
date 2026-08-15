package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestServer spins the full stack (API + web pages) over a temp SQLite
// file — the same handler tree the binary serves.
func newTestServer(t *testing.T) (*Store, *PricingTable, *httptest.Server) {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	pricing, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewAPI(store, nil).Register(mux)
	web, err := NewWeb(store, pricing)
	if err != nil {
		t.Fatal(err)
	}
	web.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return store, pricing, srv
}

func seedUser(t *testing.T, store *Store, name string) (userID int64, token string) {
	t.Helper()
	token, err := SeedUser(context.Background(), store, name, "", "北京")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.UserByName(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return user.ID, token
}

func postReport(t *testing.T, srv *httptest.Server, token string, payload any) (*http.Response, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp, out
}

func samplePayload(device, date string, rows ...HourRow) map[string]any {
	return map[string]any{
		"deviceId": device, "deviceLabel": "test-box",
		"date": date, "timezone": "Asia/Shanghai",
		"generatedAt": "2026-08-15T21:00:00+08:00",
		"hours":       rows,
	}
}

func TestReportLatestWinsAndAudit(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, token := seedUser(t, store, "alice")
	date := time.Now().Format("2006-01-02")

	resp, out := postReport(t, srv, token, samplePayload("dev1", date,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 100, Output: 50},
		HourRow{Hour: 10, Tool: "claude", Model: "claude-sonnet-4-5", Input: 70, Output: 30},
	))
	if resp.StatusCode != 200 || out["accepted"] != true {
		t.Fatalf("first report rejected: %d %v", resp.StatusCode, out)
	}

	// Replacing snapshot for the same day: old rows must vanish.
	resp, out = postReport(t, srv, token, samplePayload("dev1", date,
		HourRow{Hour: 11, Tool: "codex", Model: "gpt-5", Input: 500, Output: 250},
	))
	if resp.StatusCode != 200 {
		t.Fatalf("second report rejected: %d", resp.StatusCode)
	}
	if out["dayTokens"] != "750" {
		t.Fatalf("dayTokens = %v, want 750 (old 250 gone)", out["dayTokens"])
	}
	if int(out["deviceCount"].(float64)) != 1 {
		t.Fatalf("deviceCount = %v, want 1", out["deviceCount"])
	}

	var rows int
	store.db.QueryRow(`SELECT COUNT(*) FROM hourly_usage WHERE device_id='dev1'`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("latest-wins failed: %d rows remain", rows)
	}
	var audits int
	store.db.QueryRow(`SELECT COUNT(*) FROM reports WHERE device_id='dev1'`).Scan(&audits)
	if audits != 2 {
		t.Fatalf("audit rows = %d, want 2", audits)
	}
}

func TestDeviceLimit(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, token := seedUser(t, store, "bob")
	date := time.Now().Format("2006-01-02")
	for i := 0; i < 3; i++ {
		device := "dev-" + strconv.Itoa(i)
		resp, _ := postReport(t, srv, token, samplePayload(device, date,
			HourRow{Hour: 1, Tool: "claude", Model: "m", Input: 1}))
		if resp.StatusCode != 200 {
			t.Fatalf("device %d rejected early: %d", i, resp.StatusCode)
		}
	}
	resp, out := postReport(t, srv, token, samplePayload("dev-4", date,
		HourRow{Hour: 1, Tool: "claude", Model: "m", Input: 1}))
	if resp.StatusCode != http.StatusConflict || out["code"] != "device_limit" {
		t.Fatalf("4th device: %d %v", resp.StatusCode, out)
	}
}

func TestReportValidation(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, token := seedUser(t, store, "carol")
	today := time.Now().Format("2006-01-02")

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"bad date", samplePayload("d", "2026/08/15", HourRow{Hour: 1, Tool: "t", Model: "m"})},
		{"bad hour", samplePayload("d", today, HourRow{Hour: 24, Tool: "t", Model: "m"})},
		{"empty tool", samplePayload("d", today, HourRow{Hour: 1, Tool: "", Model: "m"})},
		{"bad json", map[string]any{"deviceId": "d", "nope": 1}},
	}
	for _, tc := range cases {
		resp, _ := postReport(t, srv, token, tc.payload)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", tc.name, resp.StatusCode)
		}
	}

	// Bad token → 401.
	resp, _ := postReport(t, srv, "cct_wrong", samplePayload("d", today))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}

	// Oversized body → 413.
	big := samplePayload("d", today)
	big["deviceLabel"] = strings.Repeat("x", 3<<20)
	body, _ := json.Marshal(big)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("huge body: %d, want 413", resp.StatusCode)
	}

	// No residue from rejected requests.
	var rows int
	store.db.QueryRow(`SELECT COUNT(*) FROM hourly_usage WHERE device_id='d'`).Scan(&rows)
	if rows != 0 {
		t.Fatalf("rejected reports left %d rows", rows)
	}
}

func TestLeaderboardPageAndFilters(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, alice := seedUser(t, store, "alice")
	_ = alice
	bobID, bob := seedUser(t, store, "bob")
	store.UpdateUserProfile(context.Background(), bobID, "上海", "")
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	postReport(t, srv, alice, samplePayload("a1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 1000, Output: 500, CacheRead: 4000},
	))
	postReport(t, srv, bob, samplePayload("b1", today,
		HourRow{Hour: 9, Tool: "codex", Model: "gpt-5", Input: 300, Output: 200},
		HourRow{Hour: 10, Tool: "claude", Model: "claude-sonnet-4-5", Input: 100, Output: 100},
	))
	postReport(t, srv, bob, samplePayload("b2", yesterday,
		HourRow{Hour: 9, Tool: "codex", Model: "gpt-5", Input: 9999, Output: 1},
	))

	get := func(path string) string {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(raw)
	}

	// Today's board: alice 5500 > bob 700.
	page := get("/?range=today")
	if !strings.Contains(page, "alice") || !strings.Contains(page, "bob") {
		t.Fatal("leaderboard missing users")
	}
	if !strings.Contains(page, "5.5K") {
		t.Fatal("alice total 5500 not shown (cache included by default)")
	}
	// Cache-off board reorders: alice 1500 vs bob 700 — order stays but the
	// number changes.
	page = get("/?range=today&cache=0")
	if !strings.Contains(page, "1.5K") {
		t.Fatal("cache-off total not shown")
	}
	// Tool filter codex → only bob.
	page = get("/?range=today&tool=codex")
	if strings.Contains(page, "alice") {
		t.Fatal("tool filter leaked alice")
	}
	// City filter.
	page = get("/?range=today&city=上海")
	if strings.Contains(page, "alice") {
		t.Fatal("city filter leaked alice")
	}
	// Range all includes yesterday's device b2.
	page = get("/?range=all&tool=codex")
	if !strings.Contains(page, "bob") {
		t.Fatal("range=all lost yesterday data")
	}
	// Community totals card present.
	if !strings.Contains(page, "全员累计消耗") {
		t.Fatal("community totals card missing")
	}
}

func TestAccountFlowAndDashboard(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, token := seedUser(t, store, "dave")
	today := time.Now().Format("2006-01-02")
	postReport(t, srv, token, samplePayload("d1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 1000, Output: 100, CacheRead: 8900},
	))
	postReport(t, srv, token, samplePayload("d1", time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 500, Output: 50},
	))
	postReport(t, srv, token, samplePayload("d1", time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 500, Output: 50},
	))

	client := srv.Client()
	// httptest's default client has no cookie jar; sessions need one.
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// /me without login redirects.
	resp, err := client.Get(srv.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("/me anonymous: %d", resp.StatusCode)
	}

	// Register via the web form.
	form := url.Values{"name": {"echo"}, "password": {"hunter22"}, "city": {"杭州"}}
	resp, err = client.PostForm(srv.URL+"/register", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("register: %d", resp.StatusCode)
	}

	// Mint a token on the settings page and use it for ingest.
	resp, err = client.PostForm(srv.URL+"/settings/tokens", url.Values{"label": {"laptop"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(raw)
	start := strings.Index(page, "cct_")
	if start < 0 {
		t.Fatal("settings page did not reveal a new token")
	}
	webToken := page[start : start+len("cct_")+48]

	resp2, out := postReport(t, srv, webToken, samplePayload("echo-1", today,
		HourRow{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: 42, Output: 8},
	))
	if resp2.StatusCode != 200 || out["accepted"] != true {
		t.Fatalf("web-minted token rejected: %d %v", resp2.StatusCode, out)
	}

	// Dashboard renders with stat cards, devices and chart data.
	resp, err = client.Get(srv.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	dash := string(raw)
	for _, want := range []string{"当日消耗", "缓存命中率", "连续活跃", "chart-hourly", "dash-data", "echo-1"} {
		if !strings.Contains(dash, want) {
			t.Fatalf("dashboard missing %q", want)
		}
	}

	// Pricing + about pages render for anonymous visitors.
	for _, path := range []string{"/pricing", "/about"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(raw), "价格") && !strings.Contains(string(raw), "上榜") {
			t.Fatalf("%s rendered empty", path)
		}
	}
	_ = fmt.Sprint()
}

func TestAnomalyFlag(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "sneaky")
	today := time.Now().Format("2006-01-02")

	// Over-threshold device-day gets flagged and hidden from the board.
	postReport(t, srv, token, samplePayload("bot", today,
		HourRow{Hour: 1, Tool: "claude", Model: "m", Input: AnomalyThresholdTokens + 1},
	))
	rows := store.Leaderboard(context.Background(), Filters{From: today, To: today, IncludeCache: true}, pricing)
	if len(rows) != 0 {
		t.Fatalf("flagged device leaked onto leaderboard: %v", rows)
	}
	// Owner still sees own data (dashboard includes flagged).
	user, _ := store.UserByName(context.Background(), "sneaky")
	dash := store.Dashboard(context.Background(), user, pricing)
	if dash.TodayTokens == 0 {
		t.Fatal("owner dashboard lost flagged data")
	}
	if !dash.FlaggedToday {
		t.Fatal("owner dashboard does not show the flag")
	}

	// An honest device under the threshold is unaffected.
	_, honest := seedUser(t, store, "honest")
	postReport(t, srv, honest, samplePayload("pc", today,
		HourRow{Hour: 1, Tool: "claude", Model: "m", Input: 1000},
	))
	rows = store.Leaderboard(context.Background(), Filters{From: today, To: today, IncludeCache: true}, pricing)
	if len(rows) != 1 || rows[0].Name != "honest" {
		t.Fatalf("honest user missing from board: %+v", rows)
	}
}

func TestPricingCostMath(t *testing.T) {
	table, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	// claude-opus-4-1-ish rates from the LiteLLM snapshot; assert against
	// the map itself so the math test doesn't rot when prices update.
	model := "claude-sonnet-4-5"
	card, ok := table.Card(model)
	if !ok || card.Source != SourceSeed {
		t.Fatalf("seed card missing for %s: %+v", model, card)
	}
	row := HourRow{Tool: "claude", Model: model, Input: 1_000_000, Output: 1_000_000,
		CacheRead: 1_000_000, Cache5m: 1_000_000, Cache1h: 1_000_000}
	want := card.Input + card.Output + card.CacheRead + card.CacheWrite5m + card.CacheWrite1h
	got := table.CostForHourRow(&row)
	if diff := got - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("cost math: got %v want %v (card %+v)", got, want, card)
	}
	// Family fallback marks unknown sibling versions estimated (the exact
	// version is absent, but its family prefix prices it).
	est, ok := table.resolve("claude-sonnet-4-9")
	if !ok || est.Source != SourceFamily {
		t.Fatalf("family fallback failed: %+v", est)
	}
	if !table.Estimated("claude-sonnet-4-9") || table.Estimated(model) {
		t.Fatal("Estimated() disagrees with resolve()")
	}
}

func TestBackfillReport(t *testing.T) {
	store, _, srv := newTestServer(t)
	_, token := seedUser(t, store, "fill")
	base := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	d1 := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	payload := map[string]any{
		"deviceId": "bf1", "deviceLabel": "bf", "timezone": "Asia/Shanghai",
		"generatedAt": "2026-08-15T22:00:00+08:00",
		"days": []map[string]any{
			{"date": base, "hours": []HourRow{{Hour: 9, Tool: "claude", Model: "m", Input: 100}}},
			{"date": d1, "hours": []HourRow{{Hour: 9, Tool: "codex", Model: "gpt-5", Input: 200}}},
			{"date": today, "hours": []HourRow{{Hour: 9, Tool: "claude", Model: "m", Input: 50}}},
		},
	}
	resp, out := postReport(t, srv, token, payload)
	if resp.StatusCode != 200 || out["accepted"] != true {
		t.Fatalf("backfill rejected: %d %v", resp.StatusCode, out)
	}
	if out["dayTokens"] != "350" {
		t.Fatalf("backfill total = %v, want 350", out["dayTokens"])
	}
	if int(out["days"].(float64)) != 3 {
		t.Fatalf("days = %v, want 3", out["days"])
	}

	// Each date is an independent latest-wins unit: re-post one day alone.
	resp, out = postReport(t, srv, token, samplePayload("bf1", d1,
		HourRow{Hour: 10, Tool: "codex", Model: "gpt-5", Input: 7}))
	if resp.StatusCode != 200 || out["dayTokens"] != "7" {
		t.Fatalf("single-day replacement broke: %v", out)
	}

	counts := map[string]int{}
	rows, _ := store.db.Query(`SELECT date, COUNT(*) FROM hourly_usage WHERE device_id='bf1' GROUP BY date`)
	for rows.Next() {
		var d string
		var c int
		rows.Scan(&d, &c)
		counts[d] = c
	}
	rows.Close()
	if counts[base] != 1 || counts[today] != 1 || counts[d1] != 1 {
		t.Fatalf("backfill day rows wrong: %v", counts)
	}

	// Malformed backfills: bad date in days, too many days.
	badDate := map[string]any{"deviceId": "bf1", "days": []map[string]any{{"date": "2026/1/1", "hours": []HourRow{}}}}
	if resp, _ := postReport(t, srv, token, badDate); resp.StatusCode != 400 {
		t.Fatalf("bad days date accepted: %d", resp.StatusCode)
	}
	tooMany := map[string]any{"deviceId": "bf1", "days": make([]map[string]any, 600)}
	if resp, _ := postReport(t, srv, token, tooMany); resp.StatusCode != 400 {
		t.Fatalf("600-day backfill accepted: %d", resp.StatusCode)
	}
}
