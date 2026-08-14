package completion

import (
	"os"
	"os/exec"
	"path/filepath"
	"ravenshell/ansi"
	"strings"
	"testing"
)

// newTestEngine builds an engine rooted at dir with a fixed command list and
// no user spec directory.
func newTestEngine(dir string, commands ...string) *Engine {
	e := New(
		func() string { return dir },
		func() []string { return commands },
		map[string]string{"print": "Print text."},
	)
	e.specDir = ""
	e.cacheDir = dir // keep man/--help caches inside the test's temp dir
	return e
}

func texts(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Text
	}
	return out
}

func contains(cands []Candidate, text string) bool {
	for _, c := range cands {
		if c.Text == text {
			return true
		}
	}
	return false
}

func TestCompleteCommandNames(t *testing.T) {
	e := newTestEngine(t.TempDir(), "print", "ps", "git")

	got := e.Complete("p", 1)
	if len(got) != 2 || got[0].Text != "print" || got[1].Text != "ps" {
		t.Fatalf("Complete(p) = %v, want [print ps]", texts(got))
	}
	// Description comes from the summaries map.
	if got[0].Desc != "Print text." {
		t.Errorf("print desc = %q, want summary", got[0].Desc)
	}
}

// raven-add takes only "path", then a directory: no file noise at either spot.
func TestRavenAddCompletion(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "bin"), 0o755)
	os.WriteFile(filepath.Join(dir, "note.txt"), nil, 0o644)
	e := newTestEngine(dir)

	if got := texts(e.Complete("raven-add ", 10)); len(got) != 1 || got[0] != "path" {
		t.Errorf("Complete(raven-add ) = %v, want [path]", got)
	}
	got := texts(e.Complete("raven-add path ", 15))
	if len(got) != 1 || !strings.HasPrefix(got[0], "bin") {
		t.Errorf("Complete(raven-add path ) = %v, want [bin/]", got)
	}
}

func TestGitSubcommandCompletion(t *testing.T) {
	e := newTestEngine(t.TempDir())

	got := e.Complete("git ch", 6)
	if len(got) != 2 || got[0].Text != "checkout" || got[1].Text != "cherry-pick" {
		t.Fatalf("Complete(git ch) = %v, want [checkout cherry-pick]", texts(got))
	}
	if got[0].Desc == "" {
		t.Error("subcommand candidates should carry descriptions")
	}

	// Full subcommand list, no files mixed in.
	all := e.Complete("git ", 4)
	if !contains(all, "commit") || !contains(all, "rebase") {
		t.Errorf("Complete(git ) missing expected subcommands, got %v", texts(all))
	}
}

func TestSubcommandFlagCompletion(t *testing.T) {
	e := newTestEngine(t.TempDir())

	got := e.Complete("git commit --a", 14)
	if !contains(got, "--amend") || !contains(got, "--all") {
		t.Fatalf("Complete(git commit --a) = %v, want --amend and --all", texts(got))
	}
	// Flags from another subcommand must not leak in.
	if contains(got, "--abort") {
		t.Errorf("commit flags include --abort from another subcommand: %v", texts(got))
	}

	// Global flags are offered alongside subcommand flags.
	got = e.Complete("git commit -", 12)
	if !contains(got, "-m") || !contains(got, "-C") {
		t.Errorf("Complete(git commit -) = %v, want -m (sub) and -C (global)", texts(got))
	}
}

func TestUnknownSubcommandFallsBackToFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "notes.txt"), "x")
	e := newTestEngine(dir)

	got := e.Complete("git frobnicate no", 17)
	if len(got) != 1 || got[0].Text != "notes.txt" {
		t.Fatalf("Complete(git frobnicate no) = %v, want [notes.txt]", texts(got))
	}
}

// A directory whose name contains spaces and an apostrophe must still complete
// to a single candidate carrying the full name; readline quotes it on insert.
func TestCompleteDirWithSpacesAndApostrophe(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "The World's Strongest Rearguard"), 0755); err != nil {
		t.Fatal(err)
	}
	e := newTestEngine(dir)

	got := e.Complete("cd The", 6)
	if len(got) != 1 || got[0].Text != "The World's Strongest Rearguard/" {
		t.Fatalf("Complete(cd The) = %v, want [The World's Strongest Rearguard/]", texts(got))
	}
}

func TestUnknownCommandCompletesFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "alpha.txt"), "x")
	mustWrite(t, filepath.Join(dir, "beta.txt"), "x")
	if err := os.Mkdir(filepath.Join(dir, "alphadir"), 0755); err != nil {
		t.Fatal(err)
	}
	e := newTestEngine(dir)

	got := e.Complete("someprog al", 11)
	want := []string{"alpha.txt", "alphadir/"}
	if len(got) != 2 || got[0].Text != want[0] || got[1].Text != want[1] {
		t.Fatalf("Complete(someprog al) = %v, want %v", texts(got), want)
	}
}

func TestDirsOnlyCompletion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "file.txt"), "x")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	e := newTestEngine(dir)

	got := e.Complete("cd ", 3)
	if len(got) != 1 || got[0].Text != "subdir/" {
		t.Fatalf("Complete(cd ) = %v, want [subdir/] (directories only)", texts(got))
	}
}

func TestDotfilesHiddenUnlessRequested(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".hidden"), "x")
	mustWrite(t, filepath.Join(dir, "visible"), "x")
	e := newTestEngine(dir)

	got := e.Complete("someprog ", 9)
	if len(got) != 1 || got[0].Text != "visible" {
		t.Fatalf("Complete(someprog ) = %v, want [visible]", texts(got))
	}

	got = e.Complete("someprog .h", 11)
	if len(got) != 1 || got[0].Text != ".hidden" {
		t.Fatalf("Complete(someprog .h) = %v, want [.hidden]", texts(got))
	}
}

func TestPathCompletionKeepsDirPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "src", "main.go"), "x")
	e := newTestEngine(dir)

	got := e.Complete("someprog src/ma", 15)
	if len(got) != 1 || got[0].Text != "src/main.go" {
		t.Fatalf("Complete(someprog src/ma) = %v, want [src/main.go]", texts(got))
	}
}

// TestCommandPositionPathCompletion verifies that a command word containing a
// slash is completed as a file path (./install.sh, ./sub/) rather than as a
// $PATH command name, and that directories keep their trailing slash so Tab can
// recurse into them.
func TestCommandPositionPathCompletion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "install.sh"), "x")
	mustWrite(t, filepath.Join(dir, "README.md"), "x")
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "scripts", "build.sh"), "x")
	// No commands are registered, so a path word can only complete via the
	// file path fallback, not as a command name.
	e := newTestEngine(dir)

	// ./inst -> ./install.sh, even though install.sh is not on $PATH.
	if got := e.Complete("./inst", 6); len(got) != 1 || got[0].Text != "./install.sh" {
		t.Fatalf("Complete(./inst) = %v, want [./install.sh]", texts(got))
	}

	// ./ lists everything in cwd at the command position.
	got := e.Complete("./", 2)
	if !contains(got, "./install.sh") || !contains(got, "./README.md") || !contains(got, "./scripts/") {
		t.Fatalf("Complete(./) = %v, want install.sh, README.md and scripts/", texts(got))
	}

	// Recurse one level down a directory in the command position.
	if got := e.Complete("./scripts/bu", 12); len(got) != 1 || got[0].Text != "./scripts/build.sh" {
		t.Fatalf("Complete(./scripts/bu) = %v, want [./scripts/build.sh]", texts(got))
	}

	// A bare word with no slash is still a command name, not a file.
	if got := e.Complete("inst", 4); len(got) != 0 {
		t.Fatalf("Complete(inst) = %v, want no command-name matches", texts(got))
	}
}

// TestWrapperCompletion verifies that completion looks through command wrappers
// (sudo, env, ...) to the command they run.
func TestWrapperCompletion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "install.sh"), "x")
	e := newTestEngine(dir, "git", "grep")

	// `sudo gi` completes the wrapped command name.
	if got := e.Complete("sudo gi", 7); len(got) != 1 || got[0].Text != "git" {
		t.Fatalf("Complete(sudo gi) = %v, want [git]", texts(got))
	}

	// `sudo git ch` completes git's subcommands, not files.
	got := e.Complete("sudo git ch", 11)
	if len(got) != 2 || got[0].Text != "checkout" || got[1].Text != "cherry-pick" {
		t.Fatalf("Complete(sudo git ch) = %v, want [checkout cherry-pick]", texts(got))
	}

	// `sudo ./inst` still path-completes through the wrapper.
	if got := e.Complete("sudo ./inst", 11); len(got) != 1 || got[0].Text != "./install.sh" {
		t.Fatalf("Complete(sudo ./inst) = %v, want [./install.sh]", texts(got))
	}

	// Options that take an argument are skipped (sudo -u root git ch).
	got = e.Complete("sudo -u root git ch", 19)
	if len(got) != 2 || got[0].Text != "checkout" || got[1].Text != "cherry-pick" {
		t.Fatalf("Complete(sudo -u root git ch) = %v, want [checkout cherry-pick]", texts(got))
	}

	// Wrappers nest: `sudo nice git ch` still reaches git's subcommands.
	got = e.Complete("sudo nice git ch", 16)
	if len(got) != 2 || got[0].Text != "checkout" || got[1].Text != "cherry-pick" {
		t.Fatalf("Complete(sudo nice git ch) = %v, want [checkout cherry-pick]", texts(got))
	}
}

func TestUnwrapCommand(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"git", "status"}, []string{"git", "status"}},
		{[]string{"sudo", "git"}, []string{"git"}},
		{[]string{"sudo"}, nil},
		{[]string{"sudo", "-u", "root", "git"}, []string{"git"}},
		{[]string{"sudo", "nice", "-n", "5", "git"}, []string{"git"}},
		{[]string{"/usr/bin/sudo", "git"}, []string{"git"}},
		{[]string{"sudo", "--", "git"}, []string{"git"}},
	}
	for _, c := range cases {
		got := unwrapCommand(append([]string(nil), c.in...))
		if len(got) != len(c.want) {
			t.Errorf("unwrapCommand(%v) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("unwrapCommand(%v) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

// TestBareArgShowsFlags verifies that a command with no spec offers its scraped
// flags (with descriptions) at the bare argument position, alongside files, so
// a plain Tab reveals the tool's options. The man cache is seeded directly to
// avoid shelling out to man in the test.
func TestBareArgShowsFlags(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "data.txt"), "x")
	e := newTestEngine(dir)
	e.manCache["mytool"] = []Candidate{
		{Text: "-v", Desc: "Verbose output"},
		{Text: "--help", Desc: "Show help"},
	}

	// Bare Tab: flags and files together.
	got := e.Complete("mytool ", 7)
	if !contains(got, "-v") || !contains(got, "--help") || !contains(got, "data.txt") {
		t.Fatalf("Complete(mytool ) = %v, want flags and files", texts(got))
	}
	// Descriptions are carried through.
	for _, c := range got {
		if c.Text == "--help" && c.Desc != "Show help" {
			t.Errorf("--help desc = %q, want %q", c.Desc, "Show help")
		}
	}

	// A leading '-' still narrows to flags only.
	got = e.Complete("mytool --", 9)
	if !contains(got, "--help") || contains(got, "data.txt") {
		t.Fatalf("Complete(mytool --) = %v, want only flags", texts(got))
	}

	// A file-like prefix matches files, not flags (flags start with '-').
	got = e.Complete("mytool da", 9)
	if len(got) != 1 || got[0].Text != "data.txt" {
		t.Fatalf("Complete(mytool da) = %v, want [data.txt]", texts(got))
	}
}

// TestTldrPages checks that the tldr page generator cleans its output: no ANSI
// escapes, no "Pages for" headers, and no multi-word lines. Skipped when tldr
// is not installed.
func TestTldrPages(t *testing.T) {
	if _, err := exec.LookPath("tldr"); err != nil {
		t.Skip("tldr not installed")
	}
	for _, c := range tldrPages("") {
		if c.Text == "" || strings.HasPrefix(c.Text, "Pages for") ||
			strings.ContainsAny(c.Text, " \t") || strings.Contains(c.Text, "\x1b") {
			t.Errorf("unclean page candidate %q", c.Text)
		}
	}
}

func TestUserSpecFile(t *testing.T) {
	specDir := t.TempDir()
	mustWrite(t, filepath.Join(specDir, "mytool.json"), `{
		"flags": [{"text": "--verbose", "desc": "Verbose output"}],
		"subcommands": [
			{"name": "serve", "desc": "Start the server",
			 "flags": [{"text": "--port", "desc": "Port to listen on"}],
			 "args": {"static": [{"text": "dev"}, {"text": "prod"}], "noFiles": true}}
		]
	}`)

	e := newTestEngine(t.TempDir())
	e.specDir = specDir

	got := e.Complete("mytool ", 7)
	if len(got) != 1 || got[0].Text != "serve" || got[0].Desc != "Start the server" {
		t.Fatalf("Complete(mytool ) = %v, want [serve]", got)
	}

	got = e.Complete("mytool serve ", 13)
	if len(got) != 2 || got[0].Text != "dev" || got[1].Text != "prod" {
		t.Fatalf("Complete(mytool serve ) = %v, want [dev prod]", texts(got))
	}

	got = e.Complete("mytool serve --", 15)
	if !contains(got, "--port") || !contains(got, "--verbose") {
		t.Fatalf("Complete(mytool serve --) = %v, want --port and --verbose", texts(got))
	}
}

func TestMakefileTargets(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Makefile"), `BINARY := app

.PHONY: all build test

all: build

build:
	go build -o $(BINARY) .

test:
	go test ./...

$(BINARY): build
`)

	got := makefileTargets(dir)
	want := map[string]bool{"all": true, "build": true, "test": true}
	if len(got) != len(want) {
		t.Fatalf("makefileTargets = %v, want keys %v", texts(got), want)
	}
	for _, c := range got {
		if !want[c.Text] {
			t.Errorf("unexpected target %q", c.Text)
		}
	}
}

func TestNpmScripts(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "package.json"), `{
		"name": "x",
		"scripts": {"build": "tsc", "dev": "vite"}
	}`)

	got := npmScripts(dir)
	if len(got) != 2 || got[0].Text != "build" || got[1].Text != "dev" {
		t.Fatalf("npmScripts = %v, want [build dev]", texts(got))
	}
	if got[0].Desc != "tsc" {
		t.Errorf("build desc = %q, want the script command", got[0].Desc)
	}
}

func TestParseHelpFlags(t *testing.T) {
	help := `Usage: mytool [options]

Options:
  -f, --force          Force the operation
      --file=PATH      Read from PATH
  -v                   Verbose output
  --dry-run            Do not write anything
  -o <file>            Output file
  not-a-flag           ignored
`
	got := parseHelpFlags(help)
	wantDesc := map[string]string{
		"-f": "Force the operation", "--force": "Force the operation",
		"--file": "Read from PATH", "-v": "Verbose output",
		"--dry-run": "Do not write anything", "-o": "Output file",
	}
	if len(got) != len(wantDesc) {
		t.Fatalf("parseHelpFlags = %v, want %d flags", got, len(wantDesc))
	}
	for _, c := range got {
		if want, ok := wantDesc[c.Text]; !ok {
			t.Errorf("unexpected flag %q", c.Text)
		} else if c.Desc != want {
			t.Errorf("flag %s desc = %q, want %q", c.Text, c.Desc, want)
		}
	}
}

func TestHelpCommandCompletesBuiltins(t *testing.T) {
	e := newTestEngine(t.TempDir())

	got := e.Complete("help pr", 7)
	if len(got) != 1 || got[0].Text != "print" || got[0].Desc != "Print text." {
		t.Fatalf("Complete(help pr) = %v, want [print] with summary", got)
	}
}

// TestFuzzyFallback verifies the light fuzzy fallback: an exact prefix match is
// always preferred, but when the typed word matches nothing as a prefix it
// falls back to a case-insensitive subsequence match on file/dir names.
func TestFuzzyFallback(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "alpha.txt"), "x")
	mustWrite(t, filepath.Join(dir, "component.go"), "x")
	if err := os.Mkdir(filepath.Join(dir, "Downloads"), 0755); err != nil {
		t.Fatal(err)
	}
	e := newTestEngine(dir)

	// Exact prefix still wins and the fuzzy path does not engage.
	if got := e.Complete("someprog al", 11); len(got) != 1 || got[0].Text != "alpha.txt" {
		t.Fatalf("Complete(someprog al) = %v, want [alpha.txt] (exact prefix)", texts(got))
	}

	// No prefix match: 'dwn' subsequence-matches Downloads/.
	if got := e.Complete("someprog dwn", 12); !contains(got, "Downloads/") {
		t.Errorf("Complete(someprog dwn) = %v, want it to fuzzy-match Downloads/", texts(got))
	}

	// No prefix match: 'cmp' subsequence-matches component.go.
	if got := e.Complete("someprog cmp", 12); !contains(got, "component.go") {
		t.Errorf("Complete(someprog cmp) = %v, want it to fuzzy-match component.go", texts(got))
	}

	// A pattern that is not a subsequence of anything matches nothing.
	if got := e.Complete("someprog zzz", 12); len(got) != 0 {
		t.Errorf("Complete(someprog zzz) = %v, want no matches", texts(got))
	}
}

func TestCaseInsensitivePrefixBeforeFuzzy(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Documents"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "deep_other_copy"), "x")
	e := newTestEngine(dir)

	got := e.Complete("someprog doc", len([]rune("someprog doc")))
	if len(got) != 1 || got[0].Text != "Documents/" {
		t.Fatalf("Complete(someprog doc) = %v, want case-insensitive prefix Documents/ only", texts(got))
	}
}

func TestCompleteUsesRuneCursorOffsets(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "résumé.txt"), "x")
	e := newTestEngine(dir)

	line := "print λ ré"
	got := e.Complete(line, len([]rune(line)))
	if len(got) != 1 || got[0].Text != "résumé.txt" {
		t.Fatalf("Complete(%q) = %v, want résumé.txt", line, texts(got))
	}
}

func TestCompleteQuotedFilenameWithSpaces(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "My Documents.txt"), "x")
	mustWrite(t, filepath.Join(dir, "My Downloads.txt"), "x")
	e := newTestEngine(dir)

	line := "print 'My Do"
	got := e.Complete(line, len([]rune(line)))
	if !contains(got, "My Documents.txt") || !contains(got, "My Downloads.txt") {
		t.Fatalf("Complete(%q) = %v, want both spaced filenames", line, texts(got))
	}
}

// TestFuzzyScore checks the subsequence matcher and its ordering preferences.
func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("Downloads", "dwn"); !ok {
		t.Error("'dwn' should be a subsequence of 'Downloads'")
	}
	if _, ok := fuzzyScore("Downloads", "dnw"); ok {
		t.Error("'dnw' is out of order and must not match 'Downloads'")
	}
	// An earlier/contiguous match scores higher than a scattered one.
	anchored, _ := fuzzyScore("config", "con")
	scattered, _ := fuzzyScore("reconfigure", "con")
	if anchored <= scattered {
		t.Errorf("anchored match score %d should beat scattered %d", anchored, scattered)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFileTypeStyles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "plain.txt"), "x")
	if err := os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "plain.txt"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	e := newTestEngine(dir)

	styles := make(map[string]string)
	for _, c := range e.Complete("someprog ", 9) {
		styles[c.Text] = c.Style
	}
	if styles["plain.txt"] != "" {
		t.Errorf("plain file style = %q, want none", styles["plain.txt"])
	}
	if styles["subdir/"] != ansi.Bold+ansi.Blue {
		t.Errorf("directory style = %q, want bold blue", styles["subdir/"])
	}
	if styles["tool"] != ansi.Green {
		t.Errorf("executable style = %q, want green", styles["tool"])
	}
	if styles["link"] != ansi.Cyan {
		t.Errorf("symlink style = %q, want cyan", styles["link"])
	}
}
