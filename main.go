package main

import (
	"bufio"
	"fmt"
	"os"
	"ravenshell/ast"
	"ravenshell/lexer"
	"ravenshell/parser"
)

const PROMPT = "# "

func main() {
	fmt.Println("Welcome to Raven Shell.")
	ravenInterpreter()
}

func ravenInterpreter() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(PROMPT)
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

		// Check for parser errors
		if len(p.Errors()) > 0 {
			printParserErrors(p.Errors())
			continue
		}

		// Print the parsed AST
		printAST(program)
	}
}

func printParserErrors(errors []string) {
	fmt.Println("Parser errors:")
	for _, err := range errors {
		fmt.Printf("  - %s\n", err)
	}
}

func printAST(program *ast.Program) {
	for _, stmt := range program.Statements {
		printStatement(stmt, 0)
	}
}

func printStatement(stmt ast.Statement, indent int) {
	switch s := stmt.(type) {
	case *ast.ExpressionStatement:
		printExpression(s.Expression, indent)
	}
}

func printExpression(expr ast.Expression, indent int) {
	prefix := getIndent(indent)

	switch e := expr.(type) {
	case *ast.Command:
		fmt.Printf("%sCommand: %s (type: %s)\n", prefix, e.Name, e.Type)
		if len(e.Arguments) > 0 {
			fmt.Printf("%s  Arguments:\n", prefix)
			for i, arg := range e.Arguments {
				fmt.Printf("%s    [%d] ", prefix, i)
				printExpressionInline(arg)
				fmt.Println()
			}
		}

	case *ast.PipeExpression:
		fmt.Printf("%sPipe:\n", prefix)
		fmt.Printf("%s  Left:\n", prefix)
		printExpression(e.Left, indent+2)
		fmt.Printf("%s  Right:\n", prefix)
		printExpression(e.Right, indent+2)

	case *ast.RedirectionExpression:
		fmt.Printf("%sRedirection (%s):\n", prefix, e.Type)
		fmt.Printf("%s  Command:\n", prefix)
		printExpression(e.Command, indent+2)
		fmt.Printf("%s  Target: ", prefix)
		printExpressionInline(e.Target)
		fmt.Println()

	case *ast.Identifier:
		fmt.Printf("%sIdentifier: %s\n", prefix, e.Value)

	case *ast.PathExpression:
		fmt.Printf("%sPath: %s\n", prefix, e.Value)

	case *ast.StringLiteral:
		fmt.Printf("%sString: \"%s\"\n", prefix, e.Value)

	case *ast.IntegerLiteral:
		fmt.Printf("%sInteger: %d\n", prefix, e.Value)

	case *ast.VariableReference:
		fmt.Printf("%sVariable: $%s\n", prefix, e.Name.Value)

	default:
		fmt.Printf("%s%s\n", prefix, expr.String())
	}
}

func printExpressionInline(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.Identifier:
		fmt.Printf("Identifier(%s)", e.Value)
	case *ast.PathExpression:
		fmt.Printf("Path(%s)", e.Value)
	case *ast.StringLiteral:
		fmt.Printf("String(\"%s\")", e.Value)
	case *ast.IntegerLiteral:
		fmt.Printf("Integer(%d)", e.Value)
	case *ast.VariableReference:
		fmt.Printf("Variable($%s)", e.Name.Value)
	default:
		fmt.Printf("%s", expr.String())
	}
}

func getIndent(level int) string {
	result := ""
	for i := 0; i < level; i++ {
		result += "  "
	}
	return result
}
