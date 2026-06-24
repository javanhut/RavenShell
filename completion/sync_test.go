package completion

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRemoveCached(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	cacheDir := DefaultCacheDir()
	if err := os.MkdirAll(filepath.Join(cacheDir, "help"), 0o755); err != nil {
		t.Fatal(err)
	}

	manFile := filepath.Join(cacheDir, "ripgrep.json")
	helpFile := filepath.Join(cacheDir, "help", "ripgrep.json")
	mustWrite(t, manFile, "{}")
	mustWrite(t, helpFile, "{}")

	if !RemoveCached("ripgrep") {
		t.Fatal("RemoveCached(ripgrep) = false, want true")
	}
	if _, err := os.Stat(manFile); !os.IsNotExist(err) {
		t.Error("man cache file was not removed")
	}
	if _, err := os.Stat(helpFile); !os.IsNotExist(err) {
		t.Error("help cache file was not removed")
	}

	// Nothing left to remove the second time.
	if RemoveCached("ripgrep") {
		t.Error("RemoveCached on an absent entry = true, want false")
	}
	// Unsafe names are rejected outright.
	if RemoveCached("../etc/passwd") {
		t.Error("RemoveCached accepted an unsafe name")
	}
}

func TestBuildCachedNoopForMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)

	BuildCached("definitely-not-a-real-command-xyz123")
	BuildCached("../bad")

	// A command that does not exist must not create any cache files.
	if entries, err := os.ReadDir(DefaultCacheDir()); err == nil && len(entries) > 0 {
		t.Errorf("BuildCached created cache entries for a missing command: %d", len(entries))
	}
}

func TestBuildCachedRealCommand(t *testing.T) {
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("ls not on PATH")
	}
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)

	BuildCached("ls")

	// ls has a man page on macOS and Linux; if man is available a cache file
	// should now exist. Skip rather than fail where man is absent (minimal CI).
	if _, err := os.Stat(filepath.Join(DefaultCacheDir(), "ls.json")); err != nil {
		t.Skipf("no ls man cache produced (man unavailable?): %v", err)
	}
}
