package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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

// builtAgentCommand returns the named agent's fresh command tree.
func builtAgentCommand(t *testing.T, agent string) *cobra.Command {
	t.Helper()
	for _, factory := range agentCommandFactories {
		cmd := factory()
		if cmd.Name() == agent {
			return cmd
		}
	}
	t.Fatalf("agent %q has no registered command", agent)
	return nil
}

// TestAgentSubcommandShortWording pins the pre-framework Short lines: codex
// and opencode say "token usage grouped by day/…", every other agent keeps
// the generic "usage grouped by date" wording.
func TestAgentSubcommandShortWording(t *testing.T) {
	want := map[string]map[string]string{
		"codex": {
			"daily":   "Show Codex token usage grouped by day",
			"monthly": "Show Codex token usage grouped by month",
			"session": "Show Codex token usage grouped by session",
		},
		"opencode": {
			"daily":   "Show OpenCode token usage grouped by day",
			"weekly":  "Show OpenCode token usage grouped by week",
			"monthly": "Show OpenCode token usage grouped by month",
			"session": "Show OpenCode token usage grouped by session",
		},
		"amp": {
			"daily":   "Show Amp token usage grouped by day",
			"monthly": "Show Amp token usage grouped by month",
			"session": "Show Amp token usage grouped by session",
		},
	}
	for agent, subs := range want {
		cmd := builtAgentCommand(t, agent)
		for name, short := range subs {
			sub, _, err := cmd.Find([]string{name})
			if err != nil {
				t.Fatalf("%s %s: %v", agent, name, err)
			}
			if sub.Short != short {
				t.Errorf("%s %s Short = %q, want %q", agent, name, sub.Short, short)
			}
		}
	}
	zcode := builtAgentCommand(t, "zcode")
	sub, _, err := zcode.Find([]string{"daily"})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Short != "Show ZCode usage grouped by date" {
		t.Errorf("zcode daily Short = %q, want the generic wording", sub.Short)
	}
}

// TestAgentParentShortWording pins each agent parent's Short line to the
// pre-framework values.
func TestAgentParentShortWording(t *testing.T) {
	want := map[string]string{
		"codex":    "Usage reports for codex.",
		"opencode": "Usage reports for opencode.",
		"amp":      "Show Amp token usage commands",
		"codebuff": "Usage reports for codebuff.",
		"droid":    "Usage reports for droid.",
		"goose":    "Usage reports for goose.",
		"hermes":   "Usage reports for hermes.",
		"kilo":     "Usage reports for kilo.",
		"pi":       "Usage reports for pi.",
		"zcode":    "Show ZCode usage commands",
		"copilot":  "Show GitHub Copilot CLI usage commands",
		"gemini":   "Show Gemini CLI usage commands",
		"openclaw": "Show OpenClaw usage commands",
		"qwen":     "Show Qwen usage commands",
		"kimi":     "Show Kimi usage commands",
	}
	for agent, short := range want {
		if got := builtAgentCommand(t, agent).Short; got != short {
			t.Errorf("%s Short = %q, want %q", agent, got, short)
		}
	}
}

// TestCodexSpeedFlagRegisteredForHelp pins --speed's cobra registration
// (parsing stays manual): HEAD rendered it on the parent and every
// subcommand with NoOptDefVal=auto, while --pi-path rendered on the pi
// parent only and --open-claw-path never rendered.
func TestCodexSpeedFlagRegisteredForHelp(t *testing.T) {
	check := func(c *cobra.Command, where string) {
		flag := c.Flags().Lookup("speed")
		if flag == nil {
			t.Errorf("%s: --speed not registered for --help", where)
			return
		}
		if flag.NoOptDefVal != "auto" {
			t.Errorf("%s: --speed NoOptDefVal = %q, want auto", where, flag.NoOptDefVal)
		}
		if !strings.Contains(flag.Usage, "Cost speed tier") {
			t.Errorf("%s: --speed usage = %q, want the Cost speed tier text", where, flag.Usage)
		}
	}
	codex := builtAgentCommand(t, "codex")
	check(codex, "codex")
	for _, sub := range codex.Commands() {
		check(sub, "codex "+sub.Name())
	}
	pi := builtAgentCommand(t, "pi")
	if flag := pi.Flags().Lookup("pi-path"); flag == nil || !strings.Contains(flag.Usage, "Path to pi agent sessions directory") {
		t.Error("pi parent should render --pi-path for --help")
	} else if flag.NoOptDefVal != "" {
		t.Errorf("pi --pi-path NoOptDefVal = %q, want empty", flag.NoOptDefVal)
	}
	for _, sub := range pi.Commands() {
		if flag := sub.Flags().Lookup("pi-path"); flag != nil {
			t.Errorf("pi %s should not render --pi-path (HEAD never did)", sub.Name())
		}
	}
	if flag := builtAgentCommand(t, "openclaw").Flags().Lookup("open-claw-path"); flag != nil {
		t.Error("openclaw --open-claw-path should stay unregistered (HEAD never rendered it)")
	}
}
