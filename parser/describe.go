package parser

import (
	"fmt"
	"strings"

	"ravenshell/token"
)

// punctuation maps the operator and delimiter token types to the text a user
// typed for them, so an error can say '{' rather than LBRACE.
var punctuation = map[token.TokenType]string{
	token.LBRACE: "{", token.RBRACE: "}",
	token.LPAREN: "(", token.RPAREN: ")",
	token.LBRACKET: "[", token.RBRACKET: "]",
	token.COMMA: ",", token.COLON: ":", token.AT: "@",
	token.ASSIGN: "=", token.PLUS: "+", token.MINUS: "-",
	token.ASTERISK: "*", token.PERCENT: "%", token.FSLASH: "/",
	token.EQ: "==", token.NOT_EQ: "!=",
	token.LT: "<", token.GT: ">", token.LTE: "<=", token.GTE: ">=",
	token.SEMICOLON: ";", token.AND: "&&", token.OR: "||", token.AMP: "&",
	token.PIPE: "|", token.DOLLAR: "$", token.LASTSTATUS: "$?",
	token.TILDE: "~", token.FULLSTOP: ".",
}

// keywordText is the inverse of token.TokenMap, restricted to one spelling
// per type so that an expected FOR reads as 'for'.
var keywordText = func() map[token.TokenType]string {
	m := map[token.TokenType]string{}
	for word, t := range token.TokenMap {
		if cur, ok := m[t]; !ok || len(word) < len(cur) || (len(word) == len(cur) && word < cur) {
			m[t] = word
		}
	}
	return m
}()

// describeToken names a token that was actually read, as the user would
// recognise it: its literal text for punctuation and keywords, and a category
// plus the text for names, numbers, and strings.
func describeToken(tok token.Token) string {
	switch tok.Type {
	case token.EOF:
		return "end of input"
	case token.STRING:
		return fmt.Sprintf("string %q", tok.Literal)
	case token.INTEGER:
		return "number " + tok.Literal
	case token.IDENT:
		return fmt.Sprintf("name '%s'", tok.Literal)
	case token.FLAG:
		return fmt.Sprintf("flag '%s'", tok.Literal)
	case token.ILLEGAL:
		return fmt.Sprintf("character '%s'", tok.Literal)
	}
	if tok.Literal != "" {
		return "'" + tok.Literal + "'"
	}
	return describeTokenType(tok.Type)
}

// describeTokenType names a token type the parser was expecting but did not
// get, so there is no literal text to quote.
func describeTokenType(t token.TokenType) string {
	switch t {
	case token.EOF:
		return "end of input"
	case token.IDENT:
		return "a name"
	case token.INTEGER:
		return "a number"
	case token.STRING:
		return "a string"
	case token.FLAG:
		return "a flag"
	}
	if s, ok := punctuation[t]; ok {
		return "'" + s + "'"
	}
	if s, ok := keywordText[t]; ok {
		return "'" + s + "'"
	}
	return strings.ToLower(string(t))
}
