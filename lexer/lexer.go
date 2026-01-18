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

// GetPos returns the current lexer position (for lookahead)
func (l *Lexer) GetPos() int {
	return l.pos
}

// SetPos sets the lexer position (for lookahead restoration)
func (l *Lexer) SetPos(pos int) {
	l.pos = pos
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

	// Skip comments (from # to end of line)
	if ch == '#' {
		for l.peek() != '\n' && l.peek() != 0 {
			l.advance()
		}
		return l.NextToken()
	}

	switch ch {
	case '|':
		if l.peekNext() == '|' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.OR, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.PIPE, Literal: string(l.advance())}
	case '&':
		if l.peekNext() == '&' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.AND, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.ILLEGAL, Literal: string(l.advance())}
	case '.':
		return token.Token{Type: token.FULLSTOP, Literal: string(l.advance())}
	case '~':
		return token.Token{Type: token.TILDE, Literal: string(l.advance())}
	case '$':
		return token.Token{Type: token.DOLLAR, Literal: string(l.advance())}
	case '/':
		return token.Token{Type: token.FSLASH, Literal: string(l.advance())}
	case '{':
		return token.Token{Type: token.LBRACE, Literal: string(l.advance())}
	case '}':
		return token.Token{Type: token.RBRACE, Literal: string(l.advance())}
	case '(':
		return token.Token{Type: token.LPAREN, Literal: string(l.advance())}
	case ')':
		return token.Token{Type: token.RPAREN, Literal: string(l.advance())}
	case '[':
		return token.Token{Type: token.LBRACKET, Literal: string(l.advance())}
	case ']':
		return token.Token{Type: token.RBRACKET, Literal: string(l.advance())}
	case ',':
		return token.Token{Type: token.COMMA, Literal: string(l.advance())}
	case ':':
		return token.Token{Type: token.COLON, Literal: string(l.advance())}
	case '+':
		return token.Token{Type: token.PLUS, Literal: string(l.advance())}
	case '-':
		return token.Token{Type: token.MINUS, Literal: string(l.advance())}
	case '*':
		return token.Token{Type: token.ASTERISK, Literal: string(l.advance())}
	case '%':
		return token.Token{Type: token.PERCENT, Literal: string(l.advance())}
	case '=':
		if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.EQ, Literal: l.input[start:l.pos]}
		} else if l.peekNext() == '~' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.REGEX_MATCH, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.ASSIGN, Literal: string(l.advance())}
	case '!':
		if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.NOT_EQ, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.NOT, Literal: string(l.advance())}
	case '>':
		if l.peekNext() == '>' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.INTO, Literal: l.input[start:l.pos]}
		} else if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.GTE, Literal: l.input[start:l.pos]}
		} else {
			// Use GT for single > (parser will disambiguate comparison vs redirection)
			return token.Token{Type: token.GT, Literal: string(l.advance())}
		}
	case '<':
		if l.peekNext() == '<' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.OUT, Literal: l.input[start:l.pos]}
		} else if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.LTE, Literal: l.input[start:l.pos]}
		} else {
			// Use LT for single < (parser will disambiguate comparison vs redirection)
			return token.Token{Type: token.LT, Literal: string(l.advance())}
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
	} else if unicode.IsLetter(rune(ch)) || ch == '_' {
		start := l.pos
		for isAlphanumeric(l.peek()) {
			l.advance()
		}
		literal := l.input[start:l.pos]
		// Check if it's a keyword
		if tokType, ok := token.TokenMap[literal]; ok {
			return token.Token{Type: tokType, Literal: literal}
		}
		return token.Token{Type: token.IDENT, Literal: literal}
	}
	return token.Token{Type: token.ILLEGAL, Literal: string(l.advance())}
}

func isAlphanumeric(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_'
}
