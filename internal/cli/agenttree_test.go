package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// The framework's test surface (ADR 0010): the option parser's reference
// messages, the support matrix as single source, and the command-tree shape
// every agent derives from it. The agents without golden coverage
// (codex/gemini/kimi/qwen/openclaw/zcode) lean on these.

func newTestFlagState() (*sharedFlags, *agentFlagState) {
	return &sharedFlags{shared: &core.SharedArgs{}}, &agentFlagState{}
}

func TestParseAgentOptionsRejectsBareWords(t *testing.T) {
	f, st := newTestFlagState()
	err := parseAgentOptions(agentOptionTable(nil), f, st, []string{"daily"})
	if err == nil || !strings.Contains(err.Error(), "Expected option, got 'daily'") {
		t.Fatalf("err = %v, want Expected option", err)
	}
}

func TestParseAgentOptionsUnknownOption(t *testing.T) {
	f, st := newTestFlagState()
	err := parseAgentOptions(agentOptionTable(nil), f, st, []string{"--bogus"})
	if err == nil || !strings.Contains(err.Error(), "Unknown option '--bogus'") {
		t.Fatalf("err = %v, want Unknown option", err)
	}
}

func TestParseAgentOptionsMissingValue(t *testing.T) {
	f, st := newTestFlagState()
	err := parseAgentOptions(agentOptionTable(nil), f, st, []string{"--since"})
	if err == nil || !strings.Contains(err.Error(), "Missing value for --since") {
		t.Fatalf("err = %v, want Missing value for --since", err)
	}
	err = parseAgentOptions(agentOptionTable(nil), f, st, []string{"--since="})
	if err == nil || !strings.Contains(err.Error(), "Missing value for --since") {
		t.Fatalf("inline empty err = %v, want Missing value for --since", err)
	}
}

func TestParseAgentOptionsInlineAndSeparateValues(t *testing.T) {
	f, st := newTestFlagState()
	if err := parseAgentOptions(agentOptionTable(nil), f, st, []string{"--since=2026-01-01", "--until", "2026-02-01", "-j"}); err != nil {
		t.Fatal(err)
	}
	if f.since != "2026-01-01" || f.until != "2026-02-01" || !f.shared.JSON {
		t.Fatalf("since=%q until=%q json=%v", f.since, f.until, f.shared.JSON)
	}
}

func TestParseAgentOptionsBareDefault(t *testing.T) {
	// codex --speed without a value takes the reference's NoOptDefVal ("auto"),
	// at the end or ahead of another flag; an explicit value still wins.
	extras := []agentExtraOption{
		{long: "--speed", bareDefault: "auto", store: func(st *agentFlagState, v string) { st.set("--speed", v) }},
	}
	for _, args := range [][]string{{"--speed"}, {"--speed", "--json"}} {
		f, st := newTestFlagState()
		if err := parseAgentOptions(agentOptionTable(extras), f, st, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got := st.get("--speed"); got != "auto" {
			t.Fatalf("%v: speed = %q, want auto", args, got)
		}
	}
	f, st := newTestFlagState()
	if err := parseAgentOptions(agentOptionTable(extras), f, st, []string{"--speed", "fast"}); err != nil {
		t.Fatal(err)
	}
	if got := st.get("--speed"); got != "fast" {
		t.Fatalf("speed = %q, want fast", got)
	}
}

func TestParseAgentOptionsVersionAndHelp(t *testing.T) {
	for _, arg := range []string{"-v", "-V", "--version"} {
		_, st := newTestFlagState()
		if err := parseAgentOptions(agentOptionTable(nil), &sharedFlags{shared: &core.SharedArgs{}}, st, []string{arg}); err != nil {
			t.Fatal(err)
		}
		if !st.version {
			t.Fatalf("%s: version not set", arg)
		}
	}
}

func TestAgentReportKindsMatrix(t *testing.T) {
	if got := agentReportKinds("opencode"); len(got) != 4 || got[1] != core.KindWeekly {
		t.Errorf("opencode kinds = %v, want daily/weekly/monthly/session", got)
	}
	if got := agentReportKinds("claude"); len(got) != 4 || got[1] != core.KindWeekly {
		t.Errorf("claude kinds = %v, want daily/weekly/monthly/session", got)
	}
	if got := agentReportKinds("zcode"); len(got) != 3 {
		t.Errorf("zcode kinds = %v, want daily/monthly/session", got)
	}
	if agentReportSupported("opencode", "weekly") != true {
		t.Error("opencode weekly should be supported")
	}
	if agentReportSupported("droid", "weekly") != false {
		t.Error("droid weekly should not be supported")
	}
	if agentReportSupported("claude", "blocks") != true || agentReportSupported("zcode", "statusline") != false {
		t.Error("blocks/statusline are claude-only")
	}
}

// TestAgentCommandTreeMatchesMatrix builds every registered agent command and
// checks its subcommand set against the support matrix — the drift tripwire
// for agents with and without golden coverage alike.
func TestAgentCommandTreeMatchesMatrix(t *testing.T) {
	commands := map[string][]string{}
	for _, factory := range agentCommandFactories {
		cmd := factory()
		var subs []string
		for _, sub := range cmd.Commands() {
			subs = append(subs, sub.Name())
		}
		commands[cmd.Name()] = subs
	}
	for _, agent := range agentNames {
		if agent == "claude" {
			// claude's tree (blocks/statusline/config) is its own command, not
			// built by the agent framework.
			continue
		}
		subs, ok := commands[agent]
		if !ok {
			t.Errorf("agent %q has no registered command", agent)
			continue
		}
		var want []string
		for _, kind := range agentReportKinds(agent) {
			want = append(want, kind.String())
		}
		sort.Strings(subs)
		sort.Strings(want)
		if strings.Join(subs, ",") != strings.Join(want, ",") {
			t.Errorf("%s subcommands = %v, want %v", agent, subs, want)
		}
	}
}

func TestUnsupportedAgentReportErrorMessage(t *testing.T) {
	err := unsupportedAgentReportError("droid", "Droid", "weekly")
	if err == nil || !strings.Contains(err.Error(), `The "weekly" report is not available for Droid usage`) {
		t.Fatalf("generic err = %v", err)
	}
	err = unsupportedAgentReportError("droid", "Droid", "blocks")
	if err == nil || !strings.Contains(err.Error(), `The "blocks" report is only available for Claude Code usage`) {
		t.Fatalf("claude-only err = %v", err)
	}
}
