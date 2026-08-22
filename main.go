package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"ravenshell/ansi"
	"ravenshell/completion"
	"ravenshell/evaluator"
	"ravenshell/lexer"
	"ravenshell/parser"
	"ravenshell/readline"
	"strings"

	"golang.org/x/term"
)

// version is the RavenShell version, overridable at build time with
// -ldflags "-X main.version=v1.2.3".
var version = "dev"

// sourceDir is the absolute path of the source tree this binary was built
// from, stamped in by install.sh / the Makefile with
// -ldflags "-X main.sourceDir=<path>". It lets `raven-update` find the source
// to rebuild from without searching. Empty for ad-hoc `go build`.
var sourceDir = ""

const usage = `RavenShell - a command-line interpreter and scripting language.

Usage:
  ravenshell                 Start the interactive shell (REPL)
  ravenshell <script.rsh>    Run a script file
  ravenshell -c "<command>"  Run a command string and exit
  ravenshell < script.rsh    Run a program read from standard input

Options:
  -c, --command <cmd>   Execute the given command string and exit
  -l, --login           Treated as interactive (accepted for login-shell use)
  -v, --version         Print the version and exit
  -h, --help            Show this help and exit`

// stdoutIsTerminal reports whether stdout is an interactive terminal, used to
// decide whether to emit ANSI color codes.
func stdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// colorize wraps s with an ANSI style only when stdout is a terminal.
func colorize(code, s string) string {
	if stdoutIsTerminal() {
		return ansi.Wrap(code, s)
	}
	return s
}

func main() {
	// Hand build metadata to the evaluator so `raven-update` can report the
	// running version and find the source tree it was built from.
	evaluator.BuildVersion = version
	evaluator.BuildSourceDir = sourceDir

	args := os.Args[1:]
	script := ""
	var scriptArgs []string

	for i := range args {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--command":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "ravenshell: -c requires an argument")
				os.Exit(2)
			}
			runCommandString(args[i+1])
			return
		case arg == "-v" || arg == "--version":
			fmt.Println("RavenShell " + version)
			return
		case arg == "-h" || arg == "--help":
			fmt.Println(usage)
			return
		case strings.HasPrefix(arg, "-"):
			// Login/interactive and other leading-dash flags (e.g. -l, -i) are
			// accepted and ignored so RavenShell works as a login shell.
			continue
		default:
			script = arg
			scriptArgs = append([]string(nil), args[i+1:]...)
		}
		if script != "" {
			break
		}
	}

	if script != "" {
		runScript(script, scriptArgs)
		return
	}

	// Interactive use requires a terminal on both stdin and stdout: with stdout
	// redirected (e.g. `ravenshell >> file`) the REPL would render its prompt
	// and cursor-position queries into the file and appear frozen. Otherwise
	// read a program from stdin (e.g. `ravenshell < script.rsh` or piped input).
	if term.IsTerminal(int(os.Stdin.Fd())) && stdoutIsTerminal() {
		fmt.Println("Welcome to Raven Shell.")
		repl()
	} else {
		runStdin()
	}
}

// runCommandString evaluates a single command string and mirrors its status.
func runCommandString(command string) {
	eval := evaluator.New()
	result := runSource(eval, command, "")
	if result.status != 0 {
		os.Exit(result.status)
	}
}

// runStdin reads a complete program from standard input and evaluates it.
func runStdin() {
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read stdin: %v\n", err)
		os.Exit(1)
	}
	eval := evaluator.New()
	result := runSource(eval, string(content), "")
	if result.status != 0 {
		os.Exit(result.status)
	}
}

type sourceResult struct {
	status        int
	exitRequested bool
}

// runSource parses and evaluates a complete source string against the given
// evaluator. label is used to prefix parse/eval error messages. The result
// carries both the process status and an explicit exit() request for the REPL.
func runSource(eval *evaluator.Evaluator, source, label string) sourceResult {
	l := lexer.NewLexer(source)
	p := parser.New(l)
	program := p.ParseProgram()

	prefix := ""
	if label != "" {
		prefix = label + " "
	}

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Fprintln(os.Stderr, colorize(ansi.Red, fmt.Sprintf("%sparse error: %s", prefix, err)))
		}
		return sourceResult{status: 2}
	}

	if err := eval.Eval(program); err != nil {
		var exit *evaluator.ExitRequest
		if errors.As(err, &exit) {
			return sourceResult{status: exit.Status, exitRequested: true}
		}
		if errors.Is(err, evaluator.ErrInterrupted) {
			fmt.Println("^C")
			return sourceResult{status: 130}
		} else {
			fmt.Fprintln(os.Stderr, colorize(ansi.Red, fmt.Sprintf("%serror: %s", prefix, err)))
		}
		return sourceResult{status: 1}
	}
	return sourceResult{status: eval.LastStatus()}
}

// inputIncomplete reports whether src is an incomplete command that the REPL
// should keep reading continuation lines for. Input is incomplete when it has
// unbalanced brackets, an unterminated string, a trailing line-continuation
// backslash, an open heredoc whose delimiter line has not arrived, or a
// trailing binary operator (a pipe `|`/`||` or logical `&&`) that still needs a
// right-hand side. A single trailing `&` is the background operator and is
// treated as complete.
func inputIncomplete(src string) bool {
	// The lexer already knows the quoting rules, so let it decide whether a
	// heredoc is still open rather than re-deriving that here.
	if lexer.NewLexer(src).UnterminatedHeredoc() {
		return true
	}

	depth := 0
	var inString byte // 0, '\'' or '"'
	triple := false
	lastMeaningful := -1
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString != 0 {
			lastMeaningful = i
			if c == '\\' {
				i++
				continue
			}
			if triple && c == inString && i+2 < len(src) && src[i+1] == c && src[i+2] == c {
				inString = 0
				triple = false
				i += 2
			} else if !triple && c == inString {
				inString = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inString = c
			triple = i+2 < len(src) && src[i+1] == c && src[i+2] == c
			if triple {
				i += 2
			}
			lastMeaningful = i
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case ' ', '\t', '\r', '\n':
			// Whitespace is not meaningful for trailing-operator detection.
		case '{', '(', '[':
			depth++
			lastMeaningful = i
		case '}', ')', ']':
			if depth > 0 {
				depth--
			}
			lastMeaningful = i
		default:
			lastMeaningful = i
		}
	}

	if depth > 0 || inString != 0 {
		return true
	}
	if lastMeaningful < 0 {
		return false
	}

	switch src[lastMeaningful] {
	case '\\':
		// Trailing line-continuation backslash.
		return true
	case '|':
		// Trailing pipe (`|` or `||`) always needs a right-hand command.
		return true
	case '&':
		// `&&` continues; a lone `&` is the background operator (complete).
		return lastMeaningful > 0 && src[lastMeaningful-1] == '&'
	}
	return false
}

// runScript executes a .rsh script file
func runScript(filename string, args []string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read file %s: %v\n", filename, err)
		os.Exit(1)
	}

	eval := evaluator.NewWithArgs(args)
	result := runSource(eval, string(content), filename)
	if result.status != 0 {
		os.Exit(result.status)
	}
}

// loadRavenRC loads and executes the .ravenrc file from the user's home
// directory as a single program, so multi-line constructs (loops, blocks)
// work the same as in a script.
func loadRavenRC(eval *evaluator.Evaluator) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	rcPath := filepath.Join(home, ".ravenrc")
	content, err := os.ReadFile(rcPath)
	if err != nil {
		// .ravenrc doesn't exist, that's okay
		return
	}

	runSource(eval, string(content), ".ravenrc")
}

// loadInitScript runs the script pointed to by $RAVEN_INIT_SCRIPT, if set.
// Terminal emulators (e.g. RavenTerminal) use it to inject prompt/detect
// functions and exports without touching the user's .ravenrc.
func loadInitScript(eval *evaluator.Evaluator) {
	path := os.Getenv("RAVEN_INIT_SCRIPT")
	if path == "" {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	runSource(eval, string(content), filepath.Base(path))
}

// makePrompt builds a colored, fish-style prompt showing the current directory
// (with the home directory abbreviated to ~).
func makePrompt(cwd string) string {
	display := cwd
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if cwd == home {
			display = "~"
		} else if strings.HasPrefix(cwd, home+string(os.PathSeparator)) {
			display = "~" + cwd[len(home):]
		}
	}
	return colorize(ansi.Cyan, display) + " " + colorize(ansi.Green+ansi.Bold, "❯") + " "
}

// shellPrompt returns the prompt for the next REPL read: the output of a
// user-defined `prompt` function when one exists (defined in .ravenrc or an
// init script), the built-in colored cwd prompt otherwise.
func shellPrompt(eval *evaluator.Evaluator) string {
	if p, ok := eval.CallPrompt(); ok {
		return p
	}
	return makePrompt(eval.GetCwd())
}

// oscWorkingDir returns the OSC 7 escape that reports dir to the terminal as the
// current working directory. Terminal emulators (Ghostty, iTerm2, WezTerm,
// kitty, ...) read OSC 7 to open new tabs and splits in the same directory; a
// custom shell like RavenShell gets no automatic cwd reporting, so it must emit
// this itself. The path is encoded as a file://<hostname>/<abs-path> URL. The
// sequence is zero-width, so it is written straight to the terminal rather than
// embedded in the width-measured prompt. Returns "" for a non-absolute dir.
func oscWorkingDir(dir string) string {
	if !filepath.IsAbs(dir) {
		return ""
	}
	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}
	u := url.URL{Scheme: "file", Host: host, Path: dir}
	return "\x1b]7;" + u.String() + "\x07"
}

func repl() {
	eval := evaluator.New()

	// Load .ravenrc configuration file, then any injected init script.
	loadRavenRC(eval)
	loadInitScript(eval)

	rl := readline.New(shellPrompt(eval))

	// Set up path completion to use evaluator's current directory
	rl.SetCwdFunc(eval.GetCwd)

	// Offer user-defined functions and PATH executables as tab completions.
	rl.SetCommandProvider(eval.AvailableCommands)

	// Fish-style completion: per-command specs (subcommands, flags, dynamic
	// arguments) with descriptions, user spec files, and a --help fallback.
	engine := completion.New(eval.GetCwd, eval.AvailableCommands, evaluator.BuiltinSummaries())
	rl.SetCompleter(func(line string, pos int) []readline.Candidate {
		cands := engine.Complete(line, pos)
		out := make([]readline.Candidate, len(cands))
		for i, c := range cands {
			out[i] = readline.Candidate{Text: c.Text, Desc: c.Desc, Style: c.Style}
		}
		return out
	})

	// Interrupt running commands/loops with Ctrl-C without killing the shell.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		for range sigCh {
			eval.Interrupt()
		}
	}()

	contPrompt := colorize(ansi.BrightBlk, "… ")
	var buf strings.Builder

	for {
		if buf.Len() > 0 {
			rl.SetPrompt(contPrompt)
		} else {
			// Refresh the prompt so it reflects the current directory after cd.
			rl.SetPrompt(shellPrompt(eval))
			// Report the cwd to the terminal (OSC 7) so new tabs and splits open
			// in the same directory. Emitted as its own zero-width sequence, not
			// through the measured prompt, on every primary prompt (after cd, etc.).
			if osc := oscWorkingDir(eval.GetCwd()); osc != "" {
				fmt.Print(osc)
			}
		}

		input, err := rl.ReadLine()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) {
				// Ctrl-C: discard any partial multi-line input.
				buf.Reset()
				continue
			}
			break // EOF
		}

		// exit/quit only when not in the middle of a multi-line construct.
		if buf.Len() == 0 && (input == "exit" || input == "quit") {
			fmt.Println("Goodbye!")
			break
		}

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(input)

		src := buf.String()
		if strings.TrimSpace(src) == "" {
			buf.Reset()
			continue
		}
		// Keep reading lines until brackets/quotes balance.
		if inputIncomplete(src) {
			continue
		}

		eval.ClearInterrupt()
		result := runSource(eval, src, "")
		buf.Reset()
		if result.exitRequested {
			fmt.Println("Goodbye!")
			break
		}
	}
}
