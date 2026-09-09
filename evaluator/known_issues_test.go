package evaluator

import (
	"bytes"
	"strings"
	"testing"

	"ravenshell/lexer"
	"ravenshell/parser"
)

// runCapturing runs src and returns stdout, stderr, $? and any script error.
func runCapturing(t *testing.T, src string) (string, string, int, error) {
	t.Helper()
	e := New()
	e.cwd = t.TempDir()
	var out, errOut bytes.Buffer
	e.stdout, e.stderr = &out, &errOut
	p := parser.New(lexer.NewLexer(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors for %q: %v", src, errs)
	}
	err := e.Eval(program)
	return out.String(), errOut.String(), e.LastStatus(), err
}

// Indexing works in command-argument position, not only in expressions.
func TestIndexInArgumentPosition(t *testing.T) {
	cases := []struct{ src, want string }{
		{"n = [10, 20, 30]\nprint n[1]\n", "20\n"},
		{"n = [10, 20, 30]\nprint $n[2]\n", "30\n"},
		{"n = [10, 20, 30]\nprint (n[1])\n", "20\n"},
		{"n = [10, 20, 30]\nprint n[0] + n[1]\n", "30\n"},
		{"m = [[1, 2], [3, 4]]\nprint m[1][0]\n", "3\n"},
		{"n = [1, 2]\necho n[1]\n", "2\n"},
		{"n = [1, 2]\nprint first n[0] last\n", "first 1 last\n"},
	}
	for _, c := range cases {
		out, _, _, err := runCapturing(t, c.src)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.src, err)
			continue
		}
		if out != c.want {
			t.Errorf("%q: output %q, want %q", c.src, out, c.want)
		}
	}
	_, _, _, err := runCapturing(t, "print abc[1]\n")
	if err == nil || !strings.Contains(err.Error(), "cannot index 'abc'") {
		t.Errorf("indexing a non-array: error = %v, want cannot index 'abc'", err)
	}
}

// A parenthesised expression is one argument, evaluated as a value.
func TestParenthesisedArgument(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print (2 + 3)\n", "5\n"},
		{"print (2 + 3) * 2\n", "10\n"},
		{"x = 1\nprint (x)\n", "1\n"},
		{"print(2 + 3)\n", "5\n"},
		{`print "a" (1)` + "\n", "a 1\n"},
		{"fn f(x) { print \"got $x\" }\nf (7)\n", "got 7\n"},
		{"echo (1 + 2)\n", "3\n"},
		{"echo (upper(\"hi\"))\n", "HI\n"},
		{"n = [4, 5]\necho (n)\n", "4 5\n"},
	}
	for _, c := range cases {
		out, _, _, err := runCapturing(t, c.src)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.src, err)
			continue
		}
		if out != c.want {
			t.Errorf("%q: output %q, want %q", c.src, out, c.want)
		}
	}
}

// A builtin that fails at its job reports to the shell's stderr (so a
// redirection can silence it), sets $? to 1, and lets the program go on.
func TestBuiltinFailureIsAStatusNotAnError(t *testing.T) {
	cases := []struct {
		src, wantOut string
		wantStatus   int
		wantErrText  string // substring expected on stderr, "" for silence
	}{
		{"ls /nonexistent 2>/dev/null\nprint reached\n", "reached\n", 0, ""},
		{"ls /nonexistent\nprint $?\n", "1\n", 0, "ls: "},
		{"cd /nonexistent || print fallback\n", "fallback\n", 0, "cd: /nonexistent"},
		{"rm /nonexistent && print yes || print no\n", "no\n", 0, "rm: "},
		{"if true { ls /nonexistent }\nprint survived\n", "survived\n", 0, "ls: "},
		{"nosuchcmd-xyz 2>/dev/null\nprint $?\n", "127\n", 0, ""},
		{"ls /nonexistent\n", "", 1, "ls: "},
	}
	for _, c := range cases {
		out, errOut, status, err := runCapturing(t, c.src)
		if err != nil {
			t.Errorf("%q: script error %v, want none", c.src, err)
			continue
		}
		if out != c.wantOut {
			t.Errorf("%q: stdout %q, want %q", c.src, out, c.wantOut)
		}
		if status != c.wantStatus {
			t.Errorf("%q: $? = %d, want %d", c.src, status, c.wantStatus)
		}
		if c.wantErrText == "" && errOut != "" {
			t.Errorf("%q: stderr %q, want silence", c.src, errOut)
		}
		if c.wantErrText != "" && !strings.Contains(errOut, c.wantErrText) {
			t.Errorf("%q: stderr %q, want it to contain %q", c.src, errOut, c.wantErrText)
		}
	}
	// Language-level failures still abort with a position.
	_, _, _, err := runCapturing(t, "x = len()\nprint after\n")
	if err == nil || !strings.HasPrefix(err.Error(), "1:1:") {
		t.Errorf("len() misuse: error = %v, want a positioned script error", err)
	}
}
