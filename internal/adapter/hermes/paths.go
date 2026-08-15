// Package hermes ports the Rust ccusage-adapter-hermes crate: usage reports
// from the Hermes CLI's SQLite state database.
package hermes

import (
	"os"
	"path/filepath"
	"strings"
)

// HermesHomeEnv overrides the Hermes home directory (comma-separated).
const HermesHomeEnv = "HERMES_HOME"

// StateDBPaths resolves every readable state.db under the Hermes homes.
func StateDBPaths() ([]string, error) {
	var homes []string
	if override := os.Getenv(HermesHomeEnv); override != "" {
		for _, part := range strings.Split(override, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				homes = append(homes, part)
			}
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, &cliError{"home directory is not set"}
		}
		homes = []string{filepath.Join(home, ".hermes")}
	}
	seen := map[string]bool{}
	var paths []string
	for _, home := range homes {
		path := filepath.Join(home, "state.db")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths, nil
}

type cliError struct{ message string }

func (e *cliError) Error() string { return e.message }
