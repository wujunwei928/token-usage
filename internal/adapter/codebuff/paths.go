// Package codebuff ports the Rust ccusage-adapter-codebuff crate: usage
// reports from Codebuff (manicode) chat-message transcripts.
package codebuff

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodebuffDataDirEnv overrides the Codebuff data root (comma-separated).
const CodebuffDataDirEnv = "CODEBUFF_DATA_DIR"

var channels = []string{"manicode", "manicode-dev", "manicode-staging"}

// DiscoverChatFiles gathers every chat-messages.json under the Codebuff
// project roots, in sorted path order.
func DiscoverChatFiles() ([]string, error) {
	var files []string
	for _, root := range codebuffProjectRoots() {
		collectFilesWithExtension(root, "json", &files)
	}
	filtered := make([]string, 0, len(files))
	for _, path := range files {
		if filepath.Base(path) == "chat-messages.json" {
			filtered = append(filtered, path)
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func codebuffProjectRoots() []string {
	var roots []string
	if override := os.Getenv(CodebuffDataDirEnv); override != "" {
		for _, part := range strings.Split(override, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				roots = append(roots, part)
			}
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		for _, channel := range channels {
			roots = append(roots, filepath.Join(home, ".config", channel))
		}
	}
	seen := map[string]bool{}
	var projectRoots []string
	for _, root := range roots {
		projectRoot := root
		if filepath.Base(root) != "projects" {
			projectRoot = filepath.Join(root, "projects")
		}
		if info, err := os.Stat(projectRoot); err != nil || !info.IsDir() {
			continue
		}
		if seen[projectRoot] {
			continue
		}
		seen[projectRoot] = true
		projectRoots = append(projectRoots, projectRoot)
	}
	return projectRoots
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
