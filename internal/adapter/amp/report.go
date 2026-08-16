package amp

import (
	"fmt"
	"os"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/terminal"
)

// ReportKind is the shared report vocabulary (ADR 0009); the local
// enum is an alias. FirstColumn and friends come from core.
type ReportKind = core.ReportKind

// Report kinds.
const (
	KindDaily   = core.KindDaily
	KindWeekly  = core.KindWeekly
	KindMonthly = core.KindMonthly
	KindSession = core.KindSession
)

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
		label := common.SummaryPeriod(row)
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
