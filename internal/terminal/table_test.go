package terminal

import (
	"strings"
	"testing"
)

func TestCompactDateCellSplitsISODates(t *testing.T) {
	if got, ok := compactDateCell("2026-05-18"); !ok || got != "2026\n05-18" {
		t.Errorf("compactDateCell(2026-05-18) = %q, %v; want %q, true", got, ok, "2026\n05-18")
	}
	if _, ok := compactDateCell("20260518"); ok {
		t.Error("compactDateCell(20260518) should not match")
	}
}

func TestWidthFittingKeepsTableWithinTerminalWhenPossible(t *testing.T) {
	widths := fitWidthsToTerminal(
		[]int{20, 40, 14, 14},
		[]Align{AlignLeft, AlignLeft, AlignRight, AlignRight},
		60,
		12,
	)
	if required := cliTableRequiredWidth(widths); required > 60 {
		t.Errorf("required width %d exceeds terminal 60 (widths %v)", required, widths)
	}
}

// Port of the Rust insta snapshot
// snapshots_full_table_with_multiline_cells_and_separators.snap.
func TestRenderLinesFullTableWithMultilineCellsAndSeparators(t *testing.T) {
	table := NewTable(
		[]string{"Date", "Models", "Input", "Output", "Cost (USD)"},
		[]Align{AlignLeft, AlignLeft, AlignRight, AlignRight, AlignRight},
		TerminalStyle{NoColor: true},
	).WithTerminalWidth(120)
	table.Push([]string{
		"2026-05-18",
		"- claude-sonnet-4\n- gpt-5.2-codex",
		"1,234",
		"56",
		"$0.42",
	})
	table.Push([]string{
		"(assuming cache warmup)",
		"",
		"0",
		"0",
		"$0.00",
	})
	table.Separator()
	table.Push([]string{
		"Total",
		"",
		"1,234",
		"56",
		"$0.42",
	})

	want := strings.Join([]string{
		"┌─────────────────────────┬───────────────────┬───────────┬───────────┬─────────────┐",
		"│ Date                    │ Models            │     Input │    Output │  Cost (USD) │",
		"├─────────────────────────┼───────────────────┼───────────┼───────────┼─────────────┤",
		"│ 2026-05-18              │ - claude-sonnet-4 │     1,234 │        56 │       $0.42 │",
		"│                         │ - gpt-5.2-codex   │           │           │             │",
		"├─────────────────────────┼───────────────────┼───────────┼───────────┼─────────────┤",
		"│ (assuming cache warmup) │                   │         0 │         0 │       $0.00 │",
		"├─────────────────────────┼───────────────────┼───────────┼───────────┼─────────────┤",
		"│ Total                   │                   │     1,234 │        56 │       $0.42 │",
		"└─────────────────────────┴───────────────────┴───────────┴───────────┴─────────────┘",
	}, "\n")
	if got := strings.Join(table.RenderLines(), "\n"); got != want {
		t.Errorf("RenderLines mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// Port of the Rust insta snapshot
// snapshots_narrow_table_with_wrapping_truncation_and_compact_dates.snap.
func TestRenderLinesNarrowTableWithWrappingTruncationAndCompactDates(t *testing.T) {
	table := NewTable(
		[]string{"Date", "Models", "Input", "Output", "Cost (USD)"},
		[]Align{AlignLeft, AlignLeft, AlignRight, AlignRight, AlignRight},
		TerminalStyle{NoColor: true},
	).WithTerminalWidth(56).WithDateCompaction(true)
	table.Push([]string{
		"2026-05-18",
		"- claude-sonnet-4-20250514\n- unusually-long-model-name-without-breaks",
		"123,456,789",
		"9,876,543",
		"$12345.67",
	})

	want := strings.Join([]string{
		"┌──────────┬────────────┬──────────┬──────────┬──────────┐",
		"│ Date     │ Models     │    Input │   Output │     Cost │",
		"│          │            │          │          │    (USD) │",
		"├──────────┼────────────┼──────────┼──────────┼──────────┤",
		"│ 2026     │ -          │ 123,456… │ 9,876,5… │ $12345.… │",
		"│ 05-18    │ claude-so… │          │          │          │",
		"│          │ -          │          │          │          │",
		"│          │ unusually… │          │          │          │",
		"└──────────┴────────────┴──────────┴──────────┴──────────┘",
	}, "\n")
	if got := strings.Join(table.RenderLines(), "\n"); got != want {
		t.Errorf("RenderLines mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestColumnWidthsUsesMaxLineNotSumForMultilineCells(t *testing.T) {
	table := NewTable(
		[]string{"Date", "Models", "Input", "Output", "Cost (USD)"},
		[]Align{AlignLeft, AlignLeft, AlignRight, AlignRight, AlignRight},
		TerminalStyle{NoColor: true},
	).WithTerminalWidth(200)
	cell := "- claude-sonnet-4-20250514 (self-serve)\n- claude-opus-4-5\n- gpt-5.2-codex\n- gemini-3.0-pro-wildly-long\n- claude-haiku-3-5-sonnet"
	table.Push([]string{
		"2026-05-18",
		cell,
		"1,234",
		"56",
		"$0.42",
	})

	widths := table.columnWidths()
	modelsWidth := widths[1]
	widestLine := VisibleWidthMaxLine(cell)
	sumOfLines := 0
	for _, line := range SplitLines(cell) {
		sumOfLines += VisibleWidth(line)
	}
	// If visible_width_sum were used, models_width would be ~180.
	if modelsWidth >= sumOfLines {
		t.Errorf("Models column width (%d) should be based on widest line (%d), not sum of all lines (%d)",
			modelsWidth, widestLine, sumOfLines)
	}
	if modelsWidth > widestLine+3 {
		t.Errorf("Models width (%d) should be close to widest line width (%d), not %d",
			modelsWidth, widestLine, sumOfLines)
	}
}
