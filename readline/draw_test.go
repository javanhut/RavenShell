package readline

import (
	"strings"
	"testing"
)

// miniTerm is a small terminal model used to verify the multi-line redraw. It
// implements only what draw emits — printable runes with xterm-style deferred
// wrap, CR, LF (column preserved, matching RavenTerminal), EL (\033[K / \033[0K),
// and cursor up/down/right — so a redraw bug shows up as duplicated rows.
type miniTerm struct {
	cols        int
	rows        [][]rune
	row, col    int
	wrapPending bool
}

func newMiniTerm(cols int) *miniTerm {
	return &miniTerm{cols: cols, rows: [][]rune{blankRow(cols)}}
}

func blankRow(cols int) []rune {
	r := make([]rune, cols)
	for i := range r {
		r[i] = ' '
	}
	return r
}

func (m *miniTerm) ensureRow(row int) {
	for len(m.rows) <= row {
		m.rows = append(m.rows, blankRow(m.cols))
	}
}

func (m *miniTerm) write(s string) {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\r':
			m.col = 0
			m.wrapPending = false
			i++
		case c == '\n':
			m.row++ // LF preserves the column, like RavenTerminal's cursorIndex
			m.wrapPending = false
			m.ensureRow(m.row)
			i++
		case c == 0x1b:
			i += m.escape(s[i:])
		case c >= 0x20 && c < 0x7f:
			if m.wrapPending {
				m.row++
				m.col = 0
				m.wrapPending = false
				m.ensureRow(m.row)
			}
			m.ensureRow(m.row)
			m.rows[m.row][m.col] = rune(c)
			if m.col == m.cols-1 {
				m.wrapPending = true // deferred wrap
			} else {
				m.col++
			}
			i++
		default:
			i++
		}
	}
}

// escape applies one ESC sequence starting at s[0]==ESC and returns its length.
func (m *miniTerm) escape(s string) int {
	if len(s) < 2 || s[1] != '[' {
		if len(s) >= 2 {
			return 2 // two-byte escape, ignored
		}
		return 1
	}
	j := 2
	for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
		j++
	}
	if j >= len(s) {
		return len(s)
	}
	final := s[j]
	n := 1
	if param := s[2:j]; param != "" {
		// Only single numeric params are emitted by draw.
		v := 0
		ok := true
		for _, ch := range param {
			if ch < '0' || ch > '9' {
				ok = false
				break
			}
			v = v*10 + int(ch-'0')
		}
		if ok {
			n = v
		}
	}
	m.wrapPending = false
	switch final {
	case 'A': // cursor up
		m.row -= n
		if m.row < 0 {
			m.row = 0
		}
	case 'B': // cursor down
		m.row += n
		m.ensureRow(m.row)
	case 'C': // cursor forward
		m.col += n
		if m.col > m.cols-1 {
			m.col = m.cols - 1
		}
	case 'K': // erase in line: 0 (or default) clears cursor..end of row
		if param := s[2:j]; param == "" || param == "0" {
			for c := m.col; c < m.cols; c++ {
				m.rows[m.row][c] = ' '
			}
		}
	}
	return j + 1
}

// promptRows counts how many rows begin with the prompt — i.e. how many copies
// of the editable line are on screen.
func (m *miniTerm) promptRows(prompt string) int {
	n := 0
	for _, row := range m.rows {
		if strings.HasPrefix(strings.TrimRight(string(row), " "), strings.TrimRight(prompt, " ")) &&
			strings.TrimRight(string(row), " ") != "" {
			n++
		}
	}
	return n
}

func (m *miniTerm) text() string {
	var b strings.Builder
	for _, row := range m.rows {
		b.WriteString(strings.TrimRight(string(row), " "))
	}
	return b.String()
}

// TestDrawDoesNotWalkDownOnWrap is the regression test for the reported bug: a
// line wide enough to wrap must redraw in place, not leave a stacked copy of the
// prompt on every keystroke.
func TestDrawDoesNotWalkDownOnWrap(t *testing.T) {
	const cols = 20
	prompt := "> "
	r := &Readline{historyIdx: -1, prompt: prompt}

	term := newMiniTerm(cols)

	// Initial render of the empty prompt, exactly as ReadLine does.
	r.resetDrawState()
	term.write(r.renderEscapes(cols, []rune{}, 0, ""))

	// Type a line long enough to wrap several times, one character at a time.
	typed := "abcdefghijklmnopqrstuvwxyz0123456789" // 36 chars + 2 prompt = 3 rows
	line := []rune{}
	for _, ch := range typed {
		line = append(line, ch)
		term.write(r.renderEscapes(cols, line, len(line), ""))
	}

	if got := term.promptRows(prompt); got != 1 {
		t.Fatalf("prompt appears on %d rows, want 1 (line walked down the screen)", got)
	}
	if want := prompt + typed; term.text() != want {
		t.Fatalf("rendered screen = %q, want %q", term.text(), want)
	}
}

// TestDrawShrinksBackToOneRow checks that deleting a wrapped line back down
// leaves a single clean prompt row with no leftover wrapped content.
func TestDrawShrinksBackToOneRow(t *testing.T) {
	const cols = 20
	prompt := "> "
	r := &Readline{historyIdx: -1, prompt: prompt}
	term := newMiniTerm(cols)

	r.resetDrawState()
	term.write(r.renderEscapes(cols, []rune{}, 0, ""))

	// Grow to a wrapped line...
	full := []rune("this is a fairly long command line that wraps")
	for i := 1; i <= len(full); i++ {
		term.write(r.renderEscapes(cols, full[:i], i, ""))
	}
	// ...then delete it all.
	for i := len(full) - 1; i >= 0; i-- {
		term.write(r.renderEscapes(cols, full[:i], i, ""))
	}

	if got := term.promptRows(prompt); got != 1 {
		t.Fatalf("after shrinking, prompt appears on %d rows, want 1", got)
	}
	// text() trims each row's trailing spaces, so the prompt's trailing space
	// drops; what matters is that nothing of the wrapped line remains.
	if want := strings.TrimRight(prompt, " "); term.text() != want {
		t.Fatalf("after shrinking, screen = %q, want just the prompt %q", term.text(), want)
	}
}
