// Package common holds ingestion helpers shared by agent adapters.
package common

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// CollectUsageFiles recursively gathers *.jsonl files under dir. Walk errors
// are silently ignored, mirroring the reference implementation.
func CollectUsageFiles(dir string, files *[]string) {
	// Raw readdir order, matching the reference's read_dir walk; callers that
	// need deterministic ordering sort explicitly afterwards.
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			CollectUsageFiles(path, files)
		} else if info.Mode().IsRegular() && strings.HasSuffix(path, ".jsonl") {
			*files = append(*files, path)
		}
	}
}

// ReadFilesParallel reads files across workers, preserving input order. The
// chunking is size-balanced but the result is order-stable regardless.
func ReadFilesParallel[T any](files []string, singleThread bool, read func(string) T) []T {
	workerCount := 1
	if !singleThread {
		workerCount = runtime.GOMAXPROCS(0)
		if workerCount > len(files) {
			workerCount = len(files)
		}
		if workerCount < 1 {
			workerCount = 1
		}
	}
	results := make([]T, len(files))
	if workerCount <= 1 || len(files) == 0 {
		for i, file := range files {
			results[i] = read(file)
		}
		return results
	}
	chunks := chunkFileIndexesBySize(files, workerCount)
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []int) {
			defer wg.Done()
			for _, index := range chunk {
				results[index] = read(files[index])
			}
		}(chunk)
	}
	wg.Wait()
	return results
}

// chunkFileIndexesBySize balances indexes across workers by file size.
func chunkFileIndexesBySize(files []string, workerCount int) [][]int {
	type sized struct {
		index int
		size  int64
	}
	sizes := make([]sized, len(files))
	for i, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			sizes[i] = sized{i, 0}
			continue
		}
		sizes[i] = sized{i, info.Size()}
	}
	sort.SliceStable(sizes, func(a, b int) bool { return sizes[a].size > sizes[b].size })
	chunks := make([][]int, workerCount)
	loads := make([]int64, workerCount)
	for _, item := range sizes {
		minChunk := 0
		for i := 1; i < workerCount; i++ {
			if loads[i] < loads[minChunk] {
				minChunk = i
			}
		}
		chunks[minChunk] = append(chunks[minChunk], item.index)
		loads[minChunk] += item.size
	}
	return chunks
}

// FilterLoadedEntriesByDate keeps entries whose date is inside the window.
func FilterLoadedEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	out := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			out = append(out, entries[i])
		}
	}
	return out
}

// SplitBytesLines splits file content on '\n', dropping a trailing empty line.
func SplitBytesLines(content []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}
