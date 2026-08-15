package terminal

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// DefaultTerminalWidth is used when neither COLUMNS nor a TTY size is available.
const DefaultTerminalWidth = 120

// IsStdoutTerminal reports whether stdout is attached to a terminal.
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// TerminalWidth resolves the effective width: COLUMNS (parsed, > 0) wins, then
// the detected TTY size, then the default.
func TerminalWidth() int {
	if columns, ok := os.LookupEnv("COLUMNS"); ok {
		if width, err := strconv.Atoi(columns); err == nil && width > 0 {
			return width
		}
	}
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return DefaultTerminalWidth
}
