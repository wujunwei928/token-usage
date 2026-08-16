package codebuff

import (
	"fmt"
	"os"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/terminal"
)

// ReportKind is the shared report vocabulary (ADR 0009); the local enum
// is an alias.
type ReportKind = core.ReportKind

// Report kinds.
const (
	KindDaily   = core.KindDaily
	KindWeekly  = core.KindWeekly
	KindMonthly = core.KindMonthly
	KindSession = core.KindSession
)

// reportLabel renders the kind's display label for the Codebuff table title.
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

// PrintTableForAgent renders the Codebuff table with its Credits column.
func PrintTableForAgent(agentName string, kind ReportKind, rows []core.UsageSummary, shared *core.SharedArgs) error {
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No %s usage data found.\n", agentName)
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := core.ShouldUseCompactLayout(shared, isTTY, terminalWidth, core.UsageCompactWidthThreshold)
	terminal.PrintBoxTitle(fmt.Sprintf("%s Token Usage Report - %s", agentName, reportLabel(kind)),
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
				common.SummaryPeriod(row),
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
				common.SummaryPeriod(row),
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
