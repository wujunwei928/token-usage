package omp

import (
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
)

func TestExtractSessionIDTopLevelFile(t *testing.T) {
	got := ExtractSessionID("sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha.jsonl")
	if got != "session-alpha" {
		t.Errorf("ExtractSessionID = %q, want session-alpha", got)
	}
}

// A file nested in a `<started-at>_<session-id>` sidecar directory belongs to
// that parent session (omp writes subagent/design sub-sessions there).
func TestExtractSessionIDSidecarAttributesToParent(t *testing.T) {
	got := ExtractSessionID("sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha/DesignSubAgent.jsonl")
	if got != "session-alpha" {
		t.Errorf("ExtractSessionID = %q, want session-alpha", got)
	}
}

// A project directory name containing '_' is not a session stem; the file's
// own stem must win.
func TestExtractSessionIDIgnoresUnderscoredProjectDir(t *testing.T) {
	got := ExtractSessionID("sessions/--code-ai-omp_test--/2026-03-01T12-00-00-000Z_session-delta.jsonl")
	if got != "session-delta" {
		t.Errorf("ExtractSessionID = %q, want session-delta", got)
	}
}

func TestExtractSessionIDFallbackStem(t *testing.T) {
	got := ExtractSessionID("sessions/--demo-omp-one--/orphan.jsonl")
	if got != "orphan" {
		t.Errorf("ExtractSessionID = %q, want orphan", got)
	}
}

func TestExtractProject(t *testing.T) {
	cases := map[string]string{
		"sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha.jsonl":     "--demo-omp-one--",
		"sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha/sub.jsonl": "--demo-omp-one--",
		"elsewhere/file.jsonl": "unknown",
	}
	for path, want := range cases {
		if got := ExtractProject(path); got != want {
			t.Errorf("ExtractProject(%q) = %q, want %q", path, got, want)
		}
	}
}

// ReadSessionFile must mirror the pi gates: assistant messages with usage
// count, zero-token and non-message records drop, omp's extra
// reasoningTokens field is ignored (it is already inside output), and
// totalTokens fills a missing output count.
func TestReadSessionFileGatesAndAttribution(t *testing.T) {
	entries, err := ReadSessionFile("../../../testdata/fixtures/omp/sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha.jsonl",
		time.UTC, core.ModeDisplay, nil)
	if err != nil {
		t.Fatal(err)
	}
	// a2, a4, a5, a6 survive; a3 (zero tokens), x1 (type other), and the
	// unparsable line drop.
	if len(entries) != 4 {
		t.Fatalf("ReadSessionFile produced %d entries, want 4", len(entries))
	}
	first := entries[0]
	if first.SessionID != "session-alpha" {
		t.Errorf("SessionID = %q, want session-alpha", first.SessionID)
	}
	if first.Project != "--demo-omp-one--" {
		t.Errorf("Project = %q, want --demo-omp-one--", first.Project)
	}
	if first.Model == nil || *first.Model != "[omp] glm-5.2" {
		t.Errorf("Model = %v, want [omp] glm-5.2", first.Model)
	}
	if got := first.Data.Message.Usage.OutputTokens; got != 200 {
		t.Errorf("reasoningTokens leaked into output: output = %d, want 200", got)
	}
	if first.Data.CostUSD == nil || *first.Data.CostUSD != 0.0123 {
		t.Errorf("CostUSD = %v, want 0.0123", first.Data.CostUSD)
	}
	// a6: input 100 with totalTokens 333 and no output fills output = 233.
	last := entries[3]
	if got := last.Data.Message.Usage.OutputTokens; got != 233 {
		t.Errorf("totalTokens fallback output = %d, want 233", got)
	}
	if last.Date != "2026-01-02" {
		t.Errorf("Date = %q, want 2026-01-02", last.Date)
	}
}

func TestReadSessionFileSidecarParentAttribution(t *testing.T) {
	entries, err := ReadSessionFile("../../../testdata/fixtures/omp/sessions/--demo-omp-one--/2026-01-02T10-00-00-000Z_session-alpha/DesignSubAgent.jsonl",
		time.UTC, core.ModeDisplay, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadSessionFile produced %d entries, want 2", len(entries))
	}
	for i := range entries {
		if entries[i].SessionID != "session-alpha" {
			t.Errorf("entry %d SessionID = %q, want parent session-alpha", i, entries[i].SessionID)
		}
		if entries[i].Project != "--demo-omp-one--" {
			t.Errorf("entry %d Project = %q, want --demo-omp-one--", i, entries[i].Project)
		}
	}
}

// Duplicate assistant records inside one file collapse to the first via the
// loader's dedupe identity.
func TestLoadEntriesDedupesReplays(t *testing.T) {
	t.Setenv(OmpAgentDirEnv, "../../../testdata/fixtures/omp/sessions")
	utc := "UTC"
	shared := &core.SharedArgs{Timezone: &utc, Mode: core.ModeDisplay, Offline: true}
	entries, err := LoadEntries(LoadOptions{Shared: shared})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := range entries {
		id := entryID(&entries[i])
		if seen[id] {
			t.Errorf("duplicate entry id survived dedup: %s", id)
		}
		seen[id] = true
	}
	// alpha(4) + sidecar(2) + delta(2) = 8 fixture entries.
	if len(entries) != 8 {
		t.Fatalf("LoadEntries produced %d entries, want 8", len(entries))
	}
}

func TestHasDataFollowsEnvRoot(t *testing.T) {
	t.Setenv(OmpAgentDirEnv, "../../../testdata/fixtures/omp/sessions")
	if !HasData() {
		t.Error("HasData = false with the fixture sessions dir set")
	}
	t.Setenv(OmpAgentDirEnv, "../../../testdata/fixtures/omp/missing")
	if HasData() {
		t.Error("HasData = true with a missing sessions dir")
	}
}
