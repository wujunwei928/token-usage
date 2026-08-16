package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// optionalValueFlags accept a value that may be omitted (`[value]` in help).
// pflag's NoOptDefVal handling does not consume a following space-separated
// value, so rewrite those to the `--flag=value` form before cobra sees them —
// exactly the reference parser's behavior.
var optionalValueFlags = map[string]bool{
	"--mode":             true,
	"--order":            true,
	"--start-of-week":    true,
	"--debug-samples":    true,
	"--session-length":   true,
	"--cost-source":      true,
	"--visual-burn-rate": true,
	"--refresh-interval": true,
	"--speed":            true,
}

// normalizeArgs rewrites space-separated values of optional-value flags into
// the `=` form and normalizes legacy `agent:report` colon commands.
func normalizeArgs(args []string) []string {
	// Legacy alias: `token-usage codex:daily` -> `token-usage codex daily`, but only
	// when the agent actually supports that report.
	if len(args) > 0 && strings.Contains(args[0], ":") {
		parts := strings.SplitN(args[0], ":", 2)
		if isAgentName(parts[0]) && len(parts[1]) > 0 && agentReportSupported(parts[0], parts[1]) {
			args = append([]string{parts[0], parts[1]}, args[1:]...)
		}
	}

	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		token := args[i]
		flag, hasInline := strings.CutPrefix(token, "--")
		if hasInline && optionalValueFlags["--"+strings.SplitN(flag, "=", 2)[0]] && !strings.Contains(token, "=") {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, token+"="+args[i+1])
				i++
				continue
			}
		}
		out = append(out, token)
	}
	return out
}

// agentReportKinds is the agent × report support matrix — the single source
// both the command tree (agenttree.go) and the legacy colon-form parser read
// (ADR 0010). claude and opencode carry weekly; every other agent offers
// daily/monthly/session.
func agentReportKinds(agent string) []core.ReportKind {
	switch agent {
	case "claude", "opencode":
		return []core.ReportKind{core.KindDaily, core.KindWeekly, core.KindMonthly, core.KindSession}
	default:
		return []core.ReportKind{core.KindDaily, core.KindMonthly, core.KindSession}
	}
}

func agentKindSupported(agent string, kind core.ReportKind) bool {
	for _, supported := range agentReportKinds(agent) {
		if supported == kind {
			return true
		}
	}
	return false
}

// agentReportSupported answers the same matrix by report name, plus the
// claude-only blocks/statusline reports.
func agentReportSupported(agent, report string) bool {
	if agent == "claude" && (report == "blocks" || report == "statusline") {
		return true
	}
	for _, kind := range agentReportKinds(agent) {
		if kind.String() == report {
			return true
		}
	}
	return false
}

// reportFlagAliasError matches the removed report-flag aliases.
func reportFlagAliasError(args []string) error {
	reportFlags := map[string]bool{
		"--daily": true, "--weekly": true, "--monthly": true,
		"--session": true, "--blocks": true, "--statusline": true,
	}
	for _, arg := range args {
		if reportFlags[arg] {
			name := strings.TrimPrefix(arg, "--")
			return &ParseError{fmt.Sprintf(
				"Report flags like %s are not supported. Use \"token-usage %s\" instead.\nRun 'token-usage --help' for usage.", arg, name)}
		}
	}
	return nil
}

var agentNames = []string{
	"claude", "codex", "opencode", "amp", "droid", "codebuff", "hermes",
	"pi", "goose", "kilo", "copilot", "gemini", "kimi", "qwen", "openclaw",
	"zcode", "omp",
}

func isAgentName(name string) bool {
	for _, agent := range agentNames {
		if agent == name {
			return true
		}
	}
	return false
}

// rewriteOSArgs applies normalizeArgs to the user arguments in place,
// preserving argv[0].
func rewriteOSArgs() {
	os.Args = append(os.Args[:1], normalizeArgs(os.Args[1:])...)
}
