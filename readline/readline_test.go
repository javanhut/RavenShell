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

func TestCompleteCommandMergesProvider(t *testing.T) {
	r := newTestReadline()
	r.commands = []string{"print", "ls", "raven-add"}
	r.SetCommandProvider(func() []string {
		return []string{"raventool", "myfunc", "raven-add"} // includes a duplicate
	})

	matches := r.completeCommand("raven")
	// Expect built-in "raven-add" + provided "raventool", deduped and sorted.
	want := map[string]bool{"raven-add": true, "raventool": true}
	if len(matches) != len(want) {
		t.Fatalf("matches = %v, want keys %v", matches, want)
	}
	for _, m := range matches {
		if !want[m] {
			t.Errorf("unexpected match %q", m)
		}
	}
	// Sorted order.
	if matches[0] != "raven-add" || matches[1] != "raventool" {
		t.Errorf("matches not sorted: %v", matches)
	}

	// A prefix matching only a provided function.
	if got := r.completeCommand("myf"); len(got) != 1 || got[0] != "myfunc" {
		t.Errorf("completeCommand(myf) = %v, want [myfunc]", got)
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

func TestCommonPrefix(t *testing.T) {
	cands := []Candidate{{Text: "checkout"}, {Text: "cherry-pick"}}
	if got := commonPrefix(cands); got != "che" {
		t.Errorf("commonPrefix = %q, want \"che\"", got)
	}
	if got := commonPrefix(nil); got != "" {
		t.Errorf("commonPrefix(nil) = %q, want \"\"", got)
	}
	if got := commonPrefix([]Candidate{{Text: "only"}}); got != "only" {
		t.Errorf("commonPrefix(single) = %q, want \"only\"", got)
	}
	if got := commonPrefix([]Candidate{{Text: "abc"}, {Text: "xyz"}}); got != "" {
		t.Errorf("commonPrefix(disjoint) = %q, want \"\"", got)
	}
}

func TestCurrentWord(t *testing.T) {
	if got := currentWord("git che"); got != "che" {
		t.Errorf("currentWord = %q, want \"che\"", got)
	}
	if got := currentWord("git "); got != "" {
		t.Errorf("currentWord after space = %q, want \"\"", got)
	}
	if got := currentWord(""); got != "" {
		t.Errorf("currentWord empty = %q, want \"\"", got)
	}
}

func TestApplyCompletionSpace(t *testing.T) {
	r := newTestReadline()

	// Full completion of a non-directory appends a space.
	line, pos := r.applyCompletion([]rune("git che"), 7, "checkout", true)
	if string(line) != "git checkout " || pos != len(line) {
		t.Errorf("applyCompletion = %q (pos %d), want \"git checkout \"", string(line), pos)
	}

	// Common-prefix insertion must not append a space.
	line, pos = r.applyCompletion([]rune("git c"), 5, "ch", false)
	if string(line) != "git ch" || pos != len(line) {
		t.Errorf("prefix insertion = %q (pos %d), want \"git ch\"", string(line), pos)
	}

	// Directories never get a trailing space.
	line, _ = r.applyCompletion([]rune("cd sr"), 5, "src/", true)
	if string(line) != "cd src/" {
		t.Errorf("dir completion = %q, want \"cd src/\"", string(line))
	}
}

// TestApplyCompletionQuoting covers wrapping candidates with spaces / special
// characters so the inserted word round-trips through the lexer.
func TestApplyCompletionQuoting(t *testing.T) {
	r := newTestReadline()
	cases := []struct {
		name       string
		line       string
		completion string
		addSpace   bool
		want       string
	}{
		// Apostrophe forces double quotes (single quotes can't hold a ').
		{"apostrophe dir", "cd The", "The World's Strongest Rearguard/", true,
			`cd "The World's Strongest Rearguard/"`},
		// A plain space uses single quotes (literal, no $ expansion).
		{"space dir", "cd My", "My Documents/", true, "cd 'My Documents/'"},
		// No special chars: inserted bare, with the usual trailing space.
		{"plain file", "cat re", "readme.md", true, "cat readme.md "},
		// Common-prefix (addSpace=false) must stay bare even with a space.
		{"prefix stays bare", "cd My", "My Doc", false, "cd My Doc"},
		// A leading ~/ stays outside the quotes so it still expands.
		{"home path", "cd ~/My", "~/My Docs/", true, "cd ~/'My Docs/'"},
	}
	for _, c := range cases {
		line, _ := r.applyCompletion([]rune(c.line), len([]rune(c.line)), c.completion, c.addSpace)
		if string(line) != c.want {
			t.Errorf("%s: applyCompletion = %q, want %q", c.name, string(line), c.want)
		}
	}
}

// TestDisplayWidth covers the ANSI-aware width used to compute how many rows the
// wrapped prompt+line occupy. Color codes must contribute zero cells.
func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"plain ascii", "hello", 5},
		{"empty", "", 0},
		{"single color code", "\033[32mgreen\033[0m", 5},
		{"realistic prompt", "[\033[32mjavanhutchinson\033[0m@\033[34mlinux\033[0m] > ", 26},
		{"only escapes", "\033[1m\033[0m", 0},
		{"bare two-byte escape", "\033Mx", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayWidth(tc.in); got != tc.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestSuggestionTail returns only the portion of an autosuggestion beyond what
// is already typed, so draw renders just the dim remainder.
func TestSuggestionTail(t *testing.T) {
	if got := suggestionTail([]rune("git "), "git status"); string(got) != "status" {
		t.Errorf("suggestionTail = %q, want \"status\"", string(got))
	}
	if got := suggestionTail([]rune("ls"), ""); got != nil {
		t.Errorf("suggestionTail with no suggestion = %q, want nil", string(got))
	}
	if got := suggestionTail([]rune("git status"), "git status"); got != nil {
		t.Errorf("suggestionTail with equal-length suggestion = %q, want nil", string(got))
	}
}
