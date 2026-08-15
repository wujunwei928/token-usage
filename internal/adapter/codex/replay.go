package codex

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// parentReplay links a forked session to the session it replayed; path is nil
// when the referenced parent log is not part of the scanned files.
type parentReplay struct {
	path     *string
	forkedAt *int64
}

type parentUsage struct {
	timestamps []*int64
	usage      []RawUsage
}

// ReplayPlan carries the fork graph and parent usage streams needed to drop
// replayed history from forked sessions.
type ReplayPlan struct {
	parentByChild map[string]parentReplay
	usageByParent map[string]parentUsage
}

// NewReplayPlan reads session metadata for every file, links children to the
// parent logs they forked from, and preloads the parents' usage streams.
func NewReplayPlan(groups []UsageFileGroup, singleThread bool) *ReplayPlan {
	type fileEntry struct {
		path        string
		sessionsDir string
	}
	var files []fileEntry
	for _, group := range groups {
		for _, file := range group.Files {
			files = append(files, fileEntry{file, group.Dir})
		}
	}
	paths := make([]string, len(files))
	for i := range files {
		paths[i] = files[i].path
	}
	metadata := common.ReadFilesParallel(paths, singleThread, readSessionMetadata)

	// The same session can be reachable from more than one source directory,
	// so keep the first file and stay independent of source ordering.
	filesBySessionID := map[string]string{}
	for i := range files {
		if metadata[i].sessionID != nil {
			if _, ok := filesBySessionID[*metadata[i].sessionID]; !ok {
				filesBySessionID[*metadata[i].sessionID] = files[i].path
			}
		}
	}
	parentByChild := map[string]parentReplay{}
	for i := range files {
		child := files[i].path
		if metadata[i].parentID == nil {
			continue
		}
		replay := parentReplay{forkedAt: metadata[i].timestamp}
		if parentPath, ok := filesBySessionID[*metadata[i].parentID]; ok && parentPath != child {
			// A session listing itself as its own parent would match its
			// whole stream and drop every event it recorded.
			replay.path = &parentPath
		}
		parentByChild[child] = replay
	}
	parentPaths := map[string]struct{}{}
	for _, parent := range parentByChild {
		if parent.path != nil {
			parentPaths[*parent.path] = struct{}{}
		}
	}
	var parentList []fileEntry
	for _, file := range files {
		if _, ok := parentPaths[file.path]; ok {
			parentList = append(parentList, file)
		}
	}

	parentPathList := make([]string, len(parentList))
	for i := range parentList {
		parentPathList[i] = parentList[i].path
	}
	usageByParent := map[string]parentUsage{}
	if len(parentList) > 0 {
		indexByPath := map[string]int{}
		for i := range parentList {
			indexByPath[parentList[i].path] = i
		}
		streams := common.ReadFilesParallel(parentPathList, singleThread, func(path string) parentUsage {
			entry := parentList[indexByPath[path]]
			timestamps, usage := readUsageEvents(entry.sessionsDir, path)
			parsed := make([]*int64, len(timestamps))
			for j, timestamp := range timestamps {
				if millis, ok := core.ParseTSTimestamp(timestamp); ok {
					value := millis
					parsed[j] = &value
				}
			}
			return parentUsage{timestamps: parsed, usage: usage}
		})
		for i := range parentList {
			usageByParent[parentList[i].path] = streams[i]
		}
	}
	return &ReplayPlan{parentByChild: parentByChild, usageByParent: usageByParent}
}

// ReplayPrefix returns the usage history that child replayed from the session
// it forked from: nil for sessions that are not forks, and an empty non-nil
// slice for forks whose parent log is unavailable (which tells the parser to
// fall back to the rewritten-burst heuristic).
func (p *ReplayPlan) ReplayPrefix(child string) []RawUsage {
	parent, ok := p.parentByChild[child]
	if !ok {
		return nil
	}
	if parent.path == nil {
		return []RawUsage{}
	}
	stream, ok := p.usageByParent[*parent.path]
	if !ok {
		return []RawUsage{}
	}
	// Usage the parent recorded after the fork was never replayed, so it must
	// not mask the child's own events.
	replayLen := len(stream.usage)
	if parent.forkedAt != nil {
		for i, timestamp := range stream.timestamps {
			if timestamp != nil && *timestamp > *parent.forkedAt {
				replayLen = i
				break
			}
		}
	}
	return stream.usage[:replayLen]
}

func readUsageEvents(sessionsDir, path string) ([]string, []RawUsage) {
	var timestamps []string
	var usage []RawUsage
	VisitSessionFile(sessionsDir, path, nil, func(event TokenUsageEvent) {
		timestamps = append(timestamps, event.Timestamp)
		usage = append(usage, RawUsage{
			InputTokens:           event.InputTokens,
			CachedInputTokens:     event.CachedInputTokens,
			OutputTokens:          event.OutputTokens,
			ReasoningOutputTokens: event.ReasoningOutputTokens,
			TotalTokens:           event.TotalTokens,
		})
	})
	return timestamps, usage
}

type sessionMetadata struct {
	sessionID *string
	parentID  *string
	timestamp *int64
}

// readSessionMetadata reads the first line of a session log for its
// session_meta payload (id, fork parent, creation timestamp).
func readSessionMetadata(path string) sessionMetadata {
	file, err := os.Open(path)
	if err != nil {
		return sessionMetadata{}
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	line, readErr := reader.ReadBytes('\n')
	if len(line) == 0 {
		return sessionMetadata{}
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(line, &value); err != nil {
		return sessionMetadata{}
	}
	_ = readErr
	metadata := sessionMetadata{}
	entryType := ""
	if raw, ok := value["type"]; ok {
		_ = json.Unmarshal(raw, &entryType)
	}
	if timestamp, hard := timestampValue(value["timestamp"]); !hard {
		if normalized := timestamp.normalize(); normalized != nil {
			if millis, ok := core.ParseTSTimestamp(*normalized); ok {
				value := millis
				metadata.timestamp = &value
			}
		}
	}
	if entryType == "session_meta" {
		if payload, ok := objectFields(value["payload"]); ok {
			var id string
			var hasID bool
			if raw, ok := payload["id"]; ok {
				hasID = json.Unmarshal(raw, &id) == nil
			}
			if hasID {
				metadata.sessionID = &id
			}
			parent := ""
			if raw, ok := payload["forked_from_id"]; ok {
				_ = json.Unmarshal(raw, &parent)
			}
			if parent == "" {
				if source, ok := objectFields(payload["source"]); ok {
					if subagent, ok := objectFields(source["subagent"]); ok {
						if spawn, ok := objectFields(subagent["thread_spawn"]); ok {
							if raw, ok := spawn["parent_thread_id"]; ok {
								_ = json.Unmarshal(raw, &parent)
							}
						}
					}
				}
			}
			if parent != "" {
				metadata.parentID = &parent
			}
		}
	}
	return metadata
}
