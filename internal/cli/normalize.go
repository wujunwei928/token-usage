package cli

import (
	"fmt"
	"os"
	"strings"
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

// agentReportSupported is the agent x report matrix the reference parser uses
// for the legacy colon form and the `token-usage <agent> <report>` subcommands.
func agentReportSupported(agent, report string) bool {
	switch agent {
	case "claude":
		switch report {
		case "daily", "weekly", "monthly", "session", "blocks", "statusline":
			return true
		}
	case "codex":
		switch report {
		case "daily", "monthly", "session":
			return true
		}
	case "opencode":
		switch report {
		case "daily", "weekly", "monthly", "session":
			return true
		}
	case "amp", "droid", "codebuff", "hermes", "pi", "goose", "kilo",
		"copilot", "gemini", "kimi", "qwen", "openclaw", "grok", "zcode":
		switch report {
		case "daily", "monthly", "session":
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
	"pi", "goose", "kilo", "copilot", "gemini", "kimi", "qwen", "openclaw", "grok",
	"zcode",
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
