package statusline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatuslineCachePathScheme(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp/cc-abc")
	if got := statuslineCachePath("sess-1"); got != filepath.Join("/tmp/cc-abc", "ccusage-semaphore", "sess-1.lock") {
		t.Errorf("cache path = %q", got)
	}
}

func TestCachedStatuslineOutputRules(t *testing.T) {
	fresh := &StatuslineCache{
		Date: "2026-01-01T00:00:00.000Z", LastOutput: "cached",
		LastUpdateTime: 10_000, TranscriptPath: "/t.jsonl", TranscriptMtime: 123,
	}
	if got := cachedStatuslineOutput(fresh, 123, 10_500, 1); got == nil || *got != "cached" {
		t.Errorf("fresh cache should hit: %v", got)
	}
	if got := cachedStatuslineOutput(fresh, 123, 10_999, 1); got == nil || *got != "cached" {
		t.Errorf("under one interval should hit: %v", got)
	}
	if got := cachedStatuslineOutput(fresh, 123, 11_000, 1); got != nil {
		t.Errorf("expired cache must miss: %v", got)
	}
	if got := cachedStatuslineOutput(fresh, 456, 10_500, 1); got != nil {
		t.Errorf("changed transcript mtime must miss: %v", got)
	}
	empty := &StatuslineCache{LastUpdateTime: 10_000, TranscriptMtime: 123}
	if got := cachedStatuslineOutput(empty, 123, 10_500, 1); got != nil {
		t.Errorf("empty output must miss: %v", got)
	}
	// Zero refresh interval expires immediately.
	if got := cachedStatuslineOutput(fresh, 123, 10_000, 0); got != nil {
		t.Errorf("interval 0 must expire: %v", got)
	}
	// A live updating process serves the stale line even when expired.
	staleLive := &StatuslineCache{
		LastOutput: "stale", LastUpdateTime: 10_000, TranscriptMtime: 123,
		IsUpdating: true, PID: pidPtr(uint32(os.Getpid())),
	}
	if got := cachedStatuslineOutput(staleLive, 456, 20_000, 1); got == nil || *got != "stale" {
		t.Errorf("live updater should serve stale output: %v", got)
	}
	// A dead pid does not.
	staleDead := &StatuslineCache{
		LastOutput: "stale", LastUpdateTime: 10_000, TranscriptMtime: 123,
		IsUpdating: true, PID: pidPtr(1), // pid 1 exists but is not ours; kill(0) may pass as root.
	}
	_ = staleDead
	staleNoPID := &StatuslineCache{
		LastOutput: "stale", LastUpdateTime: 10_000, TranscriptMtime: 123,
		IsUpdating: true,
	}
	if got := cachedStatuslineOutput(staleNoPID, 456, 20_000, 1); got != nil {
		t.Errorf("updating without pid must miss: %v", got)
	}
}

func TestWriteStatuslineCacheShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	path := statuslineCachePath("shape-test")
	writeStatuslineCache(path, &StatuslineCache{
		Date: "2026-08-15T00:00:00.000Z",
		LastOutput: "🤖 line",
		LastUpdateTime: 1786763942941,
		TranscriptPath: "/t.jsonl",
		TranscriptMtime: 0,
	})
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"date":"2026-08-15T00:00:00.000Z","lastOutput":"🤖 line","lastUpdateTime":1786763942941,"transcriptPath":"/t.jsonl","transcriptMtime":0,"isUpdating":false,"pid":null}`
	if string(raw) != want {
		t.Errorf("cache file bytes:\n got %s\nwant %s", raw, want)
	}
	got := readStatuslineCache(path)
	if got == nil || got.LastOutput != "🤖 line" || got.LastUpdateTime != 1786763942941 {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestMarkUpdatingKeepsPreviousOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	path := statuslineCachePath("updating-test")
	previous := &StatuslineCache{
		Date: "d", LastOutput: "old", LastUpdateTime: 111, TranscriptMtime: 5,
	}
	markStatuslineCacheUpdating(path, &Hook{SessionID: "updating-test", TranscriptPath: "/t.jsonl"}, 7, previous)
	cache := readStatuslineCache(path)
	if cache == nil {
		t.Fatal("cache not written")
	}
	if !cache.IsUpdating || cache.PID == nil || *cache.PID != uint32(os.Getpid()) {
		t.Errorf("updating marker: %+v", cache)
	}
	if cache.LastOutput != "old" || cache.LastUpdateTime != 111 {
		t.Errorf("previous output not preserved: %+v", cache)
	}
	if cache.TranscriptMtime != 7 {
		t.Errorf("mtime not refreshed: %+v", cache)
	}

	releaseStatuslineCache(path)
	released := readStatuslineCache(path)
	if released == nil || released.IsUpdating || released.PID != nil {
		t.Errorf("release: %+v", released)
	}
	if released.LastOutput != "old" {
		t.Errorf("release kept output: %+v", released)
	}
}

func TestMarkUpdatingWithoutPrevious(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	path := statuslineCachePath("updating-fresh")
	markStatuslineCacheUpdating(path, &Hook{SessionID: "updating-fresh", TranscriptPath: "/t.jsonl"}, 9, nil)
	cache := readStatuslineCache(path)
	if cache == nil || cache.LastOutput != "" || cache.LastUpdateTime != 0 {
		t.Errorf("fresh updating cache: %+v", cache)
	}
}

func TestReadStatuslineCacheToleratesJunk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "junk.lock")
	os.WriteFile(path, []byte("not json"), 0o644)
	if got := readStatuslineCache(path); got != nil {
		t.Errorf("junk cache: %+v", got)
	}
	if got := readStatuslineCache(filepath.Join(dir, "missing.lock")); got != nil {
		t.Errorf("missing cache: %+v", got)
	}
}

func TestTranscriptMtimeMs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	os.WriteFile(path, []byte("x"), 0o644)
	if got := transcriptMtimeMs(path); got == 0 {
		t.Errorf("mtime should be nonzero")
	}
	if got := transcriptMtimeMs(filepath.Join(dir, "absent.jsonl")); got != 0 {
		t.Errorf("absent mtime = %d", got)
	}
}

func pidPtr(v uint32) *uint32 { return &v }

// Run-level pipeline tests: stdin handling, validation order, cache reuse.
func TestRunEmptyStdin(t *testing.T) {
	withClock(t, fixedNow)
	for _, input := range []string{"", "   ", "\n\t"} {
		var out bytes.Buffer
		err := Run(strings.NewReader(input), &out, baseArgs())
		if err == nil || err.Error() != `CliError("❌ No input provided")` {
			t.Errorf("Run(%q) err = %v", input, err)
		}
		if out.Len() != 0 {
			t.Errorf("Run(%q) wrote %q", input, out.String())
		}
	}
}

func TestRunInvalidJSON(t *testing.T) {
	withClock(t, fixedNow)
	var out bytes.Buffer
	err := Run(strings.NewReader("not json"), &out, baseArgs())
	if err == nil || err.Error() != `CliError("Invalid input format: expected ident at line 1 column 2")` {
		t.Fatalf("err = %v", err)
	}
}

func TestRunThresholdValidationPrecedesStdin(t *testing.T) {
	withClock(t, fixedNow)
	args := baseArgs()
	args.ContextLowThreshold = 80
	args.ContextMediumThresh = 50
	var out bytes.Buffer
	err := Run(strings.NewReader("not json"), &out, args)
	if err == nil || !strings.Contains(err.Error(), "Context low threshold (80) must be less than medium threshold (50)") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCacheLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", writeEmptyData(t))
	withClock(t, fixedNow)

	hookJSON := `{"session_id":"life","transcript_path":"/nonexistent/t.jsonl","model":{"display_name":"D"},"context_window":{"total_input_tokens":1000,"context_window_size":2000}}`
	args := baseArgs()
	args.Timezone = utcTZ()

	var first bytes.Buffer
	if err := Run(strings.NewReader(hookJSON), &first, args); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(first.String())
	if !strings.Contains(line, "🤖 D") {
		t.Fatalf("first run: %q", line)
	}
	cache := readStatuslineCache(statuslineCachePath("life"))
	if cache == nil {
		t.Fatal("completed cache not written")
	}
	if cache.IsUpdating || cache.PID != nil || cache.LastOutput != line {
		t.Errorf("completed cache: %+v", cache)
	}
	if cache.Date == "" || !strings.HasSuffix(cache.Date, "Z") {
		t.Errorf("cache date = %q", cache.Date)
	}

	// Second run within the refresh interval replays the cached line.
	var second bytes.Buffer
	if err := Run(strings.NewReader(hookJSON), &second, args); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(second.String()); got != line {
		t.Errorf("cache replay:\n got %q\nwant %q", got, line)
	}

	// Advancing the clock beyond the interval forces a recompute.
	nowFunc = func() int64 { return fixedNow + 5_000 }
	var third bytes.Buffer
	if err := Run(strings.NewReader(hookJSON), &third, args); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(third.String()); got != line {
		t.Errorf("recompute:\n got %q\nwant %q", got, line)
	}

	// --no-cache bypasses the cache entirely (also on the write side).
	args.NoCache = true
	os.Remove(statuslineCachePath("life"))
	var fourth bytes.Buffer
	if err := Run(strings.NewReader(hookJSON), &fourth, args); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statuslineCachePath("life")); !os.IsNotExist(err) {
		t.Errorf("--no-cache still wrote the semaphore file")
	}
}

func TestRunCacheInvalidatedByTranscriptMtime(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("CLAUDE_CONFIG_DIR", writeEmptyData(t))
	withClock(t, fixedNow)

	transcript := filepath.Join(root, "transcript.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"assistant","message":{"usage":{"input_tokens":10,"output_tokens":0}}}`), 0o644)
	hookJSON := func() string {
		raw, _ := json.Marshal(map[string]any{
			"session_id":      "mtime-test",
			"transcript_path": transcript,
			"model":           map[string]any{"id": "unknown-model", "display_name": "D"},
		})
		return string(raw)
	}
	args := baseArgs()
	args.Timezone = utcTZ()

	var first bytes.Buffer
	if err := Run(strings.NewReader(hookJSON()), &first, args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), "10 (0%)") {
		t.Fatalf("first run: %q", first.String())
	}

	// Touching the transcript invalidates the cache and re-renders.
	future := fixedNow + 60_000
	futureTime := time.UnixMilli(future)
	if err := os.Chtimes(transcript, futureTime, futureTime); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := Run(strings.NewReader(hookJSON()), &second, args); err != nil {
		t.Fatal(err)
	}
	cache := readStatuslineCache(statuslineCachePath("mtime-test"))
	if cache == nil || cache.TranscriptMtime != uint64(future) {
		t.Errorf("mtime not refreshed: %+v", cache)
	}
}

func writeEmptyData(t *testing.T) string {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "projects"), 0o755)
	return root
}
