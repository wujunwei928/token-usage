package zcode

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
)

// createFixtureDB builds an analytics database with the schema subset the
// loader reads plus fixture rows covering every accounting rule: subagent
// merge, retry/failed/auxiliary counting, zero-usage skipping, and the
// message fallback with its no-double-count guard.
func createFixtureDB(t *testing.T, dir string) {
	t.Helper()
	dbDir := filepath.Join(dir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture db dir: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dbDir, DBFilename))
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	must(`CREATE TABLE session (id text primary key, parent_id text, directory text not null)`)
	must(`CREATE TABLE model_usage (
		id text primary key, session_id text not null, model_id text not null,
		started_at integer not null,
		input_tokens integer not null default 0, output_tokens integer not null default 0,
		reasoning_tokens integer not null default 0,
		cache_creation_input_tokens integer not null default 0,
		cache_read_input_tokens integer not null default 0)`)
	must(`CREATE TABLE message (id text primary key, session_id text not null, time_created integer not null, data text not null)`)

	must(`INSERT INTO session (id, parent_id, directory) VALUES ('top-1', NULL, '/work/proj')`)
	must(`INSERT INTO session (id, parent_id, directory) VALUES ('sub-1', 'top-1', '/work/proj')`)
	must(`INSERT INTO session (id, parent_id, directory) VALUES ('old-1', NULL, '/old/proj')`)

	day := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC).UnixMilli()
	hour := int64(time.Hour / time.Millisecond)
	insert := func(id, session string, offset int64, in, out, reasoning, creation, read int64) {
		must(`INSERT INTO model_usage (id, session_id, model_id, started_at, input_tokens,
			output_tokens, reasoning_tokens, cache_creation_input_tokens, cache_read_input_tokens)
			VALUES (?, ?, 'GLM-5.3', ?, ?, ?, ?, ?, ?)`,
			id, session, day+offset*hour, in, out, reasoning, creation, read)
	}
	insert("u1", "top-1", 0, 1000, 100, 50, 0, 2000) // main turn
	insert("u2", "sub-1", 1, 500, 50, 0, 0, 1000)    // subagent: merges into top-1
	insert("u3", "top-1", 2, 100, 10, 0, 0, 0)       // auxiliary call: counts
	insert("u4", "top-1", 3, 300, 0, 0, 0, 0)        // failed retry: counts
	insert("u5", "top-1", 4, 0, 0, 0, 0, 0)          // zero usage: dropped

	must(`INSERT INTO message (id, session_id, time_created, data) VALUES ('m1', 'old-1', ?,
		'{"modelID":"GLM-5.2","tokens":{"total":1210,"input":700,"output":70,"reasoning":30,"cache":{"read":400,"write":10}},"cost":0}')`,
		day+5*hour)
	must(`INSERT INTO message (id, session_id, time_created, data) VALUES ('m2', 'old-1', ?,
		'{"role":"user","contextSnapshot":{}}')`, day+6*hour) // no tokens: dropped
	must(`INSERT INTO message (id, session_id, time_created, data) VALUES ('m3', 'top-1', ?,
		'{"modelID":"GLM-5.3","tokens":{"input":999,"output":99,"cache":{"read":0}},"cost":0}')`,
		day+7*hour) // top-1 has model_usage rows: must not double count
}

func fixtureShared(mode core.CostMode) *core.SharedArgs {
	utc := "UTC"
	return &core.SharedArgs{Mode: mode, Timezone: &utc, Offline: true}
}

func loadFixture(t *testing.T, mode core.CostMode) []core.LoadedEntry {
	t.Helper()
	dir := t.TempDir()
	createFixtureDB(t, dir)
	t.Setenv(DataDirEnv, dir)
	entries, err := LoadEntries(fixtureShared(mode))
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	return entries
}

// Expected fixture totals: attempts u1..u4 plus fallback m1.
const (
	wantInput    = 1000 + 500 + 100 + 300 + 700
	wantOutput   = (100 + 50) + 50 + 10 + 0 + (70 + 30)
	wantCacheRead = 2000 + 1000 + 400
	wantCacheWrite = 10
)

func TestLoadEntriesAttemptAccounting(t *testing.T) {
	entries := loadFixture(t, core.ModeDisplay)
	if len(entries) != 5 {
		t.Fatalf("entries = %d, want 5 (u1-u4 + m1; u5 and m2 dropped)", len(entries))
	}
	var totals core.TokenCounts
	for i := range entries {
		totals.AddUsage(entries[i].Data.Message.Usage)
	}
	if totals.InputTokens != wantInput {
		t.Errorf("input = %d, want %d", totals.InputTokens, wantInput)
	}
	if totals.OutputTokens != wantOutput {
		t.Errorf("output (incl. reasoning) = %d, want %d", totals.OutputTokens, wantOutput)
	}
	if totals.CacheReadTokens != wantCacheRead {
		t.Errorf("cache read = %d, want %d", totals.CacheReadTokens, wantCacheRead)
	}
	if totals.CacheCreationTokens != wantCacheWrite {
		t.Errorf("cache write = %d, want %d", totals.CacheCreationTokens, wantCacheWrite)
	}
	for i := range entries {
		if entries[i].MissingPricingModel != nil {
			t.Errorf("display mode entry %d has MissingPricingModel", i)
		}
		if entries[i].Cost != 0 {
			t.Errorf("display mode entry %d cost = %v, want 0", i, entries[i].Cost)
		}
	}
}

func TestLoadEntriesSubagentMergesIntoParent(t *testing.T) {
	entries := loadFixture(t, core.ModeDisplay)
	rows := SummarizeEntries(entries, KindSession)
	if len(rows) != 2 {
		t.Fatalf("session rows = %d, want 2 (top-1, old-1)", len(rows))
	}
	for i := range rows {
		if rows[i].SessionID == nil {
			t.Fatalf("row %d has no session id", i)
		}
		if *rows[i].SessionID == "sub-1" {
			t.Errorf("subagent session leaked as its own row")
		}
	}
}

func TestLoadEntriesSessionAttribution(t *testing.T) {
	entries := loadFixture(t, core.ModeDisplay)
	for i := range entries {
		if entries[i].Project != "zcode" {
			t.Errorf("entry %d project = %q, want zcode", i, entries[i].Project)
		}
		if entries[i].SessionID != "top-1" && entries[i].SessionID != "old-1" {
			t.Errorf("entry %d session = %q, want a top-level session", i, entries[i].SessionID)
		}
		wantPath := "/work/proj"
		if entries[i].SessionID == "old-1" {
			wantPath = "/old/proj"
		}
		if entries[i].ProjectPath != wantPath {
			t.Errorf("entry %d projectPath = %q, want %q", i, entries[i].ProjectPath, wantPath)
		}
	}
}

func TestLoadEntriesDailySummary(t *testing.T) {
	entries := loadFixture(t, core.ModeDisplay)
	rows := SummarizeEntries(entries, KindDaily)
	if len(rows) != 1 || rows[0].Date == nil || *rows[0].Date != "2026-08-15" {
		t.Fatalf("daily rows = %+v, want one 2026-08-15 row", rows)
	}
	if rows[0].InputTokens != wantInput || rows[0].OutputTokens != wantOutput ||
		rows[0].CacheReadTokens != wantCacheRead || rows[0].CacheCreationTokens != wantCacheWrite {
		t.Errorf("daily totals = %+v", rows[0])
	}
}

func TestLoadEntriesMissingPricingInAutoMode(t *testing.T) {
	entries := loadFixture(t, core.ModeAuto)
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	for i := range entries {
		if entries[i].MissingPricingModel == nil {
			t.Errorf("entry %d (%s) should flag missing pricing", i, *entries[i].Model)
		}
		if entries[i].Cost != 0 {
			t.Errorf("entry %d cost = %v, want 0", i, entries[i].Cost)
		}
	}
}

func TestLoadEntriesNoDatabase(t *testing.T) {
	t.Setenv(DataDirEnv, t.TempDir())
	if HasData() {
		t.Error("HasData with empty dir")
	}
	entries, err := LoadEntries(fixtureShared(core.ModeDisplay))
	if err != nil {
		t.Fatalf("LoadEntries with no db: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

func TestLoadEntriesUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "db", DBFilename))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE message (id text primary key)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Setenv(DataDirEnv, dir)
	_, err = LoadEntries(fixtureShared(core.ModeDisplay))
	if err == nil {
		t.Fatal("expected unsupported-schema error")
	}
	if !strings.Contains(err.Error(), "unsupported schema") {
		t.Errorf("error = %v, want unsupported-schema message", err)
	}
}
