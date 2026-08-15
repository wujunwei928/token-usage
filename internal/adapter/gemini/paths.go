// Package gemini ports the Gemini CLI adapter (rust/adapters/gemini): log
// discovery under ~/.gemini/tmp (GEMINI_DATA_DIR overrides), the JSON/JSONL
// token-event parser, and the per-agent report summarization.
package gemini

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DataDirEnv names the comma-separated override for the Gemini data directory.
const DataDirEnv = "GEMINI_DATA_DIR"

// dataDirs resolves the directories to scan: the env override when set,
// otherwise ~/.gemini/tmp. Only existing directories are kept, in order,
// deduplicated.
func dataDirs() []string {
	if raw, ok := os.LookupEnv(DataDirEnv); ok {
		return existingDirList(raw)
	}
	home := agentHomeDir()
	if home == "" {
		return nil
	}
	return existingDirList(filepath.Join(home, ".gemini", "tmp"))
}

func existingDirList(raw string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
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

// agentHomeDir mirrors the reference home_dir(): HOME, then USERPROFILE, then
// HOMEDRIVE+HOMEPATH; empty when none resolve.
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

// DiscoverLogFiles collects *.json and *.jsonl files under the data dirs,
// sorted by path and deduplicated.
func DiscoverLogFiles() []string {
	var files []string
	for _, dir := range dataDirs() {
		collectFilesWithExtension(dir, "json", &files)
		collectFilesWithExtension(dir, "jsonl", &files)
	}
	sort.Strings(files)
	dedup := files[:0]
	for i, file := range files {
		if i == 0 || file != files[i-1] {
			dedup = append(dedup, file)
		}
	}
	return dedup
}

// collectFilesWithExtension walks dir recursively gathering regular files with
// the exact extension; walk errors are ignored, mirroring the reference.
func collectFilesWithExtension(dir, extension string, files *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			collectFilesWithExtension(path, extension, files)
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if filepath.Ext(path) == "."+extension {
			*files = append(*files, path)
		}
	}
}
