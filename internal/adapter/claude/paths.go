// Package claude is the Claude Code agent adapter: data discovery and loading.
package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
)

// CLIError mirrors the reference CliError display format.
type CLIError struct{ Message string }

func (e *CLIError) Error() string { return fmt.Sprintf("CliError(%q)", e.Message) }

// ClaudePaths resolves the Claude config directories to scan.
func ClaudePaths() ([]string, error) {
	var paths []string
	seen := map[string]struct{}{}
	if envPaths, ok := os.LookupEnv("CLAUDE_CONFIG_DIR"); ok {
		for _, raw := range strings.Split(envPaths, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			path := normalizeClaudeConfigPath(raw)
			if isDir(filepath.Join(path, "projects")) {
				if _, dup := seen[path]; !dup {
					seen[path] = struct{}{}
					paths = append(paths, path)
				}
			}
		}
		if len(paths) > 0 {
			return paths, nil
		}
		return nil, &CLIError{fmt.Sprintf(
			"No valid Claude data directories found in CLAUDE_CONFIG_DIR. Expected each path to be a Claude config directory containing 'projects/', or the 'projects/' directory itself: %s", envPaths)}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &CLIError{"home directory is not set"}
	}
	xdg, ok := os.LookupEnv("XDG_CONFIG_HOME")
	if !ok {
		xdg = filepath.Join(home, ".config")
	}
	for _, path := range []string{filepath.Join(xdg, "claude"), filepath.Join(home, ".claude")} {
		if isDir(filepath.Join(path, "projects")) {
			if _, dup := seen[path]; !dup {
				seen[path] = struct{}{}
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func normalizeClaudeConfigPath(raw string) string {
	path := expandHomePath(raw)
	if filepath.Base(path) == "projects" && isDir(path) {
		if parent := filepath.Dir(path); parent != "" {
			return parent
		}
	}
	return path
}

func expandHomePath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// IsProjectPathSegment validates a --project filter is a single path segment.
func IsProjectPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.Contains(value, "/") && !strings.Contains(value, "\\")
}

// UsageFiles collects the sorted JSONL files under the given config paths.
func UsageFiles(paths []string, projectFilter *string) []string {
	var files []string
	for _, path := range paths {
		projectsDir := filepath.Join(path, "projects")
		if projectFilter != nil && IsProjectPathSegment(*projectFilter) {
			common.CollectUsageFiles(filepath.Join(projectsDir, *projectFilter), &files)
		} else {
			common.CollectUsageFiles(projectsDir, &files)
		}
	}
	sort.Strings(files)
	return files
}

// ExtractProject returns the directory name after the first "projects" part.
func ExtractProject(path string) string {
	parts := splitPath(path)
	sawProjects := false
	for _, part := range parts {
		if sawProjects {
			if strings.TrimSpace(part) == "" {
				return "unknown"
			}
			return part
		}
		if part == "projects" {
			sawProjects = true
		}
	}
	return "unknown"
}

// ExtractSessionParts returns (sessionID, projectPath) for a usage file.
func ExtractSessionParts(path string) (string, string) {
	parts := splitPath(path)
	projectsIndex := -1
	for i, part := range parts {
		if part == "projects" {
			projectsIndex = i
			break
		}
	}
	var relative []string
	if projectsIndex >= 0 {
		relative = parts[projectsIndex+1:]
	} else {
		relative = parts
	}
	var fileSessionID string
	hasFileSessionID := false
	if len(relative) > 0 {
		name := relative[len(relative)-1]
		if strings.HasSuffix(name, ".jsonl") {
			candidate := strings.TrimSuffix(name, ".jsonl")
			if candidate != "" {
				fileSessionID = candidate
				hasFileSessionID = true
			}
		}
	}
	if len(relative) == 2 && hasFileSessionID {
		return fileSessionID, relative[0]
	}
	if len(relative) >= 4 && relative[len(relative)-2] == "subagents" {
		sessionID := relative[len(relative)-3]
		projectPath := strings.Join(relative[:len(relative)-3], string(filepath.Separator))
		if projectPath == "" {
			projectPath = "Unknown Project"
		}
		return sessionID, projectPath
	}
	// Mirror `relative.get(relative.len().saturating_sub(2))`: a lone
	// relative component is its own session id, fewer than that is "unknown".
	sessionID := "unknown"
	sessionIndex := len(relative) - 2
	if sessionIndex < 0 {
		sessionIndex = 0
	}
	if sessionIndex < len(relative) {
		sessionID = relative[sessionIndex]
	}
	var projectPath string
	if len(relative) > 2 {
		projectPath = strings.Join(relative[:len(relative)-2], string(filepath.Separator))
	} else {
		projectPath = "Unknown Project"
	}
	return sessionID, projectPath
}

func splitPath(path string) []string {
	cleaned := filepath.Clean(path)
	return strings.Split(cleaned, string(filepath.Separator))
}
