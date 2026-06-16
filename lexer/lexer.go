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
	sawWhitespace, sawNewline := l.skipWhitespaceAndComments()
	tok := l.scanToken()
	tok.PrecededByNewline = sawNewline
	tok.PrecededByWhitespace = sawWhitespace
	return tok
}

// skipWhitespaceAndComments advances past spaces, comments, and newlines,
// reporting whether any whitespace/comment was skipped and whether any of it
// was a newline (so the parser can detect token adjacency and line boundaries).
func (l *Lexer) skipWhitespaceAndComments() (sawWhitespace, sawNewline bool) {
	for {
		ch := l.peek()
		if ch == '\n' {
			sawWhitespace = true
			sawNewline = true
			l.advance()
		} else if unicode.IsSpace(rune(ch)) {
			sawWhitespace = true
			l.advance()
		} else if ch == '#' {
			// Skip a comment up to (but not including) the end of line.
			sawWhitespace = true
			for l.peek() != '\n' && l.peek() != 0 {
				l.advance()
			}
		} else if ch == '\\' && (l.peekNext() == '\n' || l.peekNext() == '\r' || l.peekNext() == 0) {
			// A backslash at end of line (or end of input) is a line
			// continuation: splice the next line onto this one. It counts as
			// whitespace but NOT a newline, so it does not terminate a command's
			// argument list — `echo a \<newline> b` is the single command `echo a b`.
			sawWhitespace = true
			l.advance() // consume '\'
			if l.peek() == '\r' {
				l.advance()
			}
			if l.peek() == '\n' {
				l.advance()
			}
		} else {
			return sawWhitespace, sawNewline
		}
	}
}

func (l *Lexer) scanToken() token.Token {
	ch := l.peek()

	switch ch {
	case '|':
		if l.peekNext() == '|' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.OR, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.PIPE, Literal: string(l.advance())}
	case ';':
		return token.Token{Type: token.SEMICOLON, Literal: string(l.advance())}
	case ':':
		return token.Token{Type: token.COLON, Literal: string(l.advance())}
	case '@':
		return token.Token{Type: token.AT, Literal: string(l.advance())}
	case '&':
		if l.peekNext() == '&' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.AND, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.AMP, Literal: string(l.advance())}
	case '.':
		return token.Token{Type: token.FULLSTOP, Literal: string(l.advance())}
	case '~':
		return token.Token{Type: token.TILDE, Literal: string(l.advance())}
	case '$':
		if l.peekNext() == '?' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.LASTSTATUS, Literal: l.input[start:l.pos]}
		}
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
	case '+':
		// A '+' immediately followed by a letter is a command word, not addition
		// (e.g. chmod's symbolic modes +x, +rwx). Arithmetic always uses spaces
		// around the operator, so `a + b` stays unambiguous while `+x` is a word.
		if unicode.IsLetter(rune(l.peekNext())) {
			start := l.pos
			l.advance() // consume the leading '+'
			for !isFlagTerminator(l.peek()) {
				l.advance()
			}
			return token.Token{Type: token.FLAG, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.PLUS, Literal: string(l.advance())}
	case '-':
		// A '-' immediately followed by a letter or another '-' is a command
		// flag (e.g. -l, --all, --max-count=5), not subtraction. Arithmetic
		// always uses spaces around the operator, so this stays unambiguous.
		next := l.peekNext()
		if unicode.IsLetter(rune(next)) || next == '-' {
			start := l.pos
			l.advance() // consume the leading '-'
			for !isFlagTerminator(l.peek()) {
				l.advance()
			}
			return token.Token{Type: token.FLAG, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.MINUS, Literal: string(l.advance())}
	case '*':
		return token.Token{Type: token.ASTERISK, Literal: string(l.advance())}
	case '%':
		// "%1" (no space) is a job reference used by kill/jobs; "%" with spaces
		// around it is the modulo operator.
		if unicode.IsDigit(rune(l.peekNext())) {
			start := l.pos
			l.advance() // consume '%'
			for unicode.IsDigit(rune(l.peek())) {
				l.advance()
			}
			return token.Token{Type: token.IDENT, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.PERCENT, Literal: string(l.advance())}
	case '=':
		if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.EQ, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.ASSIGN, Literal: string(l.advance())}
	case '!':
		if l.peekNext() == '=' {
			start := l.pos
			l.advance()
			l.advance()
			return token.Token{Type: token.NOT_EQ, Literal: l.input[start:l.pos]}
		}
		return token.Token{Type: token.ILLEGAL, Literal: string(l.advance())}
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
		// Single-quoted strings are literal (not interpolated).
		return token.Token{Type: token.STRING, Literal: literal, SingleQuoted: true}
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
		for {
			c := l.peek()
			if isAlphanumeric(c) {
				l.advance()
				continue
			}
			// Allow '-' to join words inside an identifier (raven-add,
			// docker-compose) but only when it sits between word characters,
			// so leading flags (-l) and spaced subtraction (a - b) are unaffected.
			if c == '-' && isAlphanumeric(l.peekNext()) {
				l.advance()
				continue
			}
			break
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

// isFlagTerminator reports whether ch ends a command flag token. Flags run
// until whitespace, EOF, or a shell operator/delimiter so values like
// --color=auto or --file=./path stay attached to the flag.
func isFlagTerminator(ch byte) bool {
	switch ch {
	case 0, ' ', '\t', '\n', '\r', '|', '<', '>', '{', '}', '(', ')', ',', '"', '\'':
		return true
	default:
		return false
	}
}
