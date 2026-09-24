package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// Ticket 06 (cache-overlap double count): Codex's OpenAI-style token_count
// reports an INCLUSIVE input (cached ⊂ prompt — cf. the codex cost module's
// NonCachedInputTokens). The unified Usage Entry keeps input uncached, so
// codexEntries must subtract the cached overlap instead of double counting.
func TestCodexEntriesSubtractsCachedOverlap(t *testing.T) {
	dir := t.TempDir()
	sessions := filepath.Join(dir, "sessions", "rollout-1")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"2026-08-15T01:00:00.000Z","type":"event_msg","payload":{"type":"token_count",` +
		`"info":{"model":"gpt-5.3-codex","last_token_usage":{"input_tokens":2000,"cached_input_tokens":500,` +
		`"output_tokens":300,"reasoning_output_tokens":0,"total_tokens":2500},` +
		`"total_token_usage":{"input_tokens":2000,"cached_input_tokens":500,"output_tokens":300,` +
		`"reasoning_output_tokens":0,"total_tokens":2500}}}}`
	if err := os.WriteFile(filepath.Join(sessions, "session-x.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", dir)
	utc := "UTC"
	entries, err := codexEntries(&core.SharedArgs{Timezone: &utc, Offline: true})
	if err != nil {
		t.Fatalf("codexEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	u := entries[0].Data.Message.Usage
	if u.InputTokens != 1500 {
		t.Errorf("input = %d, want 1500 (2000 inclusive - 500 cached)", u.InputTokens)
	}
	if u.CacheReadInputTokens != 500 {
		t.Errorf("cache read = %d, want 500", u.CacheReadInputTokens)
	}
	if u.OutputTokens != 300 {
		t.Errorf("output = %d, want 300", u.OutputTokens)
	}
}
