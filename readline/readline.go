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
	"unicode"
	"unicode/utf8"

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

	// Multi-line redraw bookkeeping. The editable region (prompt + line +
	// autosuggestion) can wrap across several terminal rows; draw uses these to
	// clear every row the previous render occupied before reprinting.
	oldpos  int // cursor offset within line at the previous draw
	maxrows int // most physical rows the current line has used
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

	// Print the prompt. The editable region is freshly positioned at the start
	// of its row, so reset the multi-line bookkeeping and let draw render it.
	r.printPromptHead()
	r.resetDrawState()
	r.refresh(line, pos)

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
			nl, np, submit := r.reverseSearch(line)
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
			r.resetDrawState()
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
				if cp := commonPrefix(completions); len([]rune(cp)) > len([]rune(word)) {
					line, pos = r.applyCompletion(line, pos, cp, false)
					r.refresh(line, pos)
				} else {
					r.printCandidates(completions)
					r.printPromptHead()
					r.resetDrawState()
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
func (r *Readline) reverseSearch(original []rune) ([]rune, int, bool) {
	query := []rune{}
	matchIdx := -1
	match := ""

	find := func(from int) (int, string) {
		return historyMatch(r.history, string(query), from)
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
			return append([]rune(nil), original...), len(original), false

		case b == keyCtrlC || b == keyEscape:
			// Cancel: restore the line that was being edited before search.
			return append([]rune(nil), original...), len(original), false

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

// historyMatch returns the newest matching entry at or before from. Normal
// case-insensitive substring matches retain traditional reverse-search
// semantics. If none exist, a subsequence fallback makes abbreviations such as
// "gst" find "git status" without letting a weak fuzzy hit outrank a literal
// one.
func historyMatch(history []string, query string, from int) (int, string) {
	if from >= len(history) {
		from = len(history) - 1
	}
	if from < 0 {
		return -1, ""
	}

	q := []rune(strings.ToLower(query))
	for i := from; i >= 0; i-- {
		if strings.Contains(strings.ToLower(history[i]), string(q)) {
			return i, history[i]
		}
	}
	if len(q) == 0 {
		return -1, ""
	}
	for i := from; i >= 0; i-- {
		text := []rune(strings.ToLower(history[i]))
		matched := 0
		for _, ch := range text {
			if matched < len(q) && ch == q[matched] {
				matched++
			}
		}
		if matched == len(q) {
			return i, history[i]
		}
	}
	return -1, ""
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

// resetDrawState clears the multi-line redraw bookkeeping. Call it whenever the
// editable region is freshly positioned at the start of its row with nothing of
// the line yet on screen (start of a read, after Ctrl-L, after a completion
// listing), so the next draw does not try to clear rows that aren't there.
func (r *Readline) resetDrawState() {
	r.oldpos = 0
	r.maxrows = 1
}

// suggestionTail returns the part of the autosuggestion beyond what is already
// typed (empty when there is no suggestion).
func suggestionTail(line []rune, suggestion string) []rune {
	if suggestion != "" && len(suggestion) > len(line) {
		return []rune(suggestion)[len(line):]
	}
	return nil
}

// draw redraws the editable region (prompt + line + dim autosuggestion tail) in
// place, leaving the cursor at pos within line. It correctly handles the case
// where the rendered text wraps across several terminal rows.
//
// A naive "\r\033[K" redraw only clears the cursor's current physical row, so a
// line wide enough to wrap walks down the screen leaving a stale copy on every
// keystroke. This follows linenoise's multi-line refresh: walk up from the
// cursor's previous row clearing every row the last render used, reprint from
// the top, then move the cursor down and across to pos.
func (r *Readline) draw(line []rune, pos int, suggestion string) {
	cols, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		// No usable width (e.g. stdout redirected): fall back to a single-row
		// redraw, which is correct whenever the content fits one row.
		r.drawSingleLine(line, pos, suggestion)
		return
	}
	fmt.Print(r.renderEscapes(cols, line, pos, suggestion))
}

// renderEscapes builds the escape sequence that redraws the editable region on a
// terminal cols wide and advances the multi-line bookkeeping. It is split out of
// draw so the wrap math can be tested against a terminal model without a tty.
func (r *Readline) renderEscapes(cols int, line []rune, pos int, suggestion string) string {
	plen := displayWidth(r.prompt)
	tail := suggestionTail(line, suggestion)
	blen := len(line) + len(tail) // displayed cells after the prompt

	var b strings.Builder

	// Rows the new render needs, and the cursor's row within the previous render.
	rows := max((plen+blen+cols-1)/cols, 1)
	rpos := (plen + r.oldpos + cols) / cols
	oldRows := r.maxrows
	if rows > r.maxrows {
		r.maxrows = rows
	}

	// 1) Go to the last row the previous render used.
	if oldRows-rpos > 0 {
		fmt.Fprintf(&b, "\033[%dB", oldRows-rpos)
	}
	// 2) Clear each row from the bottom up, then 3) clear the top row.
	for j := 0; j < oldRows-1; j++ {
		b.WriteString("\r\033[0K\033[1A")
	}
	b.WriteString("\r\033[0K")

	// 4) Reprint prompt, line, and the dim suggestion tail.
	b.WriteString(r.prompt)
	b.WriteString(string(line))
	if len(tail) > 0 {
		b.WriteString(ansi.BrightBlk)
		b.WriteString(string(tail))
		b.WriteString(ansi.Reset)
	}

	// 5) If the cursor is at the very end of the buffer and lands exactly at
	// column 0 of a fresh row, emit a newline so the terminal scrolls and the
	// cursor has a row to occupy.
	if pos == blen && pos != 0 && (pos+plen)%cols == 0 {
		b.WriteString("\n\r")
		rows++
		if rows > r.maxrows {
			r.maxrows = rows
		}
	}

	// 6) Move the cursor up to its target row, then across to its column.
	rpos2 := (plen + pos + cols) / cols
	if rows-rpos2 > 0 {
		fmt.Fprintf(&b, "\033[%dA", rows-rpos2)
	}
	if col := (plen + pos) % cols; col > 0 {
		fmt.Fprintf(&b, "\r\033[%dC", col)
	} else {
		b.WriteString("\r")
	}

	r.oldpos = pos
	return b.String()
}

// drawSingleLine is the width-unaware fallback used when the terminal size is
// unavailable. It clears the current row and reprints; correct when the content
// fits on one row.
func (r *Readline) drawSingleLine(line []rune, pos int, suggestion string) {
	fmt.Print("\r\033[K")
	fmt.Print(r.prompt)
	fmt.Print(string(line))

	tail := suggestionTail(line, suggestion)
	if len(tail) > 0 {
		fmt.Print(ansi.BrightBlk + string(tail) + ansi.Reset)
	}

	if end := len(line) + len(tail); end > pos {
		fmt.Printf("\033[%dD", end-pos)
	}
}

// displayWidth returns the number of terminal cells s occupies, ignoring ANSI
// escape sequences (color codes in the prompt have zero width) and counting
// every other rune as one cell. The editable line is ASCII, so single-width
// counting is sufficient.
func displayWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // ESC: skip the escape sequence
			i++
			if i < len(s) && s[i] == '[' { // CSI: ESC [ ... final byte 0x40-0x7e
				i++
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
				if i < len(s) {
					i++ // consume the final byte
				}
			} else if i < len(s) {
				i++ // two-byte escape: skip the following byte
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		w++
	}
	return w
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

// currentWord returns the logical word being completed at the end of the line,
// with surrounding/incomplete shell quotes removed. Whitespace inside quotes
// remains part of the word.
func currentWord(beforeCursor string) string {
	var word []rune
	var quote rune
	active := false
	for _, ch := range beforeCursor {
		if quote != 0 {
			active = true
			if ch == quote {
				quote = 0
			} else {
				word = append(word, ch)
			}
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			active = true
			quote = ch
		case unicode.IsSpace(ch):
			word = word[:0]
			active = false
		default:
			active = true
			word = append(word, ch)
		}
	}
	if !active {
		return ""
	}
	return string(word)
}

// commonPrefix returns the longest prefix shared by every candidate's text.
func commonPrefix(cands []Candidate) string {
	if len(cands) == 0 {
		return ""
	}
	prefix := []rune(cands[0].Text)
	for _, c := range cands[1:] {
		text := []rune(c.Text)
		for len(prefix) > len(text) || string(text[:len(prefix)]) != string(prefix) {
			prefix = prefix[:len(prefix)-1]
			if len(prefix) == 0 {
				return ""
			}
		}
	}
	return string(prefix)
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
	if pos < 0 {
		pos = 0
	} else if pos > len(line) {
		pos = len(line)
	}

	// Find the start in rune coordinates, ignoring whitespace protected by an
	// opening quote (including an unfinished quote inserted for a shared path
	// prefix such as `'My D`).
	wordStart := completionWordStart(line[:pos])

	// A trailing "/" marks a directory (no trailing space, so further
	// completion is invited). Decide this on the raw text, before quoting.
	isDir := strings.HasSuffix(completion, "/")

	// Quote a completed candidate that contains spaces or shell metacharacters
	// so it round-trips through the lexer as one argument (e.g.
	// "The World's Strongest Rearguard/"). Only full-candidate insertions are
	// quoted; common-prefix extensions (addSpace == false) are partial words
	// and must stay bare so the next keystrokes can extend them.
	if addSpace {
		completion = quoteCompletion(completion)
	} else {
		completion = quotePartialCompletion(completion)
	}

	// Build new line
	newLine := string(line[:wordStart]) + completion
	rest := ""
	if pos < len(line) {
		rest = string(line[pos:])
	}

	// Add space after completion if it's not a directory
	if addSpace && !isDir {
		newLine += " "
	}
	newLine += rest

	return []rune(newLine), len([]rune(newLine)) - len([]rune(rest))
}

// completionWordStart returns the rune index of the active shell word.
func completionWordStart(line []rune) int {
	start := len(line)
	var quote rune
	active := false
	for i, ch := range line {
		if quote != 0 {
			active = true
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			if !active {
				start = i
				active = true
			}
			quote = ch
		case unicode.IsSpace(ch):
			active = false
			start = i + 1
		default:
			if !active {
				start = i
				active = true
			}
		}
	}
	if !active {
		return len(line)
	}
	return start
}

// quotePartialCompletion opens (but deliberately does not close) a quote for
// a shared completion prefix containing whitespace. Further typing and Tabs
// therefore remain in the same shell word; a final single match replaces it
// with a normally closed quote via quoteCompletion.
func quotePartialCompletion(text string) string {
	if !needsQuoting(text) {
		return text
	}
	prefix := ""
	if strings.HasPrefix(text, "~/") {
		prefix, text = "~/", text[2:]
	}
	if strings.ContainsRune(text, '\'') {
		return prefix + `"` + text
	}
	return prefix + "'" + text
}

// quoteCompletion wraps a completion candidate in quotes when it contains
// characters the lexer would otherwise split on (spaces, apostrophes, and other
// shell metacharacters), so the inserted word round-trips as a single argument.
// Single quotes are preferred (fully literal — no variable expansion); a
// candidate containing an apostrophe falls back to double quotes. A leading
// "~/" is kept outside the quotes so home-directory expansion still fires.
//
// ponytail: a name containing both ' and one of $ " ` is not representable by
// this lexer (double quotes interpolate, single quotes can't hold the '); it is
// returned double-quoted anyway. Such filenames don't occur in practice; the
// upgrade path is backslash-escape support in the lexer.
func quoteCompletion(text string) string {
	if !needsQuoting(text) {
		return text
	}
	prefix := ""
	if strings.HasPrefix(text, "~/") {
		prefix, text = "~/", text[2:]
	}
	if strings.ContainsRune(text, '\'') {
		return prefix + `"` + text + `"`
	}
	return prefix + "'" + text + "'"
}

// needsQuoting reports whether text contains any character outside the set the
// lexer scans as a bare word: letters, digits, and the path-safe punctuation
// . _ - / ~ . Anything else (space, ' " and shell metacharacters) would split
// the word into multiple tokens, so it must be quoted.
func needsQuoting(text string) bool {
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-', r == '/', r == '~':
		default:
			return true
		}
	}
	return false
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
