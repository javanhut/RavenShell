package readline

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"ravenshell/ansi"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ErrInterrupt is returned by ReadLine when the user presses Ctrl-C, so the
// caller can discard partially-entered input.
var ErrInterrupt = errors.New("interrupt")

// Key constants
const (
	keyCtrlA     = 1
	keyCtrlC     = 3
	keyCtrlD     = 4
	keyCtrlE     = 5
	keyCtrlK     = 11
	keyCtrlL     = 12
	keyCtrlR     = 18
	keyCtrlU     = 21
	keyCtrlW     = 23
	keyBackspace = 127
	keyTab       = 9
	keyEnter     = 13
	keyEscape    = 27
)

// Candidate is one completion choice. Text replaces the word being completed;
// Desc, when non-empty, is shown dimmed next to it in the completion listing;
// Style is an optional ANSI code applied to Text in the listing (e.g.
// file-type colors), never inserted into the line.
type Candidate struct {
	Text  string
	Desc  string
	Style string
}

// styled returns the candidate's text wrapped in its display style.
func (c Candidate) styled() string {
	return ansi.Wrap(c.Style, c.Text)
}

// Completer is a function that returns completions for a given line and cursor position
type Completer func(line string, pos int) []Candidate

// Readline handles interactive line editing with history and completion
type Readline struct {
	prompt          string // final (editable) prompt line, redrawn in place
	promptHead      string // lines of a multi-line prompt above the final one
	history         []string
	historyIdx      int
	historyFile     string // Path to the persistent history file (empty disables)
	completer       Completer
	commands        []string        // Built-in commands for completion
	cwd             func() string   // Function to get current working directory
	commandProvider func() []string // Dynamic command names (functions, PATH executables)
	cprDisabled     bool            // true once the terminal proves it ignores Cursor Position Reports
}

// New creates a new Readline instance
func New(prompt string) *Readline {
	r := &Readline{
		history:    make([]string, 0),
		historyIdx: -1,
		commands: []string{
			"ls", "rm", "mkdir", "rmdir", "cd", "cwd",
			"whoami", "mkfile", "output", "print", "show",
			"clear", "export", "env", "raven-add",
			"for", "while", "if", "else", "fn", "return",
			"break", "continue", "range", "append",
			"exit", "quit",
		},
	}
	r.SetPrompt(prompt)

	// Persist history across sessions in ~/.raven_history (fish-style).
	if home, err := os.UserHomeDir(); err == nil {
		r.historyFile = filepath.Join(home, ".raven_history")
		r.loadHistory()
	}

	return r
}

// loadHistory reads the persistent history file into memory.
func (r *Readline) loadHistory() {
	file, err := os.Open(r.historyFile)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// Collapse consecutive duplicates as they are loaded.
		if len(r.history) > 0 && r.history[len(r.history)-1] == line {
			continue
		}
		r.history = append(r.history, line)
	}
}

// appendHistoryFile appends a single entry to the persistent history file.
func (r *Readline) appendHistoryFile(line string) {
	if r.historyFile == "" {
		return
	}
	file, err := os.OpenFile(r.historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, line)
}

// SetCompleter sets a custom completion function
func (r *Readline) SetCompleter(c Completer) {
	r.completer = c
}

// SetCwdFunc sets a function to get current working directory for path completion
func (r *Readline) SetCwdFunc(f func() string) {
	r.cwd = f
}

// SetCommandProvider sets a function returning additional command names to
// offer during tab completion (e.g. user-defined functions and PATH executables).
func (r *Readline) SetCommandProvider(f func() []string) {
	r.commandProvider = f
}

// AddHistory adds a line to history (in memory and on disk).
func (r *Readline) AddHistory(line string) {
	if line == "" {
		return
	}
	// Don't add duplicates at the end
	if len(r.history) > 0 && r.history[len(r.history)-1] == line {
		return
	}
	r.history = append(r.history, line)
	r.appendHistoryFile(line)
}

// suggestionFor returns the most recent history entry that begins with line
// (and is longer than it), or "" if there is none. Used for the fish-style
// inline autosuggestion.
func (r *Readline) suggestionFor(line string) string {
	if line == "" {
		return ""
	}
	for i := len(r.history) - 1; i >= 0; i-- {
		entry := r.history[i]
		if len(entry) > len(line) && strings.HasPrefix(entry, line) {
			return entry
		}
	}
	return ""
}

// tryAcceptSuggestion, when the cursor is at the end of the line and an
// autosuggestion exists, returns the accepted line and true.
func (r *Readline) tryAcceptSuggestion(line []rune, pos int) ([]rune, int, bool) {
	if pos == len(line) {
		if sug := r.suggestionFor(string(line)); sug != "" {
			accepted := []rune(sug)
			return accepted, len(accepted), true
		}
	}
	return line, pos, false
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

	// If the previous command's output left the cursor mid-line (no trailing
	// newline), move to a fresh line so the prompt always starts at column 1.
	r.ensureFreshLine()

	// Print prompt
	r.printPromptHead()
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
			// Redraw without the autosuggestion before committing the line.
			r.draw(line, len(line), "")
			fmt.Print("\r\n")
			result := string(line)
			r.AddHistory(result)
			return result, nil

		case keyCtrlC:
			r.draw(line, len(line), "")
			fmt.Print("^C\r\n")
			return "", ErrInterrupt

		case keyCtrlD:
			if len(line) == 0 {
				fmt.Print("\r\n")
				return "", fmt.Errorf("EOF")
			}
			// Delete char under cursor
			if pos < len(line) {
				line = append(line[:pos], line[pos+1:]...)
				r.refresh(line, pos)
			}

		case keyBackspace:
			if pos > 0 {
				line = append(line[:pos-1], line[pos:]...)
				pos--
				r.refresh(line, pos)
			}

		case keyCtrlA: // Home
			pos = 0
			r.refresh(line, pos)

		case keyCtrlE: // End / accept autosuggestion
			if nl, np, ok := r.tryAcceptSuggestion(line, pos); ok {
				line, pos = nl, np
			} else {
				pos = len(line)
			}
			r.refresh(line, pos)

		case keyCtrlU: // Clear line before cursor
			line = line[pos:]
			pos = 0
			r.refresh(line, pos)

		case keyCtrlK: // Clear line after cursor
			line = line[:pos]
			r.refresh(line, pos)

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
				r.refresh(line, pos)
			}

		case keyCtrlR: // Reverse incremental history search
			nl, np, submit := r.reverseSearch()
			if submit {
				r.draw(nl, len(nl), "")
				fmt.Print("\r\n")
				result := string(nl)
				r.AddHistory(result)
				return result, nil
			}
			line, pos = nl, np
			r.refresh(line, pos)

		case keyCtrlL: // Clear screen
			fmt.Print("\033[2J\033[H")
			r.printPromptHead()
			fmt.Print(r.prompt)
			r.refresh(line, pos)

		case keyTab:
			completions := r.complete(string(line), pos)
			switch {
			case len(completions) == 1:
				// Single completion - insert it
				line, pos = r.applyCompletion(line, pos, completions[0].Text, true)
				r.refresh(line, pos)
			case len(completions) > 1:
				// First extend the word to the longest common prefix of all
				// candidates (fish-style); once nothing more can be inserted,
				// a further Tab shows the listing.
				word := currentWord(string(line[:pos]))
				if cp := commonPrefix(completions); len(cp) > len(word) {
					line, pos = r.applyCompletion(line, pos, cp, false)
					r.refresh(line, pos)
				} else {
					r.printCandidates(completions)
					r.printPromptHead()
					fmt.Print(r.prompt)
					r.refresh(line, pos)
				}
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
						r.refresh(line, pos)
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
						r.refresh(line, pos)
					}

				case 'C': // Right arrow / accept autosuggestion at end of line
					if pos < len(line) {
						pos++
						r.refresh(line, pos)
					} else if nl, np, ok := r.tryAcceptSuggestion(line, pos); ok {
						line, pos = nl, np
						r.refresh(line, pos)
					}

				case 'D': // Left arrow
					if pos > 0 {
						pos--
						r.refresh(line, pos)
					}

				case 'H': // Home
					pos = 0
					r.refresh(line, pos)

				case 'F': // End / accept autosuggestion
					if nl, np, ok := r.tryAcceptSuggestion(line, pos); ok {
						line, pos = nl, np
					} else {
						pos = len(line)
					}
					r.refresh(line, pos)

				case '3': // Delete key (followed by ~)
					os.Stdin.Read(buf[:1]) // consume ~
					if pos < len(line) {
						line = append(line[:pos], line[pos+1:]...)
						r.refresh(line, pos)
					}

				case '1': // Home (alternate)
					os.Stdin.Read(buf[:1]) // consume ~
					pos = 0
					r.refresh(line, pos)

				case '4': // End (alternate) / accept autosuggestion
					os.Stdin.Read(buf[:1]) // consume ~
					if nl, np, ok := r.tryAcceptSuggestion(line, pos); ok {
						line, pos = nl, np
					} else {
						pos = len(line)
					}
					r.refresh(line, pos)
				}
			}

		default:
			// Regular character
			if buf[0] >= 32 && buf[0] < 127 {
				// Insert character at cursor position
				ch := rune(buf[0])
				line = append(line[:pos], append([]rune{ch}, line[pos:]...)...)
				pos++
				r.refresh(line, pos)
			}
		}
	}
}

// ensureFreshLine guarantees the cursor is at the start of a line before the
// prompt is drawn, so a previous command whose output lacked a trailing newline
// does not leave the prompt appended to that output. It asks the terminal for
// the cursor's column via a Cursor Position Report; a terminal that does not
// answer is asked only once and then left alone. The terminal must already be
// in raw mode (ReadLine puts it there) for the reply to be readable.
func (r *Readline) ensureFreshLine() {
	if r.cprDisabled {
		return
	}
	col, ok := r.cursorColumn()
	if !ok {
		// The terminal didn't report a position; don't keep probing it.
		r.cprDisabled = true
		return
	}
	if col > 1 {
		fmt.Print("\r\n")
	}
}

// cursorColumn queries the terminal for the cursor's 1-based column using a
// Cursor Position Report (CPR): it writes ESC [ 6 n and reads the terminal's
// ESC [ row ; col R reply. It returns the column and true on success, or 0 and
// false if the reply is missing or malformed.
func (r *Readline) cursorColumn() (int, bool) {
	// With stdout redirected the query never reaches the terminal and the
	// reply never comes, so the read below would block eating keystrokes.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return 0, false
	}
	if _, err := os.Stdout.WriteString("\x1b[6n"); err != nil {
		return 0, false
	}

	// Read the reply byte by byte up to the terminating 'R'. The cap guards
	// against a terminal that never sends 'R'.
	var resp []byte
	b := make([]byte, 1)
	for range 16 {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			return 0, false
		}
		resp = append(resp, b[0])
		if b[0] == 'R' {
			break
		}
	}
	return parseCursorColumn(string(resp))
}

// parseCursorColumn extracts the column from a Cursor Position Report of the
// form "ESC [ row ; col R" (leading bytes before '[' are ignored). It returns
// the 1-based column and true, or 0 and false if resp is not a valid report.
func parseCursorColumn(resp string) (int, bool) {
	i := strings.IndexByte(resp, '[')
	j := strings.IndexByte(resp, 'R')
	if i < 0 || j < 0 || j <= i {
		return 0, false
	}
	parts := strings.Split(resp[i+1:j], ";")
	if len(parts) != 2 {
		return 0, false
	}
	col, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || col < 1 {
		return 0, false
	}
	return col, true
}

// reverseSearch runs an incremental reverse history search (Ctrl-R). It returns
// the resulting line, cursor position, and whether the user submitted it
// (pressed Enter) versus accepting it for further editing.
func (r *Readline) reverseSearch() ([]rune, int, bool) {
	query := []rune{}
	matchIdx := -1
	match := ""

	find := func(from int) (int, string) {
		q := string(query)
		if from >= len(r.history) {
			from = len(r.history) - 1
		}
		for i := from; i >= 0; i-- {
			if strings.Contains(r.history[i], q) {
				return i, r.history[i]
			}
		}
		return -1, ""
	}

	render := func() {
		fmt.Print("\r\033[K")
		fmt.Printf("(reverse-i-search)`%s': %s", string(query), match)
	}

	render()

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return []rune(match), len([]rune(match)), false
		}
		switch b := buf[0]; {
		case b == keyEnter:
			if match != "" {
				return []rune(match), len([]rune(match)), true
			}
			return []rune{}, 0, false

		case b == keyCtrlC || b == keyEscape:
			// Cancel: drop back to an empty editing line.
			return []rune{}, 0, false

		case b == keyCtrlR:
			// Move to the next older match.
			from := len(r.history) - 1
			if matchIdx >= 0 {
				from = matchIdx - 1
			}
			if i, m := find(from); i >= 0 {
				matchIdx, match = i, m
			}
			render()

		case b == keyBackspace:
			if len(query) > 0 {
				query = query[:len(query)-1]
			}
			matchIdx, match = find(len(r.history) - 1)
			render()

		case b >= 32 && b < 127:
			query = append(query, rune(b))
			matchIdx, match = find(len(r.history) - 1)
			render()

		default:
			// Any other key accepts the current match for editing.
			return []rune(match), len([]rune(match)), false
		}
	}
}

// refresh redraws the line, showing a dim autosuggestion from history when the
// cursor is at the end of a non-empty line.
func (r *Readline) refresh(line []rune, pos int) {
	suggestion := ""
	if pos == len(line) {
		suggestion = r.suggestionFor(string(line))
	}
	r.draw(line, pos, suggestion)
}

// draw clears the current line and redraws the prompt, the line, and the dim
// tail of an optional autosuggestion, leaving the cursor at pos.
func (r *Readline) draw(line []rune, pos int, suggestion string) {
	// Move to beginning of line and clear it.
	fmt.Print("\r")
	fmt.Print("\033[K")
	fmt.Print(r.prompt)
	fmt.Print(string(line))

	// Draw the part of the suggestion beyond what's already typed, dimmed.
	var tail []rune
	if suggestion != "" && len(suggestion) > len(line) {
		tail = []rune(suggestion)[len(line):]
		fmt.Print(ansi.BrightBlk + string(tail) + ansi.Reset)
	}

	// Move the cursor back from the end of everything drawn to pos.
	end := len(line) + len(tail)
	if end > pos {
		fmt.Printf("\033[%dD", end-pos)
	}
}

// complete returns completions for the current input
func (r *Readline) complete(line string, pos int) []Candidate {
	// Use custom completer if set
	if r.completer != nil {
		return r.completer(line, pos)
	}

	// Default completion
	texts := r.defaultComplete(line, pos)
	out := make([]Candidate, len(texts))
	for i, t := range texts {
		out[i] = Candidate{Text: t}
	}
	return out
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
	// Merge built-in command names with any dynamically provided names
	// (user-defined functions and executables on the search/system PATH).
	seen := make(map[string]bool)
	var matches []string
	add := func(name string) {
		if strings.HasPrefix(name, prefix) && !seen[name] {
			seen[name] = true
			matches = append(matches, name)
		}
	}

	for _, cmd := range r.commands {
		add(cmd)
	}
	if r.commandProvider != nil {
		for _, cmd := range r.commandProvider() {
			add(cmd)
		}
	}

	sort.Strings(matches)
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

// currentWord returns the whitespace-delimited word being completed at the
// end of the given line prefix ("" when the prefix ends in a space).
func currentWord(beforeCursor string) string {
	if beforeCursor == "" || beforeCursor[len(beforeCursor)-1] == ' ' {
		return ""
	}
	fields := strings.Fields(beforeCursor)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// commonPrefix returns the longest prefix shared by every candidate's text.
func commonPrefix(cands []Candidate) string {
	if len(cands) == 0 {
		return ""
	}
	prefix := cands[0].Text
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c.Text, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// printCandidates renders the completion listing below the current line:
// fish-pager style with dimmed descriptions when the set is small enough,
// plain columns otherwise. The terminal is in raw mode, so lines end in \r\n.
func (r *Readline) printCandidates(cands []Candidate) {
	fmt.Print("\r\n")

	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}

	const maxShow = 100
	shown := cands
	if len(cands) > maxShow {
		shown = cands[:maxShow]
	}

	maxText := 0
	hasDesc := false
	for _, c := range shown {
		if len(c.Text) > maxText {
			maxText = len(c.Text)
		}
		if c.Desc != "" {
			hasDesc = true
		}
	}

	// Padding is computed from the plain text length so ANSI style codes do
	// not break column alignment.
	if hasDesc && len(shown) <= 30 {
		// One candidate per row with its description.
		for _, c := range shown {
			row := c.styled() + strings.Repeat(" ", maxText-len(c.Text))
			if c.Desc != "" {
				desc := c.Desc
				if avail := width - maxText - 3; avail > 0 && len(desc) > avail {
					if avail > 1 {
						desc = desc[:avail-1] + "…"
					} else {
						desc = ""
					}
				}
				if desc != "" {
					row += "  " + ansi.BrightBlk + desc + ansi.Reset
				}
			}
			fmt.Print(row + "\r\n")
		}
	} else {
		// Plain columns of candidate text, row-major.
		colWidth := maxText + 2
		cols := max(width/colWidth, 1)
		rows := (len(shown) + cols - 1) / cols
		for row := range rows {
			for col := range cols {
				i := row*cols + col
				if i >= len(shown) {
					break
				}
				fmt.Print(shown[i].styled())
				if col != cols-1 && i != len(shown)-1 {
					fmt.Print(strings.Repeat(" ", colWidth-len(shown[i].Text)))
				}
			}
			fmt.Print("\r\n")
		}
	}

	if extra := len(cands) - len(shown); extra > 0 {
		fmt.Printf("%s…and %d more%s\r\n", ansi.BrightBlk, extra, ansi.Reset)
	}
}

// applyCompletion replaces the word being completed with completion. addSpace
// appends a space after a fully-applied completion (skipped for directories,
// which invite further completion, and for common-prefix insertions).
func (r *Readline) applyCompletion(line []rune, pos int, completion string, addSpace bool) ([]rune, int) {
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
	if addSpace && !strings.HasSuffix(completion, "/") {
		newLine += " "
	}
	newLine += rest

	return []rune(newLine), len([]rune(newLine)) - len([]rune(rest))
}

// SetPrompt changes the prompt. A multi-line prompt is split: every line but
// the last (the "head") is printed once per read, and only the final line is
// redrawn in place while editing.
func (r *Readline) SetPrompt(prompt string) {
	if i := strings.LastIndexByte(prompt, '\n'); i >= 0 {
		r.promptHead = prompt[:i+1]
		r.prompt = prompt[i+1:]
	} else {
		r.promptHead = ""
		r.prompt = prompt
	}
}

// printPromptHead prints the informational lines of a multi-line prompt. The
// terminal is in raw mode while reading, so newlines need explicit carriage
// returns.
func (r *Readline) printPromptHead() {
	if r.promptHead == "" {
		return
	}
	fmt.Print("\r" + strings.ReplaceAll(r.promptHead, "\n", "\r\n"))
}

// ClearHistory clears the command history
func (r *Readline) ClearHistory() {
	r.history = make([]string, 0)
	r.historyIdx = -1
}
