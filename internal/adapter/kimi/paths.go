// Package kimi ports the Kimi adapter (rust/adapters/kimi): wire.jsonl
// discovery under ~/.kimi and ~/.kimi-code (KIMI_DATA_DIR overrides), the
// old StatusUpdate and new usage.record line parsers, and report
// summarization.
package kimi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DataDirEnv names the comma-separated override for the Kimi data directory.
const DataDirEnv = "KIMI_DATA_DIR"

const (
	sessionsDirName = "sessions"
	wireFileName    = "wire.jsonl"
)

// dataDirs resolves the roots to scan: the env override when set, otherwise
// ~/.kimi and ~/.kimi-code. Only existing directories are kept.
func dataDirs() []string {
	if raw, ok := os.LookupEnv(DataDirEnv); ok {
		return existingDirList(raw)
	}
	home := agentHomeDir()
	if home == "" {
		return nil
	}
	return existingDirList(filepath.Join(home, ".kimi") + "," + filepath.Join(home, ".kimi-code"))
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

// DiscoverWireFiles finds wire.jsonl files in the two supported layouts:
// sessions/<group>/<session>/wire.jsonl and
// sessions/<workspace>/<session>/agents/<agent>/wire.jsonl.
func DiscoverWireFiles() []string {
	var files []string
	for _, kimiPath := range dataDirs() {
		sessionsPath := filepath.Join(kimiPath, sessionsDirName)
		var candidates []string
		collectFilesWithExtension(sessionsPath, "jsonl", &candidates)
		for _, file := range candidates {
			if isKimiWireFile(sessionsPath, file) {
				files = append(files, file)
			}
		}
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

func isKimiWireFile(sessionsPath, filePath string) bool {
	if filepath.Base(filePath) != wireFileName {
		return false
	}
	relative, err := filepath.Rel(sessionsPath, filePath)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	normal := 0
	for _, part := range parts {
		if part != "." && part != ".." && part != "" {
			normal++
		}
	}
	return normal == 3 || normal == 5
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
