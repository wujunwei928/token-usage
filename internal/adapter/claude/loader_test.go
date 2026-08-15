package claude

import (
	"os"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// ---- fixtures (ported from the Rust UsageEntryFixture helper) ----

type usageEntryFixture struct {
	messageID      string
	requestID      string
	isSidechain    bool
	cacheReadToken uint64
	outputTokens   uint64
	speed          *string
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

func loadedUsageEntry(f usageEntryFixture) core.LoadedEntry {
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: strPtr("session-a"),
			Timestamp: "2026-03-29T07:00:00.000Z",
			Version:   strPtr("1.0.0"),
			Message: core.UsageMessage{
				Usage: core.TokenUsageRaw{
					InputTokens:          0,
					OutputTokens:         f.outputTokens,
					CacheReadInputTokens: f.cacheReadToken,
					Speed:                f.speed,
				},
				Model: strPtr("claude-sonnet-4-20250514"),
				ID:    strPtr(f.messageID),
			},
			RequestID:   strPtr(f.requestID),
			IsSidechain: boolPtr(f.isSidechain),
		},
		Project:   "project-a",
		SessionID: "session-a",
		Model:     strPtr("claude-sonnet-4-20250514"),
	}
}

func dailyEntryFixture(f usageEntryFixture, cost float64) dailyLoadedEntry {
	messageID := f.messageID
	requestID := f.requestID
	return dailyLoadedEntry{
		date:    "2026-03-29",
		project: "project-a",
		usage: core.TokenUsageRaw{
			InputTokens:          0,
			OutputTokens:         f.outputTokens,
			CacheReadInputTokens: f.cacheReadToken,
			Speed:                f.speed,
		},
		cost:        cost,
		model:       strPtr("claude-sonnet-4-20250514"),
		messageID:   &messageID,
		requestID:   &requestID,
		isSidechain: boolPtr(f.isSidechain),
	}
}

// ---- shouldReplaceDedupedEntry ordering (lib.rs semantics) ----

func TestShouldReplaceDedupedEntryOrdering(t *testing.T) {
	base := func(sidechain bool, output uint64, speed *string) core.UsageEntry {
		entry := loadedUsageEntry(usageEntryFixture{
			messageID: "msg", requestID: "req", isSidechain: sidechain,
			outputTokens: output, speed: speed,
		})
		return entry.Data
	}
	standard := core.SpeedStandard
	fast := core.SpeedFast
	tests := []struct {
		name      string
		candidate core.UsageEntry
		existing  core.UsageEntry
		want      bool
	}{
		{"non-sidechain replaces sidechain", base(false, 1, nil), base(true, 100, nil), true},
		{"sidechain never replaces non-sidechain", base(true, 100, nil), base(false, 1, nil), false},
		{"more tokens replace", base(false, 50, nil), base(false, 10, nil), true},
		{"fewer tokens do not replace", base(false, 10, nil), base(false, 50, nil), false},
		{"speed wins token tie", base(false, 10, &fast), base(false, 10, nil), true},
		{"speed loses against speed", base(false, 10, &fast), base(false, 10, &standard), false},
		{"no speed loses token tie against speed", base(false, 10, nil), base(false, 10, &standard), false},
		{"tokens beat speed", base(false, 11, nil), base(false, 10, &fast), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReplaceDedupedEntry(&tt.candidate, &tt.existing); got != tt.want {
				t.Fatalf("shouldReplaceDedupedEntry(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ---- shouldReplaceDedupedDailyEntry: cost tiebreaks before speed ----

func TestShouldReplaceDedupedDailyEntryCostTiebreak(t *testing.T) {
	fast := core.SpeedFast
	tests := []struct {
		name      string
		candidate dailyLoadedEntry
		existing  dailyLoadedEntry
		want      bool
	}{
		{
			"higher cost wins token tie",
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, nil}, 0.06),
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, nil}, 0.00),
			true,
		},
		{
			"lower cost loses token tie",
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, nil}, 0.00),
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, nil}, 0.06),
			false,
		},
		{
			"speed decides only when cost ties",
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, &fast}, 0.06),
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 10, nil}, 0.06),
			true,
		},
		{
			"non-sidechain still beats sidechain regardless of cost",
			dailyEntryFixture(usageEntryFixture{"m", "r", false, 0, 1, nil}, 0.01),
			dailyEntryFixture(usageEntryFixture{"m", "r", true, 0, 100, nil}, 9.99),
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReplaceDedupedDailyEntry(&tt.candidate, &tt.existing); got != tt.want {
				t.Fatalf("shouldReplaceDedupedDailyEntry(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ---- deduper.Push: exact + sidechain buckets (ported from lib.rs tests) ----

func TestDeduperKeepsParentWhenSidechainReplaysWithNewRequestID(t *testing.T) {
	d := newDeduper()
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-parent", "req-parent", false, 20, 10, nil}))
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-parent", "req-sidechain-replay", true, 50_000, 10, nil}))
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-sidechain-answer", "req-sidechain-answer", true, 700, 30, nil}))

	if len(d.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(d.entries))
	}
	if got := *d.entries[0].Data.Message.ID; got != "msg-parent" {
		t.Errorf("entries[0].id = %q, want msg-parent", got)
	}
	if got := *d.entries[0].Data.RequestID; got != "req-parent" {
		t.Errorf("entries[0].requestId = %q, want req-parent", got)
	}
	if got := d.entries[0].Data.Message.Usage.CacheReadInputTokens; got != 20 {
		t.Errorf("entries[0].cacheRead = %d, want 20", got)
	}
	if got := *d.entries[1].Data.Message.ID; got != "msg-sidechain-answer" {
		t.Errorf("entries[1].id = %q, want msg-sidechain-answer", got)
	}
	if got := d.entries[1].Data.Message.Usage.CacheReadInputTokens; got != 700 {
		t.Errorf("entries[1].cacheRead = %d, want 700", got)
	}
}

// The reference re-registers the dedupe hash buckets after a replacement, so
// a later exact-key duplicate of the REPLACEMENT still finds the bucket.
func TestDeduperRefreshesIndexesWhenParentReplacesSidechainReplay(t *testing.T) {
	d := newDeduper()
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-parent", "req-sidechain-replay", true, 50_000, 10, nil}))
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-parent", "req-parent", false, 20, 10, nil}))
	d.Push(loadedUsageEntry(usageEntryFixture{"msg-parent", "req-parent", false, 5, 5, nil}))

	if len(d.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(d.entries))
	}
	if got := *d.entries[0].Data.RequestID; got != "req-parent" {
		t.Errorf("requestId = %q, want req-parent", got)
	}
	if got := d.entries[0].Data.Message.Usage.CacheReadInputTokens; got != 20 {
		t.Errorf("cacheRead = %d, want 20", got)
	}
}

func TestDeduperExactDuplicateRules(t *testing.T) {
	t.Run("later entry with more tokens replaces", func(t *testing.T) {
		d := newDeduper()
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r", false, 0, 25, nil}))
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r", false, 0, 250, nil}))
		if len(d.entries) != 1 || d.entries[0].Data.Message.Usage.OutputTokens != 250 {
			t.Fatalf("want single entry with 250 output tokens, got %+v", d.entries)
		}
	})
	t.Run("later entry with fewer tokens is dropped", func(t *testing.T) {
		d := newDeduper()
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r", false, 0, 200, nil}))
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r", false, 0, 50, nil}))
		if len(d.entries) != 1 || d.entries[0].Data.Message.Usage.OutputTokens != 200 {
			t.Fatalf("want single entry with 200 output tokens, got %+v", d.entries)
		}
	})
	t.Run("same message id with different request ids both kept", func(t *testing.T) {
		d := newDeduper()
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r1", false, 0, 10, nil}))
		d.Push(loadedUsageEntry(usageEntryFixture{"m", "r2", false, 0, 20, nil}))
		if len(d.entries) != 2 {
			t.Fatalf("entries = %d, want 2 (message bucket only dedups sidechains)", len(d.entries))
		}
	})
	t.Run("entry without message id is always kept", func(t *testing.T) {
		d := newDeduper()
		e1 := loadedUsageEntry(usageEntryFixture{"", "r", false, 0, 1, nil})
		e1.Data.Message.ID = nil
		e2 := e1
		d.Push(e1)
		d.Push(e2)
		if len(d.entries) != 2 {
			t.Fatalf("entries = %d, want 2", len(d.entries))
		}
	})
}

// ---- daily dedup: no bucket refresh on replacement (daily.rs semantics) ----

func TestDailyDeduperDoesNotRefreshIndexesOnReplacement(t *testing.T) {
	d := newDailyDeduper()
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-parent", "req-sidechain-replay", true, 50_000, 10, nil}, 1.0))
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-parent", "req-parent", false, 20, 10, nil}, 0.10))
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-parent", "req-parent", false, 5, 5, nil}, 0.05))

	// The entries pipeline keeps ONE entry here (refreshed buckets); the daily
	// pipeline keeps both because the exact bucket for req-parent was never
	// registered and the message bucket no longer matches (both non-sidechain).
	if len(d.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(d.entries))
	}
	if got := *d.entries[0].requestID; got != "req-parent" {
		t.Errorf("entries[0].requestId = %q, want req-parent", got)
	}
	if got := d.entries[0].usage.CacheReadInputTokens; got != 20 {
		t.Errorf("entries[0].cacheRead = %d, want 20", got)
	}
	if got := d.entries[1].usage.CacheReadInputTokens; got != 5 {
		t.Errorf("entries[1].cacheRead = %d, want 5", got)
	}
}

func TestDailyDeduperKeepsParentWhenSidechainReplays(t *testing.T) {
	d := newDailyDeduper()
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-parent", "req-parent", false, 20, 10, nil}, 0.10))
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-parent", "req-sidechain-replay", true, 50_000, 10, nil}, 9.99))
	d.Push(dailyEntryFixture(usageEntryFixture{"msg-sidechain-answer", "req-sidechain-answer", true, 700, 30, nil}, 0.30))

	if len(d.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(d.entries))
	}
	if got := *d.entries[0].messageID; got != "msg-parent" {
		t.Errorf("entries[0].messageId = %q, want msg-parent", got)
	}
	if got := d.entries[0].usage.CacheReadInputTokens; got != 20 {
		t.Errorf("entries[0].cacheRead = %d, want 20", got)
	}
	if got := d.entries[1].usage.CacheReadInputTokens; got != 700 {
		t.Errorf("entries[1].cacheRead = %d, want 700", got)
	}
}

// ---- hasUnsupportedNullField (ported from rejects/allows Rust tests) ----

func TestHasUnsupportedNullField(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"speed null rejected", `{"message":{"usage":{"speed":null}}}`, true},
		{"model null rejected", `{"message":{"model":null,"usage":{"input_tokens":0}}}`, true},
		{"sessionId null rejected", `{"sessionId":null,"message":{"usage":{"input_tokens":0}}}`, true},
		{"content null allowed", `{"message":{"content":null,"usage":{"input_tokens":0}}}`, false},
		{"id null rejected", `{"message":{"id":null,"usage":{"input_tokens":0}}}`, true},
		{"costUSD null rejected", `{"costUSD":null,"message":{"usage":{"input_tokens":0}}}`, true},
		{"cache_read null rejected", `{"message":{"usage":{"input_tokens":1,"cache_read_input_tokens":null}}}`, true},
		{"null in string value allowed", `{"message":{"model":"has:null inside","usage":{"input_tokens":0}}}`, false},
		{"no nulls", `{"message":{"usage":{"input_tokens":1,"output_tokens":2}}}`, false},
		{"second null field still found", `{"a":null,"message":{"model":null,"usage":{"input_tokens":0}}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasUnsupportedNullField([]byte(tt.line)); got != tt.want {
				t.Fatalf("hasUnsupportedNullField(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ---- isSemverPrefix ----

func TestIsSemverPrefix(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1.2.3", true},
		{"1.2.3-rc.1", true},
		{"12.34.56", true},
		{"1.2.0", true},
		{"not-semver", false},
		{"1.2", false},
		{"1.2.", false},
		{"1.2.x", false},
		{"v1.2.3", false},
		{"", false},
		{".1.2", false},
		{"1..2", false},
	}
	for _, tt := range tests {
		if got := isSemverPrefix(tt.value); got != tt.want {
			t.Errorf("isSemverPrefix(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// ---- extractSessionParts incl. subagents grandparent rule ----

func TestExtractSessionParts(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		wantSessionID   string
		wantProjectPath string
	}{
		{"modern session file", "/home/me/.claude/projects/project-a/session-a.jsonl", "session-a", "project-a"},
		{"nested chat file", "/home/me/.claude/projects/project-a/session-a/chat.jsonl", "session-a", "project-a"},
		{"subagents grandparent", "/home/me/.claude/projects/project-a/session-a/subagents/worker.jsonl", "session-a", "project-a"},
		{"deep nested project", "/home/me/.claude/projects/-Users-a-b/session-x/chat.jsonl", "session-x", "-Users-a-b"},
		{"file directly under projects", "/home/me/.claude/projects/loose.jsonl", "loose.jsonl", "Unknown Project"},
		{"no projects component", "/home/me/other/session-a/chat.jsonl", "session-a", "/home/me/other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, projectPath := ExtractSessionParts(tt.path)
			if sessionID != tt.wantSessionID {
				t.Errorf("sessionID = %q, want %q", sessionID, tt.wantSessionID)
			}
			if projectPath != tt.wantProjectPath {
				t.Errorf("projectPath = %q, want %q", projectPath, tt.wantProjectPath)
			}
		})
	}
}

func TestExtractProject(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/me/.claude/projects/project-a/session-a/chat.jsonl", "project-a"},
		{"/home/me/.claude/projects/project-b/session-b.jsonl", "project-b"},
		{"/no/projects/here.jsonl", "here.jsonl"},
		{"/home/x/other.jsonl", "unknown"},
	}
	for _, tt := range tests {
		if got := ExtractProject(tt.path); got != tt.want {
			t.Errorf("ExtractProject(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ---- advisor iteration extraction ----

func TestAdvisorUsagesFromLine(t *testing.T) {
	// Line from the reference main.rs advisor test: the advisor iteration sits
	// at raw index 1 of the iterations array, but ids use the FILTERED index.
	line := []byte(`{"timestamp":"2026-05-22T02:34:40.000Z","version":"1.2.3","sessionId":"session1","message":{"id":"msg_123","model":"claude-sonnet-4-20250514","usage":{"input_tokens":2,"output_tokens":491,"cache_creation_input_tokens":7853,"cache_read_input_tokens":226584,"iterations":[{"type":"message","input_tokens":1,"output_tokens":45,"cache_creation_input_tokens":7192,"cache_read_input_tokens":109696},{"type":"advisor_message","model":"claude-opus-4-20250514","input_tokens":159419,"output_tokens":7805,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},{"type":"message","input_tokens":1,"output_tokens":446,"cache_creation_input_tokens":661,"cache_read_input_tokens":116888}]}},"requestId":"req_456","costUSD":1.23}`)

	advisors := advisorUsagesFromLine(line)
	if len(advisors) != 1 {
		t.Fatalf("advisors = %d, want 1", len(advisors))
	}
	if advisors[0].Model != "claude-opus-4-20250514" {
		t.Errorf("model = %q, want claude-opus-4-20250514", advisors[0].Model)
	}
	if advisors[0].Usage.InputTokens != 159419 || advisors[0].Usage.OutputTokens != 7805 {
		t.Errorf("usage = %+v, want input 159419 output 7805", advisors[0].Usage)
	}

	t.Run("filtered index used for id suffix", func(t *testing.T) {
		lf := readUsageFileForTest(t, line)
		if len(lf.entries) != 2 {
			t.Fatalf("entries = %d, want 2 (parent + advisor)", len(lf.entries))
		}
		if got := *lf.entries[1].Data.Message.ID; got != "msg_123:advisor:0" {
			t.Errorf("advisor id = %q, want msg_123:advisor:0", got)
		}
		if got := *lf.entries[1].Model; got != "claude-opus-4-20250514" {
			t.Errorf("advisor model = %q, want claude-opus-4-20250514", got)
		}
		if lf.entries[1].Data.CostUSD != nil {
			t.Errorf("advisor costUSD = %v, want nil", *lf.entries[1].Data.CostUSD)
		}
		if got := *lf.entries[1].Data.Message.ID; got != "msg_123:advisor:0" && len(advisorUsagesFromLine(line)) != 1 {
			t.Fatalf("inconsistent advisor extraction")
		}
	})

	t.Run("two advisors get distinct filtered indexes", func(t *testing.T) {
		two := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"claude-sonnet-4-20250514","usage":{"input_tokens":1,"output_tokens":1,"iterations":[{"type":"message","input_tokens":1,"output_tokens":1},{"type":"advisor_message","model":"adv-one","input_tokens":10,"output_tokens":1},{"type":"advisor_message","model":"adv-two","input_tokens":20,"output_tokens":2}]}},"requestId":"r","costUSD":0.5}`)
		lf := readUsageFileForTest(t, two)
		if len(lf.entries) != 3 {
			t.Fatalf("entries = %d, want 3", len(lf.entries))
		}
		if got := *lf.entries[1].Data.Message.ID; got != "m:advisor:0" {
			t.Errorf("advisor 1 id = %q, want m:advisor:0", got)
		}
		if got := *lf.entries[2].Data.Message.ID; got != "m:advisor:1" {
			t.Errorf("advisor 2 id = %q, want m:advisor:1", got)
		}
	})

	t.Run("advisor with null or empty model filtered out", func(t *testing.T) {
		line := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"claude-sonnet-4-20250514","usage":{"input_tokens":1,"output_tokens":1,"iterations":[{"type":"advisor_message","input_tokens":5,"output_tokens":5},{"type":"advisor_message","model":"","input_tokens":6,"output_tokens":6},{"type":"advisor_message","model":"adv-ok","input_tokens":7,"output_tokens":7}]}},"requestId":"r","costUSD":0.5}`)
		advisors := advisorUsagesFromLine(line)
		if len(advisors) != 1 || advisors[0].Model != "adv-ok" {
			t.Fatalf("advisors = %+v, want only adv-ok", advisors)
		}
	})

	t.Run("malformed iteration discards all advisors", func(t *testing.T) {
		line := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"claude-sonnet-4-20250514","usage":{"input_tokens":1,"output_tokens":1,"iterations":[{"type":"advisor_message","model":"adv-one","output_tokens":1},{"type":"advisor_message","model":"adv-two","input_tokens":2,"output_tokens":2}]}},"requestId":"r","costUSD":0.5}`)
		if advisors := advisorUsagesFromLine(line); len(advisors) != 0 {
			t.Fatalf("advisors = %d, want 0 (iteration missing input_tokens)", len(advisors))
		}
	})

	t.Run("no advisor marker returns nothing", func(t *testing.T) {
		line := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"claude-sonnet-4-20250514","usage":{"input_tokens":1,"output_tokens":1,"iterations":[{"type":"message","input_tokens":1,"output_tokens":1}]}},"requestId":"r","costUSD":0.5}`)
		if advisors := advisorUsagesFromLine(line); len(advisors) != 0 {
			t.Fatalf("advisors = %d, want 0", len(advisors))
		}
	})
}

// ---- decodeUsageEntry serde-shaped strictness ----

func TestDecodeUsageEntry(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"valid entry", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"id":"m","model":"mod","usage":{"input_tokens":1,"output_tokens":2}},"requestId":"r"}`, true},
		{"missing message", `{"timestamp":"2026-01-05T10:00:00.000Z","usage":{"input_tokens":1}}`, false},
		{"message null", `{"timestamp":"2026-01-05T10:00:00.000Z","message":null,"requestId":"r"}`, false},
		{"missing usage", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"id":"m","model":"mod"}}`, false},
		{"usage missing input_tokens", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"output_tokens":2}}}`, false},
		{"usage missing output_tokens", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":2}}}`, false},
		{"input_tokens null", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":null,"output_tokens":2}}}`, false},
		{"missing timestamp", `{"message":{"id":"m","usage":{"input_tokens":1,"output_tokens":2}}}`, false},
		{"timestamp wrong type", `{"timestamp":5,"message":{"usage":{"input_tokens":1,"output_tokens":2}}}`, false},
		{"unknown speed variant", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":1,"output_tokens":2,"speed":"turbo"}}}`, false},
		{"standard speed ok", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":1,"output_tokens":2,"speed":"standard"}}}`, true},
		{"fast speed ok", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":1,"output_tokens":2,"speed":"fast"}}}`, true},
		{"negative tokens rejected", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":-1,"output_tokens":2}}}`, false},
		{"cache_creation object ok", `{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":1,"output_tokens":2,"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":4}}}}`, true},
		{"garbage json", `{"timestamp":`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry core.UsageEntry
			if got := decodeUsageEntry([]byte(tt.line), &entry); got != tt.want {
				t.Fatalf("decodeUsageEntry = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- decodeDailyLine: direct and agent-progress variants ----

func TestDecodeDailyLine(t *testing.T) {
	t.Run("direct entry", func(t *testing.T) {
		line := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"mod","usage":{"input_tokens":1,"output_tokens":2}},"requestId":"r","costUSD":0.5}`)
		entry, ok := decodeDailyLine(line)
		if !ok {
			t.Fatal("decode failed, want direct entry")
		}
		if *entry.Timestamp != "2026-01-05T10:00:00.000Z" || *entry.Message.ID != "m" {
			t.Fatalf("entry = %+v", entry)
		}
		if entry.CostUSD == nil || *entry.CostUSD != 0.5 {
			t.Fatalf("costUSD = %v, want 0.5", entry.CostUSD)
		}
	})

	t.Run("agent progress entry propagates sidechain metadata", func(t *testing.T) {
		line := []byte(`{"data":{"message":{"timestamp":"2026-03-29T07:00:00.000Z","requestId":"req-sidechain","isSidechain":true,"message":{"usage":{"input_tokens":0,"output_tokens":10,"cache_read_input_tokens":20},"model":"claude-sonnet-4-20250514","id":"msg-sidechain"}}}}`)
		entry, ok := decodeDailyLine(line)
		if !ok {
			t.Fatal("decode failed, want agent progress entry")
		}
		if entry.IsSidechain == nil || !*entry.IsSidechain {
			t.Fatalf("isSidechain = %v, want true", entry.IsSidechain)
		}
		if *entry.Timestamp != "2026-03-29T07:00:00.000Z" {
			t.Fatalf("timestamp = %q", *entry.Timestamp)
		}
		if *entry.Message.ID != "msg-sidechain" || *entry.RequestID != "req-sidechain" {
			t.Fatalf("entry = %+v", entry)
		}
		if entry.Version != nil || entry.SessionID != nil {
			t.Fatalf("progress entries must clear version/sessionId, got %+v", entry)
		}
	})

	t.Run("neither variant matches", func(t *testing.T) {
		line := []byte(`{"timestamp":"2026-01-05T10:00:00.000Z","message":{"usage":{"input_tokens":1}}}`)
		if _, ok := decodeDailyLine(line); ok {
			t.Fatal("decode succeeded, want failure")
		}
	})
}

// ---- readUsageFile line pipeline on a temp file ----

func readUsageFileForTest(t *testing.T, lines ...[]byte) loadedFile {
	t.Helper()
	path := t.TempDir() + "/chat.jsonl"
	content := make([]byte, 0, 1024)
	for _, line := range lines {
		content = append(content, line...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return readUsageFile(path, nil, core.ModeAuto, nil)
}

func TestReadUsageFileFiltersInvalidLines(t *testing.T) {
	lf := readUsageFileForTest(t,
		[]byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m1","model":"claude-sonnet-4-20250514","usage":{"input_tokens":1,"output_tokens":2}},"requestId":"r1","costUSD":0.1}`),
		[]byte(`{"timestamp":"2026-01-05T10:00:01.000Z","version":"not-semver","sessionId":"s","message":{"id":"m2","model":"claude-sonnet-4-20250514","usage":{"input_tokens":999,"output_tokens":999}},"requestId":"r2","costUSD":9}`),
		[]byte(`{"timestamp":"2026-01-05T10:00:02.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m3","model":null,"usage":{"input_tokens":50,"output_tokens":50}},"requestId":"r3","costUSD":9}`),
		[]byte(`{"timestamp":"2026-01-05T10:00:03.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m4","model":"claude-sonnet-4-20250514","usage":{"input_tokens":3,"output_tokens":4,"speed":"fast"}},"requestId":"r4","costUSD":0.2}`),
	)
	if len(lf.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(lf.entries))
	}
	if got := *lf.entries[1].Model; got != "claude-sonnet-4-20250514-fast" {
		t.Errorf("fast model suffix = %q, want claude-sonnet-4-20250514-fast", got)
	}
	ts, ok := core.ParseTSTimestamp("2026-01-05T10:00:00.000Z")
	if !ok || lf.timestamp == nil || *lf.timestamp != ts {
		t.Errorf("file timestamp = %v, want %d", lf.timestamp, ts)
	}
}

func TestReadUsageFileSyntheticModelEntry(t *testing.T) {
	lf := readUsageFileForTest(t,
		[]byte(`{"timestamp":"2026-01-05T10:00:00.000Z","version":"1.2.3","sessionId":"s","message":{"id":"m","model":"<synthetic>","usage":{"input_tokens":7,"output_tokens":3}},"requestId":"r","costUSD":0.02}`),
	)
	if len(lf.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(lf.entries))
	}
	if lf.entries[0].Model != nil {
		t.Errorf("synthetic model = %v, want nil (counted but not listed)", *lf.entries[0].Model)
	}
	if lf.entries[0].Data.Message.Usage.InputTokens != 7 {
		t.Errorf("synthetic usage dropped: %+v", lf.entries[0].Data.Message.Usage)
	}
}
