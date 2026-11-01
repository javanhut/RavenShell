package evaluator

import (
	"errors"
	"fmt"
	"os"

	"ravenshell/readline"
)

const ravenHistoryUsage = `raven-history [clear|path]

Manage the persistent command history in ~/.raven_history. The history feeds
Up/Down recall, Ctrl+R search, and the grey inline autosuggestions.

  clear   Forget every history entry, in this session and on disk, so the
          inline suggestions start empty. Other shells that are already open
          keep their own in-memory history until they exit.
  path    Print the history file.

With no argument, print this help.`

// SetHistoryClearer registers the interactive line editor's history reset,
// which raven-history clear calls so the running session forgets its history
// too. Without one (scripts, raven -c), only the history file is removed.
func (e *Evaluator) SetHistoryClearer(f func() (int, error)) {
	e.historyClearer = f
}

// execRavenHistory implements the raven-history built-in.
func (e *Evaluator) execRavenHistory(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("raven-history: expected one action, got %d arguments", len(args))
	}
	action := ""
	if len(args) == 1 {
		action = args[0]
	}

	switch action {
	case "", "--help", "-h":
		fmt.Fprintln(e.stdout, ravenHistoryUsage)
		return "", nil

	case "path":
		path := readline.HistoryPath()
		if path == "" {
			return "", fmt.Errorf("raven-history: cannot locate the home directory")
		}
		fmt.Fprintln(e.stdout, path)
		return "", nil

	case "clear":
		if e.historyClearer != nil {
			n, err := e.historyClearer()
			if err != nil {
				return "", fmt.Errorf("raven-history: %v", err)
			}
			noun := "entries"
			if n == 1 {
				noun = "entry"
			}
			fmt.Fprintf(e.stdout, "Cleared %d history %s.\n", n, noun)
			return "", nil
		}
		path := readline.HistoryPath()
		if path == "" {
			return "", fmt.Errorf("raven-history: cannot locate the home directory")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("raven-history: %v", err)
		}
		fmt.Fprintf(e.stdout, "Cleared history file %s\n", path)
		return "", nil

	default:
		return "", fmt.Errorf("raven-history: unknown argument %q (try raven-history --help)", action)
	}
}
