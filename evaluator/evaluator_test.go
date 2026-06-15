package evaluator

import (
	"bytes"
	"os"
	"path/filepath"
	"ravenshell/lexer"
	"ravenshell/parser"
	"strings"
	"testing"
)

// run parses and evaluates src, capturing anything written to stdout.
func run(t *testing.T, src string) (*Evaluator, string) {
	t.Helper()
	e := New()
	var buf bytes.Buffer
	e.stdout = &buf

	l := lexer.NewLexer(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}
	if err := e.Eval(program); err != nil {
		t.Fatalf("eval error: %v", err)
	}
	return e, buf.String()
}

func TestArithmeticPrecedence(t *testing.T) {
	e, _ := run(t, "result = 10 + 5 * 2")
	if got := e.vars["result"]; got != int64(20) {
		t.Errorf("result = %v (%T), want 20", got, got)
	}
}

func TestModulo(t *testing.T) {
	e, _ := run(t, "r = 10 % 3")
	if got := e.vars["r"]; got != int64(1) {
		t.Errorf("r = %v, want 1", got)
	}
}

func TestStringConcatenation(t *testing.T) {
	e, _ := run(t, `s = "foo" + "bar"`)
	if got := e.vars["s"]; got != "foobar" {
		t.Errorf("s = %v, want foobar", got)
	}
}

func TestArrayIndex(t *testing.T) {
	e, _ := run(t, "nums = [10, 20, 30]\nsecond = nums[1]")
	if got := e.vars["second"]; got != int64(20) {
		t.Errorf("second = %v, want 20", got)
	}
}

func TestRangeAndAppend(t *testing.T) {
	e, _ := run(t, "x = []int\nfor i in range(3) { x = append(x, i) }")
	arr, ok := e.vars["x"].([]Value)
	if !ok {
		t.Fatalf("x is not an array: %T", e.vars["x"])
	}
	if len(arr) != 3 || arr[0] != int64(0) || arr[2] != int64(2) {
		t.Errorf("x = %v, want [0 1 2]", arr)
	}
}

func TestIfElse(t *testing.T) {
	_, out := run(t, `if 5 > 3 { print "yes" } else { print "no" }`)
	if strings.TrimSpace(out) != "yes" {
		t.Errorf("output = %q, want yes", out)
	}
}

func TestForLoopOutput(t *testing.T) {
	_, out := run(t, "for i in range(3) { print i }")
	if strings.TrimSpace(out) != "0\n1\n2" {
		t.Errorf("output = %q, want 0\\n1\\n2", out)
	}
}

func TestPipeIntoPrint(t *testing.T) {
	// print's output is captured and re-emitted by the second print.
	_, out := run(t, `print "hello" | print`)
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
}

func TestWhileLoop(t *testing.T) {
	e, _ := run(t, "i = 0\nsum = 0\nwhile i < 5 { sum = sum + i\ni = i + 1 }")
	if got := e.vars["sum"]; got != int64(10) {
		t.Errorf("sum = %v, want 10", got)
	}
}

func TestBreak(t *testing.T) {
	_, out := run(t, "for i in range(10) { if i == 3 { break }\nprint i }")
	if strings.TrimSpace(out) != "0\n1\n2" {
		t.Errorf("output = %q, want 0\\n1\\n2", out)
	}
}

func TestContinue(t *testing.T) {
	_, out := run(t, "for i in range(5) { if i % 2 == 0 { continue }\nprint i }")
	if strings.TrimSpace(out) != "1\n3" {
		t.Errorf("output = %q, want 1\\n3", out)
	}
}

func TestElseIfChain(t *testing.T) {
	_, out := run(t, `g = 75
if g >= 90 { print "A" } else if g >= 80 { print "B" } else if g >= 70 { print "C" } else { print "F" }`)
	if strings.TrimSpace(out) != "C" {
		t.Errorf("output = %q, want C", out)
	}
}

func TestFunctionCall(t *testing.T) {
	e, _ := run(t, "fn add(a, b) { return a + b }\nresult = add(3, 4)")
	if got := e.vars["result"]; got != int64(7) {
		t.Errorf("result = %v, want 7", got)
	}
}

func TestRecursion(t *testing.T) {
	e, _ := run(t, "fn fact(n) { if n <= 1 { return 1 }\nreturn n * fact(n - 1) }\nr = fact(5)")
	if got := e.vars["r"]; got != int64(120) {
		t.Errorf("r = %v, want 120", got)
	}
}

func TestFunctionScopeIsolation(t *testing.T) {
	// A parameter named like a global must not mutate the global.
	e, _ := run(t, "x = 100\nfn f(x) { return x * 2 }\ny = f(5)")
	if got := e.vars["x"]; got != int64(100) {
		t.Errorf("global x = %v, want 100 (should be untouched)", got)
	}
	if got := e.vars["y"]; got != int64(10) {
		t.Errorf("y = %v, want 10", got)
	}
}

func TestFunctionArgCountError(t *testing.T) {
	e := New()
	l := lexer.NewLexer("fn f(a, b) { return a }\nf(1)")
	p := parser.New(l)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}
	err := e.Eval(prog)
	if err == nil || !strings.Contains(err.Error(), "expects 2") {
		t.Errorf("expected arg-count error, got %v", err)
	}
}

func TestExportAndExpand(t *testing.T) {
	e, _ := run(t, `export GREETING hello world`)
	if got := e.env["GREETING"]; got != "hello world" {
		t.Errorf("env GREETING = %q, want %q", got, "hello world")
	}

	// $X should expand to the exported value.
	e2, out := run(t, "export X foo\nprint $X")
	if strings.TrimSpace(out) != "foo" {
		t.Errorf("output = %q, want foo", out)
	}
	if e2.env["X"] != "foo" {
		t.Errorf("env X = %q, want foo", e2.env["X"])
	}
}

func TestCommandSubstitution(t *testing.T) {
	// Capture a built-in's output into a variable.
	e, _ := run(t, `s = $(print "captured")`)
	if got := e.vars["s"]; got != "captured" {
		t.Errorf("s = %q, want %q", got, "captured")
	}
}

func TestCommandSubstitutionInExpression(t *testing.T) {
	e, _ := run(t, `r = $(print "ab") + "cd"`)
	if got := e.vars["r"]; got != "abcd" {
		t.Errorf("r = %q, want abcd", got)
	}
}

func TestStringBuiltins(t *testing.T) {
	cases := []struct {
		src  string
		want Value
	}{
		{`r = len("hello")`, int64(5)},
		{"r = len([1, 2, 3])", int64(3)},
		{`r = join(split("a,b,c", ","), "-")`, "a-b-c"},
		{`r = contains("hello world", "world")`, true},
		{`r = contains("hello", "xyz")`, false},
		{`r = upper("raven")`, "RAVEN"},
		{`r = lower("SHELL")`, "shell"},
		{`r = trim("  hi  ")`, "hi"},
		{`r = replace("a-b-a", "a", "x")`, "x-b-x"},
	}
	for _, c := range cases {
		e, _ := run(t, c.src)
		if got := e.vars["r"]; got != c.want {
			t.Errorf("%s => %v (%T), want %v (%T)", c.src, got, got, c.want, c.want)
		}
	}
}

func TestSplitReturnsArray(t *testing.T) {
	e, _ := run(t, `parts = split("x,y,z", ",")`)
	arr, ok := e.vars["parts"].([]Value)
	if !ok {
		t.Fatalf("parts is not an array: %T", e.vars["parts"])
	}
	if len(arr) != 3 || arr[0] != "x" || arr[2] != "z" {
		t.Errorf("parts = %v, want [x y z]", arr)
	}
}

func TestContainsArrayMembership(t *testing.T) {
	e, _ := run(t, `r = contains(["a", "b", "c"], "b")`)
	if e.vars["r"] != true {
		t.Errorf("r = %v, want true", e.vars["r"])
	}
}

func TestSearchPathLookup(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "mytool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatal(err)
	}

	e := New()
	e.searchPaths = []string{dir}

	got, err := e.lookPath("mytool")
	if err != nil {
		t.Fatalf("lookPath(mytool) error: %v", err)
	}
	if got != tool {
		t.Errorf("lookPath = %q, want %q", got, tool)
	}

	// A non-executable file in the search path is not resolved.
	plain := filepath.Join(dir, "notexec")
	os.WriteFile(plain, []byte("x"), 0644)
	if _, err := e.lookPath("notexec"); err == nil {
		t.Error("expected lookup of non-executable to fail")
	}
}

func TestAvailableCommandsIncludesFunctionsAndExecutables(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "zztool"), []byte("#!/bin/sh\n"), 0755)

	e := New()
	e.searchPaths = []string{dir}
	// Define a user function.
	run := func(src string) {
		l := lexer.NewLexer(src)
		p := parser.New(l)
		e.Eval(p.ParseProgram())
	}
	run("fn myfunc(a) { return a }")

	cmds := e.AvailableCommands()
	has := func(name string) bool {
		for _, c := range cmds {
			if c == name {
				return true
			}
		}
		return false
	}
	if !has("myfunc") {
		t.Error("AvailableCommands missing user function myfunc")
	}
	if !has("zztool") {
		t.Error("AvailableCommands missing search-path executable zztool")
	}
}

func TestStringInterpolation(t *testing.T) {
	_, out := run(t, `name = "raven"
print "hi $name and ${name}!"`)
	if strings.TrimSpace(out) != "hi raven and raven!" {
		t.Errorf("output = %q, want %q", out, "hi raven and raven!")
	}
}

func TestSingleQuotesNotInterpolated(t *testing.T) {
	e, _ := run(t, `name = "raven"
s = '$name'`)
	if got := e.vars["s"]; got != "$name" {
		t.Errorf("s = %q, want literal $name", got)
	}
}

func TestSemicolonSeparator(t *testing.T) {
	e, _ := run(t, `a = 1 ; b = 2 ; c = 3`)
	if e.vars["a"] != int64(1) || e.vars["b"] != int64(2) || e.vars["c"] != int64(3) {
		t.Errorf("got a=%v b=%v c=%v", e.vars["a"], e.vars["b"], e.vars["c"])
	}
}

func TestLogicalAndShortCircuits(t *testing.T) {
	// A failing command (status != 0) short-circuits &&.
	_, out := run(t, `git definitely-not-a-subcommand 2> /dev/null && print "ran" || print "skipped"`)
	if strings.TrimSpace(out) != "skipped" {
		t.Errorf("output = %q, want skipped", out)
	}
}

func TestGlobReturnsArray(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "c.log"), nil, 0644)

	e := New()
	e.cwd = dir
	matches, err := e.builtinGlob([]Value{"*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := matches.([]Value)
	if !ok || len(arr) != 2 {
		t.Fatalf("glob(*.txt) = %v, want 2 matches", matches)
	}
}

func TestParseSignal(t *testing.T) {
	cases := map[string]int{"TERM": 15, "SIGKILL": 9, "9": 9, "hup": 1}
	for in, want := range cases {
		sig, err := parseSignal(in)
		if err != nil || int(sig) != want {
			t.Errorf("parseSignal(%q) = %v, %v; want %d", in, int(sig), err, want)
		}
	}
	if _, err := parseSignal("NOPE"); err == nil {
		t.Error("expected error for unknown signal")
	}
}

func TestListProcessesNonEmpty(t *testing.T) {
	procs, err := listProcesses()
	if err != nil {
		t.Skipf("ps unavailable: %v", err)
	}
	if len(procs) == 0 {
		t.Error("expected at least one process")
	}
}

func TestExternalCommandNotFound(t *testing.T) {
	// Like a real shell, a missing command sets status 127 and does not abort.
	e := New()
	_, err := e.execExternal("definitely_not_a_real_command_xyz", nil)
	if err != nil {
		t.Errorf("expected no fatal error, got %v", err)
	}
	if e.lastStatus != 127 {
		t.Errorf("expected lastStatus 127, got %d", e.lastStatus)
	}
}

func TestExternalCommandVariableFallback(t *testing.T) {
	// A bare name with no args that matches a variable is a value, not a command.
	e, _ := run(t, "myvar = 42\nprint myvar")
	if got := e.vars["myvar"]; got != int64(42) {
		t.Errorf("myvar = %v, want 42", got)
	}

	// And execExternal directly resolves the variable rather than erroring.
	out, err := e.execExternal("myvar", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "42" {
		t.Errorf("fallback value = %q, want 42", out)
	}
}

// TestExternalArgsNotAbsolutized verifies that path-like arguments to an
// external command are passed verbatim (relative paths stay relative, URLs and
// scp-style remotes are untouched) while a leading '~' is still expanded. The
// child process inherits the shell's cwd, so absolutizing relative paths is both
// unnecessary and surprising.
func TestExternalArgsNotAbsolutized(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	cases := []struct {
		src  string
		want string
	}{
		{"echo ./build/output", "./build/output"},
		{"echo src/main.go", "src/main.go"},
		{"echo https://github.com/javanhut/RavenShell.git", "https://github.com/javanhut/RavenShell.git"},
		{"echo git@github.com:javanhut/RavenShell.git", "git@github.com:javanhut/RavenShell.git"},
		{"echo ~/foo", filepath.Join(home, "foo")},
	}
	for _, c := range cases {
		_, out := run(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%q: got %q, want %q", c.src, got, c.want)
		}
	}
}
