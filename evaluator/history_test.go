package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRavenHistoryClearUsesLineEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	e := New()
	called := false
	e.SetHistoryClearer(func() (int, error) { called = true; return 3, nil })
	var out strings.Builder
	e.stdout = &out

	if _, err := e.execRavenHistory([]string{"clear"}); err != nil {
		t.Fatalf("raven-history clear: %v", err)
	}
	if !called {
		t.Fatal("raven-history clear did not call the line editor's clearer")
	}
	if !strings.Contains(out.String(), "Cleared 3 history entries") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRavenHistoryClearWithoutLineEditorRemovesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hist := filepath.Join(home, ".raven_history")
	if err := os.WriteFile(hist, []byte("ls\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, out := run(t, "raven-history clear\n")
	if _, err := os.Stat(hist); !os.IsNotExist(err) {
		t.Errorf("history file not removed: %v", err)
	}
	if !strings.Contains(out, hist) {
		t.Errorf("output = %q, want it to name %s", out, hist)
	}
}

func TestRavenHistoryArguments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := New()
	var out strings.Builder
	e.stdout = &out

	if _, err := e.execRavenHistory([]string{"path"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != filepath.Join(home, ".raven_history") {
		t.Errorf("raven-history path = %q", got)
	}
	if _, err := e.execRavenHistory([]string{"bogus"}); err == nil {
		t.Error("expected an error for an unknown argument")
	}
	if _, err := e.execRavenHistory([]string{"clear", "path"}); err == nil {
		t.Error("expected an error for two actions")
	}
}
