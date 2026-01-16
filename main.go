package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"ravenshell/evaluator"
	"ravenshell/lexer"
	"ravenshell/parser"
	"ravenshell/readline"
)

func main() {
	// Check if a script file was provided as argument
	if len(os.Args) > 1 {
		runScript(os.Args[1])
		return
	}

	fmt.Println("Welcome to Raven Shell.")
	repl()
}

// runScript executes a .rsh script file
func runScript(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("error: cannot read file %s: %v\n", filename, err)
		os.Exit(1)
	}

	eval := evaluator.New()

	l := lexer.NewLexer(string(content))
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Printf("parse error: %s\n", err)
		}
		os.Exit(1)
	}

	if err := eval.Eval(program); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}

// loadRavenRC loads and executes the .ravenrc file from the user's home directory
func loadRavenRC(eval *evaluator.Evaluator) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	rcPath := filepath.Join(home, ".ravenrc")
	file, err := os.Open(rcPath)
	if err != nil {
		// .ravenrc doesn't exist, that's okay
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			// Skip empty lines and comments
			continue
		}

		l := lexer.NewLexer(line)
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) > 0 {
			for _, err := range p.Errors() {
				fmt.Printf(".ravenrc parse error: %s\n", err)
			}
			continue
		}

		if err := eval.Eval(program); err != nil {
			fmt.Printf(".ravenrc error: %s\n", err)
		}
	}
}

func repl() {
	eval := evaluator.New()

	// Load .ravenrc configuration file
	loadRavenRC(eval)

	rl := readline.New("# ")

	// Set up path completion to use evaluator's current directory
	rl.SetCwdFunc(eval.GetCwd)

	for {
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

		l := lexer.NewLexer(input)
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) > 0 {
			for _, err := range p.Errors() {
				fmt.Printf("parse error: %s\n", err)
			}
			continue
		}

		if err := eval.Eval(program); err != nil {
			fmt.Printf("error: %s\n", err)
		}
	}
}
