package main

import "testing"

func TestInputIncomplete(t *testing.T) {
	cases := []struct {
		src  string
		want bool
		note string
	}{
		// Complete inputs.
		{"echo hi", false, "plain command"},
		{`echo "hello world"`, false, "balanced double quote"},
		{"echo 'literal'", false, "balanced single quote"},
		{"for x in [1, 2] { echo x }", false, "balanced brackets/braces"},
		{"echo foo &", false, "trailing single & is background (complete)"},
		{"echo a && echo b", false, "&& with a right-hand side"},
		{"echo a | cat", false, "pipe with a right-hand side"},
		{"echo a #b", false, "trailing comment is not a trailing operator"},

		// Incomplete: unbalanced brackets / strings.
		{`echo "unterminated`, true, "open double quote"},
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
	}

	for _, c := range cases {
		if got := inputIncomplete(c.src); got != c.want {
			t.Errorf("inputIncomplete(%q) = %v, want %v (%s)", c.src, got, c.want, c.note)
		}
	}
}
