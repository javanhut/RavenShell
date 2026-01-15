package lexer

import (
	"ravenshell/token"
	"unicode"
)

type Lexer struct {
	input string
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: input, pos: 0}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) advance() byte {
	ch := l.peek()
	l.pos++
	return ch
}

func (l *Lexer) peekNext() byte {
	if l.pos+1 >= len(l.input) {
		return 0
	}
	return l.input[l.pos+1]
}

func (l *Lexer) NextToken() token.Token {
	ch := l.peek()
	if unicode.IsSpace(rune(ch)) {
		l.advance()
		return l.NextToken()
	}

	switch ch {
	case '|':
		return token.Token{Type: token.PIPE, Literal: string(l.advance())}
	case '.':
		return token.Token{Type: token.FULLSTOP, Literal: string(l.advance())}
	case '$':
		return token.Token{Type: token.DOLLAR, Literal: string(l.advance())}
	case '/':
		return token.Token{Type: token.FSLASH, Literal: string(l.advance())}
	case '>':
		if l.peekNext() == '>' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.INTO, Literal: l.input[start:l.pos]}
		} else {
			return token.Token{Type: token.GREATER, Literal: string(l.advance())}
		}
	case '<':
		if l.peekNext() == '<' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.OUT, Literal: l.input[start:l.pos]}
		} else {
			return token.Token{Type: token.LESS, Literal: string(l.advance())}
		}
	case '"':
		// 1. Skip the opening quote
		l.advance()
		start := l.pos

		// 2. Read until we find the closing quote or EOF
		for l.peek() != '"' && l.peek() != 0 {
			l.advance()
		}

		// Capture the string content
		literal := l.input[start:l.pos]

		// 3. Skip the closing quote (if it exists)
		if l.peek() == '"' {
			l.advance()
		} else {
			// Optional: Handle unclosed string error here
			return token.Token{Type: token.ILLEGAL, Literal: literal}
		}
		return token.Token{Type: token.STRING, Literal: literal}
	case '\'':

		// 1. Skip the opening quote
		l.advance()
		start := l.pos

		// 2. Read until we find the closing quote or EOF
		for l.peek() != '\'' && l.peek() != 0 {
			l.advance()
		}

		// Capture the string content
		literal := l.input[start:l.pos]

		// 3. Skip the closing quote (if it exists)
		if l.peek() == '\'' {
			l.advance()
		} else {
			// Optional: Handle unclosed string error here
			return token.Token{Type: token.ILLEGAL, Literal: literal}
		}
		return token.Token{Type: token.STRING, Literal: literal}
	case 0:
		return token.Token{Type: token.EOF, Literal: ""}
	}

	if unicode.IsDigit(rune(ch)) {
		start := l.pos
		for unicode.IsDigit(rune(l.peek())) {
			l.advance()
		}
		return token.Token{Type: token.INTEGER, Literal: l.input[start:l.pos]}
	} else if unicode.IsLetter(rune(ch)) {
		start := l.pos
		for isAlphanumeric(l.peek()) {
			l.advance()
		}
		return token.Token{Type: token.IDENT, Literal: l.input[start:l.pos]}
	}
	return token.Token{Type: token.ILLEGAL, Literal: string(l.advance())}
}

func isAlphanumeric(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_'
}
