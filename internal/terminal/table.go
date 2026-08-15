package terminal

import (
	"fmt"
	"io"
	"strings"
)

// Align is the horizontal alignment of a table column.
type Align int

// Column alignments.
const (
	AlignLeft Align = iota
	AlignRight
)

// SimpleTable is a byte-exact port of the reference ccusage-terminal renderer.
type SimpleTable struct {
	headers       []string
	aligns        []Align
	rows          [][]string // nil entry = separator
	style         TerminalStyle
	terminalWidth int
	compactDates  bool
}

// NewTable creates a table with headers, alignments, and a style.
func NewTable(headers []string, aligns []Align, style TerminalStyle) *SimpleTable {
	return &SimpleTable{
		headers:       append([]string(nil), headers...),
		aligns:        append([]Align(nil), aligns...),
		style:         style,
		terminalWidth: DefaultTerminalWidth,
	}
}

// WithTerminalWidth overrides the width used to fit columns.
func (t *SimpleTable) WithTerminalWidth(width int) *SimpleTable {
	t.terminalWidth = width
	return t
}

// WithDateCompaction enables the "YYYY\nMM-DD" date cell shortening.
func (t *SimpleTable) WithDateCompaction(compact bool) *SimpleTable {
	t.compactDates = compact
	return t
}

// Push appends a data row.
func (t *SimpleTable) Push(row []string) {
	t.rows = append(t.rows, row)
}

// Separator inserts a horizontal rule between rows.
func (t *SimpleTable) Separator() {
	t.rows = append(t.rows, nil)
}

// ColumnCount reports the number of header columns.
func (t *SimpleTable) ColumnCount() int {
	return len(t.headers)
}

// Print writes the rendered table lines to w.
func (t *SimpleTable) Print(w io.Writer) error {
	for _, line := range t.RenderLines() {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// RenderLines renders the complete table as display lines.
func (t *SimpleTable) RenderLines() []string {
	widths := t.columnWidths()
	var lines []string
	lines = append(lines, border('┌', '┬', '┐', widths))
	for _, headerRow := range expandMultilineRow(t.headers, len(t.headers), widths) {
		for i := range headerRow {
			headerRow[i] = Colorize(t.style, headerRow[i], ColorBlue)
		}
		lines = append(lines, tableLine(headerRow, t.aligns, widths))
	}
	lines = append(lines, border('├', '┼', '┤', widths))
	for rowIndex, row := range t.rows {
		if row != nil {
			compactRow := t.compactDateRow(row, widths)
			for _, physicalRow := range expandMultilineRow(compactRow, len(t.headers), widths) {
				lines = append(lines, tableLine(physicalRow, t.aligns, widths))
			}
		} else {
			lines = append(lines, border('├', '┼', '┤', widths))
		}
		if row != nil && rowIndex+1 < len(t.rows) && t.rows[rowIndex+1] != nil {
			lines = append(lines, border('├', '┼', '┤', widths))
		}
	}
	lines = append(lines, border('└', '┴', '┘', widths))
	return lines
}

func (t *SimpleTable) columnWidths() []int {
	contentWidths := make([]int, len(t.headers))
	for i, header := range t.headers {
		contentWidths[i] = VisibleWidthMaxLine(header)
	}
	for _, row := range t.rows {
		if row == nil {
			continue
		}
		for i, cell := range row {
			if i >= len(contentWidths) {
				break
			}
			if w := VisibleWidthMaxLine(cell); w > contentWidths[i] {
				contentWidths[i] = w
			}
		}
	}
	widths := make([]int, len(contentWidths))
	for i, w := range contentWidths {
		switch {
		case i < len(t.aligns) && t.aligns[i] == AlignRight:
			widths[i] = maxInt(w+3, 11)
		case i == 1:
			widths[i] = maxInt(w+2, 15)
		default:
			widths[i] = maxInt(w+2, 10)
		}
	}
	totalRequired := cliTableRequiredWidth(widths)
	firstColumnMin := 10
	if t.compactDates && totalRequired <= t.terminalWidth {
		firstColumnMin = 12
	}
	return fitWidthsToTerminal(widths, t.aligns, t.terminalWidth, firstColumnMin)
}

func (t *SimpleTable) compactDateRow(row []string, widths []int) []string {
	if !t.compactDates || len(widths) == 0 || widths[0] > 10 {
		return row
	}
	out := append([]string(nil), row...)
	if len(out) > 0 {
		if compact, ok := compactDateCell(out[0]); ok {
			out[0] = compact
		}
	}
	return out
}

func expandMultilineRow(row []string, columnCount int, widths []int) [][]string {
	cells := make([][]string, columnCount)
	for i := 0; i < columnCount; i++ {
		contentWidth := 0
		if i < len(widths) {
			contentWidth = maxInt(widths[i]-2, 0)
		}
		if i < len(row) {
			cells[i] = wrapCellLines(row[i], contentWidth)
		}
		if len(cells[i]) == 0 {
			cells[i] = []string{""}
		}
	}
	height := 1
	for _, lines := range cells {
		if len(lines) > height {
			height = len(lines)
		}
	}
	out := make([][]string, height)
	for lineIndex := 0; lineIndex < height; lineIndex++ {
		out[lineIndex] = make([]string, columnCount)
		for i := 0; i < columnCount; i++ {
			if lineIndex < len(cells[i]) {
				out[lineIndex][i] = cells[i][lineIndex]
			}
		}
	}
	return out
}

func fitWidthsToTerminal(widths []int, aligns []Align, terminalWidth, firstColumnMin int) []int {
	if cliTableRequiredWidth(widths) <= terminalWidth {
		return widths
	}
	minimums := make([]int, len(widths))
	for i := range widths {
		switch {
		case i < len(aligns) && aligns[i] == AlignRight:
			minimums[i] = 10
		case i == 0:
			minimums[i] = firstColumnMin
		case i == 1:
			minimums[i] = 12
		default:
			minimums[i] = 8
		}
	}
	availableWidth := terminalWidth - (len(widths) + 1)
	if availableWidth < 0 {
		availableWidth = 0
	}
	totalContentWidth := 0
	for _, w := range widths {
		totalContentWidth += w
	}
	if totalContentWidth > 0 {
		scale := float64(availableWidth) / float64(totalContentWidth)
		for i, w := range widths {
			widths[i] = maxInt(int(float64(w)*scale), minimums[i])
		}
	}
	for cliTableRequiredWidth(widths) > terminalWidth {
		best := -1
		for i, w := range widths {
			// >= matches Rust max_by_key, which keeps the LAST column on ties.
			if w > minimums[i] && (best < 0 || w >= widths[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		widths[best]--
	}
	return widths
}

func cliTableRequiredWidth(widths []int) int {
	sum := 0
	for _, w := range widths {
		sum += w
	}
	return sum + len(widths) + 1
}

func wrapCellLines(cell string, width int) []string {
	if width == 0 {
		return []string{""}
	}
	var lines []string
	for _, line := range SplitLines(cell) {
		if VisibleWidth(line) <= width {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, wrapCellLine(line, width)...)
	}
	return lines
}

func wrapCellLine(line string, width int) []string {
	if len(strings.Fields(line)) <= 1 {
		return []string{TruncateToWidth(line, width)}
	}
	var lines []string
	var current string
	for _, word := range strings.Fields(line) {
		var candidateWidth int
		if current == "" {
			candidateWidth = VisibleWidth(word)
		} else {
			candidateWidth = VisibleWidth(current) + 1 + VisibleWidth(word)
		}
		if candidateWidth <= width {
			if current != "" {
				current += " "
			}
			current += word
		} else {
			if current != "" {
				lines = append(lines, current)
			}
			if VisibleWidth(word) > width {
				current = TruncateToWidth(word, width)
			} else {
				current = word
			}
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func compactDateCell(value string) (string, bool) {
	b := []byte(value)
	if len(b) == 10 && b[4] == '-' && b[7] == '-' &&
		allDigits(b[0:4]) && allDigits(b[5:7]) && allDigits(b[8:10]) {
		return value[:4] + "\n" + value[5:], true
	}
	return "", false
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(b) > 0
}

func tableLine(cells []string, aligns []Align, widths []int) string {
	var line strings.Builder
	line.WriteRune('│')
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		align := AlignLeft
		if i < len(aligns) {
			align = aligns[i]
		}
		if i == 0 && strings.HasPrefix(cell, "(assuming ") {
			align = AlignRight
		}
		line.WriteByte(' ')
		line.WriteString(padCell(cell, maxInt(width-2, 0), align))
		line.WriteByte(' ')
		line.WriteRune('│')
	}
	return line.String()
}

func padCell(cell string, width int, align Align) string {
	visible := VisibleWidth(cell)
	if visible >= width {
		return cell
	}
	padding := strings.Repeat(" ", width-visible)
	if align == AlignLeft {
		return cell + padding
	}
	return padding + cell
}

func border(left, middle, right rune, widths []int) string {
	var line strings.Builder
	line.WriteRune(left)
	for i, width := range widths {
		line.WriteString(strings.Repeat("─", width))
		if i+1 == len(widths) {
			line.WriteRune(right)
		} else {
			line.WriteRune(middle)
		}
	}
	return line.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
