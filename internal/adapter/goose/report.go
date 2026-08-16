package goose

import (
	"fmt"
	"math"
	"os"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/terminal"
)

// ReportKind is the shared report vocabulary (ADR 0009); the local enum is
// an alias.
type ReportKind = core.ReportKind

// Report kinds.
const (
	KindDaily   = core.KindDaily
	KindWeekly  = core.KindWeekly
	KindMonthly = core.KindMonthly
	KindSession = core.KindSession
)

// reportLabel renders the kind's display label for the table title.
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

func PrintTableForAgent(agentName string, kind ReportKind, rows []core.UsageSummary, shared *core.SharedArgs) error {
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No %s usage data found.\n", agentName)
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := core.ShouldUseCompactLayout(shared, isTTY, terminalWidth, core.UsageCompactWidthThreshold)
	style := core.TerminalStyleFromShared(shared)
	terminal.PrintBoxTitle(fmt.Sprintf("%s Token Usage Report - %s", agentName, reportLabel(kind)), style)

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
		label := common.SummaryPeriod(row)
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
