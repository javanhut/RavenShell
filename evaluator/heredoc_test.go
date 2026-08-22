package evaluator

import "testing"

// `print` with no arguments echoes whatever is on stdin, so it doubles as a
// built-in `cat` for exercising heredoc redirection without an external process.
func TestHeredocFeedsStdin(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		{"print <<EOF\nhello\nworld\nEOF\n", "hello\nworld\n", "plain heredoc"},
		{"print <<EOF\nEOF\n", "", "empty body"},
		{"print <<-END\n\tone\n\t\ttwo\n\tEND\n", "one\ntwo\n", "<<- strips leading tabs"},
		{"print <<EOF\nbody\nEOF", "body\n", "no trailing newline after delimiter"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}

func TestHeredocExpansion(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		{"export NAME raven\nprint <<EOF\nhi $NAME\nEOF\n", "hi raven\n", "unquoted delimiter expands"},
		{"export NAME raven\nprint <<'EOF'\nhi $NAME\nEOF\n", "hi $NAME\n", "single-quoted delimiter is literal"},
		{"export NAME raven\nprint <<\"EOF\"\nhi $NAME\nEOF\n", "hi $NAME\n", "double-quoted delimiter is literal"},
		{"export NAME raven\nprint <<\\EOF\nhi $NAME\nEOF\n", "hi $NAME\n", "backslash-quoted delimiter is literal"},
		{"export NAME raven\nprint <<EOF\n${NAME}s\nEOF\n", "ravens\n", "braced expansion"},
		{"print <<\"EOF\"\n$(print x)\nEOF\n", "$(print x)\n", "double-quoted delimiter leaves $(...) literal"},
		{"print <<\\EOF\n$(print x)\nEOF\n", "$(print x)\n", "backslash-quoted delimiter leaves $(...) literal"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}

// An unquoted delimiter runs command substitution on the body as well, so
// $(...) behaves the same there as it does on a command line.
func TestHeredocCommandSubstitution(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		{"print <<EOF\ntoday is $(print monday)\nEOF\n", "today is monday\n", "unquoted delimiter expands command substitution"},
		{"print <<'EOF'\ntoday is $(print monday)\nEOF\n", "today is $(print monday)\n", "quoted delimiter leaves $(...) literal"},
		{"print <<EOF\n$(print $(print x))\nEOF\n", "x\n", "nested substitution"},
		{"export NAME raven\nprint <<EOF\n$NAME: $(print hi)\nEOF\n", "raven: hi\n", "variable and substitution in one body"},
		{"print <<EOF\nbroken $( here\nEOF\n", "broken $( here\n", "unbalanced $( stays literal"},
		{"print <<EOF\n$$(print x)\nEOF\n", "$(print x)\n", "$$ escapes the substitution"},
		{"print <<EOF\n$(print \"a)b\")\nEOF\n", "a)b\n", "paren inside quotes does not end the substitution"},
		{"print <<EOF\nempty [$(print \"\")]\nEOF\n", "empty []\n", "empty output substitutes nothing"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}

// Text that looks like an expansion but is not one must survive a heredoc
// intact. Blanking it instead would quietly corrupt generated Makefiles and
// shell scripts, which is worse than leaving it alone.
func TestHeredocLeavesUnsupportedExpansionsLiteral(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		// RavenShell has no arithmetic expansion; running $((...)) as a command
		// substitution would silently produce an empty string.
		{"print <<EOF\nsum=$((1 + 2))\nEOF\n", "sum=$((1 + 2))\n", "arithmetic expansion stays literal"},
		{"print <<EOF\ni=$((i + 1))\nEOF\n", "i=$((i + 1))\n", "arithmetic with a name stays literal"},
		// A backslash escapes the dollar, as it does in bash.
		{"print <<EOF\na \\$(print x) b\nEOF\n", "a $(print x) b\n", "backslash escapes a substitution"},
		{"export N raven\nprint <<EOF\na \\$N b\nEOF\n", "a $N b\n", "backslash escapes a variable"},
		{"export N raven\nprint <<EOF\n\\$N and $N\nEOF\n", "$N and raven\n", "escaped and live in one line"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}

// The same escape applies in a double-quoted string, which shares the scanner.
func TestStringBackslashEscapesDollar(t *testing.T) {
	out, err := evalScript(t, t.TempDir(), "export N raven\nprint \"a \\$N b $N c\"\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "a $N b raven c\n" {
		t.Fatalf("output = %q, want %q", out, "a $N b raven c\n")
	}
}

// A double-quoted string is not a heredoc: it keeps expanding $VAR only, so the
// two share a scanner but not this behaviour.
func TestStringDoesNotRunCommandSubstitution(t *testing.T) {
	out, err := evalScript(t, t.TempDir(), "print \"s $(print monday)\"\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "s $(print monday)\n" {
		t.Fatalf("output = %q, want %q", out, "s $(print monday)\n")
	}
}

// The command line continues normally after the heredoc operator, and the
// statements after the body run as usual.
func TestHeredocInContext(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		{"print <<EOF\nbody\nEOF\nprint \"after\"\n", "body\nafter\n", "statement after the body"},
		{"if 1 == 1 {\nprint <<EOF\ninside\nEOF\n}\n", "inside\n", "inside an if block"},
		{"for x in [1, 2] {\nprint <<EOF\niter $x\nEOF\n}\n", "iter 1\niter 2\n", "inside a for loop, re-expanded per iteration"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}

// An unterminated heredoc is not an error: like a shell, the text collected
// before end of input is what the command receives.
func TestHeredocUnterminatedUsesWhatWasCollected(t *testing.T) {
	out, err := evalScript(t, t.TempDir(), "print <<EOF\norphan body\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "orphan body\n" {
		t.Fatalf("output = %q, want %q", out, "orphan body\n")
	}
}

// Two heredocs on one command both get collected, but only the last one is
// still attached to stdin when the command runs, as in bash.
func TestStackedHeredocLastWins(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		src  string
		want string
		note string
	}{
		{"print <<A <<B\nfirst\nA\nsecond\nB\n", "second\n", "second heredoc wins"},
		{"print <<A <<B <<C\n1\nA\n2\nB\n3\nC\n", "3\n", "third heredoc wins"},
	}
	for _, c := range cases {
		out, err := evalScript(t, dir, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.note, out, c.want)
		}
	}
}
