package main

import (
	"bufio"
	"fmt"
	"os"
	"ravenshell/evaluator"
	"ravenshell/lexer"
	"ravenshell/parser"
)

func main() {
	fmt.Println("Welcome to Raven Shell.")
	repl()
}

func repl() {
	scanner := bufio.NewScanner(os.Stdin)
	eval := evaluator.New()

	for {
		fmt.Print("# ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
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
