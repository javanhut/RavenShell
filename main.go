package main

import (
	"bufio"
	"fmt"
	"os"
	"ravenshell/lexer"
	"ravenshell/token"
)

func main() {
	fmt.Println("Welcome to Raven Shell.")
	ravenInterpreter()
}

func ravenInterpreter() {
	linePrefix := "#"
	var inputCmd string
	tokenMap := token.TokenMap
	scanner := bufio.NewScanner(os.Stdin)
	totalInput := fmt.Sprintf("%s %s", linePrefix, inputCmd)
	for {
		if inputCmd == "exit" || inputCmd == "quit" {
			break
		}
		fmt.Print(totalInput)
		if scanner.Scan() {
			inputCmd = scanner.Text()
		}
		l := lexer.NewLexer(inputCmd)
		for {
			tok := l.NextToken()
			ttype := tok.Type
			literal := tok.Literal
			if ttype == token.IDENT {
				kwType, ok := tokenMap[literal]
				if ok {
					recognizedTokenStr := fmt.Sprintf("Keyword: %s, Literal: [%s]", kwType, literal)
					fmt.Println(recognizedTokenStr)
				}
			} else if ttype == token.EOF {
				break
			} else {
				otherText := fmt.Sprintf("Type: %s Literal: [%s]", ttype, literal)
				fmt.Println(otherText)
			}

		}
	}
}
