package lexer

import (
	"ravenshell/token"
	"testing"
)

// firstHeredoc returns the single heredoc collected from src, failing if the
// input did not produce exactly one.
func firstHeredoc(t *testing.T, src string) *Heredoc {
	t.Helper()
	l := NewLexer(src)
	if len(l.heredocs) != 1 {
		t.Fatalf("collected %d heredocs from %q, want 1", len(l.heredocs), src)
	}
	for _, h := range l.heredocs {
		return h
	}
	return nil
}

func TestHeredocBodyCollection(t *testing.T) {
	cases := []struct {
		src       string
		wantBody  string
		wantDelim string
		quoted    bool
		note      string
	}{
		{"cat <<EOF\nhello\nworld\nEOF\n", "hello\nworld\n", "EOF", false, "plain"},
		{"cat <<EOF\nEOF\n", "", "EOF", false, "empty body"},
		{"cat <<'EOF'\n$VAR\nEOF\n", "$VAR\n", "EOF", true, "single-quoted delimiter"},
		{"cat <<\"EOF\"\n$VAR\nEOF\n", "$VAR\n", "EOF", true, "double-quoted delimiter"},
		{"cat <<\\EOF\n$VAR\nEOF\n", "$VAR\n", "EOF", true, "backslash-quoted delimiter"},
		{"cat <<-END\n\tone\n\t\ttwo\n\tEND\n", "one\ntwo\n", "END", false, "<<- strips leading tabs"},
		{"cat <<EOF\nlast line has no newline\nEOF", "last line has no newline\n", "EOF", false, "no trailing newline"},
		// The body is content, not source: shell metacharacters in it are inert.
		{"cat <<'EOF'\n# not a comment\na << b\n\"open quote\nEOF\n", "# not a comment\na << b\n\"open quote\n", "EOF", true, "metacharacters in body"},
		// A line that merely contains the delimiter does not end the heredoc.
		{"cat <<EOF\nxEOF\nEOF x\nEOF\n", "xEOF\nEOF x\n", "EOF", false, "delimiter must be the whole line"},
	}

	for _, c := range cases {
		h := firstHeredoc(t, c.src)
		if h.Body != c.wantBody {
			t.Errorf("%s: body = %q, want %q", c.note, h.Body, c.wantBody)
		}
		if h.Delimiter != c.wantDelim {
			t.Errorf("%s: delimiter = %q, want %q", c.note, h.Delimiter, c.wantDelim)
		}
		if h.Quoted != c.quoted {
			t.Errorf("%s: quoted = %v, want %v", c.note, h.Quoted, c.quoted)
		}
		if !h.Terminated {
			t.Errorf("%s: terminated = false, want true", c.note)
		}
	}
}

// The body must be invisible to the tokenizer: the tokens either side of a
// heredoc are exactly what they would be if the body were not there.
func TestHeredocBodyProducesNoTokens(t *testing.T) {
	l := NewLexer("cat <<EOF | wc\nnot ; a || command\nEOF\nprint done\n")

	var got []string
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		got = append(got, tok.Literal)
	}

	want := []string{"cat", "<<", "EOF", "|", "wc", "print", "done"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens = %v, want %v", got, want)
		}
	}
}

// Two heredocs on one line stack their bodies: the second starts where the
// first one's delimiter line ended.
func TestHeredocStacked(t *testing.T) {
	src := "cat <<A <<B\nfirst\nA\nsecond\nB\n"
	l := NewLexer(src)
	if len(l.heredocs) != 2 {
		t.Fatalf("collected %d heredocs, want 2", len(l.heredocs))
	}

	opA := len("cat ")
	opB := len("cat <<A ")
	if h := l.HeredocAt(opA); h == nil || h.Body != "first\n" || h.Delimiter != "A" {
		t.Errorf("heredoc at %d = %+v, want body %q delim A", opA, h, "first\n")
	}
	if h := l.HeredocAt(opB); h == nil || h.Body != "second\n" || h.Delimiter != "B" {
		t.Errorf("heredoc at %d = %+v, want body %q delim B", opB, h, "second\n")
	}
}

func TestHeredocUnterminated(t *testing.T) {
	// Still open: the delimiter line never arrives, so the REPL keeps reading.
	for _, src := range []string{"cat <<EOF", "cat <<EOF\n", "cat <<EOF\nbody so far\n"} {
		l := NewLexer(src)
		if !l.UnterminatedHeredoc() {
			t.Errorf("UnterminatedHeredoc(%q) = false, want true", src)
		}
	}
	// Closed, or never a heredoc at all.
	for _, src := range []string{"cat <<EOF\nbody\nEOF\n", "echo hi", "cat < file.txt"} {
		l := NewLexer(src)
		if l.UnterminatedHeredoc() {
			t.Errorf("UnterminatedHeredoc(%q) = true, want false", src)
		}
	}
}

// A `<<` that is not an operator must not start a heredoc, or the lines after
// it would silently vanish from the program.
func TestHeredocNotTriggered(t *testing.T) {
	cases := []struct{ src, note string }{
		{"echo '<<EOF'\nstill code\n", "inside a single-quoted string"},
		{"echo \"<<EOF\"\nstill code\n", "inside a double-quoted string"},
		{"echo hi # <<EOF\nstill code\n", "inside a comment"},
		{"print x <<< y\nstill code\n", "here-string, not a heredoc"},
	}
	for _, c := range cases {
		l := NewLexer(c.src)
		if len(l.heredocs) != 0 {
			t.Errorf("%s: collected %d heredocs, want 0", c.note, len(l.heredocs))
		}
	}
}

func TestHeredocOperatorTokens(t *testing.T) {
	// <<- must lex as one operator; otherwise the '-' starts a flag token and
	// swallows the delimiter.
	l := NewLexer("cat <<-EOF\n\tbody\n\tEOF\n")
	l.NextToken() // cat
	op := l.NextToken()
	if op.Type != token.OUT || op.Literal != "<<-" {
		t.Fatalf("operator token = %s %q, want OUT \"<<-\"", op.Type, op.Literal)
	}
	delim := l.NextToken()
	if delim.Type != token.IDENT || delim.Literal != "EOF" {
		t.Fatalf("delimiter token = %s %q, want IDENT \"EOF\"", delim.Type, delim.Literal)
	}
}
