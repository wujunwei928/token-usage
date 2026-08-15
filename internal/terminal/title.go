package terminal

import (
	"fmt"
	"strings"
)

// PrintBoxTitle writes the rounded-corner title box; suppressed at log level 0.
func PrintBoxTitle(title string, style TerminalStyle) {
	if style.LogLevel != nil && *style.LogLevel == 0 {
		return
	}
	for _, line := range BoxTitleLines(title, style) {
		fmt.Println(line)
	}
}

// BoxTitleLines renders the title box: blank line, ╭─╮ frame, centered title
// lines (blue), closing frame, blank line. Content width is at least 40.
func BoxTitleLines(title string, style TerminalStyle) []string {
	titleLines := SplitLines(title)
	contentWidth := 40
	for _, line := range titleLines {
		if w := VisibleWidth(line); w > contentWidth {
			contentWidth = w
		}
	}
	contentWidth += 2
	lines := make([]string, 0, len(titleLines)+5)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("╭%s╮", strings.Repeat("─", contentWidth+2)))
	lines = append(lines, fmt.Sprintf("│%s│", strings.Repeat(" ", contentWidth+2)))
	for _, line := range titleLines {
		padding := contentWidth - VisibleWidth(line)
		if padding < 0 {
			padding = 0
		}
		leftPadding := padding / 2
		rightPadding := padding - leftPadding
		lines = append(lines, fmt.Sprintf("│ %s%s%s │",
			strings.Repeat(" ", leftPadding),
			Colorize(style, line, ColorBlue),
			strings.Repeat(" ", rightPadding)))
	}
	lines = append(lines, fmt.Sprintf("│%s│", strings.Repeat(" ", contentWidth+2)))
	lines = append(lines, fmt.Sprintf("╰%s╯", strings.Repeat("─", contentWidth+2)))
	lines = append(lines, "")
	return lines
}
