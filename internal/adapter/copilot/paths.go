// Package copilot is the GitHub Copilot CLI agent adapter: it reads the
// OpenTelemetry JSONL export under ~/.copilot/otel plus an explicit export
// file named by COPILOT_OTEL_FILE_EXPORTER_PATH.
package copilot

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
)

// CopilotOtelFileExporterPathEnv names an explicit OTel export file to read.
const CopilotOtelFileExporterPathEnv = "COPILOT_OTEL_FILE_EXPORTER_PATH"

// Paths resolves the OTel export files: every *.jsonl under ~/.copilot/otel
// (when present) plus the env-named export file (when it is a regular file),
// deduplicated and sorted like rust/adapters/copilot/src/paths.rs.
func Paths() []string {
	var files []string
	if home, err := os.UserHomeDir(); err == nil {
		defaultDir := filepath.Join(home, ".copilot", "otel")
		if isDir(defaultDir) {
			common.CollectUsageFiles(defaultDir, &files)
		}
	}
	if path := copilotExporterPath(); path != "" {
		files = append(files, path)
	}
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(files))
	for _, path := range files {
		if _, dup := seen[path]; !dup {
			seen[path] = struct{}{}
			unique = append(unique, path)
		}
	}
	sort.Strings(unique)
	return unique
}

func copilotExporterPath() string {
	raw, ok := os.LookupEnv(CopilotOtelFileExporterPathEnv)
	if !ok {
		return ""
	}
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
