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

// evalScript runs src end-to-end (lexer -> parser -> evaluator) against a fresh
// evaluator rooted at dir, returning captured stdout and the evaluation error
// (if any). Parser errors fail the test; eval errors are returned so tests can
// assert on both success and failure paths.
func evalScript(t *testing.T, dir, src string) (string, error) {
	t.Helper()
	e := New()
	e.cwd = dir
	var buf bytes.Buffer
	e.stdout = &buf

	l := lexer.NewLexer(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors for %q: %v", src, errs)
	}
	err := e.Eval(program)
	return buf.String(), err
}

// writeFile is a test helper that creates a file (and parents) with content.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestReadTrickyFilenames is the end-to-end regression for the path-splitting
// bug: reading multi-dot names, dotfiles, dotfiles-with-extensions, and paths
// whose segments are reserved words or numbers must return the file's contents.
func TestReadTrickyFilenames(t *testing.T) {
	cases := []struct {
		file    string
		content string
	}{
		{"archive.tar.gz", "tarball-bytes"},
		{"bundle.min.js", "console.log(1)"},
		{".env.local", "SECRET=42"},
		{".gitignore", "node_modules/"},
		{"config/output.log", "log-line"}, // 'output' is a reserved word
		{"lib/print.txt", "printed"},      // 'print' is a reserved word
		{"data/v1.2.3.json", "{\"v\":1}"}, // numeric segments
		{"notes/.env", "A=B"},             // dotfile that is a reserved word
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.content)
			out, err := evalScript(t, dir, "read "+tc.file)
			if err != nil {
				t.Fatalf("read %s errored: %v", tc.file, err)
			}
			if out != tc.content {
				t.Errorf("read %s = %q, want %q", tc.file, out, tc.content)
			}
		})
	}
}

// TestReadAliasesEquivalent checks read/view/show behave identically.
func TestReadAliasesEquivalent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "hello")
	for _, cmd := range []string{"show", "read", "view"} {
		out, err := evalScript(t, dir, cmd+" doc.md")
		if err != nil || out != "hello" {
			t.Errorf("%s doc.md = %q, err=%v; want %q", cmd, out, err, "hello")
		}
	}
}

// TestWriteReadRoundTrip exercises output redirection followed by reading back.
func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := evalScript(t, dir, `print "first line" > log.txt`); err != nil {
		t.Fatalf("redirect write failed: %v", err)
	}
	if _, err := evalScript(t, dir, `print "second line" >> log.txt`); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	out, err := evalScript(t, dir, "read log.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "first line") || !strings.Contains(out, "second line") {
		t.Errorf("round-trip content = %q, want both lines", out)
	}
}

// TestPipeListIntoPrint checks a pipe between a builtin and print.
func TestPipeListIntoPrint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "alpha.txt", "")
	writeFile(t, dir, "beta.txt", "")
	out, err := evalScript(t, dir, "ls | print")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alpha.txt") || !strings.Contains(out, "beta.txt") {
		t.Errorf("ls | print = %q, want both files", out)
	}
}

// TestCommandSubstitutionCapturesBuiltin checks $(...) capturing a builtin's output.
func TestCommandSubstitutionCapturesBuiltin(t *testing.T) {
	dir := t.TempDir()
	out, err := evalScript(t, dir, "here = $(whereami)\nprint here")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != dir {
		t.Errorf("$(whereami) captured %q, want %q", strings.TrimSpace(out), dir)
	}
}

// TestExitStatusSuccessAndMissing checks $? after a success and after an
// unknown external command (which must not abort the program).
func TestExitStatusSuccessAndMissing(t *testing.T) {
	dir := t.TempDir()

	out, err := evalScript(t, dir, "whereami\nprint $?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\n0") && !strings.HasSuffix(strings.TrimSpace(out), "0") {
		t.Errorf("expected exit status 0 after success, got %q", out)
	}

	out, err = evalScript(t, dir, "definitely_not_a_real_command_xyz\nprint $?")
	if err != nil {
		t.Fatalf("a missing command should not abort the program: %v", err)
	}
	if !strings.Contains(out, "127") {
		t.Errorf("expected exit status 127 for missing command, got %q", out)
	}
}

// TestForLoopCreatesFiles exercises control flow combined with a file command.
func TestForLoopCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	src := `for i in range(3) {
    name = "file" + i + ".txt"
    touch name
}`
	if _, err := evalScript(t, dir, src); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, "file"+string(rune('0'+i))+".txt")
		if _, err := os.Stat(name); err != nil {
			t.Errorf("loop did not create %s: %v", name, err)
		}
	}
}

// TestMakeAndRemoveLifecycle walks a full create/list/remove lifecycle using
// the human-readable aliases.
func TestMakeAndRemoveLifecycle(t *testing.T) {
	dir := t.TempDir()

	if _, err := evalScript(t, dir, "makedir project\nnewfile project/readme.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "readme.md")); err != nil {
		t.Fatalf("expected project/readme.md to exist: %v", err)
	}

	// rmdir without force must fail on a non-empty directory.
	if _, err := evalScript(t, dir, "rmdir project"); err == nil {
		t.Error("rmdir on a non-empty directory should fail without --force")
	}
	if _, err := os.Stat(filepath.Join(dir, "project")); err != nil {
		t.Errorf("project should still exist after failed rmdir: %v", err)
	}

	// rmdir --force removes it recursively.
	if _, err := evalScript(t, dir, "rmdir project --force"); err != nil {
		t.Fatalf("rmdir --force failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project")); !os.IsNotExist(err) {
		t.Errorf("project should be gone after rmdir --force (err=%v)", err)
	}
}

// TestHelpEndToEnd smoke-checks raven-help and the help alias.
func TestHelpEndToEnd(t *testing.T) {
	dir := t.TempDir()
	out, err := evalScript(t, dir, "raven-help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"built-in commands", "whereami", "rmdir"} {
		if !strings.Contains(out, want) {
			t.Errorf("raven-help overview missing %q", want)
		}
	}

	out, err = evalScript(t, dir, "help rmdir")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("help rmdir should mention --force, got: %q", out)
	}
}
