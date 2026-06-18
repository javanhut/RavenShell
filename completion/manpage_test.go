package completion

import (
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRunJobsCoversEveryItemOnce(t *testing.T) {
	names := make([]string, 50)
	for i := range names {
		names[i] = "cmd" + strings.Repeat("x", i)
	}

	var mu sync.Mutex
	counts := make(map[string]int)
	runJobs(io.Discard, "test", names, func(cmd string) {
		mu.Lock()
		counts[cmd]++
		mu.Unlock()
	})

	if len(counts) != len(names) {
		t.Fatalf("ran %d distinct items, want %d", len(counts), len(names))
	}
	for _, n := range names {
		if counts[n] != 1 {
			t.Errorf("%s ran %d times, want 1", n, counts[n])
		}
	}
}

// descFor returns the description recorded for flag text in cands, and whether
// the flag was present at all.
func descFor(cands []Candidate, text string) (string, bool) {
	for _, c := range cands {
		if c.Text == text {
			return c.Desc, true
		}
	}
	return "", false
}

func TestParseManFlagsTwoLine(t *testing.T) {
	// nroff/groff layout: the flag sits on its own line and the description is
	// indented beneath it.
	text := `NAME
       ls - list directory contents

OPTIONS
       -a, --all
              do not ignore entries starting with .

       -A, --almost-all
              do not list implied . and ..

       --block-size=SIZE
              with -l, scale sizes by SIZE when printing them

SEE ALSO
       dir(1)
`
	got := parseManFlags(text)

	for _, flag := range []string{"-a", "--all", "-A", "--almost-all", "--block-size"} {
		if _, ok := descFor(got, flag); !ok {
			t.Errorf("expected flag %q to be parsed; got %+v", flag, got)
		}
	}
	if d, _ := descFor(got, "-a"); d != "do not ignore entries starting with ." {
		t.Errorf("-a description = %q", d)
	}
	if d, _ := descFor(got, "--block-size"); d != "with -l, scale sizes by SIZE when printing them" {
		t.Errorf("--block-size description = %q", d)
	}
	// The placeholder value must be stripped from the flag itself.
	if _, ok := descFor(got, "--block-size=SIZE"); ok {
		t.Errorf("flag should not include its value placeholder")
	}
}

func TestParseManFlagsInline(t *testing.T) {
	// mandoc layout: the flag and the start of its description share a line,
	// separated by a run of spaces.
	text := `DESCRIPTION
     The following options are available:

     -A      Include directory entries whose names begin with a dot (.)
             except for . and ...

     -1      Force output to be one entry per line.

     -o, --output=FILE  Write to FILE instead of stdout.
`
	got := parseManFlags(text)

	if d, ok := descFor(got, "-A"); !ok || d != "Include directory entries whose names begin with a dot (.)" {
		t.Errorf("-A = %q (ok=%v)", d, ok)
	}
	if d, ok := descFor(got, "-1"); !ok || d != "Force output to be one entry per line." {
		t.Errorf("-1 = %q (ok=%v)", d, ok)
	}
	if _, ok := descFor(got, "-o"); !ok {
		t.Errorf("expected -o to be parsed; got %+v", got)
	}
	if _, ok := descFor(got, "--output"); !ok {
		t.Errorf("expected --output to be parsed; got %+v", got)
	}
}

func TestParseManFlagsIgnoresNonFlags(t *testing.T) {
	// Synopsis brackets, bullet dashes, and prose must not be mistaken for flags.
	text := `SYNOPSIS
       ls [OPTION]... [FILE]...

DESCRIPTION
       List information about the FILEs.

       - a plain bullet, not a flag
       really a paragraph that mentions -x somewhere in the middle
`
	got := parseManFlags(text)
	if len(got) != 0 {
		t.Errorf("expected no flags, got %+v", got)
	}
}

func TestParseFlagSpellings(t *testing.T) {
	cases := map[string][]string{
		"-f, --force":        {"-f", "--force"},
		"-T, --tabsize=COLS": {"-T", "--tabsize"},
		"-o <file>":          {"-o"},
		"--color[=WHEN]":     {"--color"},
		"-a/--all":           {"-a", "--all"},
		"-":                  nil,
		"--":                 nil,
		"not a flag at all":  nil,
		"-v, -v, --verbose":  {"-v", "--verbose"},
	}
	for in, want := range cases {
		if got := parseFlagSpellings(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseFlagSpellings(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDecodeOverstrike(t *testing.T) {
	// "ab" bold (each char is X\bX), then "c" underlined (_\bc).
	raw := []byte("a\bab\bb_\bc")
	if got := decodeOverstrike(raw); got != "abc" {
		t.Errorf("decodeOverstrike = %q, want %q", got, "abc")
	}
	if got := decodeOverstrike([]byte("plain")); got != "plain" {
		t.Errorf("decodeOverstrike(plain) = %q", got)
	}
}

func TestCleanDesc(t *testing.T) {
	if got := cleanDesc("  multiple   spaces\tand\ttabs  "); got != "multiple spaces and tabs" {
		t.Errorf("cleanDesc = %q", got)
	}
	long := cleanDesc(string(make([]byte, 0)) + "word " + repeat("verylongword ", 20))
	if len(long) == 0 || []rune(long)[len([]rune(long))-1] != '…' {
		t.Errorf("expected long description to be truncated with an ellipsis, got %q", long)
	}
}

func repeat(s string, n int) string {
	var out strings.Builder
	for range n {
		out.WriteString(s)
	}
	return out.String()
}

func TestManName(t *testing.T) {
	cases := map[string]string{
		"ls.1.gz":          "ls",
		"tar.5":            "tar",
		"git-commit.1":     "git-commit",
		"foo.bar.1.bz2":    "foo.bar",
		"noSection":        "",
		".hidden.1":        "",
		"docker-run.1.zst": "docker-run", // compression suffix then section stripped
	}
	for in, want := range cases {
		if got := manName(in); got != want {
			t.Errorf("manName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestManCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.json")
	want := &manCacheFile{
		Command: "demo",
		Source:  "/usr/share/man/man1/demo.1.gz",
		ModTime: 1700000000,
		Flags:   []Candidate{{Text: "-x", Desc: "do x"}, {Text: "--yes"}},
	}
	if err := writeManCache(path, want); err != nil {
		t.Fatalf("writeManCache: %v", err)
	}
	got := readManCache(path)
	if got == nil {
		t.Fatal("readManCache returned nil")
	}
	if got.Command != want.Command || got.Source != want.Source || got.ModTime != want.ModTime {
		t.Errorf("metadata mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Flags, want.Flags) {
		t.Errorf("flags = %+v, want %+v", got.Flags, want.Flags)
	}
	if readManCache(filepath.Join(dir, "missing.json")) != nil {
		t.Error("expected nil for a missing cache file")
	}
}

// TestManFlagsIntegration exercises the full path against the real `man`
// command. It is skipped where man (or the ls man page) is unavailable.
func TestManFlagsIntegration(t *testing.T) {
	if err := exec.Command("man", "-w", "ls").Run(); err != nil {
		t.Skip("man/ls man page unavailable; skipping integration test")
	}
	cacheDir := t.TempDir()

	e := &Engine{
		userSpecs: make(map[string]*Spec),
		helpCache: make(map[string]*helpResult),
		manCache:  make(map[string][]Candidate),
		cacheDir:  cacheDir,
	}

	flags := e.manFlags("ls")
	if len(flags) == 0 {
		t.Fatal("expected man-page flags for ls, got none")
	}
	if _, ok := descFor(flags, "-l"); !ok {
		t.Errorf("expected ls to offer -l; got %d flags", len(flags))
	}

	// The result must have been cached to disk.
	if readManCache(filepath.Join(cacheDir, "ls.json")) == nil {
		t.Error("expected ls completions to be written to the cache")
	}

	// A rejected (unsafe) command name must not run man or panic.
	if got := e.manFlags("../etc/passwd"); got != nil {
		t.Errorf("unsafe command name should yield nil, got %+v", got)
	}
}
