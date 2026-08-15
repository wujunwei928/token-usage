package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// valueTakingOptions mirrors the reference parser's option_takes_value list:
// after these tokens the next argument is a value, never a command token.
var valueTakingOptions = map[string]bool{
	"-s": true, "--since": true, "-u": true, "--until": true, "--last": true,
	"-m": true, "--mode": true, "--debug-samples": true, "-o": true, "--order": true,
	"-z": true, "--timezone": true, "-q": true, "--jq": true, "--config": true,
	"-p": true, "--project": true, "--project-aliases": true, "-w": true, "--start-of-week": true,
	"-i": true, "--id": true, "-t": true, "--token-limit": true, "-n": true, "--session-length": true,
	"-B": true, "--visual-burn-rate": true, "--cost-source": true, "--refresh-interval": true,
	"--context-low-threshold": true, "--context-medium-threshold": true, "--speed": true,
	"--pi-path": true, "--open-claw-path": true, "--sections": true,
}

// commandTokens collects the non-flag tokens (commands) from raw args.
func commandTokens(args []string) []string {
	var tokens []string
	index := 0
	for index < len(args) {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			if valueTakingOptions[arg] && !strings.Contains(arg, "=") {
				index += 2
			} else {
				index++
			}
			continue
		}
		tokens = append(tokens, arg)
		index++
	}
	return tokens
}

// agentFilterOptionError matches the removed --agent / -a agent filters. The
// short form stays valid on blocks commands.
func agentFilterOptionError(args []string) error {
	tokens := commandTokens(args)
	blocksContext := false
	if len(tokens) > 0 {
		blocksContext = tokens[0] == "blocks" || (tokens[0] == "claude" && len(tokens) > 1 && tokens[1] == "blocks")
	}
	for _, arg := range args {
		flag := ""
		switch {
		case arg == "--agent" || strings.HasPrefix(arg, "--agent="):
			flag = "--agent"
		case arg == "-a" && !blocksContext, strings.HasPrefix(arg, "-a="):
			flag = "-a"
		}
		if flag != "" {
			return &ParseError{fmt.Sprintf(
				"Agent filters like %s are not supported. Use \"ccusage <agent> <report>\", for example \"ccusage codex daily\".\nRun 'ccusage --help' for usage.", flag)}
		}
	}
	return nil
}

// agentDisplayName maps agent ids to the reference's display names.
func agentDisplayName(agent string) string {
	names := map[string]string{
		"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode",
		"amp": "Amp", "droid": "Droid", "codebuff": "Codebuff", "hermes": "Hermes",
		"pi": "pi-agent", "goose": "Goose", "openclaw": "OpenClaw", "kilo": "Kilo",
		"copilot": "GitHub Copilot CLI", "gemini": "Gemini CLI", "kimi": "Kimi",
		"qwen": "Qwen", "grok": "Grok",
	}
	if name, ok := names[agent]; ok {
		return name
	}
	return agent
}

// reformatCobraFlagError maps cobra's unknown-flag and unknown-command errors
// onto the reference messages.
func reformatCobraFlagError(root *cobra.Command, raw []string, err error) error {
	msg := err.Error()
	flag := ""
	if strings.HasPrefix(msg, "unknown flag: ") {
		flag = strings.TrimPrefix(msg, "unknown flag: ")
	} else if strings.HasPrefix(msg, "unknown shorthand flag: ") {
		// "unknown shorthand flag: 'a' in -a"
		rest := strings.TrimPrefix(msg, "unknown shorthand flag: ")
		if idx := strings.LastIndex(rest, " in "); idx >= 0 {
			flag = rest[idx+4:]
		}
	} else if strings.HasPrefix(msg, "unknown command ") {
		// unknown command "x" for "ccusage" / for "ccusage claude"
		var token, parent string
		if n, _ := fmt.Sscanf(msg, "unknown command %q for %q", &token, &parent); n == 2 {
			if parent == "ccusage" {
				return &ParseError{fmt.Sprintf("Unknown command '%s'\nRun 'ccusage --help' for usage.", token)}
			}
			agent := strings.TrimPrefix(parent, "ccusage ")
			return &ParseError{fmt.Sprintf("The %q report is not available for %s usage.\nRun 'ccusage --help' for usage.", token, agentDisplayName(agent))}
		}
		return nil
	} else {
		return nil
	}
	cmd := commandContext(root, raw)
	if cmd == nil || cmd == root {
		return &ParseError{fmt.Sprintf("Unknown option '%s'\nRun 'ccusage --help' for usage.", flag)}
	}
	return &ParseError{fmt.Sprintf("Unknown %s option '%s'\nRun 'ccusage --help' for usage.", cmd.Name(), flag)}
}

// commandContext walks raw args through the command tree to find the deepest
// command reached before the failing flag.
func commandContext(root *cobra.Command, raw []string) *cobra.Command {
	cmd := root
	depth := 0
	index := 0
	for index < len(raw) {
		arg := raw[index]
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			long := strings.SplitN(name, "=", 2)[0]
			var flag *pflag.Flag
			if strings.HasPrefix(arg, "--") {
				flag = cmd.Flags().Lookup(long)
			} else if len(long) == 1 {
				flag = cmd.Flags().ShorthandLookup(long)
			}
			if flag != nil && flag.NoOptDefVal == "" && !strings.Contains(arg, "=") {
				index += 2
			} else {
				index++
			}
			continue
		}
		if child := findSubCommand(cmd, arg); child != nil && depth < 2 {
			cmd = child
			depth++
			index++
			continue
		}
		break
	}
	return cmd
}

func findSubCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
