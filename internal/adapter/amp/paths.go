// Package amp is the Amp agent adapter: JSON thread files under
// ~/.local/share/amp (or AMP_DATA_DIR), each holding one JSON object with the
// thread id, chat messages, and an optional usage ledger.
package amp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// ampDataDirEnv overrides the data directory list (comma-separated).
const ampDataDirEnv = "AMP_DATA_DIR"

// Paths resolves the Amp data directories to scan, mirroring
// rust/adapters/amp/src/paths.rs: AMP_DATA_DIR wins entirely when set (each
// entry must be an existing directory), otherwise ~/.local/share/amp.
func Paths() ([]string, error) {
	var paths []string
	seen := map[string]struct{}{}
	if envPaths, ok := os.LookupEnv(ampDataDirEnv); ok {
		for _, raw := range strings.Split(envPaths, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
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
		return nil, &core.CLIError{Message: "home directory is not set"}
	}
	path := filepath.Join(home, ".local", "share", "amp")
	if isDir(path) {
		paths = append(paths, path)
	}
	return paths, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
