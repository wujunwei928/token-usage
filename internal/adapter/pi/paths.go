// Package pi is the pi-agent adapter: session JSONL discovery, parsing, and
// the pi usage reports. Port of rust/adapters/pi.
package pi

import (
	"os"
	"path/filepath"
	"strings"
)

// PiAgentDirEnv overrides the default pi sessions directory.
const PiAgentDirEnv = "PI_AGENT_DIR"

// Paths resolves the pi session roots: an explicit --pi-path wins, then
// PI_AGENT_DIR, then ~/.pi/agent/sessions. `--pi-path` and PI_AGENT_DIR keep
// their pre-existing no-`~`-expansion semantics (named store paths in
// ccusage.json expand `~`, but those are handled by the config layer).
func Paths(customPath *string) ([]string, error) {
	if customPath != nil && strings.TrimSpace(*customPath) != "" {
		return existingPathList(*customPath), nil
	}
	if envPaths, ok := os.LookupEnv(PiAgentDirEnv); ok && strings.TrimSpace(envPaths) != "" {
		return existingPathList(envPaths), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &adapterError{"home directory is not set"}
	}
	path := filepath.Join(home, ".pi", "agent", "sessions")
	if isDir(path) {
		return []string{path}, nil
	}
	return nil, nil
}

// existingPathList splits a comma-separated path list, dropping empty entries,
// non-directories, and duplicates while preserving order.
func existingPathList(raw string) []string {
	var paths []string
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !isDir(part) {
			continue
		}
		if _, dup := seen[part]; dup {
			continue
		}
		seen[part] = struct{}{}
		paths = append(paths, part)
	}
	return paths
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// adapterError mirrors the reference CliError display format.
type adapterError struct{ Message string }

func (e *adapterError) Error() string { return "CliError(\"" + e.Message + "\")" }
