package evaluator

import (
	"os"
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

func writeGoMod(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
