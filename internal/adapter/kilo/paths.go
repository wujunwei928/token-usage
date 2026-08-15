// Package kilo is the Kilo Code agent adapter: reads message rows from the
// Kilo SQLite database. Port of rust/adapters/kilo.
package kilo

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// KiloDataDirEnv overrides the Kilo data directory (comma-separated).
const KiloDataDirEnv = "KILO_DATA_DIR"

// DBFileName is the Kilo database file name.
const DBFileName = "kilo.db"

// DataDirs resolves the Kilo data directories: KILO_DATA_DIR (comma-separated,
// existing dirs only, deduplicated) or ~/.local/share/kilo.
func DataDirs() []string {
	var paths []string
	seen := map[string]struct{}{}
	if envPaths, ok := os.LookupEnv(KiloDataDirEnv); ok {
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
		return paths
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".local", "share", "kilo")
		if isDir(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// DBPath returns the kilo.db inside a data directory when it exists.
func DBPath(kiloDir string) string {
	path := filepath.Join(kiloDir, DBFileName)
	if isFile(path) {
		return path
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// openReadOnly opens a SQLite database in read-only mode.
func openReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}
