package codebuff

import (
	"fmt"
	"os"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// ReportKind selects the Codebuff report granularity.
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// String returns the CLI name of the kind.
func (k ReportKind) String() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "session"
	}
}

// FirstColumn returns the table's first column header for the kind.
func (k ReportKind) FirstColumn() string {
	switch k {
	case KindDaily:
		return "Date"
	case KindWeekly:
		return "Week"
	case KindMonthly:
		return "Month"
	default:
		return "Session"
	}
}

// RowsKey returns the JSON key holding the rows for the kind.
func (k ReportKind) RowsKey() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "sessions"
	}
}

func (k ReportKind) periodKey() string {
	switch k {
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

func (k ReportKind) reportLabel() string {
	switch k {
	case KindDaily:
		return "Daily"
	case KindWeekly:
		return "Weekly"
	case KindMonthly:
		return "Monthly"
	default:
		return "Session"
	}
}

// SummarizeEntries groups entries into report rows for the kind.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindDaily:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(key string) (string, *string) { return key, nil })
	case KindMonthly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		rows := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.SessionID },
			func(key string) (string, *string) { return key, nil })
		for i := range rows {
			if rows[i].Date != nil {
				sessionID := *rows[i].Date
				rows[i].SessionID = &sessionID
				rows[i].Date = nil
			}
		}
		return rows
	default: // KindWeekly
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	}
}

// SummaryPeriod picks the sort key of a row.
func SummaryPeriod(row *core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Week != nil:
		return *row.Week
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	default:
		return ""
	}
}

// ReportFromRows builds the JSON report object for the kind.
func ReportFromRows(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = agentSummaryJSON(&rows[i], kind)
	}
	return core.JObjV(
		kind.RowsKey(), core.JArrV(items...),
		"totals", core.TotalsJSON(rows),
	)
}

func agentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		kind.periodKey(), core.JStrV(SummaryPeriod(row)),
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

// FilterLoadedEntriesByDate applies the since/until window to entry dates.
func FilterLoadedEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	filtered := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			filtered = append(filtered, entries[i])
		}
	}
	return filtered
}

// LoadSummaries loads entries and summarizes them for the kind, applying the
// shared date window first (the reference agent run order).
func LoadSummaries(shared *core.SharedArgs, kind ReportKind) ([]core.UsageSummary, error) {
	entries, err := LoadEntries(shared)
	if err != nil {
		return nil, err
	}
	entries = FilterLoadedEntriesByDate(entries, shared)
	return SummarizeEntries(entries, kind), nil
}

// PrintTableForAgent renders the Codebuff table with its Credits column.
func PrintTableForAgent(agentName string, kind ReportKind, rows []core.UsageSummary, shared *core.SharedArgs) error {
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No %s usage data found.\n", agentName)
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := core.ShouldUseCompactLayout(shared, isTTY, terminalWidth, core.UsageCompactWidthThreshold)
	terminal.PrintBoxTitle(fmt.Sprintf("%s Token Usage Report - %s", agentName, kind.reportLabel()),
		core.TerminalStyleFromShared(shared))
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
	table := terminal.NewTable(headers, aligns, core.TerminalStyleFromShared(shared)).
		WithTerminalWidth(terminalWidth).
		WithDateCompaction(true)
	for i := range rows {
		row := &rows[i]
		credits := 0.0
		if row.Credits != nil {
			credits = *row.Credits
		}
		if compact {
			values := []string{
				SummaryPeriod(row),
				core.FormatModelsMultiline(row.ModelsUsed),
				core.FormatNumber(row.InputTokens),
				core.FormatNumber(row.OutputTokens),
				fmt.Sprintf("%.2f", credits),
				core.FormatCurrency(row.TotalCost),
			}
			if shared.NoCost {
				values = values[:len(values)-1]
			}
			table.Push(values)
		} else {
			values := []string{
				SummaryPeriod(row),
				core.FormatModelsMultiline(row.ModelsUsed),
				core.FormatNumber(row.InputTokens),
				core.FormatNumber(row.OutputTokens),
				core.FormatNumber(row.CacheCreationTokens),
				core.FormatNumber(row.CacheReadTokens),
				core.FormatNumber(row.TotalTokens()),
				fmt.Sprintf("%.2f", credits),
				core.FormatCurrency(row.TotalCost),
			}
			if shared.NoCost {
				values = values[:len(values)-1]
			}
			table.Push(values)
		}
	}
	var input, output, cacheCreate, cacheRead, extra uint64
	totalCost := 0.0
	totalCredits := 0.0
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreationTokens
		cacheRead += rows[i].CacheReadTokens
		extra += rows[i].ExtraTotalTokens
		totalCost += rows[i].TotalCost
		if rows[i].Credits != nil {
			totalCredits += *rows[i].Credits
		}
	}
	style := core.TerminalStyleFromShared(shared)
	table.Separator()
	var totalRow []string
	if compact {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			terminal.Colorize(style, core.FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, fmt.Sprintf("%.2f", totalCredits), terminal.ColorYellow),
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
			terminal.Colorize(style, core.FormatNumber(input+output+cacheCreate+cacheRead+extra), terminal.ColorYellow),
			terminal.Colorize(style, fmt.Sprintf("%.2f", totalCredits), terminal.ColorYellow),
			terminal.Colorize(style, core.FormatCurrency(totalCost), terminal.ColorYellow),
		}
	}
	if shared.NoCost {
		totalRow = totalRow[:len(totalRow)-1]
	}
	table.Push(totalRow)
	return table.Print(os.Stdout)
}
