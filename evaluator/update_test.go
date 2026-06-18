package evaluator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsRavenSource(t *testing.T) {
	dir := t.TempDir()
	if isRavenSource(dir) {
		t.Fatal("a directory without go.mod should not be a RavenShell source")
	}

	writeGoMod(t, dir, "module somethingelse\n\ngo 1.25\n")
	if isRavenSource(dir) {
		t.Fatal("a non-ravenshell module should not match")
	}

	writeGoMod(t, dir, "module ravenshell\n\ngo 1.25\n")
	if !isRavenSource(dir) {
		t.Fatal("the ravenshell module should match")
	}
}

func TestResolveSourceDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "module ravenshell\n")

	t.Setenv("RAVENSHELL_SRC", dir)
	got, err := resolveSourceDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Fatalf("resolveSourceDir() = %q, want %q", got, dir)
	}
}

func TestResolveSourceDirRejectsBadEnv(t *testing.T) {
	// RAVENSHELL_SRC points at a real directory, but not a RavenShell tree.
	t.Setenv("RAVENSHELL_SRC", t.TempDir())
	if _, err := resolveSourceDir(); err == nil {
		t.Fatal("expected an error when RAVENSHELL_SRC is not a source tree")
	}
}

func TestExecRavenUpdateRejectsUnknownArg(t *testing.T) {
	e := New()
	if _, err := e.execRavenUpdate([]string{"--bogus"}); err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}

func TestExecRavenUpdateHelp(t *testing.T) {
	e := New()
	// --help must short-circuit without touching the filesystem or git.
	if _, err := e.execRavenUpdate([]string{"--help"}); err != nil {
		t.Fatalf("--help returned an error: %v", err)
	}
}

// TestToolPathUsesAugmentedResolution covers the raven-update PATH fix: build
// tools must be resolved through the shell's own command resolution (search
// paths and built-in default dirs), not just the bare process PATH. A
// GUI-launched shell often inherits a minimal PATH that lacks Go's directory
// even though `go` runs fine interactively, which used to make raven-update
// report "Go is required".
func TestToolPathUsesAugmentedResolution(t *testing.T) {
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A PATH that deliberately excludes dir, so a bare process-PATH lookup fails.
	t.Setenv("PATH", filepath.Join(dir, "nonexistent"))
	if _, err := exec.LookPath("go"); err == nil {
		t.Skip("environment unexpectedly resolves go on PATH; cannot isolate")
	}

	e := New()
	e.searchPaths = []string{dir} // as if registered via `raven-add path`

	// The augmented resolution finds the tool by absolute path...
	if got := e.toolPath("go"); got != fakeGo {
		t.Errorf("toolPath(go) = %q, want %q (should resolve via search paths)", got, fakeGo)
	}
	// ...and lookPath agrees (this is what the "Go is required" check now uses).
	if got, err := e.lookPath("go"); err != nil || got != fakeGo {
		t.Errorf("lookPath(go) = %q, err=%v; want %q", got, err, fakeGo)
	}
	// Unknown tools fall back to the bare name, preserving prior behavior.
	if got := e.toolPath("definitely-not-a-tool-xyz"); got != "definitely-not-a-tool-xyz" {
		t.Errorf("toolPath(unknown) = %q, want the bare name", got)
	}
}

// TestRunCapturedHonorsEnv verifies the run helper applies the supplied
// environment — the mechanism that hands go/git the shell's augmented PATH.
func TestRunCapturedHonorsEnv(t *testing.T) {
	out, err := runCaptured([]string{"RAVEN_TEST_VAR=present"}, "", "/bin/sh", "-c", "printf %s \"$RAVEN_TEST_VAR\"")
	if err != nil {
		t.Fatalf("runCaptured: %v", err)
	}
	if out != "present" {
		t.Errorf("runCaptured output = %q, want %q", out, "present")
	}
}

func writeGoMod(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
