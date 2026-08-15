package all

import (
	"sort"

	"github.com/wujunwei928/token-usage/internal/adapter/codex"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The codex adapter plugs into the unified report at roster index 1.
func init() {
	RegisterSpec(1, func(shared *core.SharedArgs) Spec {
		loaderShared := *shared
		loaderShared.JSON = true
		pricing := core.LoadWithOverrides(shared.Offline, logLevelNotQuiet(), shared.PricingOverrides)
		speed := codex.ResolveSpeed(codex.SpeedAuto)
		return Spec{
			Index: 1,
			Agent: "codex",
			Load: func(kind ReportKind) (AgentRows, error) {
				return loadCodexRows(kind, &loaderShared, pricing, speed)
			},
		}
	})
}

func logLevelNotQuiet() bool {
	level := core.LogLevel()
	return level == nil || *level != 0
}

// loadCodexRows mirrors load_codex_rows: without date bounds it loads groups
// directly; with since/until it loads every event first so detection is based
// on the unfiltered data, then filters and aggregates.
func loadCodexRows(kind ReportKind, shared *core.SharedArgs, pricing *core.PricingMap, speed codex.SpeedPolicy) (AgentRows, error) {
	codexKind := codex.KindDaily
	if kind == KindSession {
		codexKind = codex.KindSession
	}
	if shared.Since == nil && shared.Until == nil {
		groups, err := codex.LoadGroups(shared, codexKind)
		if err != nil {
			return AgentRows{}, err
		}
		return AgentRows{Rows: codexGroupRows(groups, pricing, speed), Detected: groups.Len() > 0}, nil
	}
	events, err := codex.LoadCodexEvents(shared)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(events) > 0
	if err := codex.FilterEventsByDate(&events, shared); err != nil {
		return AgentRows{}, err
	}
	groups, err := codex.AggregateEvents(events, codexKind, shared.Timezone)
	if err != nil {
		return AgentRows{}, err
	}
	return AgentRows{Rows: codexGroupRows(groups, pricing, speed), Detected: detected}, nil
}

func codexGroupRows(groups *codex.Groups, pricing *core.PricingMap, speed codex.SpeedPolicy) []Row {
	entries := groups.Sorted()
	rows := make([]Row, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, codexGroupRow(entry.Period, entry.Group, pricing, speed))
	}
	return rows
}

// codexGroupRow converts one codex period group into a unified row with the
// codex-specific metadata (last activity and reasoning tokens).
func codexGroupRow(period string, group *codex.Group, pricing *core.PricingMap, speed codex.SpeedPolicy) Row {
	modelNames := group.SortedModels()
	breakdowns := make([]core.ModelBreakdown, 0, len(modelNames))
	for _, model := range modelNames {
		usage := group.Models[model]
		breakdowns = append(breakdowns, core.ModelBreakdown{
			ModelName:           model,
			InputTokens:         codex.NonCachedInputTokens(usage.InputTokens, usage.CachedInputTokens),
			OutputTokens:        usage.OutputTokens,
			CacheCreationTokens: 0,
			CacheReadTokens:     usage.CachedInputTokens,
			ExtraTotalTokens:    0,
			Cost:                codex.CalculateModelCost(model, usage, pricing, speed),
			MissingPricing:      codex.ModelMissingPricing(model, usage, pricing),
		})
	}
	sort.SliceStable(breakdowns, func(i, j int) bool { return breakdowns[i].Cost > breakdowns[j].Cost })
	lastActivity := core.JNullV
	if group.LastActivity != nil {
		lastActivity = core.JStrV(*group.LastActivity)
	}
	metadata := core.JObjV(
		"lastActivity", lastActivity,
		"reasoningOutputTokens", core.JUintV(group.ReasoningOutputTokens),
	)
	return Row{
		Period:          period,
		Agent:           "codex",
		ModelsUsed:      modelNames,
		InputTokens:     codex.NonCachedInputTokens(group.InputTokens, group.CachedInputTokens),
		OutputTokens:    group.OutputTokens,
		CacheCreation:   0,
		CacheRead:       group.CachedInputTokens,
		TotalTokens:     group.TotalTokens,
		TotalCost:       codex.CalculateGroupCost(group, pricing, speed),
		Metadata:        &metadata,
		MetadataAgents:  []string{"codex"},
		AgentBreakdowns: nil,
		ModelBreakdowns: breakdowns,
	}
}
