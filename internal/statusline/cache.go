package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// StatuslineCache is the semaphore file payload stored per session under
// ${TMPDIR}/ccusage-semaphore/<session>.lock.
type StatuslineCache struct {
	Date            string  `json:"date"`
	LastOutput      string  `json:"lastOutput"`
	LastUpdateTime  uint64  `json:"lastUpdateTime"`
	TranscriptPath  string  `json:"transcriptPath"`
	TranscriptMtime uint64  `json:"transcriptMtime"`
	IsUpdating      bool    `json:"isUpdating"`
	PID             *uint32 `json:"pid"`
}

func completedCache(hook *Hook, lastOutput string, transcriptMtime, lastUpdateTime uint64) *StatuslineCache {
	return &StatuslineCache{
		Date:            core.FormatRFC3339Millis(int64(lastUpdateTime)),
		LastOutput:      lastOutput,
		LastUpdateTime:  lastUpdateTime,
		TranscriptPath:  hook.TranscriptPath,
		TranscriptMtime: transcriptMtime,
		IsUpdating:      false,
		PID:             nil,
	}
}

func updatingCache(hook *Hook, transcriptMtime uint64, previous *StatuslineCache, now uint64) *StatuslineCache {
	cache := &StatuslineCache{
		Date:            core.FormatRFC3339Millis(int64(now)),
		TranscriptPath:  hook.TranscriptPath,
		TranscriptMtime: transcriptMtime,
		IsUpdating:      true,
	}
	pid := uint32(os.Getpid())
	cache.PID = &pid
	if previous != nil {
		cache.LastOutput = previous.LastOutput
		cache.LastUpdateTime = previous.LastUpdateTime
	}
	return cache
}

// cachedStatuslineOutput decides whether the cached line can be reused:
// fresh and transcript-unchanged always reuses; an expired or stale entry
// still reuses while a live process holds the updating marker.
func cachedStatuslineOutput(cache *StatuslineCache, currentMtime, now, refreshInterval uint64) *string {
	if cache.LastOutput == "" {
		return nil
	}
	elapsed := uint64(0)
	if now > cache.LastUpdateTime {
		elapsed = now - cache.LastUpdateTime
	}
	expired := elapsed >= refreshInterval*1000
	fileModified := cache.TranscriptMtime != currentMtime
	if expired || fileModified {
		if cache.IsUpdating && cache.PID != nil && processIsAlive(*cache.PID) {
			output := cache.LastOutput
			return &output
		}
		return nil
	}
	output := cache.LastOutput
	return &output
}

func processIsAlive(pid uint32) bool {
	return pid != 0 && syscall.Kill(int(pid), 0) == nil
}

func statuslineCachePath(sessionID string) string {
	return filepath.Join(os.TempDir(), "ccusage-semaphore", sessionID+".lock")
}

func readStatuslineCache(path string) *StatuslineCache {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache StatuslineCache
	if json.Unmarshal(raw, &cache) != nil {
		return nil
	}
	return &cache
}

func writeStatuslineCache(path string, cache *StatuslineCache) {
	if parent := filepath.Dir(path); parent != "" {
		_ = os.MkdirAll(parent, 0o755)
	}
	raw, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644)
}

func markStatuslineCacheUpdating(path string, hook *Hook, transcriptMtime uint64, previous *StatuslineCache) {
	writeStatuslineCache(path, updatingCache(hook, transcriptMtime, previous, nowMillis()))
}

func releaseStatuslineCache(path string) {
	if cache := readStatuslineCache(path); cache != nil {
		cache.IsUpdating = false
		cache.PID = nil
		writeStatuslineCache(path, cache)
	}
}

func transcriptMtimeMs(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	millis := info.ModTime().UnixMilli()
	if millis < 0 {
		return 0
	}
	return uint64(millis)
}

func nowMillis() uint64 {
	millis := nowFunc()
	if millis < 0 {
		return 0
	}
	return uint64(millis)
}
