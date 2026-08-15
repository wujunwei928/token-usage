// Package openclaw ports the OpenClaw adapter (rust/adapters/openclaw):
// session-file discovery under ~/.openclaw and siblings (--open-claw-path and
// OPENCLAW_DIR overrides), the message/model_change line parser, and report
// summarization.
package openclaw

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEnv names the comma-separated override for the OpenClaw data directory.
const DirEnv = "OPENCLAW_DIR"

// DataPaths resolves the roots to scan: the custom --open-claw-path value
// first, then the OPENCLAW_DIR env var, then the home defaults. Only existing
// directories are kept, in order, deduplicated.
func DataPaths(customPath *string) []string {
	if customPath != nil && strings.TrimSpace(*customPath) != "" {
		return existingDirList(*customPath)
	}
	if raw, ok := os.LookupEnv(DirEnv); ok && strings.TrimSpace(raw) != "" {
		return existingDirList(raw)
	}
	home := agentHomeDir()
	if home == "" {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, dir := range []string{".openclaw", ".clawdbot", ".moltbot", ".moldbot"} {
		path := filepath.Join(home, dir)
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

// collectSessionFiles recursively gathers OpenClaw session files under root,
// sorted by path. Symlinks are skipped.
func collectSessionFiles(root string) ([]string, error) {
	var files []string
	if err := collectSessionFilesInner(root, &files); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func collectSessionFilesInner(path string, files *[]string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			if err := collectSessionFilesInner(child, files); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && isOpenClawSessionFile(entry.Name()) {
			*files = append(*files, child)
		}
	}
	return nil
}

// isOpenClawSessionFile accepts live, deleted, and reset session files.
func isOpenClawSessionFile(name string) bool {
	index := strings.Index(name, ".jsonl")
	if index < 0 {
		return false
	}
	suffix := name[index:]
	return suffix == ".jsonl" ||
		strings.HasPrefix(suffix, ".jsonl.deleted.") ||
		strings.HasPrefix(suffix, ".jsonl.reset.")
}
