package main

import (
	"fmt"
	"os"
	"path/filepath"
	"ravenshell/ansi"
	"ravenshell/evaluator"
	"ravenshell/lexer"
	"ravenshell/parser"
	"ravenshell/readline"
	"strings"

	"golang.org/x/term"
)

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
	// Check if a script file was provided as argument
	if len(os.Args) > 1 {
		runScript(os.Args[1])
		return
	}

	fmt.Println("Welcome to Raven Shell.")
	repl()
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
		fmt.Println(colorize(ansi.Red, fmt.Sprintf("%serror: %s", prefix, err)))
	}
	return true
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

	for {
		// Refresh the prompt so it reflects the current directory after cd.
		rl.SetPrompt(makePrompt(eval.GetCwd()))

		input, err := rl.ReadLine()
		if err != nil {
			// EOF or error
			break
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		if input == "" {
			continue
		}

		runSource(eval, input, "")
	}
}
