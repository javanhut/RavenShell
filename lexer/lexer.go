package lexer

import (
	"ravenshell/token"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Lexer struct {
	input      string
	pos        int
	lineStarts []int

	// heredocs maps the byte offset of a heredoc operator to the body collected
	// for it; skips are the body regions that must not be tokenized. Both are
	// filled once, up front, by scanHeredocs.
	heredocs map[int]*Heredoc
	skips    []heredocSkip
}

func NewLexer(input string) *Lexer {
	starts := []int{0}
	for i := 0; i < len(input); i++ {
		if input[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	l := &Lexer{input: input, pos: 0, lineStarts: starts, heredocs: map[int]*Heredoc{}}
	l.scanHeredocs()
	return l
}

// GetPos returns the current lexer position (for lookahead)
func (l *Lexer) GetPos() int {
	return l.pos
}

// SetPos sets the lexer position (for lookahead restoration)
func (l *Lexer) SetPos(pos int) {
	l.pos = pos
}

// BraceGroupClosesAt reports whether the text starting at pos (the position just
// past an opening '{') forms a valid brace-expansion group: it balances to a
// matching '}' with a top-level ',' or '..', and contains no whitespace. The
// parser uses it to tell an argument like {a,b} from a control-flow block '{'.
func (l *Lexer) BraceGroupClosesAt(pos int) bool {
	depth := 1
	sawSep := false
	for i := pos; i < len(l.input); i++ {
		switch l.input[i] {
		case ' ', '\t', '\n', '\r':
			return false // whitespace inside braces: not an expansion
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return sawSep
			}
		case ',':
			if depth == 1 {
				sawSep = true
			}
		case '.':
			if depth == 1 && i+1 < len(l.input) && l.input[i+1] == '.' {
				sawSep = true // {1..5}-style sequence
			}
		}
	}
	return false
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

func (l *Lexer) hasTripleQuote(quote byte) bool {
	return l.pos+2 < len(l.input) && l.input[l.pos] == quote &&
		l.input[l.pos+1] == quote && l.input[l.pos+2] == quote
}

func (l *Lexer) NextToken() token.Token {
	sawWhitespace, sawNewline := l.skipWhitespaceAndComments()
	start := l.pos
	tok := l.scanToken()
	tok.Offset = start
	tok.End = l.pos
	lineIndex := sort.Search(len(l.lineStarts), func(i int) bool { return l.lineStarts[i] > start }) - 1
	tok.Line = lineIndex + 1
	tok.Column = utf8.RuneCountInString(l.input[l.lineStarts[lineIndex]:start]) + 1
	tok.PrecededByNewline = sawNewline
	tok.PrecededByWhitespace = sawWhitespace
	return tok
}

// skipWhitespaceAndComments advances past spaces, comments, and newlines,
// reporting whether any whitespace/comment was skipped and whether any of it
// was a newline (so the parser can detect token adjacency and line boundaries).
func (l *Lexer) skipWhitespaceAndComments() (sawWhitespace, sawNewline bool) {
	for {
		// A heredoc body is content for the command, not source to tokenize:
		// step over it as if it were whitespace. The command line it belongs to
		// has already ended, so this counts as a newline.
		if end, ok := l.heredocSkipAt(l.pos); ok {
			l.pos = end
			sawWhitespace = true
			sawNewline = true
			continue
		}
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
		if l.peekNext() == '>' { // &> or &>>: redirect both stdout and stderr
			return l.scanRedirFrom(l.pos)
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
		if l.peekNext() == '&' { // >&1: duplicate stdout onto another fd
			return l.scanRedirFrom(l.pos)
		}
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
		if l.peekNext() == '&' { // <&0: duplicate an fd onto stdin
			return l.scanRedirFrom(l.pos)
		}
		if l.peekNext() == '<' {
			start := l.pos
			l.advance()
			l.advance()
			// <<- is the tab-stripping heredoc form. The '-' belongs to the
			// operator; without this it would lex as the start of a flag and
			// swallow the delimiter (<<-EOF becoming `<<` and `-EOF`).
			if l.peek() == '-' {
				l.advance()
			}
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
	case '"', '\'':
		return l.scanString(ch)
	case 0:
		return token.Token{Type: token.EOF, Literal: ""}
	}

	if unicode.IsDigit(rune(ch)) {
		start := l.pos
		for unicode.IsDigit(rune(l.peek())) {
			l.advance()
		}
		// Digits glued directly to a redirection operator are an fd prefix
		// (e.g. 2>file, 2>&1, 1>>log), not an integer literal. Spaced digits
		// like `a > b` are unaffected since whitespace was already skipped.
		if l.peek() == '>' || l.peek() == '<' {
			return l.scanRedirFrom(start)
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

// scanString handles regular and Python-style triple-quoted strings. Double
// quotes interpolate at evaluation time; single quotes remain literal. Common
// backslash escapes are decoded by the language rather than passed through as
// shell syntax.
func (l *Lexer) scanString(quote byte) token.Token {
	triple := l.hasTripleQuote(quote)
	width := 1
	if triple {
		width = 3
	}
	for range width {
		l.advance()
	}
	start := l.pos
	for l.peek() != 0 {
		if triple {
			if l.hasTripleQuote(quote) {
				literal := decodeStringEscapes(l.input[start:l.pos], quote)
				l.advance()
				l.advance()
				l.advance()
				return token.Token{Type: token.STRING, Literal: literal, SingleQuoted: quote == '\''}
			}
		} else if l.peek() == quote {
			literal := decodeStringEscapes(l.input[start:l.pos], quote)
			l.advance()
			return token.Token{Type: token.STRING, Literal: literal, SingleQuoted: quote == '\''}
		}
		if l.peek() == '\\' && l.peekNext() != 0 {
			l.advance()
			l.advance()
			continue
		}
		l.advance()
	}
	return token.Token{Type: token.ILLEGAL, Literal: l.input[start:l.pos]}
}

func decodeStringEscapes(s string, quote byte) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case '\\':
			out.WriteByte('\\')
		case quote:
			out.WriteByte(quote)
		default:
			out.WriteByte('\\')
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// scanRedirFrom builds a single fd-aware redirection token starting at byte
// offset `start`. The lexer position must sit at the first operator char: '&'
// (for &>/&>>), '>' or '<' (optionally with an fd prefix already consumed into
// start). It consumes the operator, any >>/<< doubling, and an optional '&N'
// fd-duplication suffix, so forms like 2>, 2>>, &>, >&1, and 2>&1 each become
// one REDIR token whose literal is the exact operator text.
func (l *Lexer) scanRedirFrom(start int) token.Token {
	if l.peek() == '&' { // &> / &>>
		l.advance()
	}
	op := l.peek() // '>' or '<'
	l.advance()
	doubled := l.peek() == op
	if doubled { // >> or <<
		l.advance()
	}
	if doubled && op == '<' && l.peek() == '-' { // 0<<- tab-stripping heredoc
		l.advance()
	}
	if l.peek() == '&' { // fd duplication: >&1, 2>&1
		l.advance()
		for unicode.IsDigit(rune(l.peek())) {
			l.advance()
		}
	}
	return token.Token{Type: token.REDIR, Literal: l.input[start:l.pos]}
}

func isAlphanumeric(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_'
}

// isFlagTerminator reports whether ch ends a command flag token. Flags run
// until whitespace, EOF, or a shell operator/delimiter so values like
// --color=auto or --file=./path stay attached to the flag. The command
// separators ';' and '&' (and ']' , matching '[') terminate the flag too, so a
// flag glued to one of them — `echo -n;echo`, `cmd -x&&y`, `cmd -x&` — still
// splits correctly; a literal ';' or '&' in a flag value must be quoted.
func isFlagTerminator(ch byte) bool {
	switch ch {
	case 0, ' ', '\t', '\n', '\r', '|', '<', '>', '{', '}', '(', ')', '[', ']', ',', ';', '&', '"', '\'':
		return true
	default:
		return false
	}
}
