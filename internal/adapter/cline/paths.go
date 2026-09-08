// Package cline is the Cline CLI adapter: session discovery under
// ~/.cline/data/sessions, <id>.messages.json parsing, and the cline usage
// reports. Each session directory pairs a manifest (<id>.json: model,
// provider, workspace) with the conversation log (<id>.messages.json);
// every assistant message carrying a metrics block is one API call's
// Usage Entry.
package cline

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DataDirEnv names the comma-separated override for the Cline data directory.
const DataDirEnv = "CLINE_DATA_DIR"

// DiscoverMessageFiles returns the <task>/<task>.messages.json paths under
// every data root's sessions directory, sorted; only existing files count.
func DiscoverMessageFiles() []string {
	var files []string
	for _, root := range dataDirs() {
		sessions := filepath.Join(root, "sessions")
		taskDirs, err := os.ReadDir(sessions)
		if err != nil {
			continue
		}
		for _, taskDir := range taskDirs {
			if !taskDir.IsDir() {
				continue
			}
			candidate := filepath.Join(sessions, taskDir.Name(), taskDir.Name()+".messages.json")
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				files = append(files, candidate)
			}
		}
	}
	sort.Strings(files)
	return files
}

// HasData reports whether any Cline session messages file exists, even when
// date filters leave no entries.
func HasData() bool {
	return len(DiscoverMessageFiles()) > 0
}

// dataDirs resolves the roots to scan: the env override when set, otherwise
// ~/.cline/data. Only existing directories are kept.
func dataDirs() []string {
	if raw, ok := os.LookupEnv(DataDirEnv); ok {
		var roots []string
		for _, item := range strings.Split(raw, ",") {
			if path := strings.TrimSpace(item); path != "" {
				roots = append(roots, path)
			}
		}
		return existingDirs(roots)
	}
	home := agentHomeDir()
	if home == "" {
		return nil
	}
	return existingDirs([]string{filepath.Join(home, ".cline", "data")})
}

func existingDirs(paths []string) []string {
	var out []string
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			out = append(out, path)
		}
	}
	return out
}

// agentHomeDir mirrors the reference home_dir(): HOME, then USERPROFILE,
// then HOMEDRIVE+HOMEPATH; empty when none resolve.
func agentHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		return profile
	}
	drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH")
	if drive != "" && path != "" {
		return drive + path
	}
	return ""
}
