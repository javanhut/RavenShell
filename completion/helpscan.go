package completion

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
)

// Flag completion for commands without a spec falls back to scraping the
// command's --help output, the lightweight equivalent of fish's man-page
// parsing. Results (including empty ones) are cached for the session so each
// command is run at most once.

// scrapedHelpFlags returns flags parsed from `cmd --help`, caching the result.
func (e *Engine) scrapedHelpFlags(cmd string) []Candidate {
	e.mu.Lock()
	if cached, ok := e.helpCache[cmd]; ok {
		e.mu.Unlock()
		return cached
	}
	e.mu.Unlock()

	flags := e.runHelp(cmd)

	e.mu.Lock()
	e.helpCache[cmd] = flags
	e.mu.Unlock()
	return flags
}

// runHelp executes `cmd --help` (bounded by generatorTimeout) and parses
// flags out of the combined output; many tools print usage to stderr.
func (e *Engine) runHelp(cmd string) []Candidate {
	ctx, cancel := context.WithTimeout(context.Background(), generatorTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, "--help")
	c.Dir = e.currentDir()
	out, _ := c.CombinedOutput()
	return parseHelpFlags(string(out))
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
	for _, line := range strings.Split(help, "\n") {
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
		for _, tok := range strings.Split(parts[0], ",") {
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
