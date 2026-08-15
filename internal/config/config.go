// Package config implements ccusage.json discovery, parsing, and merging,
// ported from rust/crates/ccusage-config/src/config.rs.
//
// A config context is built from the raw CLI arguments (FromArgs): the
// arguments reveal which command runs (detectConfigCommand) and an explicit
// --config path (ScanConfigPath). The first readable, object-shaped JSON file
// wins; unreadable, malformed, or non-object files are skipped silently.
//
// Precedence (low to high) across config sections, verified against the
// reference binary:
//
//	defaults
//	commands."<agent> <report>"   (e.g. commands."claude daily")
//	commands."<report>"           (e.g. commands.daily)
//	commands."<agent>:<report>"   (e.g. commands."claude:daily")
//	<agent>.defaults              (e.g. claude.defaults)
//	<agent>.commands.<report>     (e.g. claude.commands.daily)
//
// CLI flags always win over config; config always wins over built-in defaults.
// The only fatal config errors are malformed pi.stores entries, and those only
// surface for the all-agent reports (daily/weekly/monthly/session).
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CommandInfo identifies the command a config context applies to.
type CommandInfo struct {
	// Raw is the compound command name used for the commands section key:
	// "claude daily" for agent reports, "daily" otherwise.
	Raw string
	// Agent is the agent name for agent commands ("" for all-agent reports).
	Agent string
	// Report is the report name (daily/weekly/monthly/session/blocks/statusline).
	Report string
}

// Context mirrors ccusage_config::ConfigContext.
type Context struct {
	// value is the parsed root object, nil when no config was found.
	value map[string]any
	// command is the detected command.
	command CommandInfo
	// piStores holds validated pi.stores entries.
	piStores []NamedPiStore
	// err holds the pi.stores parse error, if any.
	err error
}

// FromArgs builds a context from the raw CLI arguments (excluding argv[0]),
// mirroring ConfigContext::from_args.
func FromArgs(args []string) *Context {
	command := DetectConfigCommand(args)
	value := LoadConfigValue(ScanConfigPath(args))
	stores, err := ParseNamedPIStores(value)
	return &Context{
		value:    value,
		command:  command,
		piStores: stores,
		err:      err,
	}
}

// FromValue builds a context around an explicit config document (tests).
func FromValue(value map[string]any, command CommandInfo) *Context {
	stores, err := ParseNamedPIStores(value)
	return &Context{value: value, command: command, piStores: stores, err: err}
}

// Command returns the detected command.
func (c *Context) Command() CommandInfo { return c.command }

// PIStores returns the validated pi.stores entries.
func (c *Context) PIStores() []NamedPiStore { return c.piStores }

// ConfigError returns the fatal pi.stores error when the command surfaces it
// (all-agent daily/weekly/monthly/session reports), mirroring
// ConfigContext::config_error.
func (c *Context) ConfigError() error {
	if c.err == nil || !CommandUsesNamedPIStores(c.command.Agent, c.command.Report) {
		return nil
	}
	return c.err
}

// OptionMaps returns the config option maps for the detected command in
// precedence order (lowest first; later maps override earlier ones). Every map
// may carry shared keys; command-specific keys simply no-op where unused.
func (c *Context) OptionMaps() []map[string]any {
	return optionMapsFor(c.value, c.command)
}

// OptionMapsFor returns the option maps for an explicit (raw, agent, report)
// command over this context's root value; raw is the compound commands key
// ("claude daily" or "daily").
func (c *Context) OptionMapsFor(raw, agent, report string) []map[string]any {
	return optionMapsFor(c.value, CommandInfo{Raw: raw, Agent: agent, Report: report})
}

func optionMapsFor(root map[string]any, command CommandInfo) []map[string]any {
	var maps []map[string]any
	if root == nil {
		return maps
	}
	if defaults := objectAt(root, "defaults"); defaults != nil {
		maps = append(maps, defaults)
	}
	if commands := objectAt(root, "commands"); commands != nil {
		if raw := objectAt(commands, command.Raw); raw != nil {
			maps = append(maps, raw)
		}
		if command.Agent != "" {
			if report := objectAt(commands, command.Report); report != nil {
				maps = append(maps, report)
			}
			if agentReport := objectAt(commands, command.Agent+":"+command.Report); agentReport != nil {
				maps = append(maps, agentReport)
			}
		}
	}
	if command.Agent != "" {
		if agent := objectAt(root, command.Agent); agent != nil {
			if defaults := objectAt(agent, "defaults"); defaults != nil {
				maps = append(maps, defaults)
			}
			if commands := objectAt(agent, "commands"); commands != nil {
				if report := objectAt(commands, command.Report); report != nil {
					maps = append(maps, report)
				}
			}
		}
	}
	return maps
}

func objectAt(object map[string]any, key string) map[string]any {
	value, ok := object[key]
	if !ok {
		return nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

// ---------------------------------------------------------------------------
// Discovery and loading
// ---------------------------------------------------------------------------

// LoadConfigValue reads the config named by path, or discovers one when path
// is empty. The first path that reads, parses as JSON, and is an object wins;
// everything else is skipped silently (matching load_config_value).
func LoadConfigValue(path string) map[string]any {
	paths := []string{path}
	if path == "" {
		paths = DiscoverConfigPaths()
	}
	for _, candidate := range paths {
		content, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if value, ok := parseConfigObject(string(content)); ok {
			return value
		}
	}
	return nil
}

// parseConfigObject parses a strict JSON document (no trailing garbage) and
// returns its root object. Numbers stay json.Number so integer-typed options
// can distinguish 5 from 5.0, exactly like serde_json::Value.
func parseConfigObject(content string) (map[string]any, bool) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	// serde_json::from_str rejects trailing non-whitespace content.
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	object, ok := value.(map[string]any)
	return object, ok
}

// TokenUsageConfigDirEnv names the comma-separated override for the
// token-usage config directories.
const TokenUsageConfigDirEnv = "TOKEN_USAGE_CONFIG_DIR"

// DiscoverConfigPaths lists candidate config paths in discovery order:
// ./.token-usage/config.json relative to the working directory, then each
// token-usage config directory's config.json. ccusage-go keeps its own
// namespace so its config never shares files with the upstream ccusage,
// which discovers ccusage.json from the Claude config directories instead
// (ADR 0007).
func DiscoverConfigPaths() []string {
	var paths []string
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".token-usage", "config.json"))
	}
	for _, dir := range TokenUsageConfigDirs() {
		paths = append(paths, filepath.Join(dir, "config.json"))
	}
	return paths
}

// TokenUsageConfigDirs resolves the token-usage config directories:
// TOKEN_USAGE_CONFIG_DIR (comma-separated) when set, otherwise
// ~/.config/token-usage and ~/.token-usage.
func TokenUsageConfigDirs() []string {
	if envPaths, ok := os.LookupEnv(TokenUsageConfigDirEnv); ok {
		var dirs []string
		for _, raw := range strings.Split(envPaths, ",") {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				dirs = append(dirs, raw)
			}
		}
		return dirs
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".config", "token-usage"), filepath.Join(home, ".token-usage")}
}

// ScanConfigPath extracts an explicit --config path from raw CLI arguments,
// accepting both "--config path" and "--config=path" (non-empty value only).
func ScanConfigPath(args []string) string {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if flag, value, found := strings.Cut(arg, "="); found {
			if flag == "--config" && value != "" {
				return value
			}
			continue
		}
		if arg == "--config" {
			if index+1 < len(args) {
				return args[index+1]
			}
			return ""
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Command detection
// ---------------------------------------------------------------------------

// AgentNames mirrors ccusage_core::BUILT_IN_AGENT_NAMES.
var AgentNames = []string{
	"claude", "codex", "opencode", "amp", "droid", "codebuff", "hermes",
	"pi", "goose", "openclaw", "kilo", "copilot", "gemini", "kimi", "qwen", "grok",
}

// reservedNamedPIStoreNames are the store names that collide with built-in
// agent commands ("all" plus every agent name).
func reservedNamedPIStoreNames() []string {
	return append([]string{"all"}, AgentNames...)
}

func isAgentCommand(command string) bool {
	for _, agent := range AgentNames {
		if agent == command {
			return true
		}
	}
	return false
}

func isReportCommand(command string) bool {
	switch command {
	case "daily", "monthly", "weekly", "session", "blocks", "statusline":
		return true
	}
	return false
}

// DetectConfigCommand identifies the running command from raw CLI arguments
// (flag values are skipped via optionTakesValue, exactly like the reference).
func DetectConfigCommand(args []string) CommandInfo {
	tokens := CommandTokens(args)
	if len(tokens) == 0 {
		return CommandInfo{Raw: "daily", Report: "daily"}
	}
	first := tokens[0]
	if agent, report, found := strings.Cut(first, ":"); found {
		return CommandInfo{Raw: agent + " " + report, Agent: agent, Report: report}
	}
	if isAgentCommand(first) {
		report := "daily"
		if len(tokens) > 1 && isReportCommand(tokens[1]) {
			report = tokens[1]
		}
		return CommandInfo{Raw: first + " " + report, Agent: first, Report: report}
	}
	return CommandInfo{Raw: first, Report: first}
}

// CommandTokens returns the non-flag tokens of raw CLI arguments.
func CommandTokens(args []string) []string {
	var tokens []string
	index := 0
	for index < len(args) {
		arg := args[index]
		if flag, _, found := strings.Cut(arg, "="); found && strings.HasPrefix(flag, "-") {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if optionTakesValue(arg) {
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

// optionTakesValue lists the flags whose following argument is a value, from
// the reference parser; it accepts both long and short spellings and compares
// the name part of "--flag=value" forms.
func optionTakesValue(arg string) bool {
	name := arg
	if flag, _, found := strings.Cut(arg, "="); found {
		name = flag
	}
	switch name {
	case "-s", "--since",
		"-u", "--until",
		"-m", "--mode",
		"--debug-samples",
		"-o", "--order",
		"-z", "--timezone",
		"-q", "--jq",
		"--config",
		"-t", "--token-limit",
		"-n", "--session-length",
		"-w", "--start-of-week",
		"-p", "--project",
		"--project-aliases",
		"--pi-path",
		"--speed",
		"-B", "--visual-burn-rate",
		"--cost-source",
		"--refresh-interval",
		"--context-low-threshold",
		"--context-medium-threshold":
		return true
	}
	return false
}

// CommandUsesNamedPIStores reports whether a command surfaces pi.stores config
// errors: the all-agent daily/weekly/monthly/session reports.
func CommandUsesNamedPIStores(agent, report string) bool {
	return agent == "" && func() bool {
		switch report {
		case "daily", "weekly", "monthly", "session":
			return true
		}
		return false
	}()
}

// ---------------------------------------------------------------------------
// pi.stores parsing
// ---------------------------------------------------------------------------

// NamedPIStoreNamePattern is the schema pattern for pi.stores names.
const NamedPIStoreNamePattern = "^[a-z][a-z0-9_-]{0,31}$"

func configError(format string, args ...any) error {
	return fmt.Errorf("Invalid token-usage config: "+format, args...)
}

// ParseNamedPIStores validates the pi.stores section, returning the stores or
// an error whose message matches the reference byte for byte.
func ParseNamedPIStores(value map[string]any) ([]NamedPiStore, error) {
	if value == nil {
		return nil, nil
	}
	pi := objectAt(value, "pi")
	if pi == nil {
		return nil, nil
	}
	storesValue, ok := pi["stores"]
	if !ok {
		return nil, nil
	}
	stores, ok := storesValue.([]any)
	if !ok {
		return nil, configError("pi.stores must be an array")
	}

	seen := map[string]bool{}
	parsed := make([]NamedPiStore, 0, len(stores))
	for index, raw := range stores {
		store, ok := raw.(map[string]any)
		if !ok {
			return nil, configError("pi.stores[%d] must contain string fields 'name' and 'path'", index)
		}
		name, ok := store["name"].(string)
		if !ok {
			return nil, configError("pi.stores[%d] must contain string fields 'name' and 'path'", index)
		}
		path, ok := store["path"].(string)
		if !ok {
			return nil, configError("pi.stores[%d] must contain string fields 'name' and 'path'", index)
		}
		if strings.TrimSpace(path) == "" {
			return nil, configError("pi.stores[%d] ('%s'): path must be a non-empty string", index, name)
		}
		if !MatchesNamedPIStoreNamePattern(name) {
			return nil, configError("pi.stores[%d].name must match %s", index, NamedPIStoreNamePattern)
		}
		if reservedNamedPIStoreName(name) {
			return nil, configError("pi.stores name '%s' collides with a built-in agent", name)
		}
		if seen[name] {
			return nil, configError("duplicate pi.stores name '%s'", name)
		}
		seen[name] = true
		parsed = append(parsed, NamedPiStore{Name: name, Path: path})
	}
	return parsed, nil
}

func reservedNamedPIStoreName(name string) bool {
	for _, reserved := range reservedNamedPIStoreNames() {
		if reserved == name {
			return true
		}
	}
	return false
}

// MatchesNamedPIStoreNamePattern validates ^[a-z][a-z0-9_-]{0,31}$.
func MatchesNamedPIStoreNamePattern(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	if first < 'a' || first > 'z' {
		return false
	}
	rest := name[1:]
	if len(rest) > 31 {
		return false
	}
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

// NormalizeDateBound strips date separators, mirroring normalize_date_bound.
func NormalizeDateBound(value string) string {
	return strings.ReplaceAll(value, "-", "")
}
