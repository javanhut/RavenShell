package readline

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestReadline builds a Readline with controlled history and no persistence.
func newTestReadline(history ...string) *Readline {
	return &Readline{
		historyIdx: -1,
		history:    history,
	}
}

func TestSuggestionForReturnsMostRecentPrefixMatch(t *testing.T) {
	r := newTestReadline("git status", "git commit -m x", "ls")

	if got := r.suggestionFor("git "); got != "git commit -m x" {
		t.Errorf(`suggestionFor("git ") = %q, want "git commit -m x"`, got)
	}
	if got := r.suggestionFor("ls"); got != "" {
		t.Errorf(`suggestionFor("ls") = %q, want "" (no longer match)`, got)
	}
	if got := r.suggestionFor(""); got != "" {
		t.Errorf(`suggestionFor("") = %q, want ""`, got)
	}
	if got := r.suggestionFor("zzz"); got != "" {
		t.Errorf(`suggestionFor("zzz") = %q, want ""`, got)
	}
}

func TestTryAcceptSuggestion(t *testing.T) {
	r := newTestReadline("print hello world")

	line := []rune("print ")
	nl, np, ok := r.tryAcceptSuggestion(line, len(line))
	if !ok {
		t.Fatal("expected suggestion to be accepted")
	}
	if string(nl) != "print hello world" || np != len(nl) {
		t.Errorf("accepted = %q (pos %d), want full suggestion at end", string(nl), np)
	}

	// Not at end of line -> no acceptance.
	if _, _, ok := r.tryAcceptSuggestion(line, 2); ok {
		t.Error("should not accept when cursor is not at end of line")
	}
}

func TestAddHistoryDedupConsecutive(t *testing.T) {
	r := newTestReadline()
	r.AddHistory("ls")
	r.AddHistory("ls")
	r.AddHistory("cd /")
	if len(r.history) != 2 {
		t.Errorf("history = %v, want 2 entries (consecutive dup collapsed)", r.history)
	}
}

func TestHistoryPersistence(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, ".raven_history")

	r := newTestReadline()
	r.historyFile = histPath
	r.AddHistory("print one")
	r.AddHistory("print two")

	// A fresh instance pointed at the same file should load both entries.
	r2 := newTestReadline()
	r2.historyFile = histPath
	r2.loadHistory()

	if len(r2.history) != 2 || r2.history[0] != "print one" || r2.history[1] != "print two" {
		t.Errorf("loaded history = %v, want [print one, print two]", r2.history)
	}

	if _, err := os.Stat(histPath); err != nil {
		t.Errorf("history file not written: %v", err)
	}
}
