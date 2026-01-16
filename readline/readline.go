package readline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// Key constants
const (
	keyCtrlA     = 1
	keyCtrlC     = 3
	keyCtrlD     = 4
	keyCtrlE     = 5
	keyCtrlK     = 11
	keyCtrlL     = 12
	keyCtrlU     = 21
	keyCtrlW     = 23
	keyBackspace = 127
	keyTab       = 9
	keyEnter     = 13
	keyEscape    = 27
)

// Completer is a function that returns completions for a given line and cursor position
type Completer func(line string, pos int) []string

// Readline handles interactive line editing with history and completion
type Readline struct {
	prompt     string
	history    []string
	historyIdx int
	completer  Completer
	commands   []string // Built-in commands for completion
	cwd        func() string // Function to get current working directory
}

// New creates a new Readline instance
func New(prompt string) *Readline {
	return &Readline{
		prompt:     prompt,
		history:    make([]string, 0),
		historyIdx: -1,
		commands: []string{
			"ls", "rm", "mkdir", "rmdir", "cd", "cwd",
			"whoami", "mkfile", "output", "print", "show",
			"exit", "quit",
		},
	}
}

// SetCompleter sets a custom completion function
func (r *Readline) SetCompleter(c Completer) {
	r.completer = c
}

// SetCwdFunc sets a function to get current working directory for path completion
func (r *Readline) SetCwdFunc(f func() string) {
	r.cwd = f
}

// AddHistory adds a line to history
func (r *Readline) AddHistory(line string) {
	if line == "" {
		return
	}
	// Don't add duplicates at the end
	if len(r.history) > 0 && r.history[len(r.history)-1] == line {
		return
	}
	r.history = append(r.history, line)
}

// ReadLine reads a line with editing support
func (r *Readline) ReadLine() (string, error) {
	// Get terminal state
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Line buffer and cursor position
	line := []rune{}
	pos := 0
	r.historyIdx = len(r.history)
	savedLine := ""

	// Print prompt
	fmt.Print(r.prompt)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf[:1])
		if err != nil || n == 0 {
			fmt.Println()
			return "", err
		}

		switch buf[0] {
		case keyEnter:
			fmt.Print("\r\n")
			result := string(line)
			r.AddHistory(result)
			return result, nil

		case keyCtrlC:
			fmt.Print("^C\r\n")
			return "", nil

		case keyCtrlD:
			if len(line) == 0 {
				fmt.Print("\r\n")
				return "", fmt.Errorf("EOF")
			}
			// Delete char under cursor
			if pos < len(line) {
				line = append(line[:pos], line[pos+1:]...)
				r.redraw(line, pos)
			}

		case keyBackspace:
			if pos > 0 {
				line = append(line[:pos-1], line[pos:]...)
				pos--
				r.redraw(line, pos)
			}

		case keyCtrlA: // Home
			pos = 0
			r.redraw(line, pos)

		case keyCtrlE: // End
			pos = len(line)
			r.redraw(line, pos)

		case keyCtrlU: // Clear line before cursor
			line = line[pos:]
			pos = 0
			r.redraw(line, pos)

		case keyCtrlK: // Clear line after cursor
			line = line[:pos]
			r.redraw(line, pos)

		case keyCtrlW: // Delete word before cursor
			if pos > 0 {
				// Find start of word
				start := pos - 1
				for start > 0 && line[start-1] == ' ' {
					start--
				}
				for start > 0 && line[start-1] != ' ' {
					start--
				}
				line = append(line[:start], line[pos:]...)
				pos = start
				r.redraw(line, pos)
			}

		case keyCtrlL: // Clear screen
			fmt.Print("\033[2J\033[H")
			fmt.Print(r.prompt)
			r.redraw(line, pos)

		case keyTab:
			completions := r.complete(string(line), pos)
			if len(completions) == 1 {
				// Single completion - insert it
				newLine, newPos := r.applyCompletion(line, pos, completions[0])
				line = newLine
				pos = newPos
				r.redraw(line, pos)
			} else if len(completions) > 1 {
				// Multiple completions - show them
				fmt.Print("\r\n")
				for _, c := range completions {
					fmt.Printf("%s  ", c)
				}
				fmt.Print("\r\n")
				fmt.Print(r.prompt)
				r.redraw(line, pos)
			}

		case keyEscape:
			// Read escape sequence
			n, _ = os.Stdin.Read(buf[:2])
			if n == 2 && buf[0] == '[' {
				switch buf[1] {
				case 'A': // Up arrow - history previous
					if r.historyIdx > 0 {
						if r.historyIdx == len(r.history) {
							savedLine = string(line)
						}
						r.historyIdx--
						line = []rune(r.history[r.historyIdx])
						pos = len(line)
						r.redraw(line, pos)
					}

				case 'B': // Down arrow - history next
					if r.historyIdx < len(r.history) {
						r.historyIdx++
						if r.historyIdx == len(r.history) {
							line = []rune(savedLine)
						} else {
							line = []rune(r.history[r.historyIdx])
						}
						pos = len(line)
						r.redraw(line, pos)
					}

				case 'C': // Right arrow
					if pos < len(line) {
						pos++
						fmt.Print("\033[C")
					}

				case 'D': // Left arrow
					if pos > 0 {
						pos--
						fmt.Print("\033[D")
					}

				case 'H': // Home
					pos = 0
					r.redraw(line, pos)

				case 'F': // End
					pos = len(line)
					r.redraw(line, pos)

				case '3': // Delete key (followed by ~)
					os.Stdin.Read(buf[:1]) // consume ~
					if pos < len(line) {
						line = append(line[:pos], line[pos+1:]...)
						r.redraw(line, pos)
					}

				case '1': // Home (alternate)
					os.Stdin.Read(buf[:1]) // consume ~
					pos = 0
					r.redraw(line, pos)

				case '4': // End (alternate)
					os.Stdin.Read(buf[:1]) // consume ~
					pos = len(line)
					r.redraw(line, pos)
				}
			}

		default:
			// Regular character
			if buf[0] >= 32 && buf[0] < 127 {
				// Insert character at cursor position
				ch := rune(buf[0])
				line = append(line[:pos], append([]rune{ch}, line[pos:]...)...)
				pos++
				r.redraw(line, pos)
			}
		}
	}
}

// redraw clears the line and redraws it with cursor at pos
func (r *Readline) redraw(line []rune, pos int) {
	// Move to beginning of line
	fmt.Print("\r")
	// Clear entire line
	fmt.Print("\033[K")
	// Print prompt and line
	fmt.Print(r.prompt)
	fmt.Print(string(line))
	// Move cursor to correct position
	if pos < len(line) {
		// Move cursor back from end to pos
		fmt.Printf("\033[%dD", len(line)-pos)
	}
}

// complete returns completions for the current input
func (r *Readline) complete(line string, pos int) []string {
	// Use custom completer if set
	if r.completer != nil {
		return r.completer(line, pos)
	}

	// Default completion
	return r.defaultComplete(line, pos)
}

// defaultComplete provides basic command and path completion
func (r *Readline) defaultComplete(line string, pos int) []string {
	// Get the word being completed
	lineUpToPos := line[:pos]
	parts := strings.Fields(lineUpToPos)

	// Check if we're completing a partial word
	endsWithSpace := len(lineUpToPos) > 0 && lineUpToPos[len(lineUpToPos)-1] == ' '

	if len(parts) == 0 || (len(parts) == 1 && !endsWithSpace) {
		// Complete command name
		prefix := ""
		if len(parts) == 1 {
			prefix = parts[0]
		}
		return r.completeCommand(prefix)
	}

	// Complete file/directory path
	prefix := ""
	if !endsWithSpace {
		prefix = parts[len(parts)-1]
	}
	return r.completePath(prefix)
}

// completeCommand returns matching command names
func (r *Readline) completeCommand(prefix string) []string {
	var matches []string
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// completePath returns matching file/directory paths
func (r *Readline) completePath(prefix string) []string {
	cwd := "."
	if r.cwd != nil {
		cwd = r.cwd()
	}

	// Handle different path prefixes
	searchDir := cwd
	searchPrefix := prefix

	if prefix == "" {
		searchDir = cwd
		searchPrefix = ""
	} else if strings.HasPrefix(prefix, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if idx := strings.LastIndex(prefix, "/"); idx != -1 {
				searchDir = filepath.Join(home, prefix[2:idx+1])
				searchPrefix = prefix[idx+1:]
			} else {
				searchDir = home
				searchPrefix = prefix[2:]
			}
		}
	} else if strings.HasPrefix(prefix, "/") {
		if idx := strings.LastIndex(prefix, "/"); idx != -1 {
			searchDir = prefix[:idx+1]
			searchPrefix = prefix[idx+1:]
		}
	} else if strings.Contains(prefix, "/") {
		if idx := strings.LastIndex(prefix, "/"); idx != -1 {
			searchDir = filepath.Join(cwd, prefix[:idx+1])
			searchPrefix = prefix[idx+1:]
		}
	} else {
		searchDir = cwd
		searchPrefix = prefix
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	var matches []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, searchPrefix) {
			// Build the full completion
			completion := name
			if prefix != "" {
				if idx := strings.LastIndex(prefix, "/"); idx != -1 {
					completion = prefix[:idx+1] + name
				}
			}
			if entry.IsDir() {
				completion += "/"
			}
			matches = append(matches, completion)
		}
	}

	return matches
}

// applyCompletion applies a completion to the line
func (r *Readline) applyCompletion(line []rune, pos int, completion string) ([]rune, int) {
	lineStr := string(line[:pos])
	parts := strings.Fields(lineStr)

	// Find the start of the word being completed
	var wordStart int
	if len(parts) == 0 {
		wordStart = 0
	} else if len(lineStr) > 0 && lineStr[len(lineStr)-1] == ' ' {
		wordStart = pos
	} else {
		// Find the last word
		wordStart = strings.LastIndex(lineStr, " ")
		if wordStart == -1 {
			wordStart = 0
		} else {
			wordStart++
		}
	}

	// Build new line
	newLine := string(line[:wordStart]) + completion
	rest := ""
	if pos < len(line) {
		rest = string(line[pos:])
	}

	// Add space after completion if it's not a directory
	if !strings.HasSuffix(completion, "/") {
		newLine += " "
	}
	newLine += rest

	return []rune(newLine), len([]rune(newLine)) - len([]rune(rest))
}

// SetPrompt changes the prompt
func (r *Readline) SetPrompt(prompt string) {
	r.prompt = prompt
}

// ClearHistory clears the command history
func (r *Readline) ClearHistory() {
	r.history = make([]string, 0)
	r.historyIdx = -1
}
