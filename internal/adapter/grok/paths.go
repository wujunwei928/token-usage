// Package grok ports the Rust ccusage-adapter-grok crate: usage reports
// from the Grok CLI's per-session update streams.
package grok

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GrokHomeEnv overrides the Grok data root (single root).
const GrokHomeEnv = "GROK_HOME"

// SessionFiles pairs one session's updates.jsonl with its optional summary.
type SessionFiles struct {
	Updates string
	Summary *string
}

// DiscoverSessionFiles finds every sessions/**/updates.jsonl under the
// resolved root, sorted by path.
func DiscoverSessionFiles() ([]SessionFiles, error) {
	var files []SessionFiles
	root, ok := resolveRoot()
	if !ok {
		return files, nil
	}
	sessions := filepath.Join(root, "sessions")
	if info, err := os.Stat(sessions); err == nil && info.IsDir() {
		var updates []string
		collectFilesWithExtension(sessions, "jsonl", &updates)
		for _, updatesPath := range updates {
			if filepath.Base(updatesPath) != "updates.jsonl" {
				continue
			}
			files = append(files, SessionFiles{
				Updates: updatesPath,
				Summary: siblingSummary(updatesPath),
			})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Updates < files[j].Updates })
	return files, nil
}

// resolveRoot resolves the Grok data root from GROK_HOME, then ~/.grok.
func resolveRoot() (string, bool) {
	if home := os.Getenv(GrokHomeEnv); strings.TrimSpace(home) != "" {
		path := strings.TrimSpace(home)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, true
		}
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(home, ".grok")
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, true
	}
	return "", false
}

func siblingSummary(updates string) *string {
	summary := filepath.Join(filepath.Dir(updates), "summary.json")
	if info, err := os.Stat(summary); err == nil && info.Mode().IsRegular() {
		return &summary
	}
	return nil
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
