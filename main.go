package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"ravenshell/ansi"
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
	args := os.Args[1:]
	script := ""

	for i := 0; i < len(args); i++ {
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
		}
		if script != "" {
			break
		}
	}

	if script != "" {
		runScript(script)
		return
	}

	// A terminal on stdin means interactive use; otherwise read a program from
	// stdin (e.g. `ravenshell < script.rsh` or piped input).
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("Welcome to Raven Shell.")
		repl()
	} else {
		runStdin()
	}
}

// runCommandString evaluates a single command string (the -c flag) and exits
// non-zero if it fails to parse.
func runCommandString(command string) {
	eval := evaluator.New()
	if !runSource(eval, command, "") {
		os.Exit(1)
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
	if !runSource(eval, string(content), "") {
		os.Exit(1)
	}
}

// runSource parses and evaluates a complete source string against the given
// evaluator. label is used to prefix parse/eval error messages. It reports
// whether parsing succeeded.
func runSource(eval *evaluator.Evaluator, source, label string) bool {
	l := lexer.NewLexer(source)
	p := parser.New(l)
	program := p.ParseProgram()

	prefix := ""
	if label != "" {
		prefix = label + " "
	}

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Println(colorize(ansi.Red, fmt.Sprintf("%sparse error: %s", prefix, err)))
		}
		return false
	}

	if err := eval.Eval(program); err != nil {
		if errors.Is(err, evaluator.ErrInterrupted) {
			fmt.Println("^C")
		} else {
			fmt.Println(colorize(ansi.Red, fmt.Sprintf("%serror: %s", prefix, err)))
		}
	}
	return true
}

// inputIncomplete reports whether src has unbalanced brackets or an unterminated
// string, meaning the REPL should keep reading continuation lines.
func inputIncomplete(src string) bool {
	depth := 0
	var inString byte // 0, '\'' or '"'
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString != 0 {
			if c == inString {
				inString = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inString = c
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0 || inString != 0
}

// runScript executes a .rsh script file
func runScript(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("error: cannot read file %s: %v\n", filename, err)
		os.Exit(1)
	}

	eval := evaluator.New()
	if !runSource(eval, string(content), "script") {
		os.Exit(1)
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

func repl() {
	eval := evaluator.New()

	// Load .ravenrc configuration file
	loadRavenRC(eval)

	rl := readline.New(makePrompt(eval.GetCwd()))

	// Set up path completion to use evaluator's current directory
	rl.SetCwdFunc(eval.GetCwd)

	// Offer user-defined functions and PATH executables as tab completions.
	rl.SetCommandProvider(eval.AvailableCommands)

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
			rl.SetPrompt(makePrompt(eval.GetCwd()))
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
		runSource(eval, src, "")
		buf.Reset()
	}
}
