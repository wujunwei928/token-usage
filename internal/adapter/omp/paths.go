// Package omp is the oh-my-pi adapter: session JSONL discovery under
// ~/.omp/agent, pi-format parsing, and the omp usage reports. oh-my-pi is a
// pi fork and writes the same session JSONL layout under its own root, so
// the parsing pipeline mirrors the pi adapter with omp's own identity
// (paths, model display prefix, dedupe namespace) and sidecar-directory
// sub-session attribution.
package omp

import (
	"os"
	"path/filepath"
	"strings"
)

// OmpAgentDirEnv overrides the default omp sessions directory.
const OmpAgentDirEnv = "OMP_AGENT_DIR"

// Paths resolves the omp session roots: an explicit --omp-path wins, then
// OMP_AGENT_DIR, then ~/.omp/agent/sessions. Like pi's --pi-path, --omp-path
// and OMP_AGENT_DIR keep no-`~`-expansion semantics (named store paths in
// token-usage.json expand `~`, but those are handled by the config layer).
func Paths(customPath *string) ([]string, error) {
	if customPath != nil && strings.TrimSpace(*customPath) != "" {
		return existingPathList(*customPath), nil
	}
	if envPaths, ok := os.LookupEnv(OmpAgentDirEnv); ok && strings.TrimSpace(envPaths) != "" {
		return existingPathList(envPaths), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &adapterError{"home directory is not set"}
	}
	path := filepath.Join(home, ".omp", "agent", "sessions")
	if isDir(path) {
		return []string{path}, nil
	}
	return nil, nil
}

// HasData reports whether an omp sessions directory exists (the default root
// or the env override), even when date filters leave no entries.
func HasData() bool {
	paths, err := Paths(nil)
	if err != nil {
		return false
	}
	return len(paths) > 0
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
