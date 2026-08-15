// Package opencode is the OpenCode agent adapter: data dir discovery,
// SQLite + message-file loading, and the per-kind reports.
package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CLIError mirrors the reference CliError display format.
type CLIError struct{ Message string }

func (e *CLIError) Error() string { return fmt.Sprintf("CliError(%q)", e.Message) }

// envPaths splits a comma-separated OPENCODE_DATA_DIR value into non-empty,
// trimmed entries, preserving order.
func envPaths(raw string) []string {
	var paths []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			paths = append(paths, part)
		}
	}
	return paths
}

// Paths resolves the OpenCode data directories to scan: OPENCODE_DATA_DIR
// overrides discovery entirely (only existing directories are kept), otherwise
// ~/.local/share/opencode.
func Paths() ([]string, error) {
	var paths []string
	seen := map[string]struct{}{}
	if raw, ok := os.LookupEnv("OPENCODE_DATA_DIR"); ok {
		for _, raw := range envPaths(raw) {
			if isDir(raw) {
				if _, dup := seen[raw]; !dup {
					seen[raw] = struct{}{}
					paths = append(paths, raw)
				}
			}
		}
		return paths, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &CLIError{"home directory is not set"}
	}
	path := filepath.Join(home, ".local", "share", "opencode")
	if isDir(path) {
		paths = append(paths, path)
	}
	return paths, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
