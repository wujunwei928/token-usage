// Package goose is the Goose agent adapter: reads session rows from the
// Goose SQLite sessions database. Port of rust/adapters/goose.
package goose

import (
	"os"
	"path/filepath"
	"strings"
)

// GoosePathRootEnv overrides the Goose data root (its data/sessions/sessions.db
// lives under the root).
const GoosePathRootEnv = "GOOSE_PATH_ROOT"

// DBFileName is the Goose session database file name.
const DBFileName = "sessions.db"

// DBPaths resolves the candidate Goose databases, canonicalizing, filtering to
// existing files, and dropping duplicates in candidate order.
func DBPaths() []string {
	var candidates []string
	if root, ok := os.LookupEnv(GoosePathRootEnv); ok {
		root = strings.TrimSpace(root)
		if root == "" {
			candidates = defaultDBCandidates()
		} else {
			candidates = []string{filepath.Join(root, "data", "sessions", DBFileName)}
		}
	} else {
		candidates = defaultDBCandidates()
	}

	var paths []string
	seen := map[string]struct{}{}
	for _, path := range candidates {
		resolved := canonicalize(path)
		if isFile(resolved) {
			if _, dup := seen[resolved]; !dup {
				seen[resolved] = struct{}{}
				paths = append(paths, resolved)
			}
		}
	}
	return paths
}

func defaultDBCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".local", "share", "goose", "sessions", DBFileName),
		filepath.Join(home, "Library", "Application Support", "goose", "sessions", DBFileName),
		filepath.Join(home, ".local", "share", "Block", "goose", "sessions", DBFileName),
	}
}

// canonicalize resolves symlinks and makes the path absolute; unresolvable
// paths are returned unchanged, like the reference's canonicalize fallback.
func canonicalize(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
