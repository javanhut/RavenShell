package main

import (
	"fmt"
	"ravenshell/evaluator"
	"ravenshell/lexer"
	"ravenshell/parser"
	"ravenshell/readline"
)

func main() {
	fmt.Println("Welcome to Raven Shell.")
	repl()
}

func repl() {
	eval := evaluator.New()
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
