// Package ansi provides ANSI escape codes and small helpers for terminal
// styling used across the shell (prompt, ls output, errors, autosuggestions).
package ansi

const (
	Reset = "\033[0m"

	Bold      = "\033[1m"
	Dim       = "\033[2m"
	BrightBlk = "\033[90m" // grey, used for autosuggestions

	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
)

// Wrap returns s surrounded by the given style code and a reset.
func Wrap(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + Reset
}
