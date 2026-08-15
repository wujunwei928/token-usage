package goose

import (
	"fmt"
	"math"
	"os"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// ReportKind selects the Goose report granularity.
type ReportKind int

// Report kinds.
const (
	ReportDaily ReportKind = iota
	ReportWeekly
	ReportMonthly
	ReportSession
)

// RowsKey returns the JSON rows key for the kind.
func (k ReportKind) RowsKey() string {
	switch k {
	case ReportDaily:
		return "daily"
	case ReportWeekly:
		return "weekly"
	case ReportMonthly:
		return "monthly"
	default:
		return "sessions"
	}
}

// PeriodKey returns the JSON period field for the kind.
func (k ReportKind) PeriodKey() string {
	switch k {
	case ReportDaily:
		return "date"
	case ReportWeekly:
		return "week"
	case ReportMonthly:
		return "month"
	default:
		return "sessionId"
	}
}

// FirstColumn returns the table's first column header for the kind.
func (k ReportKind) FirstColumn() string {
	switch k {
	case ReportDaily:
		return "Date"
	case ReportWeekly:
		return "Week"
	case ReportMonthly:
		return "Month"
	default:
		return "Session"
	}
}

// Label returns the report title suffix for the kind.
func (k ReportKind) Label() string {
	switch k {
	case ReportDaily:
		return "Daily"
	case ReportWeekly:
		return "Weekly"
	case ReportMonthly:
		return "Monthly"
	default:
		return "Session"
	}
}

// SummaryPeriod picks the row's display/sort period: date, month, then
// session id (the Goose report has no week key).
func SummaryPeriod(row *core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	default:
		return ""
	}
}

// SummarizeEntries groups loaded Goose entries into report rows. Session rows
// move the grouping key from the date slot into sessionId.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case ReportDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(date string) (string, *string) { return date, nil })
	case ReportMonthly:
		daily := SummarizeEntries(entries, ReportDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case ReportSession:
		rows := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.SessionID },
			func(sessionID string) (string, *string) { return sessionID, nil })
		for i := range rows {
			rows[i].SessionID = rows[i].Date
			rows[i].Date = nil
		}
		return rows
	case ReportWeekly:
		daily := SummarizeEntries(entries, ReportDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
	return nil
}

// ReportFromRows builds {<kind>: rows, totals} for JSON output.
func ReportFromRows(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = AgentSummaryJSON(&rows[i], kind, false)
	}
	return core.JObjV(kind.RowsKey(), core.JArrV(items...), "totals", core.TotalsJSON(rows))
}

// AgentSummaryJSON renders one row in the agent report JSON shape.
func AgentSummaryJSON(row *core.UsageSummary, kind ReportKind, includeSessionMetadata bool) core.J {
	pairs := []any{
		kind.PeriodKey(), core.JStrV(SummaryPeriod(row)),
		"inputTokens", core.JUintV(row.InputTokens),
		"outputTokens", core.JUintV(row.OutputTokens),
		"cacheCreationTokens", core.JUintV(row.CacheCreationTokens),
		"cacheReadTokens", core.JUintV(row.CacheReadTokens),
		"totalTokens", core.JUintV(row.TotalTokens()),
		"totalCost", core.JFloatV(row.TotalCost),
		"modelsUsed", modelsUsedJ(row.ModelsUsed),
		"modelBreakdowns", modelBreakdownsJ(row.ModelBreakdowns),
	}
	if row.Credits != nil {
		pairs = append(pairs, "credits", core.JFloatV(*row.Credits))
	}
	if row.MessageCount != nil {
		pairs = append(pairs, "messageCount", core.JUintV(*row.MessageCount))
	}
	if includeSessionMetadata {
		pairs = append(pairs,
			"lastActivity", core.JOptStrV(row.LastActivity),
			"firstActivity", core.JOptStrV(row.FirstActivity),
			"projectPath", core.JOptStrV(row.ProjectPath),
		)
	}
	return core.JObjV(pairs...)
}

func modelsUsedJ(models []string) core.J {
	items := make([]core.J, 0, len(models))
	for _, m := range models {
		items = append(items, core.JStrV(m))
	}
	return core.JArrV(items...)
}

func modelBreakdownsJ(breakdowns []core.ModelBreakdown) core.J {
	items := make([]core.J, 0, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items = append(items, core.JObjV(
			"modelName", core.JStrV(b.ModelName),
			"inputTokens", core.JUintV(b.InputTokens),
			"outputTokens", core.JUintV(b.OutputTokens),
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			"cost", core.JFloatV(b.Cost),
		))
	}
	return core.JArrV(items...)
}

// PrintTableForAgent renders the Goose report table (print_table_for_agent in
// the reference): a Credits column, no missing-pricing warnings, and the
// agent-specific empty-data message.
func PrintTableForAgent(agentName string, kind ReportKind, rows []core.UsageSummary, shared *core.SharedArgs) error {
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No %s usage data found.\n", agentName)
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := core.ShouldUseCompactLayout(shared, isTTY, terminalWidth, core.UsageCompactWidthThreshold)
	style := core.TerminalStyleFromShared(shared)
	terminal.PrintBoxTitle(fmt.Sprintf("%s Token Usage Report - %s", agentName, kind.Label()), style)

	firstColumn := kind.FirstColumn()
	var headers []string
	var aligns []terminal.Align
	if compact {
		headers = []string{firstColumn, "Models", "Input", "Output", "Credits", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
	} else {
		headers = []string{firstColumn, "Models", "Input", "Output", "Cache Create", "Cache Read", "Total Tokens", "Credits", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
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
		label := SummaryPeriod(row)
		models := core.FormatModelsMultiline(row.ModelsUsed)
		credits := 0.0
		if row.Credits != nil {
			credits = *row.Credits
		}
		var values []string
		if compact {
			values = []string{
				label,
				models,
				core.FormatNumber(row.InputTokens),
				core.FormatNumber(row.OutputTokens),
				fmt.Sprintf("%.2f", credits),
				core.FormatCurrency(row.TotalCost),
			}
		} else {
			values = []string{
				label,
				models,
				core.FormatNumber(row.InputTokens),
				core.FormatNumber(row.OutputTokens),
				core.FormatNumber(row.CacheCreationTokens),
				core.FormatNumber(row.CacheReadTokens),
				core.FormatNumber(row.TotalTokens()),
				fmt.Sprintf("%.2f", credits),
				core.FormatCurrency(row.TotalCost),
			}
		}
		if shared.NoCost {
			values = values[:len(values)-1]
		}
		table.Push(values)
	}

	// Totals mirror totals_json: the cost sum starts at negative zero (Rust's
	// empty-float-sum behavior) and credits surface only when positive.
	var input, output, cacheCreate, cacheRead, totalTokens uint64
	var credits float64
	totalCost := math.Copysign(0, -1)
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreationTokens
		cacheRead += rows[i].CacheReadTokens
		totalTokens += rows[i].TotalTokens()
		if rows[i].Credits != nil {
			credits += *rows[i].Credits
		}
		totalCost += rows[i].TotalCost
	}
	if credits <= 0 {
		credits = 0
	}
	table.Separator()
	var totalRow []string
	if compact {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			terminal.Colorize(style, core.FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, fmt.Sprintf("%.2f", credits), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
		}
	} else {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			terminal.Colorize(style, core.FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(cacheCreate), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(cacheRead), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(totalTokens), terminal.ColorYellow),
			terminal.Colorize(style, fmt.Sprintf("%.2f", credits), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
		}
	}
	if shared.NoCost {
		totalRow = totalRow[:len(totalRow)-1]
	}
	table.Push(totalRow)
	return table.Print(os.Stdout)
}
