package codex

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// jsonFloat mirrors the reference json_float: whole finite floats render as
// integers.
func jsonFloat(v float64) core.J {
	if !isInfNaN(v) && v == truncFloat(v) && v >= -9223372036854775808.0 && v <= 9223372036854775807.0 {
		return core.JIntV(int64(v))
	}
	return core.JFloatV(v)
}

func isInfNaN(v float64) bool {
	return math.IsInf(v, 0) || math.IsNaN(v)
}

func truncFloat(v float64) float64 {
	return math.Trunc(v)
}

// ReportJSON builds {<kind>: [...], totals: {...}} for the codex reports.
func ReportJSON(groups *Groups, kind Kind, pricing *core.PricingMap, speed SpeedPolicy) core.J {
	rows := make([]core.J, 0, groups.Len())
	for _, entry := range groups.Sorted() {
		rows = append(rows, groupJSON(entry.Period, entry.Group, kind, pricing, speed))
	}
	return core.JObjV(
		kind.String(), core.JArrV(rows...),
		"totals", totalsJSON(groups, pricing, speed),
	)
}

func periodKey(kind Kind) string {
	switch kind {
	case KindDaily:
		return "date"
	case KindWeekly:
		return "week"
	case KindMonthly:
		return "month"
	default:
		return "sessionId"
	}
}

func groupJSON(period string, group *Group, kind Kind, pricing *core.PricingMap, speed SpeedPolicy) core.J {
	cost := CalculateGroupCost(group, pricing, speed)
	inputTokens := NonCachedInputTokens(group.InputTokens, group.CachedInputTokens)
	modelPairs := []any{}
	for _, model := range group.SortedModels() {
		modelPairs = append(modelPairs, model, modelUsageJSON(group.Models[model]))
	}
	pairs := []any{
		periodKey(kind), core.JStrV(period),
		"inputTokens", core.JUintV(inputTokens),
		"cacheCreationTokens", core.JUintV(0),
		"cacheReadTokens", core.JUintV(group.CachedInputTokens),
		"outputTokens", core.JUintV(group.OutputTokens),
		"reasoningOutputTokens", core.JUintV(group.ReasoningOutputTokens),
		"totalTokens", core.JUintV(group.TotalTokens),
		"costUSD", jsonFloat(cost),
		"models", core.JObjV(modelPairs...),
	}
	if kind == KindSession {
		lastActivity := core.JNullV
		if group.LastActivity != nil {
			lastActivity = core.JStrV(*group.LastActivity)
		}
		separator := strings.LastIndex(period, "/")
		sessionFile := period
		directory := ""
		if separator >= 0 {
			sessionFile = period[separator+1:]
			directory = period[:separator]
		}
		pairs = append(pairs,
			"lastActivity", lastActivity,
			"sessionFile", core.JStrV(sessionFile),
			"directory", core.JStrV(directory),
		)
	}
	return core.JObjV(pairs...)
}

func modelUsageJSON(usage *ModelUsage) core.J {
	return core.JObjV(
		"inputTokens", core.JUintV(NonCachedInputTokens(usage.InputTokens, usage.CachedInputTokens)),
		"cacheCreationTokens", core.JUintV(0),
		"cacheReadTokens", core.JUintV(usage.CachedInputTokens),
		"outputTokens", core.JUintV(usage.OutputTokens),
		"reasoningOutputTokens", core.JUintV(usage.ReasoningOutputTokens),
		"totalTokens", core.JUintV(usage.TotalTokens),
		"isFallback", core.JBoolV(usage.IsFallback),
	)
}

func totalsJSON(groups *Groups, pricing *core.PricingMap, speed SpeedPolicy) core.J {
	var input, cached, output, reasoning, total uint64
	cost := 0.0
	for _, entry := range groups.Sorted() {
		group := entry.Group
		input += NonCachedInputTokens(group.InputTokens, group.CachedInputTokens)
		cached += group.CachedInputTokens
		output += group.OutputTokens
		reasoning += group.ReasoningOutputTokens
		total += group.TotalTokens
		cost += CalculateGroupCost(group, pricing, speed)
	}
	return core.JObjV(
		"inputTokens", core.JUintV(input),
		"cacheCreationTokens", core.JUintV(0),
		"cacheReadTokens", core.JUintV(cached),
		"outputTokens", core.JUintV(output),
		"reasoningOutputTokens", core.JUintV(reasoning),
		"totalTokens", core.JUintV(total),
		"costUSD", jsonFloat(cost),
	)
}

// PrintTable renders the codex report table.
func PrintTable(groups *Groups, kind Kind, pricing *core.PricingMap, speed SpeedPolicy, shared *core.SharedArgs) error {
	if groups.Len() == 0 {
		fmt.Fprintln(os.Stderr, "No Codex usage data found.")
		return nil
	}
	firstColumn := map[Kind]string{
		KindDaily: "Date", KindWeekly: "Week", KindMonthly: "Month", KindSession: "Session",
	}[kind]
	kindName := map[Kind]string{
		KindDaily: "Daily", KindWeekly: "Weekly", KindMonthly: "Monthly", KindSession: "Session",
	}[kind]
	terminal.PrintBoxTitle(fmt.Sprintf("Codex Token Usage Report - %s", kindName), core.TerminalStyleFromShared(shared))
	headers := []string{firstColumn, "Models", "Input", "Output", "Reasoning", "Cache Read", "Total Tokens", "Cost (USD)"}
	aligns := []terminal.Align{
		terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight,
		terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight,
	}
	if shared.NoCost {
		headers = headers[:len(headers)-1]
		aligns = aligns[:len(aligns)-1]
	}
	table := terminal.NewTable(headers, aligns, core.TerminalStyleFromShared(shared)).
		WithTerminalWidth(terminal.TerminalWidth()).
		WithDateCompaction(true)
	var totalInput, totalCached, totalOutput, totalReasoning, totalTokens uint64
	totalCost := 0.0
	for _, entry := range groups.Sorted() {
		group := entry.Group
		inputTokens := NonCachedInputTokens(group.InputTokens, group.CachedInputTokens)
		cost := CalculateGroupCost(group, pricing, speed)
		totalInput += inputTokens
		totalCached += group.CachedInputTokens
		totalOutput += group.OutputTokens
		totalReasoning += group.ReasoningOutputTokens
		totalTokens += group.TotalTokens
		totalCost += cost
		models := core.FormatModelsMultiline(group.SortedModels())
		row := []string{
			entry.Period,
			models,
			core.FormatNumber(inputTokens),
			core.FormatNumber(group.OutputTokens),
			core.FormatNumber(group.ReasoningOutputTokens),
			core.FormatNumber(group.CachedInputTokens),
			core.FormatNumber(group.TotalTokens),
			core.FormatCurrency(cost),
		}
		if shared.NoCost {
			row = row[:len(row)-1]
		}
		table.Push(row)
	}
	table.Separator()
	style := core.TerminalStyleFromShared(shared)
	totalRow := []string{
		terminal.Colorize(style, "Total", terminal.ColorYellow),
		"",
		terminal.Colorize(style, core.FormatNumber(totalInput), terminal.ColorYellow),
		terminal.Colorize(style, core.FormatNumber(totalOutput), terminal.ColorYellow),
		terminal.Colorize(style, core.FormatNumber(totalReasoning), terminal.ColorYellow),
		terminal.Colorize(style, core.FormatNumber(totalCached), terminal.ColorYellow),
		terminal.Colorize(style, core.FormatNumber(totalTokens), terminal.ColorYellow),
		terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
	}
	if shared.NoCost {
		totalRow = totalRow[:len(totalRow)-1]
	}
	table.Push(totalRow)
	if err := table.Print(os.Stdout); err != nil {
		return err
	}
	core.PrintMissingPricingWarningsForModels(MissingPricingModels(groups, pricing), shared.Offline)
	return nil
}
