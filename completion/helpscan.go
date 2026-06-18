package completion

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// For commands without a built-in or user spec, completions are scraped from
// the command's --help output: flags (the lightweight equivalent of fish's
// man-page parsing) and, just as importantly, subcommands — the "Commands:"
// sections printed by modern CLIs (kubectl, gh, docker, cargo, ...) that man
// pages almost never list. One --help run yields both. Results, including
// empty ones, are cached in memory and on disk (keyed by the executable's
// modification time) so each command is run at most once per installed version.

// helpResult holds the flags and subcommands parsed from a command's --help.
type helpResult struct {
	flags []Candidate
	subs  []Candidate
}

// helpCacheFile is the on-disk form of a scraped command, pinned to the
// executable it came from so it can be invalidated when the binary changes.
type helpCacheFile struct {
	Command     string      `json:"command"`
	Source      string      `json:"source"`
	ModTime     int64       `json:"mtime"`
	Flags       []Candidate `json:"flags"`
	Subcommands []Candidate `json:"subcommands"`
}

// fallbackFlags supplies flag candidates for a command without a built-in or
// user spec. It prefers the command's man page (fish-style, cached to disk) and
// falls back to scraping --help for commands that have no man page.
func (e *Engine) fallbackFlags(cmd string) []Candidate {
	if f := e.manFlags(cmd); len(f) > 0 {
		return f
	}
	return e.helpScrape(cmd).flags
}

// helpSubcommands returns the subcommands scraped from cmd's --help, or nil for
// shell built-ins (which have none) and commands not found on PATH.
func (e *Engine) helpSubcommands(cmd string) []Candidate {
	if e.summaries[cmd] != "" {
		return nil // a RavenShell built-in: no external subcommands
	}
	return e.helpScrape(cmd).subs
}

// helpScrape returns the parsed --help result for cmd, using the in-memory
// cache, then the on-disk cache, then a fresh run. Unsafe names yield an empty
// result without executing anything.
func (e *Engine) helpScrape(cmd string) *helpResult {
	if !safeCmdName.MatchString(cmd) {
		return &helpResult{}
	}

	e.mu.Lock()
	if r, ok := e.helpCache[cmd]; ok {
		e.mu.Unlock()
		return r
	}
	cacheDir := e.cacheDir
	e.mu.Unlock()

	r := loadOrScrapeHelp(cmd, cacheDir, e.currentDir())

	e.mu.Lock()
	e.helpCache[cmd] = r
	e.mu.Unlock()
	return r
}

// loadOrScrapeHelp returns cmd's --help result from a still-valid disk cache,
// or by running --help afresh (and rewriting the cache). A command not found on
// PATH yields an empty result and is not executed. cacheDir may be empty, in
// which case nothing is cached to disk.
func loadOrScrapeHelp(cmd, cacheDir, dir string) *helpResult {
	bin, err := exec.LookPath(cmd)
	if err != nil {
		return &helpResult{} // not an external command
	}
	src := bin
	if rp, err := filepath.EvalSymlinks(bin); err == nil {
		src = rp
	}
	var modTime int64
	if info, err := os.Stat(src); err == nil {
		modTime = info.ModTime().Unix()
	}

	path := ""
	if cacheDir != "" {
		path = filepath.Join(cacheDir, "help", cmd+".json")
		if cf := readHelpCache(path); cf != nil && cf.Source == src && cf.ModTime == modTime {
			return &helpResult{flags: cf.Flags, subs: cf.Subcommands}
		}
	}

	flags, subs := scrapeHelp(cmd, dir)
	if path != "" {
		_ = writeHelpCache(path, &helpCacheFile{
			Command: cmd, Source: src, ModTime: modTime,
			Flags: flags, Subcommands: subs,
		})
	}
	return &helpResult{flags: flags, subs: subs}
}

// scrapeHelp runs `cmd --help` (bounded by manTimeout) and parses both flags
// and subcommands from its combined output; many tools print usage to stderr.
func scrapeHelp(cmd, dir string) (flags, subs []Candidate) {
	ctx, cancel := context.WithTimeout(context.Background(), manTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, "--help")
	c.Dir = dir
	out, _ := c.CombinedOutput()
	text := string(out)
	return parseHelpFlags(text), parseSubcommands(text)
}

// flagToken matches a flag at the start of a string: -x or --long-name.
var flagToken = regexp.MustCompile(`^(--?[A-Za-z0-9?][A-Za-z0-9_-]*)`)

// descSplit separates the flag column from the description column (two or
// more spaces, or a tab).
var descSplit = regexp.MustCompile(`\t|\s{2,}`)

// parseHelpFlags extracts option candidates from help text. It recognizes the
// conventional layout: lines whose first non-space token is a flag, optionally
// followed by aliases ("-f, --force") and a description separated by two or
// more spaces.
func parseHelpFlags(help string) []Candidate {
	seen := make(map[string]bool)
	var out []Candidate
	for line := range strings.SplitSeq(help, "\n") {
		s := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(s, "-") {
			continue
		}

		parts := descSplit.Split(s, 2)
		desc := ""
		if len(parts) == 2 {
			desc = strings.TrimSpace(parts[1])
		}

		// The flag column may hold several comma-separated spellings, each
		// possibly with a value placeholder ("--file=PATH", "-o <file>").
		for tok := range strings.SplitSeq(parts[0], ",") {
			tok = strings.TrimSpace(tok)
			flag := flagToken.FindString(tok)
			if flag == "" || flag == "-" || flag == "--" || seen[flag] {
				continue
			}
			seen[flag] = true
			out = append(out, Candidate{Text: flag, Desc: desc})
		}
	}
	return out
}

// subName matches a plausible subcommand: a lowercase-led word.
var subName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// maxSubcommands caps how many subcommands are taken from one command, a guard
// against runaway parsing of unusually-formatted help.
const maxSubcommands = 500

// parseSubcommands extracts subcommand candidates from help text. It looks for
// a heading line that mentions "command" (e.g. "Available Commands:",
// "Management Commands:", "The commands are:") and reads the indented entries
// beneath it as "name  description" rows, also handling comma-separated lists
// (npm) and entries with no description.
func parseSubcommands(help string) []Candidate {
	lines := strings.Split(help, "\n")
	seen := make(map[string]bool)
	var out []Candidate
	inSection := false

	for _, raw := range lines {
		lt := strings.TrimLeft(raw, " \t")
		if lt == "" {
			continue // a blank line does not end a section
		}
		if indentWidth(raw) == 0 {
			// Column-0 lines are headings or prose: a commands heading opens a
			// section, anything else (Flags:, Examples:, ...) closes it.
			inSection = isCommandsHeading(lt)
			continue
		}
		if !inSection || lt[0] == '-' {
			continue
		}

		parts := descSplit.Split(strings.TrimRight(lt, " \t"), 2)
		var names []string
		desc := ""
		switch {
		case len(parts) == 2:
			names = splitNames(parts[0])
			desc = cleanDesc(parts[1])
		case strings.Contains(parts[0], ","):
			names = splitNames(parts[0]) // comma list with no descriptions
		case !strings.ContainsAny(parts[0], " \t"):
			names = []string{parts[0]} // a bare subcommand name
		default:
			continue // prose, not a subcommand row
		}

		for _, name := range names {
			if !subName.MatchString(name) || seen[name] || isSubStopword(name) {
				continue
			}
			seen[name] = true
			out = append(out, Candidate{Text: name, Desc: desc})
			if len(out) >= maxSubcommands {
				return out
			}
		}
	}
	return out
}

// isCommandsHeading reports whether a help line introduces a list of
// subcommands: it mentions "command" and reads as a heading (ends with a colon
// or is written in all caps, like a man-page SUBCOMMANDS section).
func isCommandsHeading(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(low, "command") {
		return false
	}
	return strings.HasSuffix(low, ":") || s == strings.ToUpper(s)
}

// isSubStopword filters words that appear in command sections but are never
// real subcommands.
func isSubStopword(name string) bool {
	switch name {
	case "the", "commands", "command", "options", "usage", "aliases", "flags":
		return true
	}
	return false
}

// splitNames splits a name column into individual tokens on commas and spaces.
func splitNames(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '|' || r == '/'
	})
}

// readHelpCache loads a help cache file, returning nil if absent or malformed.
func readHelpCache(path string) *helpCacheFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cf helpCacheFile
	if json.Unmarshal(data, &cf) != nil {
		return nil
	}
	return &cf
}

// writeHelpCache writes a help cache file, creating the directory as needed.
func writeHelpCache(path string, cf *helpCacheFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
