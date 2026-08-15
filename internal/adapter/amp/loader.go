package amp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers and parses every Amp thread file, returning entries
// sorted by timestamp (stable, so same-timestamp entries keep file order).
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	var entries []core.LoadedEntry
	tz := core.ParseTZ(shared.Timezone)
	paths, err := Paths()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		threadsDir := filepath.Join(path, "threads")
		var files []string
		collectFilesWithExtension(threadsDir, "json", &files)
		perFile := common.ReadFilesParallel(files, shared.SingleThread, func(file string) []core.LoadedEntry {
			fileEntries, err := readThreadFile(file, tz, shared.Mode, pricing)
			if err != nil {
				core.DebugLog(shared, "Failed to read Amp thread file "+file+": "+err.Error())
				return nil
			}
			return fileEntries
		})
		for _, fileEntries := range perFile {
			entries = append(entries, fileEntries...)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

// collectFilesWithExtension mirrors the reference's recursive extension
// collector: regular files whose Path::extension equals ext (a leading-dot
// file like ".json" has no extension).
func collectFilesWithExtension(dir, ext string, files *[]string) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range dirEntries {
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			collectFilesWithExtension(path, ext, files)
		} else if info.Mode().IsRegular() && fileExtension(entry.Name()) == ext {
			*files = append(*files, path)
		}
	}
}

// fileExtension returns the text after the last dot when it is not the name's
// first character (Rust Path::extension semantics).
func fileExtension(name string) string {
	if idx := strings.LastIndexByte(name, '.'); idx > 0 {
		return name[idx+1:]
	}
	return ""
}
