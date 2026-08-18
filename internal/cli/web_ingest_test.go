package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/report"
	"github.com/wujunwei928/token-usage/internal/server"
)

// webFixture writes a Claude config dir carrying usage lines stamped today.
func webFixture(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	project := filepath.Join(dir, "projects", "web-proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "session-w.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// webUsageLine builds one Claude JSONL usage entry stamped at when.
func webUsageLine(id, model, requestID string, when time.Time, input, output, cacheRead, cache5m, cache1h uint64) string {
	cacheBlock := "null"
	if cache5m > 0 || cache1h > 0 {
		cacheBlock = fmt.Sprintf(`{"ephemeral_5m_input_tokens":%d,"ephemeral_1h_input_tokens":%d}`, cache5m, cache1h)
	}
	return fmt.Sprintf(`{"sessionId":"s-%s","timestamp":"%s","version":"2.0.0","requestId":"%s",`+
		`"message":{"id":"msg_%s","model":"%s","usage":{"input_tokens":%d,"output_tokens":%d,`+
		`"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"cache_creation":%s}},`+
		`"costUSD":0.01}`,
		id, when.UTC().Format("2006-01-02T15:04:05.000Z"), requestID, id, model,
		input, output, cacheRead, cache5m+cache1h, cacheBlock)
}

// rowsOf dumps a store's hourly_usage as comparable lines.
func rowsOf(t *testing.T, s *server.Store) []string {
	t.Helper()
	rows, err := s.DumpHourly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// The web command's in-process ingest must be row-for-row identical to the
// HTTP ingest path over the same fixture — the two paths share one
// Latest-wins store unit, and nothing may drift between them.
func TestInProcessIngestMatchesHTTPIngest(t *testing.T) {
	now := time.Now()
	today9 := time.Date(now.Year(), now.Month(), now.Day(), 9, 5, 0, 0, now.Location())
	today21 := time.Date(now.Year(), now.Month(), now.Day(), 21, 5, 0, 0, now.Location())
	// Hermetic environment: every adapter source must resolve inside temp
	// dirs or the fixture, never the developer's real logs.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", webFixture(t,
		webUsageLine("w1", "claude-sonnet-4-5", "rw1", today9, 1000, 200, 5000, 300, 100),
		webUsageLine("w2", "claude-opus-4-1", "rw2", today21, 400, 80, 0, 0, 0),
	))
	t.Setenv("TZ", "Asia/Shanghai")

	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	snap := report.BuildSnapshot(shared, "dev-web", "web-box", time.Now())

	// Worked example (independent of either ingest path): the two lines sum
	// to (1000+200+5000+300+100) + (400+80) = 7080 tokens.
	if got := snap.TotalTokens(); got != 7080 {
		t.Fatalf("snapshot total = %d, want 7080", got)
	}

	ctx := context.Background()

	// Path A: the web command's in-process ingest.
	storeA, err := server.OpenStore(t.TempDir() + "/a.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	userA, err := storeA.CreateUser(ctx, "local-a", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingestSnapshots(ctx, storeA, userA, []*report.Snapshot{snap}); err != nil {
		t.Fatal(err)
	}

	// Path B: the same snapshot over the HTTP ingest API.
	storeB, err := server.OpenStore(t.TempDir() + "/b.db")
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	token, err := server.SeedUser(ctx, storeB, "local-b", "", "")
	if err != nil {
		t.Fatal(err)
	}
	userB, _ := storeB.UserByName(ctx, "local-b")
	mux := http.NewServeMux()
	server.NewAPI(storeB, nil).Register(mux)
	api := httptest.NewServer(mux)
	defer api.Close()
	body, _ := json.Marshal(snap)
	req, _ := http.NewRequest("POST", api.URL+"/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP ingest rejected: %d", resp.StatusCode)
	}

	// Same user/device wiring on both sides keeps the comparison honest.
	if userA != userB.ID {
		t.Fatalf("user ids diverged: %d vs %d", userA, userB.ID)
	}
	gotA, gotB := rowsOf(t, storeA), rowsOf(t, storeB)
	if len(gotA) == 0 {
		t.Fatal("in-process ingest wrote no rows")
	}
	if strings.Join(gotA, ";") != strings.Join(gotB, ";") {
		t.Fatalf("in-process and HTTP ingest diverged:\nA: %v\nB: %v", gotA, gotB)
	}

	// Hand-computed expectation: two rows with the exact counters above.
	today := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	want := []string{
		fmt.Sprintf("dev-web|%s|9|claude|claude-sonnet-4-5|1000|200|5000|300|100|0", today),
		fmt.Sprintf("dev-web|%s|21|claude|claude-opus-4-1|400|80|0|0|0|0", today),
	}
	if strings.Join(gotA, ";") != strings.Join(want, ";") {
		t.Fatalf("rows != worked example:\n got %v\nwant %v", gotA, want)
	}
}

// A machine with no readable agent logs must ingest nothing and clear
// nothing: an empty snapshot leaves the stored day untouched.
func TestIngestSkipsEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, err := store.CreateUser(ctx, "solo", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceDay(ctx, userID, "dev-web", "box",
		time.Now().Format("2006-01-02"),
		[]server.HourRow{{Hour: 9, Tool: "claude", Model: "m", Input: 42}}); err != nil {
		t.Fatal(err)
	}

	if err := ingestSnapshots(ctx, store, userID,
		[]*report.Snapshot{{DeviceID: "dev-web", Date: time.Now().Format("2006-01-02")}}); err != nil {
		t.Fatal(err)
	}
	if rows := rowsOf(t, store); len(rows) != 1 {
		t.Fatalf("empty snapshot must not clear the day: %v", rows)
	}
}
