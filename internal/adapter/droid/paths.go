// Package droid ports the Rust ccusage-adapter-droid crate: usage reports
// from Factory AI's Droid CLI session settings files.
package droid

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DroidSessionsDirEnv overrides the Droid sessions root (comma-separated).
const DroidSessionsDirEnv = "DROID_SESSIONS_DIR"

// DiscoverSettingsFiles gathers every *.settings.json under the Droid session
// roots, in sorted path order.
func DiscoverSettingsFiles() ([]string, error) {
	var files []string
	for _, root := range droidSessionPaths() {
		collectFilesWithExtension(root, "json", &files)
	}
	filtered := make([]string, 0, len(files))
	for _, path := range files {
		if strings.HasSuffix(filepath.Base(path), ".settings.json") {
			filtered = append(filtered, path)
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func droidSessionPaths() []string {
	var rawPaths []string
	if override := os.Getenv(DroidSessionsDirEnv); override != "" {
		for _, part := range strings.Split(override, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				rawPaths = append(rawPaths, part)
			}
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		rawPaths = []string{filepath.Join(home, ".factory", "sessions")}
	}
	seen := map[string]bool{}
	var paths []string
	for _, path := range rawPaths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func collectFilesWithExtension(dir, extension string, files *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && strings.HasSuffix(path, "."+extension) {
			*files = append(*files, path)
		} else if info.IsDir() {
			collectFilesWithExtension(path, extension, files)
		}
	}
}
