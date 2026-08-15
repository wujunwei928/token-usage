// Package codex is the Go port of the Rust ccusage-adapter-codex crate: it
// reads Codex session logs (~/.codex/sessions and archived_sessions), replays
// forked-session history dedup, and aggregates token usage into period groups.
package codex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// UsageSource is one directory that may hold Codex session logs, plus the home
// directory whose relative paths scope its dedupe keys.
type UsageSource struct {
	Dir         string
	dedupeScope string
}

// usageSources resolves every directory to scan from the configured homes.
func usageSources() ([]UsageSource, error) {
	homes, err := CodexHomePaths()
	if err != nil {
		return nil, err
	}
	return usageSourcesFromHomes(homes), nil
}

func usageSourcesFromHomes(homes []string) []UsageSource {
	var paths []UsageSource
	seen := map[string]struct{}{}
	for _, home := range homes {
		sessions := filepath.Join(home, "sessions")
		archived := filepath.Join(home, "archived_sessions")
		foundUsageDir := false
		if isDir(sessions) {
			if _, ok := seen[sessions]; !ok {
				seen[sessions] = struct{}{}
				paths = append(paths, UsageSource{Dir: sessions, dedupeScope: home})
			}
			foundUsageDir = true
		}
		if isDir(archived) {
			if _, ok := seen[archived]; !ok {
				seen[archived] = struct{}{}
				paths = append(paths, UsageSource{Dir: archived, dedupeScope: home})
			}
			foundUsageDir = true
		}
		if !foundUsageDir {
			if _, ok := seen[home]; !ok {
				seen[home] = struct{}{}
				paths = append(paths, UsageSource{Dir: home, dedupeScope: home})
			}
		}
	}
	return paths
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// UsageFileGroup is one source directory plus the files it contributes after
// cross-source dedupe.
type UsageFileGroup struct {
	Dir   string
	Files []string
}

// CollectUsageFiles gathers and sorts the *.jsonl files under dir.
func CollectUsageFiles(dir string) []string {
	var files []string
	common.CollectUsageFiles(dir, &files)
	sort.Strings(files)
	return files
}

// CollectDedupedUsageFiles drops files whose (home, relative path) pair was
// already contributed by an earlier source, so an archived copy never shadows
// the active session file of the same name.
func CollectDedupedUsageFiles(sources []UsageSource) []UsageFileGroup {
	type key struct{ scope, relative string }
	seen := map[key]struct{}{}
	groups := make([]UsageFileGroup, 0, len(sources))
	for _, source := range sources {
		var files []string
		for _, file := range CollectUsageFiles(source.Dir) {
			relative, err := filepath.Rel(source.Dir, file)
			if err != nil {
				relative = file
			}
			k := key{source.dedupeScope, relative}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			files = append(files, file)
		}
		groups = append(groups, UsageFileGroup{Dir: source.Dir, Files: files})
	}
	return groups
}

// CodexHomePaths resolves the CODEX_HOME env var (comma-separated, trimmed,
// non-empty entries) or falls back to ~/.codex.
func CodexHomePaths() ([]string, error) {
	if envPaths, ok := os.LookupEnv("CODEX_HOME"); ok {
		var paths []string
		for _, part := range strings.Split(envPaths, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				paths = append(paths, trimmed)
			}
		}
		return paths, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &core.CLIError{Message: "home directory is not set"}
	}
	return []string{filepath.Join(home, ".codex")}, nil
}
