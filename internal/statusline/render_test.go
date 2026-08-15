package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixedNow anchors the injected clock: 2026-06-15T12:00:00Z.
var fixedNow = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC).UnixMilli()

var entrySeq int

func withClock(t *testing.T, now int64) {
	t.Helper()
	previous := nowFunc
	nowFunc = func() int64 { return now }
	t.Cleanup(func() { nowFunc = previous })
	entrySeq = 0
}

// writeClaudeData creates a CLAUDE_CONFIG_DIR with one session JSONL file and
// points the environment at it.
func writeClaudeData(t *testing.T, sessionID, content string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "proj-x", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	return root
}

func entryLine(t time.Time, model string, input, output, cacheCreate, cacheRead uint64) string {
	entrySeq++
	return `{"timestamp":"` + t.UTC().Format("2006-01-02T15:04:05.000Z") +
		`","sessionId":"sess","message":{"id":"m-` + strconv.Itoa(entrySeq) + `","model":"` + model +
		`","usage":{"input_tokens":` + strconv.FormatUint(input, 10) +
		`,"output_tokens":` + strconv.FormatUint(output, 10) +
		`,"cache_creation_input_tokens":` + strconv.FormatUint(cacheCreate, 10) +
		`,"cache_read_input_tokens":` + strconv.FormatUint(cacheRead, 10) + `}},"version":"1.0.0"}`
}

func baseArgs() *Args {
	return &Args{
		Offline:             true,
		VisualBurnRate:      BurnRateOff,
		CostSource:          CostSourceAuto,
		Cache:               true,
		RefreshInterval:     1,
		ContextLowThreshold: 50,
		ContextMediumThresh: 80,
	}
}

func utcTZ() *string {
	tz := "UTC"
	return &tz
}

func TestRenderBasicLineWithoutData(t *testing.T) {
	withClock(t, fixedNow)
	writeClaudeData(t, "no-sessions", "")
	hook := &Hook{
		SessionID:      "missing",
		TranscriptPath: "/nonexistent/transcript.jsonl",
		Model:          HookModel{DisplayName: "Sonnet 4"},
		ContextWindow:  &HookContext{TotalInputTokens: 100000, ContextWindowSize: 200000},
	}
	args := baseArgs()
	args.Timezone = utcTZ()
	got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	want := "🤖 Sonnet 4 | 💰 $-0.00 session / $-0.00 today / No active block | 🧠 100,000 (50%)"
	if got != want {
		t.Errorf("render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderCostSources(t *testing.T) {
	withClock(t, fixedNow)
	at := time.UnixMilli(fixedNow).UTC()
	noon := at.Add(-30 * time.Minute)
	data := entryLine(noon, "claude-sonnet-4-20250514", 1000, 500, 0, 0) + "\n" +
		entryLine(noon.Add(10*time.Minute), "claude-sonnet-4-20250514", 1000, 500, 0, 0) + "\n"
	writeClaudeData(t, "sess", data)

	newHook := func() *Hook {
		return &Hook{
			SessionID:      "sess",
			TranscriptPath: "/nonexistent",
			Model:          HookModel{ID: strPtr("claude-sonnet-4-20250514"), DisplayName: "Sonnet 4"},
		}
	}
	// ccusage-computed cost: 2 x (1000*3e-6 + 500*15e-6) = $0.02.
	cases := []struct {
		name       string
		source     CostSource
		hookCost   *HookCost
		wantPrefix string
	}{
		{"auto with hook cost", CostSourceAuto, &HookCost{TotalCostUSD: 1.5}, "$1.50 session"},
		{"auto without hook cost", CostSourceAuto, nil, "$0.02 session"},
		{"cc", CostSourceCc, &HookCost{TotalCostUSD: 2.25}, "$2.25 session"},
		{"cc without hook cost", CostSourceCc, nil, "N/A session"},
		{"ccusage", CostSourceCcusage, &HookCost{TotalCostUSD: 9.99}, "$0.02 session"},
		{"both", CostSourceBoth, &HookCost{TotalCostUSD: 2.25}, "($2.25 cc / $0.02 ccusage) session"},
		{"both without hook cost", CostSourceBoth, nil, "(N/A cc / $0.02 ccusage) session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := baseArgs()
			args.Timezone = utcTZ()
			args.CostSource = tc.source
			hook := newHook()
			hook.Cost = tc.hookCost
			got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tc.wantPrefix) {
				t.Errorf("cost segment: got %q, want it to contain %q", got, tc.wantPrefix)
			}
		})
	}
}

func TestRenderTodayCost(t *testing.T) {
	withClock(t, fixedNow)
	at := time.UnixMilli(fixedNow).UTC()
	todayEntry := entryLine(at.Add(-2*time.Hour), "claude-sonnet-4-20250514", 400000, 100000, 0, 0) + "\n"
	// Yesterday's entry must not count.
	yesterday := entryLine(at.Add(-26*time.Hour), "claude-sonnet-4-20250514", 400000, 100000, 0, 0) + "\n"
	writeClaudeData(t, "sess", todayEntry+yesterday)

	args := baseArgs()
	args.Timezone = utcTZ()
	hook := &Hook{
		SessionID:      "sess",
		TranscriptPath: "/nonexistent",
		Model:          HookModel{DisplayName: "D"},
		ContextWindow:  &HookContext{TotalInputTokens: 1, ContextWindowSize: 200},
	}
	got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	// Long-context tiered pricing applies above 200k input tokens, so the
	// exact figure comes from the pricing table; what matters is that ONLY
	// today's entry counts. Compute the session total for comparison: the
	// session filter matches both entries, today's filter only one.
	if !strings.Contains(got, "today") {
		t.Fatalf("malformed line: %q", got)
	}
	sessionCost, todayCost := parseCosts(t, got)
	if sessionCost == todayCost {
		t.Errorf("yesterday's entry leaked into today cost: %q", got)
	}
	if todayCost <= 0 {
		t.Errorf("today cost missing: %q", got)
	}
}

func parseCosts(t *testing.T, line string) (session, today float64) {
	t.Helper()
	marker := "💰 "
	rest := line[strings.Index(line, marker)+len(marker):]
	segments := strings.Split(rest, " ")
	// "$X session / $Y today / ..." or "(... ) session / $Y today / ..."
	for i, seg := range segments {
		if seg == "session" && i > 0 {
			session = parseUSD(t, segments[i-1])
		}
		if seg == "today" && i > 0 {
			today = parseUSD(t, segments[i-1])
			return
		}
	}
	t.Fatalf("no cost segments in %q", line)
	return
}

func parseUSD(t *testing.T, s string) float64 {
	t.Helper()
	s = strings.TrimSuffix(strings.TrimPrefix(s, "$"), ")")
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		t.Fatalf("bad cost %q: %v", s, err)
	}
	return v
}

func TestRenderBlockAndBurnRateModes(t *testing.T) {
	withClock(t, fixedNow)
	at := time.UnixMilli(fixedNow).UTC()
	// Two entries 30 minutes apart inside the current 5h window: an active
	// block spanning the current hour boundary (floor-to-hour start).
	start := at.Add(-50 * time.Minute)
	data := entryLine(start, "claude-sonnet-4-20250514", 30000, 10000, 0, 0) + "\n" +
		entryLine(start.Add(30*time.Minute), "claude-sonnet-4-20250514", 30000, 10000, 0, 0) + "\n"
	writeClaudeData(t, "sess", data)

	render := func(mode VisualBurnRate) string {
		args := baseArgs()
		args.Timezone = utcTZ()
		args.VisualBurnRate = mode
		args.CostSource = CostSourceCc
		hook := &Hook{
			SessionID:      "sess",
			TranscriptPath: "/nonexistent",
			Model:          HookModel{DisplayName: "D"},
			ContextWindow:  &HookContext{TotalInputTokens: 1, ContextWindowSize: 1000},
		}
		got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	off := render(BurnRateOff)
	// Block cost: 60000 input * 3e-6 + 20000 output * 15e-6 = 0.18 + 0.30 = $0.48.
	if !strings.Contains(off, "$0.48 block (") {
		t.Errorf("block segment missing in %q", off)
	}
	// Off mode still shows the plain cost/hr burn; only the indicator is gone.
	if !strings.Contains(off, "🔥 $0.96/hr |") {
		t.Errorf("plain burn missing in off mode: %q", off)
	}
	for _, indicator := range []string{"🟢", "⚠️", "🚨", "(Normal)", "(Moderate)", "(High)"} {
		if strings.Contains(off, indicator) {
			t.Errorf("indicator %q leaked into off mode: %q", indicator, off)
		}
	}
	// Block start floors to 11:00Z, end 16:00Z, now 12:00Z -> 4h left.
	if !strings.Contains(off, "(4h 0m left)") {
		t.Errorf("remaining time: %q", off)
	}
	emoji := render(BurnRateEmoji)
	// Non-cache tokens 80000 over 30 minutes = 2666.7/min -> Moderate (⚠️).
	if !strings.Contains(emoji, "🔥 $0.96/hr ⚠️ |") {
		t.Errorf("emoji mode: %q", emoji)
	}
	if strings.Contains(emoji, "(Moderate)") {
		t.Errorf("text leaked into emoji mode: %q", emoji)
	}
	text := render(BurnRateText)
	if !strings.Contains(text, "🔥 $0.96/hr (Moderate) |") || strings.Contains(text, "⚠️") {
		t.Errorf("text mode: %q", text)
	}
	both := render(BurnRateEmojiText)
	if !strings.Contains(both, "🔥 $0.96/hr ⚠️ (Moderate) |") {
		t.Errorf("emoji-text mode: %q", both)
	}
}

func TestBurnIndicatorThresholds(t *testing.T) {
	// burn = non-cache tokens / elapsed minutes; thresholds 2000 and 5000.
	cases := []struct {
		name        string
		nonCache    uint64
		minutes     int
		wantEmojTxt string
	}{
		{"just below 2000", 19999, 10, "🟢 (Normal)"},
		{"exactly 2000", 20000, 10, "⚠️ (Moderate)"},
		{"just below 5000", 49999, 10, "⚠️ (Moderate)"},
		{"exactly 5000", 50000, 10, "🚨 (High)"},
		{"above 5000", 80000, 10, "🚨 (High)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withClock(t, fixedNow)
			at := time.UnixMilli(fixedNow).UTC()
			start := at.Add(-40 * time.Minute)
			first := entryLine(start, "claude-sonnet-4-20250514", tc.nonCache/2, tc.nonCache/2, 0, 0) + "\n" +
				entryLine(start.Add(time.Duration(tc.minutes)*time.Minute), "claude-sonnet-4-20250514", 0, 0, 0, 0) + "\n"
			writeClaudeData(t, "sess", first)
			args := baseArgs()
			args.Timezone = utcTZ()
			args.VisualBurnRate = BurnRateEmojiText
			args.CostSource = CostSourceCc
			hook := &Hook{
				SessionID:      "sess",
				TranscriptPath: "/nonexistent",
				Model:          HookModel{DisplayName: "D"},
				ContextWindow:  &HookContext{TotalInputTokens: 1, ContextWindowSize: 1000},
			}
			got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tc.wantEmojTxt) {
				t.Errorf("indicator: got %q, want %q", got, tc.wantEmojTxt)
			}
		})
	}
}

func TestContextThresholdColors(t *testing.T) {
	// Percentages are rounded before the threshold comparison.
	cases := []struct {
		tokens uint64
		want   string
	}{
		{98000, "🧠 98,000 (49%)"},   // 49.0 -> green (below low)
		{99999, "🧠 99,999 (50%)"},   // 49.9995 rounds to 50 -> yellow
		{160000, "🧠 160,000 (80%)"}, // exactly 80 -> still yellow
		{161000, "🧠 161,000 (81%)"}, // 80.5 rounds to 81 -> red
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			withClock(t, fixedNow)
			writeClaudeData(t, "sess", "")
			args := baseArgs()
			args.Timezone = utcTZ()
			hook := &Hook{
				SessionID:      "sess",
				TranscriptPath: "/nonexistent",
				Model:          HookModel{DisplayName: "D"},
				ContextWindow:  &HookContext{TotalInputTokens: tc.tokens, ContextWindowSize: 200000},
			}
			got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(got, tc.want) {
				t.Errorf("context: got %q, want suffix %q", got, tc.want)
			}
		})
	}
}

func TestContextThresholdsCustom(t *testing.T) {
	withClock(t, fixedNow)
	writeClaudeData(t, "sess", "")
	args := baseArgs()
	args.Timezone = utcTZ()
	args.ContextLowThreshold = 20
	args.ContextMediumThresh = 40
	hook := &Hook{
		SessionID:      "sess",
		TranscriptPath: "/nonexistent",
		Model:          HookModel{DisplayName: "D"},
		ContextWindow:  &HookContext{TotalInputTokens: 45000, ContextWindowSize: 100000},
	}
	got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "🧠 45,000 (45%)") {
		t.Errorf("custom thresholds: %q", got)
	}
}

func TestRenderContextFromTranscript(t *testing.T) {
	withClock(t, fixedNow)
	writeClaudeData(t, "sess", "")
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	content := strings.Join([]string{
		`{"type":"user","message":{"usage":{"input_tokens":999}}}`,
		`not json`,
		`{"type":"assistant","message":{"usage":{"input_tokens":1000,"output_tokens":999}}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":2000,"cache_creation_input_tokens":100,"cache_read_input_tokens":50,"output_tokens":888}}}`,
	}, "\n")
	if err := os.WriteFile(transcript, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	args := baseArgs()
	args.Timezone = utcTZ()
	hook := &Hook{
		SessionID:      "sess",
		TranscriptPath: transcript,
		Model:          HookModel{DisplayName: "D"},
	}
	got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "🧠 2,150 (1%)") {
		t.Errorf("transcript context: %q", got)
	}
}

func TestRenderContextMissingTranscript(t *testing.T) {
	withClock(t, fixedNow)
	writeClaudeData(t, "sess", "")
	args := baseArgs()
	args.Timezone = utcTZ()
	hook := &Hook{
		SessionID:      "sess",
		TranscriptPath: "/nonexistent/transcript.jsonl",
		Model:          HookModel{DisplayName: "D"},
	}
	got, err := renderStatusline(hook, args, statuslineShared(args), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "🧠 N/A") {
		t.Errorf("missing transcript: %q", got)
	}
}

func TestRenderModelSegment(t *testing.T) {
	aliases := map[string]string{
		"arn:aws:bedrock:ap-northeast-1:012345678910:application-inference-profile/abcde12345": "claude-opus-4-6",
	}
	if got := resolveModelLabel(aliases, "Opus 4.1"); got != "Opus 4.1" {
		t.Errorf("fallback label = %q", got)
	}
	arn := "arn:aws:bedrock:ap-northeast-1:012345678910:application-inference-profile/abcde12345"
	if got := resolveModelLabel(aliases, arn); got != "claude-opus-4-6" {
		t.Errorf("alias label = %q", got)
	}
	if got := formatModelSegment("Fable 5", nil); got != "Fable 5" {
		t.Errorf("no effort: %q", got)
	}
	if got := formatModelSegment("Fable 5", &HookEffort{Level: ""}); got != "Fable 5" {
		t.Errorf("empty effort: %q", got)
	}
	if got := formatModelSegment("Fable 5", &HookEffort{Level: "high"}); got != "Fable 5 (high)" {
		t.Errorf("effort: %q", got)
	}
}

func TestStatuslineSharedOfflineSemantics(t *testing.T) {
	// offline default: -O default true, no --no-offline -> offline.
	args := baseArgs()
	shared := statuslineShared(args)
	if !shared.OfflineEffective() {
		t.Errorf("default statusline should resolve offline")
	}
	// --no-offline wins even with CCUSAGE_OFFLINE set in the environment.
	t.Setenv("CCUSAGE_OFFLINE", "1")
	args = baseArgs()
	args.Offline = true
	args.NoOffline = true
	shared = statuslineShared(args)
	if shared.OfflineEffective() {
		t.Errorf("--no-offline should disable offline despite CCUSAGE_OFFLINE")
	}
}

func strPtr(s string) *string { return &s }
