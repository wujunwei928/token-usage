package amp

import (
	"fmt"
	"os"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// ReportKind selects the report granularity (mirrors AgentReportKind).
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// FirstColumn renders the table's first column header for the kind.
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

// SummarizeEntries groups loaded entries into report rows exactly like
// rust/adapters/amp/src/report.rs.
func SummarizeEntries(entries []core.LoadedEntry, kind ReportKind) []core.UsageSummary {
	switch kind {
	case KindMonthly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketMonthly, core.Sunday)
	case KindSession:
		rows := core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.SessionID },
			func(key string) (string, *string) { return key, nil })
		for i := range rows {
			rows[i].SessionID = rows[i].Date
			rows[i].Date = nil
		}
		return rows
	case KindWeekly:
		daily := SummarizeEntries(entries, KindDaily)
		return core.SummarizeSummariesByBucket(daily, core.BucketWeekly, core.Sunday)
	default:
		return core.SummarizeByKey(entries,
			func(e *core.LoadedEntry) string { return e.Date },
			func(key string) (string, *string) { return key, nil })
	}
}

// SummaryPeriod returns the row's period label (date, week, month, or session).
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
	}
	return ""
}

// ReportJSON builds {<kind>: [...], totals: {...}} for JSON output; rows use
// the agent summary shape (period key varies by kind, credits included).
func ReportJSON(rows []core.UsageSummary, kind ReportKind) core.J {
	items := make([]core.J, len(rows))
	for i := range rows {
		items[i] = agentSummaryJSON(&rows[i], kind)
	}
	return core.JObjV(
		rowsKey(kind), core.JArrV(items...),
		"totals", core.TotalsJSON(rows),
	)
}

func rowsKey(kind ReportKind) string {
	switch kind {
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

// agentSummaryJSON mirrors ccusage-core agent_report.rs (no session metadata).
func agentSummaryJSON(row *core.UsageSummary, kind ReportKind) core.J {
	pairs := []any{
		periodKey(kind), core.JStrV(SummaryPeriod(row)),
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

func periodKey(kind ReportKind) string {
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

func modelsUsedJ(models []string) core.J {
	items := make([]core.J, len(models))
	for i, m := range models {
		items[i] = core.JStrV(m)
	}
	return core.JArrV(items...)
}

func modelBreakdownsJ(breakdowns []core.ModelBreakdown) core.J {
	items := make([]core.J, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items[i] = core.JObjV(
			"modelName", core.JStrV(b.ModelName),
			"inputTokens", core.JUintV(b.InputTokens),
			"outputTokens", core.JUintV(b.OutputTokens),
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			"cost", core.JFloatV(b.Cost),
		)
	}
	return core.JArrV(items...)
}

// PrintTable renders the Amp report table, which carries a Credits column
// (print_table_for_agent in the reference; no breakdown rows, no compact-mode
// notes, no missing-pricing warnings).
func PrintTable(kind ReportKind, rows []core.UsageSummary, shared *core.SharedArgs) error {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No Amp usage data found.")
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := core.ShouldUseCompactLayout(shared, isTTY, terminalWidth, core.UsageCompactWidthThreshold)
	style := core.TerminalStyleFromShared(shared)
	terminal.PrintBoxTitle(fmt.Sprintf("Amp Token Usage Report - %s", reportLabel(kind)), style)

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

	credits := func(row *core.UsageSummary) string {
		if row.Credits != nil {
			return fmt.Sprintf("%.2f", *row.Credits)
		}
		return "0.00"
	}
	for i := range rows {
		row := &rows[i]
		label := SummaryPeriod(row)
		models := core.FormatModelsMultiline(row.ModelsUsed)
		var values []string
		if compact {
			values = []string{
				label,
				models,
				core.FormatNumber(row.InputTokens),
				core.FormatNumber(row.OutputTokens),
				credits(row),
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
				credits(row),
				core.FormatCurrency(row.TotalCost),
			}
		}
		if shared.NoCost {
			values = values[:len(values)-1]
		}
		table.Push(values)
	}

	table.Separator()
	var input, output, cacheCreate, cacheRead, totalTokens uint64
	totalCost := 0.0
	totalCredits := 0.0
	hasCredits := false
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreationTokens
		cacheRead += rows[i].CacheReadTokens
		totalTokens += rows[i].TotalTokens()
		totalCost += rows[i].TotalCost
		if rows[i].Credits != nil {
			totalCredits += *rows[i].Credits
			hasCredits = true
		}
	}
	// The totals row reads credits back from totals_json, which only carries
	// the field when the sum is positive.
	if !hasCredits || totalCredits <= 0 {
		totalCredits = 0
	}
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
			terminal.Colorize(style, core.FormatNumber(totalTokens), terminal.ColorYellow),
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

func reportLabel(kind ReportKind) string {
	switch kind {
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
