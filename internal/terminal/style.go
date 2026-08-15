// Package terminal is the Go port of the ccusage-terminal crate: ANSI-aware
// display width, styling, box titles, and the SimpleTable renderer. Output must
// stay byte-identical with the reference implementation (ADR-0003).
package terminal

import (
	"fmt"
	"os"
)

// Color enumerates the palette used by table rendering.
type Color int

// Color codes match the reference implementation's style module.
const (
	ColorBlue Color = iota
	ColorGreen
	ColorGrey
	ColorRed
	ColorYellow
)

// TerminalStyle carries the resolved color/log preferences for rendering.
type TerminalStyle struct {
	Color    bool
	LogLevel *int
	NoColor  bool
}

// Colorize wraps value in the ANSI escape for color when colors are enabled.
// Color rules (priority order): --no-color and NO_COLOR always disable;
// --color, FORCE_COLOR, or stdout being a TTY enable.
func Colorize(style TerminalStyle, value string, color Color) string {
	if !useColor(style) {
		return value
	}
	var code int
	switch color {
	case ColorBlue:
		code = 34
	case ColorGreen:
		code = 32
	case ColorGrey:
		code = 90
	case ColorRed:
		code = 31
	case ColorYellow:
		code = 33
	}
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", code, value)
}

func useColor(style TerminalStyle) bool {
	if style.NoColor {
		return false
	}
	if _, present := os.LookupEnv("NO_COLOR"); present {
		return false
	}
	if style.Color {
		return true
	}
	if _, present := os.LookupEnv("FORCE_COLOR"); present {
		return true
	}
	return IsStdoutTerminal()
}
