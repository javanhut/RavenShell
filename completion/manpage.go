package completion

// Man-page completion is RavenShell's equivalent of fish's
// fish_update_completions: it renders a command's man page, scrapes the option
// flags and their descriptions out of it, and caches the result so any command
// with a man page gains flag completion automatically — no hand-written spec
// required.
//
// Results are cached to disk under DefaultCacheDir() (one JSON file per
// command) and validated against the man page's modification time, so a cache
// entry is reused until the underlying man page changes. `raven-completions
// update` pre-warms the cache for every installed man page; lazy completion
// fills it in on demand for whatever the user actually tabs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// manTimeout bounds how long rendering a single man page may take. It is more
// generous than generatorTimeout because man/groff formatting a cold page can
// be slow on first run; the result is cached so the cost is paid once.
const manTimeout = 3 * time.Second

// safeCmdName guards which command names we are willing to hand to `man` and to
// use as a cache filename: it must look like a command, not a flag or a path.
var safeCmdName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]*$`)

// ansiSGR matches the SGR (and similar) escape sequences modern groff emits
// when it is not in overstrike mode, so they can be stripped from rendered text.
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")

// compExt lists the compression suffixes man pages are commonly stored with.
var compExt = []string{".gz", ".bz2", ".xz", ".lzma", ".Z", ".br", ".zst"}

// DefaultCacheDir returns the directory man-page completions are cached in,
// honoring XDG_CACHE_HOME. It returns "" only when the home directory is
// unknown.
func DefaultCacheDir() string {
	if x := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); x != "" {
		return filepath.Join(x, "ravenshell", "completions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "ravenshell", "completions")
}

// manCacheFile is the on-disk format of one cached command's flags. Source and
// ModTime pin the entry to the man page it came from so it can be invalidated
// when that page changes.
type manCacheFile struct {
	Command string      `json:"command"`
	Source  string      `json:"source"`
	ModTime int64       `json:"mtime"`
	Flags   []Candidate `json:"flags"`
}

// manFlags returns the option flags scraped from cmd's man page, using the
// in-memory cache, then the on-disk cache, then a fresh render. A command with
// no man page yields nil (cached in memory for the session).
func (e *Engine) manFlags(cmd string) []Candidate {
	if !safeCmdName.MatchString(cmd) {
		return nil
	}

	e.mu.Lock()
	if c, ok := e.manCache[cmd]; ok {
		e.mu.Unlock()
		return c
	}
	cacheDir := e.cacheDir
	e.mu.Unlock()

	flags := loadOrBuildMan(cmd, cacheDir, e.currentDir())

	e.mu.Lock()
	e.manCache[cmd] = flags
	e.mu.Unlock()
	return flags
}

// loadOrBuildMan returns cmd's flags from a still-valid disk cache, or renders
// the man page afresh (and rewrites the cache) when the cache is missing or
// stale. cacheDir may be empty, in which case nothing is cached.
func loadOrBuildMan(cmd, cacheDir, dir string) []Candidate {
	if cacheDir != "" {
		if cf := readManCache(filepath.Join(cacheDir, cmd+".json")); cf != nil {
			if info, err := os.Stat(cf.Source); err == nil && info.ModTime().Unix() == cf.ModTime {
				return cf.Flags
			}
		}
	}
	flags, _ := buildAndCacheMan(cmd, dir, cacheDir)
	return flags
}

// buildAndCacheMan renders and parses cmd's man page and, when cacheDir is set,
// writes the result to disk. It reports the parsed flags and whether a man page
// existed at all (so callers can distinguish "no flags" from "no page").
func buildAndCacheMan(cmd, dir, cacheDir string) (flags []Candidate, existed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), manTimeout)
	defer cancel()

	src := manPagePath(ctx, cmd, dir)
	if src == "" {
		return nil, false
	}
	text, ok := renderManText(ctx, cmd, dir)
	if !ok {
		return nil, false
	}
	flags = parseManFlags(text)

	if cacheDir != "" {
		if info, err := os.Stat(src); err == nil {
			_ = writeManCache(filepath.Join(cacheDir, cmd+".json"), &manCacheFile{
				Command: cmd,
				Source:  src,
				ModTime: info.ModTime().Unix(),
				Flags:   flags,
			})
		}
	}
	return flags, true
}

// manPagePath returns the path of cmd's man page via `man -w`, or "" if there
// is none. The path lets the cache be invalidated by the page's mtime.
func manPagePath(ctx context.Context, cmd, dir string) string {
	c := exec.CommandContext(ctx, "man", "-w", cmd)
	c.Dir = dir
	c.Env = append(os.Environ(), "LC_ALL=C")
	out, err := c.Output()
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if i := strings.IndexByte(path, '\n'); i >= 0 {
		path = path[:i] // first page when several sections match
	}
	return path
}

// renderManText renders cmd's man page to plain text. It asks groff for
// overstrike output (GROFF_NO_SGR) and decodes that, and also strips any SGR
// escapes for renderers that emit them anyway.
func renderManText(ctx context.Context, cmd, dir string) (string, bool) {
	c := exec.CommandContext(ctx, "man", cmd)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"MANPAGER=cat", "PAGER=cat", "MANWIDTH=120", "GROFF_NO_SGR=1", "LC_ALL=C")
	out, err := c.Output()
	if err != nil && len(out) == 0 {
		return "", false
	}
	text := ansiSGR.ReplaceAllString(decodeOverstrike(out), "")
	return text, true
}

// decodeOverstrike collapses backspace overstrike sequences (the bold/underline
// encoding `X\bX` and `_\bX` that nroff produces) into the final characters,
// the same job `col -b` does.
func decodeOverstrike(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, r := range string(b) {
		if r == '\b' {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// parseManFlags extracts option flags and one-line descriptions from rendered
// man-page text. It handles both layouts seen in practice: nroff's two-line
// form (flag on its own line, description indented beneath) and mandoc's
// one-line form (flag and description on the same line, separated by spaces).
func parseManFlags(text string) []Candidate {
	lines := strings.Split(text, "\n")
	seen := make(map[string]bool)
	var out []Candidate

	for i := range lines {
		trimmed := strings.TrimLeft(lines[i], " \t")
		if trimmed == "" || trimmed[0] != '-' || flagToken.FindString(trimmed) == "" {
			continue
		}
		flagIndent := indentWidth(lines[i])

		// Separate the flag column from an inline description (two or more
		// spaces, or a tab, between them).
		parts := descSplit.Split(trimmed, 2)
		flags := parseFlagSpellings(parts[0])
		if len(flags) == 0 {
			continue
		}

		desc := ""
		if len(parts) == 2 {
			desc = parts[1]
		} else {
			// Two-line form: the description is the following lines indented
			// deeper than the flag, up to a blank line or the next flag.
			var cont []string
			for j := i + 1; j < len(lines); j++ {
				lt := strings.TrimLeft(lines[j], " \t")
				if lt == "" || indentWidth(lines[j]) <= flagIndent {
					break
				}
				cont = append(cont, lt)
			}
			desc = strings.Join(cont, " ")
		}
		desc = cleanDesc(desc)

		for _, f := range flags {
			if seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, Candidate{Text: f, Desc: desc})
		}
	}
	return out
}

// parseFlagSpellings pulls the distinct flag tokens out of a flag column such
// as "-f, --force" or "-T, --tabsize=COLS", dropping value placeholders.
func parseFlagSpellings(col string) []string {
	fields := strings.FieldsFunc(col, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '/' || r == '|' || r == '['
	})
	seen := make(map[string]bool)
	var flags []string
	for _, tok := range fields {
		f := flagToken.FindString(tok)
		if f == "" || f == "-" || f == "--" || seen[f] {
			continue
		}
		seen[f] = true
		flags = append(flags, f)
	}
	return flags
}

// indentWidth returns the visual indent of a line, counting a tab as 8 columns.
func indentWidth(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			n++
		case '\t':
			n += 8
		default:
			return n
		}
	}
	return n
}

// cleanDesc collapses whitespace and trims a description to a single, listing
// friendly line.
func cleanDesc(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 80
	if len(s) > max {
		s = strings.TrimSpace(s[:max]) + "…"
	}
	return s
}

// readManCache loads a cache file, returning nil if it is absent or malformed.
func readManCache(path string) *manCacheFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cf manCacheFile
	if json.Unmarshal(data, &cf) != nil {
		return nil
	}
	return &cf
}

// writeManCache writes a cache file, creating the cache directory as needed.
func writeManCache(path string, cf *manCacheFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Stats summarizes a GenerateAll run.
type Stats struct {
	Scanned         int // commands considered
	WithFlags       int // commands that yielded at least one flag
	Updated         int // commands (re)rendered this run (vs. cache hits)
	WithSubcommands int // commands that yielded subcommands (deep runs only)
	Elapsed         time.Duration
}

// GenerateAll renders every installed man page and caches its flags — fish's
// fish_update_completions. When deep is set it additionally runs `--help` on
// every section-1 command to harvest subcommands (this executes those programs,
// so it is opt-in). Already-fresh entries are skipped, so re-runs are cheap.
// Progress is written to w.
func GenerateAll(w io.Writer, deep bool) (Stats, error) {
	start := time.Now()

	cacheDir := DefaultCacheDir()
	if cacheDir == "" {
		return Stats{}, errors.New("cannot determine cache directory")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return Stats{}, err
	}

	names := manCommandNames()
	if len(names) == 0 {
		return Stats{}, errors.New("no man pages found (is 'man' installed?)")
	}

	stats := Stats{}
	var statsMu sync.Mutex

	// Phase 1: flags from man pages (all sections).
	runJobs(w, "flags", names, func(cmd string) {
		updated, hasFlags := generateOne(cmd, cacheDir)
		statsMu.Lock()
		stats.Scanned++
		if hasFlags {
			stats.WithFlags++
		}
		if updated {
			stats.Updated++
		}
		statsMu.Unlock()
	})

	// Phase 2: subcommands from --help (section-1 commands only, opt-in).
	if deep {
		fmt.Fprintln(w, "Probing --help for subcommands (this runs each program)...")
		runJobs(w, "subcommands", man1CommandNames(), func(cmd string) {
			hasSubs := generateHelpOne(cmd, cacheDir)
			statsMu.Lock()
			if hasSubs {
				stats.WithSubcommands++
			}
			statsMu.Unlock()
		})
	}

	stats.Elapsed = time.Since(start)
	return stats, nil
}

// runJobs runs do over names with a bounded worker pool, reporting progress to
// w under the given label. do runs concurrently and must guard any shared
// state it touches; runJobs only serializes its own progress counter.
func runJobs(w io.Writer, label string, names []string, do func(cmd string)) {
	total := len(names)
	if total == 0 {
		return
	}
	const workers = 8
	jobs := make(chan string)
	var progressMu sync.Mutex
	done := 0

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for cmd := range jobs {
				do(cmd)
				progressMu.Lock()
				done++
				if done%200 == 0 {
					fmt.Fprintf(w, "  %s %d/%d...\n", label, done, total)
				}
				progressMu.Unlock()
			}
		})
	}
	for _, n := range names {
		jobs <- n
	}
	close(jobs)
	wg.Wait()
}

// generateHelpOne scrapes (or refreshes) one command's --help subcommands,
// returning whether any were found.
func generateHelpOne(cmd, cacheDir string) bool {
	return len(loadOrScrapeHelp(cmd, cacheDir, ".").subs) > 0
}

// generateOne builds (or refreshes) the cache for one command, skipping the
// render when the existing cache entry is still valid.
func generateOne(cmd, cacheDir string) (updated, hasFlags bool) {
	path := filepath.Join(cacheDir, cmd+".json")
	if cf := readManCache(path); cf != nil {
		if info, err := os.Stat(cf.Source); err == nil && info.ModTime().Unix() == cf.ModTime {
			return false, len(cf.Flags) > 0
		}
	}
	flags, existed := buildAndCacheMan(cmd, ".", cacheDir)
	return existed, len(flags) > 0
}

// CachedCount returns how many commands currently have a cached completion.
func CachedCount() int {
	entries, err := os.ReadDir(DefaultCacheDir())
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// ClearCache deletes every cached completion and returns how many were removed.
func ClearCache() (int, error) {
	dir := DefaultCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n, nil
}

// manCommandNames returns the sorted, de-duplicated set of command names that
// have a man page in the user-command sections (1, 6, 8) of the man path.
func manCommandNames() []string {
	return manNamesInSections("man1", "man6", "man8")
}

// man1CommandNames returns just the section-1 (user command) names, the set
// worth probing with --help for subcommands.
func man1CommandNames() []string {
	return manNamesInSections("man1")
}

// manNamesInSections enumerates command names across the given man sections.
func manNamesInSections(sections ...string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, dir := range manDirs() {
		for _, sec := range sections {
			entries, err := os.ReadDir(filepath.Join(dir, sec))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if name := manName(entry.Name()); name != "" && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

// manName turns a man-page filename into its command name, stripping any
// compression suffix and the trailing section ("ls.1.gz" -> "ls"). It returns
// "" for names that don't look like a plain command.
func manName(file string) string {
	for _, ext := range compExt {
		if strings.HasSuffix(file, ext) {
			file = file[:len(file)-len(ext)]
			break
		}
	}
	dot := strings.LastIndexByte(file, '.')
	if dot <= 0 {
		return ""
	}
	name := file[:dot]
	if !safeCmdName.MatchString(name) {
		return ""
	}
	return name
}

// manDirs returns the directories on the man path, preferring `manpath`, then
// $MANPATH, then a few conventional locations.
func manDirs() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "manpath").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return filepath.SplitList(s)
		}
	}
	if mp := strings.TrimSpace(os.Getenv("MANPATH")); mp != "" {
		return filepath.SplitList(mp)
	}
	return []string{
		"/usr/share/man",
		"/usr/local/share/man",
		"/opt/homebrew/share/man",
	}
}
