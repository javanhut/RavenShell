package completion

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func subNames(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Text
	}
	sort.Strings(out)
	return out
}

func TestParseSubcommandsCobra(t *testing.T) {
	// kubectl/gh/docker-style help with multiple command groups and a Flags
	// section that must not be mistaken for commands.
	help := `Usage:
  kubectl [flags]

Basic Commands (Beginner):
  create        Create a resource from a file or from stdin
  expose        Take a replication controller and expose it

Available Commands:
  apply         Apply a configuration to a resource by file name
  get           Display one or many resources

Flags:
  -h, --help    help for kubectl
  -v, --verbose verbose output
`
	got := parseSubcommands(help)
	want := []string{"apply", "create", "expose", "get"}
	if names := subNames(got); !equalStrings(names, want) {
		t.Errorf("parseSubcommands = %v, want %v", names, want)
	}
	if d, _ := descFor(got, "apply"); d != "Apply a configuration to a resource by file name" {
		t.Errorf("apply desc = %q", d)
	}
	// Flags must not leak in as subcommands.
	if _, ok := descFor(got, "--help"); ok {
		t.Error("flags must not be parsed as subcommands")
	}
}

func TestParseSubcommandsGoStyle(t *testing.T) {
	// `go help`-style: prose heading ending in a colon, tab-indented entries.
	help := "The commands are:\n\n\tbuild\tcompile packages and dependencies\n\ttest\ttest packages\n\tvet\treport likely mistakes\n\nUse \"go help <command>\" for more.\n"
	got := parseSubcommands(help)
	if names := subNames(got); !equalStrings(names, []string{"build", "test", "vet"}) {
		t.Errorf("parseSubcommands = %v", names)
	}
}

func TestParseSubcommandsCommaList(t *testing.T) {
	// npm-style: a comma-separated list with no descriptions.
	help := `All commands:

    access, adduser, audit, bugs, cache
`
	got := parseSubcommands(help)
	if names := subNames(got); !equalStrings(names, []string{"access", "adduser", "audit", "bugs", "cache"}) {
		t.Errorf("parseSubcommands = %v", names)
	}
}

func TestParseSubcommandsRejectsProseAndEmpty(t *testing.T) {
	// A commands section with a prose line (no description, has spaces, no
	// commas) must be skipped; a real row beside it must still be found.
	help := `Commands:
    this is just a sentence with many words
    realcmd   does the real thing
`
	got := parseSubcommands(help)
	if names := subNames(got); !equalStrings(names, []string{"realcmd"}) {
		t.Errorf("parseSubcommands = %v, want [realcmd]", names)
	}

	// Help with no commands section yields nothing.
	none := `Usage: tool [options]

Options:
  -f, --force   force it
`
	if got := parseSubcommands(none); len(got) != 0 {
		t.Errorf("expected no subcommands, got %v", subNames(got))
	}
}

func TestIsCommandsHeading(t *testing.T) {
	yes := []string{"Commands:", "Available Commands:", "Management Commands:", "The commands are:", "SUBCOMMANDS"}
	no := []string{"Flags:", "Options:", "Examples:", "Usage:", "These are guides"}
	for _, s := range yes {
		if !isCommandsHeading(s) {
			t.Errorf("isCommandsHeading(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isCommandsHeading(s) {
			t.Errorf("isCommandsHeading(%q) = true, want false", s)
		}
	}
}

// TestHelpSubcommandsEndToEnd drives the full lazy subcommand path through
// Complete() against a fake tool placed on PATH, so it needs no installed CLI.
func TestHelpSubcommandsEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture not portable to Windows")
	}
	binDir := t.TempDir()
	cacheDir := t.TempDir()

	script := "#!/bin/sh\n" +
		"echo 'Usage: mytool <command>'\n" +
		"echo ''\n" +
		"echo 'Available Commands:'\n" +
		"echo '  alpha   Do the alpha thing'\n" +
		"echo '  beta    Do the beta thing'\n" +
		"echo ''\n" +
		"echo 'Flags:'\n" +
		"echo '  -h, --help   help'\n"
	tool := filepath.Join(binDir, "mytool")
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	e := New(func() string { return binDir }, func() []string { return nil }, nil)
	e.specDir = ""
	e.cacheDir = cacheDir

	got := e.Complete("mytool ", len("mytool "))
	if names := subNames(got); !equalStrings(names, []string{"alpha", "beta"}) {
		t.Fatalf("Complete subcommands = %v, want [alpha beta]", names)
	}
	if d, _ := descFor(got, "alpha"); d != "Do the alpha thing" {
		t.Errorf("alpha desc = %q", d)
	}

	// Completing a prefix narrows to the matching subcommand.
	if names := subNames(e.Complete("mytool al", len("mytool al"))); !equalStrings(names, []string{"alpha"}) {
		t.Errorf("prefix complete = %v, want [alpha]", names)
	}

	// The scrape must have been cached to disk.
	if readHelpCache(filepath.Join(cacheDir, "help", "mytool.json")) == nil {
		t.Error("expected mytool help completions to be cached")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
