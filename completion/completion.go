// Package completion implements fish-style tab completion for RavenShell.
//
// Completions come from per-command specs that describe subcommands, flags
// (both with descriptions), and argument sources. Argument sources can be
// static lists, generator commands run at completion time (e.g. git branches),
// or Go functions (e.g. Makefile targets). Specs are looked up in three
// places, in order: the built-in registry, lazily-loaded user spec files in
// ~/.config/ravenshell/completions/<cmd>.json, and — for flag words only — a
// cached scrape of the command's --help output.
package completion

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"ravenshell/ansi"
	"sort"
	"strings"
	"sync"
	"time"
)

// Candidate is a single completion choice. Text replaces the word being
// completed; Desc is shown alongside it in the completion listing; Style is
// an optional ANSI code applied to Text in the listing (file-type colors).
type Candidate struct {
	Text  string `json:"text"`
	Desc  string `json:"desc,omitempty"`
	Style string `json:"style,omitempty"`
}

// ArgSpec describes where a command's (or subcommand's) positional argument
// candidates come from. All sources are merged; file completion is added
// unless NoFiles is set.
type ArgSpec struct {
	Static   []Candidate                  // fixed candidates
	Command  string                       // shell command run at completion time; one candidate per line, "text<TAB>desc"
	Generate func(cwd string) []Candidate // Go-native generator (e.g. Makefile targets)
	NoFiles  bool                         // suppress the file-path fallback
	DirsOnly bool                         // file fallback offers directories only
}

// SubSpec describes one subcommand of a command.
type SubSpec struct {
	Name  string
	Desc  string
	Flags []Candidate
	Args  *ArgSpec
}

// Spec describes how to complete one command.
type Spec struct {
	Flags       []Candidate // flags valid anywhere on the command line
	Subcommands []SubSpec
	Args        *ArgSpec // positional args when the command has no subcommands
}

// generatorTimeout bounds how long a completion-time generator command (or a
// --help scrape) may run before being abandoned.
const generatorTimeout = time.Second

// commandWrappers are programs that take another command as their argument and
// run it (sudo apt …, nice cmd …, nohup cmd …). Completion looks through them
// so the word after the wrapper completes as a command name and later words
// complete against the wrapped command's spec, exactly as if the wrapper were
// not there. ("env" is deliberately absent: in RavenShell it is a built-in that
// prints the environment, not the command-runner of the same name.)
var commandWrappers = map[string]bool{
	"sudo": true, "doas": true, "command": true, "exec": true,
	"nohup": true, "setsid": true, "nice": true,
	"stdbuf": true, "time": true,
}

// wrapperArgOpts lists, per wrapper, the options that consume the following
// token (e.g. `sudo -u root cmd`), so that token is skipped rather than
// mistaken for the wrapped command. Options written as --opt=val carry their
// value inline and are handled by the generic option skip.
var wrapperArgOpts = map[string]map[string]bool{
	"sudo":   {"-u": true, "-g": true, "-U": true, "-p": true, "-C": true, "-c": true, "-r": true, "-t": true, "-T": true, "-R": true, "--user": true, "--group": true, "--prompt": true},
	"doas":   {"-u": true, "-C": true},
	"nice":   {"-n": true},
	"stdbuf": {"-i": true, "-o": true, "-e": true},
}

// unwrapCommand strips leading command wrappers (and the options that belong to
// them) from words, returning the words of the command actually being run. It
// stops at the first real command token, so `sudo -u root git` yields `git`.
// When the wrapper is the last completed word (the command after it is still
// being typed), the result is empty and the caller completes a command name.
func unwrapCommand(words []string) []string {
	for len(words) > 0 && commandWrappers[filepath.Base(words[0])] {
		argOpts := wrapperArgOpts[filepath.Base(words[0])]
		rest := words[1:]
		for len(rest) > 0 {
			tok := rest[0]
			switch {
			case tok == "--":
				rest = rest[1:] // end of options; next token is the command
			case strings.HasPrefix(tok, "-") && len(tok) > 1:
				rest = rest[1:]
				if argOpts[tok] && len(rest) > 0 {
					rest = rest[1:] // option takes a separate argument
				}
				continue
			}
			break
		}
		words = rest
	}
	return words
}

// Engine resolves completions for a command line.
type Engine struct {
	cwd       func() string
	commands  func() []string   // all invokable command names
	summaries map[string]string // command name -> one-line description

	builtins map[string]*Spec

	mu        sync.Mutex
	userSpecs map[string]*Spec       // lazy-loaded user spec files; nil = known absent
	helpCache map[string]*helpResult // scraped --help flags+subcommands, per session
	manCache  map[string][]Candidate // man-page flags; nil entry = no man page this session
	specDir   string                 // user spec directory (overridable in tests)
	cacheDir  string                 // man-page / --help completion cache directory
}

// New creates a completion engine. cwd supplies the shell's current directory,
// commands the full set of invokable command names, and summaries optional
// one-line descriptions keyed by command name.
func New(cwd func() string, commands func() []string, summaries map[string]string) *Engine {
	e := &Engine{
		cwd:       cwd,
		commands:  commands,
		summaries: summaries,
		userSpecs: make(map[string]*Spec),
		helpCache: make(map[string]*helpResult),
		manCache:  make(map[string][]Candidate),
		cacheDir:  DefaultCacheDir(),
	}
	if home, err := os.UserHomeDir(); err == nil {
		e.specDir = filepath.Join(home, ".config", "ravenshell", "completions")
	}
	e.builtins = builtinSpecs(e)
	return e
}

// Complete returns the candidates for the word at pos in line.
func (e *Engine) Complete(line string, pos int) []Candidate {
	if pos > len(line) {
		pos = len(line)
	}
	before := line[:pos]
	words := strings.Fields(before)

	// The word being completed: the trailing token, unless the line ends in a
	// space (then a new, empty word is being started).
	cur := ""
	if len(words) > 0 && !strings.HasSuffix(before, " ") {
		cur = words[len(words)-1]
		words = words[:len(words)-1]
	}

	// Look through command wrappers (sudo, env, nice, ...) so completion
	// applies to the command they run, not to the wrapper.
	words = unwrapCommand(words)

	if len(words) == 0 {
		// Command position. A word containing a slash (./install.sh,
		// ../bin/tool, /usr/bin/ls, ~/bin/x) names a path rather than a $PATH
		// command, so complete it as a file path. Directory candidates keep
		// their trailing slash, so repeated Tab recurses one level at a time.
		if strings.Contains(cur, "/") {
			return finish(e.completePath(cur, false), cur)
		}
		return e.completeCommandNames(cur)
	}

	cmd := filepath.Base(words[0])
	args := words[1:]
	spec := e.specFor(cmd)

	// Locate the subcommand context: the first non-flag argument decides it.
	var sub *SubSpec
	subConsumed := false
	if spec != nil {
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			subConsumed = true
			for i := range spec.Subcommands {
				if spec.Subcommands[i].Name == a {
					sub = &spec.Subcommands[i]
				}
			}
			break
		}
	}

	// Flag completion.
	if strings.HasPrefix(cur, "-") {
		var flags []Candidate
		if spec != nil {
			flags = append(flags, spec.Flags...)
			if sub != nil {
				flags = append(flags, sub.Flags...)
			}
		}
		if len(flags) == 0 {
			flags = e.fallbackFlags(cmd)
		}
		return finish(flags, cur)
	}

	// Subcommand position: offer the subcommand names.
	if spec != nil && !subConsumed && len(spec.Subcommands) > 0 {
		out := make([]Candidate, 0, len(spec.Subcommands))
		for _, s := range spec.Subcommands {
			out = append(out, Candidate{Text: s.Name, Desc: s.Desc})
		}
		return finish(out, cur)
	}

	// Commands with no spec at all (kubectl, gh, terraform, ...) get their
	// subcommands scraped from --help. They are offered only at the first
	// non-flag position and only when something matches what's typed, so a path
	// argument still falls through to file completion below.
	if spec == nil && !subConsumed {
		if subs := e.helpSubcommands(cmd); len(subs) > 0 {
			if matched := finish(subs, cur); len(matched) > 0 {
				return matched
			}
		}
	}

	// Positional argument: spec-provided sources plus the file fallback.
	var argspec *ArgSpec
	if spec != nil {
		if sub != nil {
			argspec = sub.Args
		} else if !subConsumed {
			argspec = spec.Args
		}
	}

	var out []Candidate
	dirsOnly := false
	files := true
	if argspec != nil {
		out = append(out, argspec.Static...)
		if argspec.Command != "" {
			out = append(out, e.runGenerator(argspec.Command)...)
		}
		if argspec.Generate != nil {
			out = append(out, argspec.Generate(e.currentDir())...)
		}
		files = !argspec.NoFiles
		dirsOnly = argspec.DirsOnly
	}
	// For a command we have no spec for, also offer its flags (scraped from the
	// man page / --help) at the argument position, so a bare Tab reveals the
	// tool's options instead of only files. They are still narrowed normally
	// once a leading '-' is typed (handled by the flag branch above). Commands
	// with a spec are left alone: their subcommands/arguments are the point, and
	// flags would just be noise.
	if spec == nil {
		out = append(out, e.fallbackFlags(cmd)...)
	}
	if files {
		out = append(out, e.completePath(cur, dirsOnly)...)
	}
	return finish(out, cur)
}

// completeCommandNames returns command names matching prefix, with the
// one-line summary attached when one is known.
func (e *Engine) completeCommandNames(prefix string) []Candidate {
	var names []string
	if e.commands != nil {
		names = e.commands()
	}
	out := make([]Candidate, 0, len(names))
	for _, name := range names {
		out = append(out, Candidate{Text: name, Desc: e.summaries[name]})
	}
	return finish(out, prefix)
}

// specFor returns the completion spec for cmd: built-in registry first, then a
// user spec file. Returns nil if neither exists.
func (e *Engine) specFor(cmd string) *Spec {
	if s, ok := e.builtins[cmd]; ok {
		return s
	}
	return e.userSpec(cmd)
}

// currentDir returns the shell's current directory, or "." if unknown.
func (e *Engine) currentDir() string {
	if e.cwd != nil {
		if d := e.cwd(); d != "" {
			return d
		}
	}
	return "."
}

// runGenerator runs a generator command in the current directory and turns
// each output line into a candidate. A line may carry a description after a
// tab. Failures and timeouts yield no candidates.
func (e *Engine) runGenerator(cmdline string) []Candidate {
	ctx, cancel := context.WithTimeout(context.Background(), generatorTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdline)
	cmd.Dir = e.currentDir()
	outBytes, err := cmd.Output()
	if err != nil && len(outBytes) == 0 {
		return nil
	}

	var out []Candidate
	for line := range strings.SplitSeq(string(outBytes), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		text, desc, _ := strings.Cut(line, "\t")
		text = strings.TrimSpace(text)
		if text != "" {
			out = append(out, Candidate{Text: text, Desc: strings.TrimSpace(desc)})
		}
	}
	return out
}

// completePath returns file/directory candidates for the path prefix, rooted
// at the shell's current directory. Each candidate carries the full prefix
// (including any leading directories) so it can replace the word as-is.
func (e *Engine) completePath(prefix string, dirsOnly bool) []Candidate {
	searchDir := e.currentDir()
	dirPart := "" // part of the prefix kept in front of each candidate
	namePart := prefix

	if i := strings.LastIndex(prefix, "/"); i != -1 {
		dirPart = prefix[:i+1]
		namePart = prefix[i+1:]
		switch {
		case strings.HasPrefix(prefix, "~/"):
			if home, err := os.UserHomeDir(); err == nil {
				searchDir = filepath.Join(home, dirPart[2:])
			}
		case strings.HasPrefix(prefix, "/"):
			searchDir = dirPart
		default:
			searchDir = filepath.Join(searchDir, dirPart)
		}
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	var out []Candidate
	for _, entry := range entries {
		name := entry.Name()
		// Matching the typed text (prefix first, fuzzy fallback) is left to
		// finish, which sees the full word; here we only decide which entries
		// are eligible at all (dotfile and dirs-only visibility).
		//
		// Hide dotfiles unless the user is explicitly completing one.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(namePart, ".") {
			continue
		}
		isDir := entry.IsDir()
		isLink := entry.Type()&os.ModeSymlink != 0
		if isLink && !isDir {
			if info, err := os.Stat(filepath.Join(searchDir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		if dirsOnly && !isDir {
			continue
		}

		// Color by file type, matching the ls scheme: directories blue,
		// symlinks cyan, executables green.
		style := ""
		switch {
		case isLink:
			style = ansi.Cyan
		case isDir:
			style = ansi.Bold + ansi.Blue
		default:
			if info, err := entry.Info(); err == nil && info.Mode()&0111 != 0 {
				style = ansi.Green
			}
		}

		text := dirPart + name
		if isDir {
			text += "/"
		}
		out = append(out, Candidate{Text: text, Style: style})
	}
	return out
}

// finish matches candidates against the typed prefix, dedupes (keeping the
// first occurrence, so spec candidates win over file-fallback duplicates), and
// sorts the result.
//
// Matching is prefix-first: an exact (case-sensitive) prefix match is always
// preferred and preserves the precise behavior of tab-completion. Only when the
// prefix matches nothing does a light fuzzy fallback kick in, accepting any
// candidate whose text contains the typed characters as a case-insensitive
// subsequence (so "dwn" still finds "Downloads"). Fuzzy results are ordered by
// match quality.
func finish(cands []Candidate, prefix string) []Candidate {
	matched := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(c.Text, prefix) {
			matched = append(matched, c)
		}
	}

	fuzzy := len(matched) == 0 && prefix != ""
	scores := map[string]int{}
	if fuzzy {
		for _, c := range cands {
			if s, ok := fuzzyScore(c.Text, prefix); ok {
				matched = append(matched, c)
				if s > scores[c.Text] {
					scores[c.Text] = s
				}
			}
		}
	}

	seen := make(map[string]bool, len(matched))
	out := matched[:0]
	for _, c := range matched {
		if seen[c.Text] {
			continue
		}
		seen[c.Text] = true
		out = append(out, c)
	}

	if fuzzy {
		// Best fuzzy score first; ties broken alphabetically for stability.
		sort.SliceStable(out, func(i, j int) bool {
			if si, sj := scores[out[i].Text], scores[out[j].Text]; si != sj {
				return si > sj
			}
			return out[i].Text < out[j].Text
		})
	} else {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	}
	return out
}

// fuzzyScore reports whether pattern matches text as a case-insensitive
// subsequence and, if so, a score where higher is a better match. Matches that
// are contiguous and near the start of the text score highest; shorter
// candidates are mildly preferred. It returns (0, false) when pattern is not a
// subsequence of text.
func fuzzyScore(text, pattern string) (int, bool) {
	pat := []rune(strings.ToLower(pattern))
	if len(pat) == 0 {
		return 0, true
	}
	txt := []rune(strings.ToLower(text))

	score, pi, prev := 0, 0, -2
	for ti := 0; ti < len(txt) && pi < len(pat); ti++ {
		if txt[ti] != pat[pi] {
			continue
		}
		if ti == prev+1 {
			score += 6 // contiguous with the previous match
		}
		if ti == 0 {
			score += 8 // anchored at the start of the candidate
		}
		score -= ti / 4 // earlier matches are mildly better
		prev = ti
		pi++
	}
	if pi < len(pat) {
		return 0, false // not all of pattern was found in order
	}
	score -= len(txt) / 8 // prefer shorter, denser candidates
	return score, true
}
