package all

import (
	"fmt"
	"os"
	"sort"

	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/terminal"
)

// AgentLabel renders an agent id for display.
func AgentLabel(agent string) string {
	labels := map[string]string{
		"all": "All", "claude": "Claude", "codex": "Codex", "opencode": "OpenCode",
		"amp": "Amp", "droid": "Droid", "codebuff": "Codebuff", "hermes": "Hermes",
		"pi": "pi-agent", "goose": "Goose", "openclaw": "OpenClaw", "kilo": "Kilo",
		"copilot": "GitHub Copilot CLI", "gemini": "Gemini CLI", "kimi": "Kimi",
		"qwen": "Qwen", "omp": "oh-my-pi", "cline": "Cline",
	}
	if label, ok := labels[agent]; ok {
		return label
	}
	return agent
}

// ReportTitle renders the multi-line box title with the detected agents line.
func ReportTitle(kind ReportKind, rows []Row, detectedAgents []string) string {
	kindName := map[ReportKind]string{
		KindDaily: "Daily", KindWeekly: "Weekly", KindMonthly: "Monthly", KindSession: "Session",
	}[kind]
	return fmt.Sprintf("Coding (Agent) CLI Usage Report - %s\nDetected: %s", kindName, detectedAgentLabels(rows, detectedAgents))
}

func detectedAgentLabels(rows []Row, detectedAgents []string) string {
	set := map[string]struct{}{}
	if len(detectedAgents) == 0 {
		for i := range rows {
			if rows[i].MetadataAgents != nil {
				for _, agent := range rows[i].MetadataAgents {
					set[agent] = struct{}{}
				}
			} else if rows[i].Agent != "all" {
				set[rows[i].Agent] = struct{}{}
			}
			if rows[i].AgentBreakdowns != nil {
				for _, breakdown := range *rows[i].AgentBreakdowns {
					set[breakdown.Agent] = struct{}{}
				}
			}
		}
	} else {
		for _, agent := range detectedAgents {
			set[agent] = struct{}{}
		}
	}
	if len(set) == 0 {
		return "None"
	}
	agents := make([]string, 0, len(set))
	for agent := range set {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	labels := make([]string, len(agents))
	for i, agent := range agents {
		labels[i] = AgentLabel(agent)
	}
	out := ""
	for i, label := range labels {
		if i > 0 {
			out += ", "
		}
		out += label
	}
	return out
}

func firstColumn(kind ReportKind) string {
	return map[ReportKind]string{
		KindDaily: "Date", KindWeekly: "Week", KindMonthly: "Month", KindSession: "Session",
	}[kind]
}

// PrintTable renders the unified report table.
func PrintTable(rows []Row, kind ReportKind, shared *core.SharedArgs, detectedAgents []string) error {
	style := terminal.TerminalStyle{Color: shared.Color, LogLevel: core.LogLevel(), NoColor: shared.NoColor}
	terminal.PrintBoxTitle(ReportTitle(kind, rows, detectedAgents), style)
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No usage data found.")
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	compact := core.ShouldUseCompactLayout(shared, terminal.IsStdoutTerminal(), terminalWidth, core.UsageCompactWidthThreshold)
	var headers []string
	var aligns []terminal.Align
	if compact {
		headers = []string{firstColumn(kind), "Agent", "Models", "Input", "Output", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
	} else {
		headers = []string{firstColumn(kind), "Agent", "Models", "Input", "Output", "Cache Create", "Cache Read", "Total Tokens", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
	}
	if shared.NoCost {
		headers = headers[:len(headers)-1]
		aligns = aligns[:len(aligns)-1]
	}
	table := terminal.NewTable(headers, aligns, style).
		WithTerminalWidth(terminalWidth).
		WithDateCompaction(true)
	for i := range rows {
		row := &rows[i]
		table.Push(tableRow(row, compact, false, shared.NoCost))
		if row.AgentBreakdowns != nil {
			for b := range *row.AgentBreakdowns {
				breakdown := &(*row.AgentBreakdowns)[b]
				table.Push(tableRow(breakdown, compact, true, shared.NoCost))
				if shared.Breakdown && len(breakdown.ModelBreakdowns) > 0 {
					pushModelBreakdownRows(table, breakdown.ModelBreakdowns, compact, shared)
				}
			}
		} else if shared.Breakdown && len(row.ModelBreakdowns) > 0 {
			pushModelBreakdownRows(table, row.ModelBreakdowns, compact, shared)
		}
	}
	table.Separator()
	input, output, cacheCreate, cacheRead, tableTotalTokens, totalCost := totalsParts(rows)
	var totalRow []string
	if compact {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			"",
			terminal.Colorize(style, core.FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
		}
	} else {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			"",
			terminal.Colorize(style, core.FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(cacheCreate), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(cacheRead), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(tableTotalTokens), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
		}
	}
	if shared.NoCost {
		totalRow = totalRow[:len(totalRow)-1]
	}
	table.Push(totalRow)
	if err := table.Print(os.Stdout); err != nil {
		return err
	}
	PrintMissingPricingWarnings(rows, shared.OfflineEffective())
	if compact {
		fmt.Fprintln(os.Stderr, "\nRunning in Compact Mode")
		fmt.Fprintln(os.Stderr, "Expand terminal width to see cache metrics and total tokens")
	}
	return nil
}

func tableTotalTokens(row *Row) uint64 {
	return row.InputTokens + row.OutputTokens + row.CacheCreation + row.CacheRead
}

func totalsParts(rows []Row) (input, output, cacheCreate, cacheRead, totalTableTokens uint64, totalCost float64) {
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreation
		cacheRead += rows[i].CacheRead
		totalTableTokens += tableTotalTokens(&rows[i])
		totalCost += rows[i].TotalCost
	}
	return
}

func tableRow(row *Row, compact, breakdown, noCost bool) []string {
	period := ""
	if !breakdown {
		period = row.Period
	}
	agent := AgentLabel(row.Agent)
	if breakdown {
		agent = "- " + AgentLabel(row.Agent)
	} else if row.AgentBreakdowns != nil {
		agent = "All"
	}
	models := ""
	if row.AgentBreakdowns == nil {
		models = core.FormatModelsMultiline(row.ModelsUsed)
	}
	var values []string
	if compact {
		values = []string{
			period,
			agent,
			models,
			core.FormatNumber(row.InputTokens),
			core.FormatNumber(row.OutputTokens),
			core.FormatCurrency(row.TotalCost),
		}
	} else {
		values = []string{
			period,
			agent,
			models,
			core.FormatNumber(row.InputTokens),
			core.FormatNumber(row.OutputTokens),
			core.FormatNumber(row.CacheCreation),
			core.FormatNumber(row.CacheRead),
			core.FormatNumber(tableTotalTokens(row)),
			core.FormatCurrency(row.TotalCost),
		}
	}
	if noCost {
		values = values[:len(values)-1]
	}
	return values
}

func pushModelBreakdownRows(table *terminal.SimpleTable, breakdowns []core.ModelBreakdown, compact bool, shared *core.SharedArgs) {
	style := terminal.TerminalStyle{Color: shared.Color, LogLevel: core.LogLevel(), NoColor: shared.NoColor}
	for i := range breakdowns {
		b := &breakdowns[i]
		total := b.InputTokens + b.OutputTokens + b.CacheCreationTokens + b.CacheReadTokens
		model := terminal.Colorize(style, "- "+core.ShortModelName(b.ModelName), terminal.ColorGrey)
		var row []string
		if compact {
			row = []string{
				"",
				"",
				model,
				terminal.Colorize(style, core.FormatNumber(b.InputTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatNumber(b.OutputTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatCurrency(b.Cost), terminal.ColorGrey),
			}
		} else {
			row = []string{
				"",
				"",
				model,
				terminal.Colorize(style, core.FormatNumber(b.InputTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatNumber(b.OutputTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatNumber(b.CacheCreationTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatNumber(b.CacheReadTokens), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatNumber(total), terminal.ColorGrey),
				terminal.Colorize(style, core.FormatCurrency(b.Cost), terminal.ColorGrey),
			}
		}
		if shared.NoCost {
			row = row[:len(row)-1]
		}
		table.Push(row)
	}
}

// PrintMissingPricingWarnings emits warnings for models without pricing.
func PrintMissingPricingWarnings(rows []Row, offline bool) {
	var models []string
	for i := range rows {
		for b := range rows[i].ModelBreakdowns {
			if rows[i].ModelBreakdowns[b].MissingPricing {
				models = append(models, rows[i].ModelBreakdowns[b].ModelName)
			}
		}
	}
	core.PrintMissingPricingWarningsForModels(models, offline)
}

// ReportJSON builds {<kind>: [...], totals: {...}} for the unified report.
func ReportJSON(rows []Row, kind ReportKind, includeAgents bool) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = rowJSON(&rows[i], includeAgents)
	}
	return core.JObjV(
		kind.String(), core.JArrV(items...),
		"totals", totalsJSON(rows),
	)
}

// Section is one rendered section of a --sections run.
type Section struct {
	Kind ReportKind
	Rows []Row
}

// SectionsReportJSON builds the ordered sections output; totals come from the
// command's own section.
func SectionsReportJSON(sections []Section, commandKind ReportKind, includeAgents bool) core.J {
	fields := []any{}
	for _, section := range sections {
		items := make([]core.J, len(section.Rows))
		for i := range section.Rows {
			items[i] = rowJSON(&section.Rows[i], includeAgents)
		}
		fields = append(fields, section.Kind.String(), core.JArrV(items...))
	}
	var commandRows []Row
	for _, section := range sections {
		if section.Kind == commandKind {
			commandRows = section.Rows
			break
		}
	}
	fields = append(fields, "totals", totalsJSON(commandRows))
	return core.JOrdObjV(fields...)
}

func rowJSON(row *Row, includeAgents bool) core.J {
	fields := agentJSONFields(row)
	fields = append(fields, "period", core.JStrV(row.Period))
	if row.MetadataAgents != nil {
		metadata := core.JObjV("agents", stringsArrJ(row.MetadataAgents))
		if row.Metadata != nil {
			metadata = *row.Metadata
		}
		fields = append(fields, "metadata", metadata)
	} else if row.Metadata != nil {
		fields = append(fields, "metadata", *row.Metadata)
	}
	if includeAgents && row.AgentBreakdowns != nil {
		breakdowns := make([]core.J, len(*row.AgentBreakdowns))
		for i := range *row.AgentBreakdowns {
			breakdowns[i] = core.JObjV(agentJSONFields(&(*row.AgentBreakdowns)[i])...)
		}
		fields = append(fields, "agents", core.JArrV(breakdowns...))
	}
	// Row objects serialize with sorted keys (serde Value maps); only the
	// sections wrapper preserves insertion order.
	return core.JObjV(fields...)
}

func agentJSONFields(row *Row) []any {
	breakdowns := make([]core.J, len(row.ModelBreakdowns))
	for i := range row.ModelBreakdowns {
		b := &row.ModelBreakdowns[i]
		breakdowns[i] = core.JObjV(
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			// The reference serializes model_breakdowns as raw structs, so a
			// breakdown cost is always an f64 (0.0), never json_float's
			// integer form.
			"cost", core.JFloatV(b.Cost),
			"inputTokens", core.JUintV(b.InputTokens),
			"modelName", core.JStrV(b.ModelName),
			"outputTokens", core.JUintV(b.OutputTokens),
		)
	}
	return []any{
		"agent", core.JStrV(row.Agent),
		"modelsUsed", stringsArrJ(row.ModelsUsed),
		"inputTokens", core.JUintV(row.InputTokens),
		"outputTokens", core.JUintV(row.OutputTokens),
		"cacheCreationTokens", core.JUintV(row.CacheCreation),
		"cacheReadTokens", core.JUintV(row.CacheRead),
		"totalTokens", core.JUintV(row.TotalTokens),
		"totalCost", jsonFloatJ(row.TotalCost),
		"modelBreakdowns", core.JArrV(breakdowns...),
	}
}

func totalsJSON(rows []Row) core.J {
	var input, output, cacheCreate, cacheRead, totalTokens uint64
	totalCost := negZero()
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreation
		cacheRead += rows[i].CacheRead
		totalTokens += rows[i].TotalTokens
		totalCost += rows[i].TotalCost
	}
	return core.JObjV(
		"inputTokens", core.JUintV(input),
		"outputTokens", core.JUintV(output),
		"cacheCreationTokens", core.JUintV(cacheCreate),
		"cacheReadTokens", core.JUintV(cacheRead),
		"totalTokens", core.JUintV(totalTokens),
		"totalCost", jsonFloatJ(totalCost),
	)
}

func stringsArrJ(values []string) core.J {
	items := make([]core.J, len(values))
	for i, v := range values {
		items[i] = core.JStrV(v)
	}
	return core.JArrV(items...)
}

func negZero() float64 { return -0.0 }
