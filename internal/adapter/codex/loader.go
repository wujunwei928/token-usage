package codex

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// LoadCodexEvents loads and dedupes every usage event from the configured
// Codex homes, keeping the first copy of events repeated across sessions and
// merging their recorded service tiers.
func LoadCodexEvents(shared *core.SharedArgs) ([]TokenUsageEvent, error) {
	sources, err := usageSources()
	if err != nil {
		return nil, err
	}
	if len(sources) == 1 {
		return LoadCodexEventsFromDirectory(sources[0].Dir, shared.SingleThread)
	}
	groups := CollectDedupedUsageFiles(sources)
	plan := NewReplayPlan(groups, shared.SingleThread)
	var events []TokenUsageEvent
	for _, group := range groups {
		events = append(events, readSessionFiles(group.Dir, group.Files, plan, shared.SingleThread)...)
	}
	dedupeCodexEvents(&events)
	return events, nil
}

// LoadCodexEventsFromDirectory loads events from a single sessions directory.
func LoadCodexEventsFromDirectory(sessionsDir string, singleThread bool) ([]TokenUsageEvent, error) {
	files := CollectUsageFiles(sessionsDir)
	plan := NewReplayPlan([]UsageFileGroup{{Dir: sessionsDir, Files: files}}, singleThread)
	events := readSessionFiles(sessionsDir, files, plan, singleThread)
	dedupeCodexEvents(&events)
	return events, nil
}

func readSessionFiles(sessionsDir string, files []string, plan *ReplayPlan, singleThread bool) []TokenUsageEvent {
	perFile := common.ReadFilesParallel(files, singleThread, func(path string) []TokenUsageEvent {
		var events []TokenUsageEvent
		VisitSessionFile(sessionsDir, path, plan.ReplayPrefix(path), func(event TokenUsageEvent) {
			events = append(events, event)
		})
		return events
	})
	var events []TokenUsageEvent
	for _, fileEvents := range perFile {
		events = append(events, fileEvents...)
	}
	return events
}

// dedupeModelKey distinguishes a missing model from an empty one, matching
// the reference's Option<CompactString> key.
type dedupeModelKey struct {
	hasModel bool
	model    string
}

type dedupeKey struct {
	timestamp             string
	model                 dedupeModelKey
	inputTokens           uint64
	cachedInputTokens     uint64
	outputTokens          uint64
	reasoningOutputTokens uint64
	totalTokens           uint64
}

func dedupeCodexEvents(events *[]TokenUsageEvent) {
	indexes := map[dedupeKey]int{}
	deduped := make([]TokenUsageEvent, 0, len(*events))
	for i := range *events {
		event := &(*events)[i]
		modelKey := dedupeModelKey{}
		if event.Model != nil {
			modelKey = dedupeModelKey{hasModel: true, model: *event.Model}
		}
		key := dedupeKey{
			timestamp:             event.Timestamp,
			model:                 modelKey,
			inputTokens:           event.InputTokens,
			cachedInputTokens:     event.CachedInputTokens,
			outputTokens:          event.OutputTokens,
			reasoningOutputTokens: event.ReasoningOutputTokens,
			totalTokens:           event.TotalTokens,
		}
		if index, ok := indexes[key]; ok {
			deduped[index].ServiceTier = mergeServiceTiers(deduped[index].ServiceTier, event.ServiceTier)
		} else {
			indexes[key] = len(deduped)
			deduped = append(deduped, *event)
		}
	}
	*events = deduped
}
