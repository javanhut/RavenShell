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

// runIn parses and evaluates src against an evaluator rooted at dir, capturing
// stdout. It is like run() but lets filesystem commands operate in a temp dir.
func runIn(t *testing.T, dir, src string) (*Evaluator, string) {
	t.Helper()
	e := New()
	e.cwd = dir
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

// sameDir reports whether two paths refer to the same directory, resolving
// symlinks (macOS temp dirs live under /var -> /private/var).
func sameDir(a, b string) bool {
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	return ea == nil && eb == nil && ra == rb
}

// TestCdSyncsProcessCwd verifies that cd moves the OS process working directory,
// not just the shell's tracked cwd — so a host terminal reading the process cwd
// (RavenTerminal, for new tabs/splits) follows the user into and back out of
// directories. Mirrors: cd into /test/person, then cd .. back to /test.
func TestCdSyncsProcessCwd(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	root := t.TempDir()
	sub := filepath.Join(root, "person")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	e := New()
	e.cwd = root
	var buf bytes.Buffer
	e.stdout = &buf

	if _, err := e.execChangeDir([]string{"person"}); err != nil {
		t.Fatalf("cd person: %v", err)
	}
	if wd, _ := os.Getwd(); !sameDir(wd, sub) {
		t.Errorf("process cwd = %q, want %q", wd, sub)
	}
	if !sameDir(e.GetCwd(), sub) {
		t.Errorf("e.GetCwd() = %q, want %q", e.GetCwd(), sub)
	}

	if _, err := e.execChangeDir([]string{".."}); err != nil {
		t.Fatalf("cd ..: %v", err)
	}
	if wd, _ := os.Getwd(); !sameDir(wd, root) {
		t.Errorf("after cd .., process cwd = %q, want %q", wd, root)
	}
}

// TestLocationAliases checks that whereami/wai behave like cwd.
func TestLocationAliases(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range []string{"cwd", "whereami", "wai"} {
		_, out := runIn(t, dir, cmd)
		if strings.TrimSpace(out) != dir {
			t.Errorf("%s printed %q, want %q", cmd, strings.TrimSpace(out), dir)
		}
	}
}

// TestReadAliasReadsFile checks read/view print file contents like show.
func TestReadAliasReadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi there"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"show", "read", "view"} {
		_, out := runIn(t, dir, cmd+" hello.txt")
		if out != "hi there" {
			t.Errorf("%s hello.txt = %q, want %q", cmd, out, "hi there")
		}
	}
}

// TestCreateAliases checks makefile/newfile/touch create files and makedir
// creates directories.
func TestCreateAliases(t *testing.T) {
	dir := t.TempDir()
	for i, cmd := range []string{"mkfile", "makefile", "newfile", "touch"} {
		fname := "f" + string(rune('0'+i)) + ".txt"
		runIn(t, dir, cmd+" "+fname)
		if _, err := os.Stat(filepath.Join(dir, fname)); err != nil {
			t.Errorf("%s did not create %s: %v", cmd, fname, err)
		}
	}

	runIn(t, dir, "makedir sub/nested")
	info, err := os.Stat(filepath.Join(dir, "sub", "nested"))
	if err != nil || !info.IsDir() {
		t.Errorf("makedir did not create nested directory: %v", err)
	}
}

// TestRemoveAliases checks remove/delete remove files and directories.
func TestRemoveAliases(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gone.txt")
	for _, cmd := range []string{"remove", "delete"} {
		if err := os.WriteFile(target, nil, 0644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, cmd+" gone.txt")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("%s did not remove the file (err=%v)", cmd, err)
		}
	}
}

// TestRmdirEmptyOnlyWithoutForce verifies plain rmdir refuses a non-empty
// directory and points at --force.
func TestRmdirEmptyOnlyWithoutForce(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "full")
	if err := os.MkdirAll(filepath.Join(full, "child"), 0755); err != nil {
		t.Fatal(err)
	}

	e := New()
	e.cwd = dir
	_, err := e.execRemoveDir([]string{"full"})
	if err == nil {
		t.Fatal("rmdir on a non-empty dir should fail without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should suggest --force, got: %v", err)
	}
	if _, statErr := os.Stat(full); statErr != nil {
		t.Errorf("directory should still exist after failed rmdir: %v", statErr)
	}
}

// TestRmdirForceRecurses verifies rmdir -f / --force removes a non-empty dir.
func TestRmdirForceRecurses(t *testing.T) {
	for _, flag := range []string{"-f", "--force"} {
		dir := t.TempDir()
		full := filepath.Join(dir, "full")
		if err := os.MkdirAll(filepath.Join(full, "child"), 0755); err != nil {
			t.Fatal(err)
		}
		e := New()
		e.cwd = dir
		if _, err := e.execRemoveDir([]string{"full", flag}); err != nil {
			t.Fatalf("rmdir full %s failed: %v", flag, err)
		}
		if _, err := os.Stat(full); !os.IsNotExist(err) {
			t.Errorf("rmdir %s did not remove the directory (err=%v)", flag, err)
		}
	}
}

// TestParseFlags exercises the flag splitter used by file commands.
func TestParseFlags(t *testing.T) {
	pos, flags := parseFlags([]string{"a", "-rf", "--force", "b", "-", "--max=5"})
	wantPos := []string{"a", "b", "-"}
	if strings.Join(pos, ",") != strings.Join(wantPos, ",") {
		t.Errorf("positionals = %v, want %v", pos, wantPos)
	}
	for _, f := range []string{"r", "f", "force", "max"} {
		if !flags[f] {
			t.Errorf("flag %q missing from %v", f, flags)
		}
	}
}

// TestRavenHelpOverview checks the no-argument help lists commands and aliases.
func TestRavenHelpOverview(t *testing.T) {
	dir := t.TempDir()
	_, out := runIn(t, dir, "raven-help")
	for _, want := range []string{"built-in commands", "whereami", "remove", "rmdir"} {
		if !strings.Contains(out, want) {
			t.Errorf("raven-help overview missing %q\n%s", want, out)
		}
	}
}

// TestRavenHelpForAlias checks that help resolves aliases to their command and
// that the `help` alias works too.
func TestRavenHelpForAlias(t *testing.T) {
	dir := t.TempDir()
	_, out := runIn(t, dir, "help read")
	if !strings.Contains(out, "show") {
		t.Errorf("help read should resolve to the show command:\n%s", out)
	}
	if !strings.Contains(out, "aliases:") || !strings.Contains(out, "view") {
		t.Errorf("help read should list aliases including view:\n%s", out)
	}
}

// TestRavenHelpUnknown checks an unknown command name is an error.
func TestRavenHelpUnknown(t *testing.T) {
	e := New()
	if _, err := e.execRavenHelp([]string{"nope"}); err == nil {
		t.Error("expected error for unknown command")
	}
}

// TestBuiltinsInCompletion checks built-in names and aliases are completable.
func TestBuiltinsInCompletion(t *testing.T) {
	e := New()
	got := make(map[string]bool)
	for _, name := range e.AvailableCommands() {
		got[name] = true
	}
	for _, want := range []string{"whereami", "read", "remove", "raven-help"} {
		if !got[want] {
			t.Errorf("AvailableCommands missing built-in %q", want)
		}
	}
}
