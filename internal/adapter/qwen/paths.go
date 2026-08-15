// Package qwen ports the Qwen adapter (rust/adapters/qwen): chat-file
// discovery under ~/.qwen/projects (QWEN_DATA_DIR overrides), the
// usageMetadata line parser, and report summarization.
package qwen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DataDirEnv names the comma-separated override for the Qwen data directory.
const DataDirEnv = "QWEN_DATA_DIR"

// dataDirs resolves the roots to scan: the env override when set, otherwise
// ~/.qwen. Only existing directories are kept, in order, deduplicated.
func dataDirs() []string {
	if raw, ok := os.LookupEnv(DataDirEnv); ok {
		return existingDirList(raw)
	}
	home := agentHomeDir()
	if home == "" {
		return nil
	}
	return existingDirList(filepath.Join(home, ".qwen"))
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

// DiscoverChatFiles finds projects/<project>/chats/*.jsonl chat files.
func DiscoverChatFiles() []string {
	var files []string
	for _, root := range dataDirs() {
		projects := filepath.Join(root, "projects")
		if info, err := os.Stat(projects); err != nil || !info.IsDir() {
			continue
		}
		var rootFiles []string
		collectFilesWithExtension(projects, "jsonl", &rootFiles)
		for _, file := range rootFiles {
			if isChatFile(projects, file) {
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	return files
}

func isChatFile(projects, file string) bool {
	relative, err := filepath.Rel(projects, file)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 3 {
		return false
	}
	return parts[0] != "" && parts[1] == "chats" && strings.HasSuffix(parts[2], ".jsonl")
}

// ProjectFromFile extracts the project directory from a
// .../projects/<project>/chats/<file> path (last match wins).
func ProjectFromFile(file string) (string, bool) {
	parts := strings.Split(file, string(filepath.Separator))
	for end := len(parts); end >= 4; end-- {
		if parts[end-4] == "projects" && parts[end-2] == "chats" {
			return parts[end-3], true
		}
	}
	return "", false
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
