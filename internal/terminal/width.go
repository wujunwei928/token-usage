package terminal

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// skipANSIEscape returns the index just past the escape sequence at index.
// Only CSI sequences carry parameters; a bare ESC advances one byte and an
// "ESC [" run consumes bytes through its first alphabetic final byte.
func skipANSIEscape(b []byte, index int) int {
	index++
	if index < len(b) && b[index] == '[' {
		index++
		for index < len(b) && !isANSIFinal(b[index]) {
			index++
		}
		if index < len(b) {
			index++
		}
	}
	return index
}

func isANSIFinal(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// VisibleWidth reports the display columns of value, skipping ANSI escapes.
func VisibleWidth(value string) int {
	b := []byte(value)
	width := 0
	index := 0
	for index < len(b) {
		if b[index] == 0x1b {
			index = skipANSIEscape(b, index)
			continue
		}
		r, size := utf8.DecodeRune(b[index:])
		if r == utf8.RuneError && size <= 1 {
			break
		}
		width += charDisplayWidth(r)
		index += size
	}
	return width
}

// VisibleWidthMaxLine reports the widest line in a (possibly multi-line) value.
func VisibleWidthMaxLine(value string) int {
	max := 0
	for _, line := range SplitLines(value) {
		if w := VisibleWidth(line); w > max {
			max = w
		}
	}
	return max
}

// TruncateToWidth shortens value to at most width display columns, marking the
// cut with "…". ANSI escapes pass through without spending width and a reset is
// appended before the ellipsis when cutting inside a colored run.
func TruncateToWidth(value string, width int) string {
	if VisibleWidth(value) <= width {
		return value
	}
	if width == 0 {
		return ""
	}
	if width == 1 {
		return "…"
	}
	var out strings.Builder
	currentWidth := 0
	index := 0
	b := []byte(value)
	for index < len(b) {
		if b[index] == 0x1b {
			start := index
			index = skipANSIEscape(b, index)
			out.WriteString(value[start:index])
			continue
		}
		r, size := utf8.DecodeRune(b[index:])
		if r == utf8.RuneError && size <= 1 {
			break
		}
		cw := charDisplayWidth(r)
		// Stop one column early so the ellipsis stays inside width.
		if currentWidth+cw >= width {
			break
		}
		out.WriteRune(r)
		currentWidth += cw
		index += size
	}
	output := out.String()
	if strings.ContainsRune(value, 0x1b) && !strings.HasSuffix(output, "\x1b[0m") {
		output += "\x1b[0m"
	}
	return output + "…"
}

func charDisplayWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 0 {
		return 0
	}
	return w
}

// SplitLines mirrors Rust's str::lines: split on '\n', dropping a trailing
// empty segment, and stripping '\r' at line ends.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}
