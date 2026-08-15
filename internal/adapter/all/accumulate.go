package all

import (
	"math"
	"sort"

	"github.com/wujunwei/ccusage-go/internal/core"
)

func isInfNaN(v float64) bool {
	return math.IsInf(v, 0) || math.IsNaN(v)
}

func trunc(v float64) float64 { return math.Trunc(v) }

// accumulator merges rows that share a period, keeping per-agent breakdowns.
type accumulator struct {
	inputTokens     uint64
	outputTokens    uint64
	cacheCreation   uint64
	cacheRead       uint64
	totalTokens     uint64
	totalCost       float64
	models          map[string]struct{}
	agents          map[string]struct{}
	agentBreakdowns []Row
	agentIndexes    map[string]int
}

func (a *accumulator) add(row Row) {
	a.inputTokens += row.InputTokens
	a.outputTokens += row.OutputTokens
	a.cacheCreation += row.CacheCreation
	a.cacheRead += row.CacheRead
	a.totalTokens += row.TotalTokens
	a.totalCost += row.TotalCost
	if a.models == nil {
		a.models = map[string]struct{}{}
	}
	for _, model := range row.ModelsUsed {
		a.models[model] = struct{}{}
	}
	if a.agents == nil {
		a.agents = map[string]struct{}{}
	}
	if row.MetadataAgents != nil {
		for _, agent := range row.MetadataAgents {
			a.agents[agent] = struct{}{}
		}
	} else if row.Agent != "all" {
		a.agents[row.Agent] = struct{}{}
	}
	if a.agentIndexes == nil {
		a.agentIndexes = map[string]int{}
	}
	if index, ok := a.agentIndexes[row.Agent]; ok {
		mergeAgentBreakdown(&a.agentBreakdowns[index], row)
	} else {
		a.agentIndexes[row.Agent] = len(a.agentBreakdowns)
		breakdown := row
		breakdown.MetadataAgents = []string{row.Agent}
		breakdown.AgentBreakdowns = nil
		a.agentBreakdowns = append(a.agentBreakdowns, breakdown)
	}
}

func (a *accumulator) intoRow(period string) Row {
	breakdowns := a.agentBreakdowns
	for i := range breakdowns {
		breakdowns[i].Period = period
	}
	sort.SliceStable(breakdowns, func(i, j int) bool { return breakdowns[i].Agent < breakdowns[j].Agent })
	modelBreakdowns := aggregateModelBreakdowns(breakdowns)
	sort.SliceStable(modelBreakdowns, func(i, j int) bool {
		return modelBreakdowns[i].Cost > modelBreakdowns[j].Cost
	})
	models := make([]string, 0, len(a.models))
	for model := range a.models {
		models = append(models, model)
	}
	sort.Strings(models)
	agents := make([]string, 0, len(a.agents))
	for agent := range a.agents {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	return Row{
		Period:          period,
		Agent:           "all",
		ModelsUsed:      models,
		InputTokens:     a.inputTokens,
		OutputTokens:    a.outputTokens,
		CacheCreation:   a.cacheCreation,
		CacheRead:       a.cacheRead,
		TotalTokens:     a.totalTokens,
		TotalCost:       a.totalCost,
		MetadataAgents:  agents,
		AgentBreakdowns: &breakdowns,
		ModelBreakdowns: modelBreakdowns,
	}
}

func mergeAgentBreakdown(target *Row, source Row) {
	target.InputTokens += source.InputTokens
	target.OutputTokens += source.OutputTokens
	target.CacheCreation += source.CacheCreation
	target.CacheRead += source.CacheRead
	target.TotalTokens += source.TotalTokens
	target.TotalCost += source.TotalCost
	merged := append(target.ModelsUsed, source.ModelsUsed...)
	sort.Strings(merged)
	deduped := merged[:0]
	for i, m := range merged {
		if i == 0 || m != merged[i-1] {
			deduped = append(deduped, m)
		}
	}
	target.ModelsUsed = deduped
	target.ModelBreakdowns = mergeModelBreakdowns(target.ModelBreakdowns, source.ModelBreakdowns)
}

func mergeModelBreakdowns(existing, additional []core.ModelBreakdown) []core.ModelBreakdown {
	indexes := map[string]int{}
	var breakdowns []core.ModelBreakdown
	for _, item := range append(append([]core.ModelBreakdown(nil), existing...), additional...) {
		index, ok := indexes[item.ModelName]
		if !ok {
			index = len(breakdowns)
			indexes[item.ModelName] = index
			breakdowns = append(breakdowns, core.ModelBreakdown{ModelName: item.ModelName})
		}
		b := &breakdowns[index]
		b.InputTokens += item.InputTokens
		b.OutputTokens += item.OutputTokens
		b.CacheCreationTokens += item.CacheCreationTokens
		b.CacheReadTokens += item.CacheReadTokens
		b.ExtraTotalTokens += item.ExtraTotalTokens
		b.Cost += item.Cost
		b.MissingPricing = b.MissingPricing || item.MissingPricing
	}
	sort.SliceStable(breakdowns, func(i, j int) bool { return breakdowns[i].Cost > breakdowns[j].Cost })
	return breakdowns
}

func aggregateModelBreakdowns(rows []Row) []core.ModelBreakdown {
	indexes := map[string]int{}
	var breakdowns []core.ModelBreakdown
	for r := range rows {
		for _, item := range rows[r].ModelBreakdowns {
			index, ok := indexes[item.ModelName]
			if !ok {
				index = len(breakdowns)
				indexes[item.ModelName] = index
				breakdowns = append(breakdowns, core.ModelBreakdown{ModelName: item.ModelName})
			}
			b := &breakdowns[index]
			b.InputTokens += item.InputTokens
			b.OutputTokens += item.OutputTokens
			b.CacheCreationTokens += item.CacheCreationTokens
			b.CacheReadTokens += item.CacheReadTokens
			b.ExtraTotalTokens += item.ExtraTotalTokens
			b.Cost += item.Cost
			b.MissingPricing = b.MissingPricing || item.MissingPricing
		}
	}
	return breakdowns
}
