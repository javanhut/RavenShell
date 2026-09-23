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

func TestProtectedPath(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "usr-link")
	if err := os.Symlink("/usr", link); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, ".bashrc", "")
	if err := os.Mkdir(filepath.Join(home, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := map[string]bool{
		"/":                             true,
		"/usr":                          true,
		"/usr/":                         true,
		"/usr/..":                       true,
		"/etc/../home":                  true,
		home:                            true,
		home + "/projects/..":           true,
		"/usr/bin":                      false,
		dir:                             false,
		link:                            false, // removing the link, not /usr
		"/no-such-root":                 false,
		filepath.Join(home, "projects"): false,
		filepath.Join(home, ".bashrc"):  false,
	}
	for p, want := range cases {
		if got := protectedPath(p) != ""; got != want {
			t.Errorf("protectedPath(%q) = %v, want %v", p, got, want)
		}
	}
	// Other users' homes: any directory directly under /home.
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, en := range entries {
			if en.IsDir() && protectedPath("/home/"+en.Name()) == "" {
				t.Errorf("/home/%s not protected", en.Name())
			}
		}
	}
}

// TestRemoveRefusesHome checks `rm -rf ~` and friends are refused while the
// home directory's contents can still be removed.
func TestRemoveRefusesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"rm -rf ~", "rm -rf ~/", "rm -rf " + home, "rmdir -f ~/junk/.."} {
		e, stderr := evalWithStderr(t, home, src)
		if e.lastStatus != 1 || !strings.Contains(stderr, "your home directory") {
			t.Errorf("%s: status %d, stderr %q; want refusal", src, e.lastStatus, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "junk")); err != nil {
		t.Fatalf("home contents removed: %v", err)
	}
	if e, stderr := evalWithStderr(t, home, "rm -rf ~/junk"); e.lastStatus != 0 {
		t.Errorf("rm -rf ~/junk: status %d, stderr %q", e.lastStatus, stderr)
	}
}

// TestRemoveRefusesRoot checks rm/rmdir refuse / even with
// --no-preserve-root, and that nothing else on the line is removed first.
func TestRemoveRefusesRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.txt", "")
	for _, src := range []string{
		"rm -rf / --no-preserve-root",
		"rm -fr keep.txt /",
		"rm -rf /usr",
		"remove -r /home",
		"rmdir -f /",
	} {
		e, stderr := evalWithStderr(t, dir, src)
		if e.lastStatus != 1 || !strings.Contains(stderr, "refusing to remove") {
			t.Errorf("%s: status %d, stderr %q; want refusal", src, e.lastStatus, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Errorf("keep.txt removed before the refusal: %v", err)
	}
}

// TestExternalRmRefusesRoot checks rm reached through sudo, or by path, is
// refused before it is started. The "sudo" and "rm" here are stand-ins that
// only record that they ran, so a failure cannot delete anything.
func TestExternalRmRefusesRoot(t *testing.T) {
	bin := t.TempDir()
	ran := filepath.Join(bin, "ran")
	for _, name := range []string{"sudo", "rm"} {
		script := "#!/bin/sh\necho \"$@\" >> " + ran + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Called by full path: lookPath searches registered paths before $PATH,
	// so a bare "sudo" could reach the real one.
	sudo := filepath.Join(bin, "sudo")
	for _, src := range []string{
		sudo + " rm -rf / --no-preserve-root",
		sudo + " -E rm -rf /etc",
		bin + "/rm -rf /usr",
	} {
		e, stderr := evalWithStderr(t, bin, src)
		if e.lastStatus != 1 || !strings.Contains(stderr, "refusing to remove") {
			t.Errorf("%s: status %d, stderr %q; want refusal", src, e.lastStatus, stderr)
		}
	}
	if out, err := os.ReadFile(ran); err == nil {
		t.Errorf("wrapped rm was started: %s", out)
	}

	// An ordinary sudo rm still runs.
	if e, stderr := evalWithStderr(t, bin, sudo+" rm -rf build"); e.lastStatus != 0 {
		t.Errorf("sudo rm -rf build: status %d, stderr %q", e.lastStatus, stderr)
	}
	if out, _ := os.ReadFile(ran); strings.TrimSpace(string(out)) != "rm -rf build" {
		t.Errorf("sudo got %q, want \"rm -rf build\"", out)
	}
}

// evalWithStderr runs src and returns the evaluator and its stderr output.
func evalWithStderr(t *testing.T, dir, src string) (*Evaluator, string) {
	t.Helper()
	e := New()
	e.cwd = dir
	var stderr bytes.Buffer
	e.stdout = &bytes.Buffer{}
	e.stderr = &stderr
	p := parser.New(lexer.NewLexer(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors for %q: %v", src, errs)
	}
	if err := e.Eval(program); err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return e, stderr.String()
}

// TestRemoveStillWorks makes sure ordinary removals are unaffected.
func TestRemoveStillWorks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "sub/deep/f", "")
	if _, err := evalScript(t, dir, "rm -rf sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); !os.IsNotExist(err) {
		t.Errorf("sub still exists: %v", err)
	}
}
