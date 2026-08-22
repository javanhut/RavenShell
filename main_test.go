package main

import (
	"ravenshell/evaluator"
	"testing"
)

func TestRunSourceReturnsStatusAndExitRequest(t *testing.T) {
	e := evaluator.New()
	if got := runSource(e, "definitely_missing_raven_command", ""); got.status != 127 {
		t.Fatalf("missing command status = %d, want 127", got.status)
	}
	if got := runSource(evaluator.New(), "exit(7)", ""); got.status != 7 || !got.exitRequested {
		t.Fatalf("exit result = %+v, want status 7 and exit request", got)
	}
	if got := runSource(evaluator.New(), "x = [1][4]", ""); got.status != 1 {
		t.Fatalf("runtime error status = %d, want 1", got.status)
	}
}

func TestInputIncomplete(t *testing.T) {
	cases := []struct {
		src  string
		want bool
		note string
	}{
		// Complete inputs.
		{"echo hi", false, "plain command"},
		{`echo "hello world"`, false, "balanced double quote"},
		{"print \"\"\"hello\nworld\"\"\"", false, "balanced triple quote"},
		{"echo 'literal'", false, "balanced single quote"},
		{"for x in [1, 2] { echo x }", false, "balanced brackets/braces"},
		{"echo foo &", false, "trailing single & is background (complete)"},
		{"echo a && echo b", false, "&& with a right-hand side"},
		{"echo a | cat", false, "pipe with a right-hand side"},
		{"echo a #b", false, "trailing comment is not a trailing operator"},
		{"cat <<EOF\nbody\nEOF", false, "heredoc closed by its delimiter line"},
		{"cat <<-EOF\n\tbody\n\tEOF", false, "<<- heredoc closed by a tab-indented delimiter"},
		{"echo '<<EOF'", false, "<< inside a string does not open a heredoc"},

		// Incomplete: unbalanced brackets / strings.
		{`echo "unterminated`, true, "open double quote"},
		{"print \"\"\"unterminated", true, "open triple quote"},
		{"echo 'unterminated", true, "open single quote"},
		{"for x in [1, 2] {", true, "open brace"},
		{"echo (1 + 2", true, "open paren"},
		{"echo [1, 2", true, "open bracket"},

		// Incomplete: trailing continuation operators.
		{"echo a \\", true, "trailing line-continuation backslash"},
		{"echo hi |", true, "trailing pipe"},
		{"echo hi ||", true, "trailing logical or"},
		{"echo a &&", true, "trailing logical and"},
		{"echo a |   ", true, "trailing pipe with trailing whitespace"},
		{"echo a | # note", true, "trailing pipe before a comment"},

		// Incomplete: a heredoc still waiting for its delimiter line.
		{"cat <<EOF", true, "heredoc opened, no body yet"},
		{"cat <<EOF\nbody so far", true, "heredoc body still being typed"},
		{"cat <<EOF\nnotEOF", true, "delimiter must be the whole line"},
		{"cat <<'EOF'\nbody", true, "quoted-delimiter heredoc still open"},
	}

	for _, c := range cases {
		if got := inputIncomplete(c.src); got != c.want {
			t.Errorf("inputIncomplete(%q) = %v, want %v (%s)", c.src, got, c.want, c.note)
		}
	}
}
