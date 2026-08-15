package core

import "fmt"

// CLIError mirrors the reference CliError display format:
// Error: CliError("message")
type CLIError struct{ Message string }

func (e *CLIError) Error() string { return fmt.Sprintf("CliError(%q)", e.Message) }
