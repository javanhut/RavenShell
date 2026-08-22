package lexer

import (
	"ravenshell/token"
	"strings"
)

// Heredoc is the body collected for one heredoc redirection operator, keyed in
// the lexer by the byte offset of that operator so the parser can attach it to
// the redirection it belongs to.
type Heredoc struct {
	Delimiter string // the word after <<, with any quoting removed
	Body      string // the collected lines, each one newline-terminated
	Quoted    bool   // delimiter was quoted (<<'EOF'): the body is used verbatim
	StripTabs bool   // <<- form: leading tabs come off the body and delimiter lines
	// DelimEnd is the byte offset just past the delimiter word. The parser uses
	// it to consume however many tokens the delimiter spans without having to
	// re-derive the word.
	DelimEnd int
	// Terminated is false when the input ran out before a delimiter line was
	// seen. The REPL uses this to keep reading continuation lines.
	Terminated bool
}

// heredocSkip is a body region the lexer jumps over. The lines between a
// heredoc operator's line and its delimiter line are content, not shell source,
// so no token may be produced from them.
type heredocSkip struct{ start, end int }

// scanHeredocs collects every heredoc body in the input up front, before the
// parser pulls its first token. It runs a throwaway sub-lexer to do the
// scanning, which keeps the quoting and comment rules in one place: a `<<`
// inside a string or after a `#` is not an operator. The sub-lexer registers
// each body region in its own skip list as it goes, so it steps over bodies for
// the rest of its scan exactly the way the real lexer will.
func (l *Lexer) scanHeredocs() {
	sub := &Lexer{input: l.input, lineStarts: l.lineStarts, heredocs: map[int]*Heredoc{}}

	// bodyCursor is where the next body may start. Two heredocs on one line
	// stack their bodies, so the second begins where the first one ended.
	bodyCursor := 0

	for {
		tok := sub.NextToken()
		if tok.Type == token.EOF {
			break
		}
		stripTabs, ok := heredocOperator(sub.input, tok)
		if !ok {
			continue
		}

		delim, quoted, delimEnd, ok := sub.scanDelimiter()
		if !ok {
			// `<<` with nothing usable after it on the same line. Leave the
			// diagnostic to the parser rather than inventing an empty body.
			continue
		}

		start := max(bodyCursor, lineStartAfter(sub.input, delimEnd))
		body, end, terminated := collectBody(sub.input, start, delim, stripTabs)

		sub.heredocs[tok.Offset] = &Heredoc{
			Delimiter:  delim,
			Body:       body,
			Quoted:     quoted,
			StripTabs:  stripTabs,
			DelimEnd:   delimEnd,
			Terminated: terminated,
		}
		if start < end {
			sub.skips = append(sub.skips, heredocSkip{start, end})
		}
		bodyCursor = end
	}

	l.heredocs, l.skips = sub.heredocs, sub.skips
}

// heredocOperator reports whether tok opens a heredoc, and whether it is the
// <<- form. It accepts both the bare `<<` (an OUT token) and an fd-prefixed
// `0<<` (a REDIR token). A `<<<` here-string is not a heredoc; the lexer splits
// it into `<<` followed by `<`, so it is rejected by looking at the next byte.
func heredocOperator(input string, tok token.Token) (stripTabs, ok bool) {
	lit := tok.Literal
	switch tok.Type {
	case token.OUT:
	case token.REDIR:
		i := 0
		for i < len(lit) && lit[i] >= '0' && lit[i] <= '9' {
			i++
		}
		lit = lit[i:]
	default:
		return false, false
	}
	if !strings.HasPrefix(lit, "<<") {
		return false, false
	}
	rest := lit[2:]
	if strings.HasPrefix(rest, "<") || (rest == "" && tok.End < len(input) && input[tok.End] == '<') {
		return false, false
	}
	return rest == "-", true
}

// scanDelimiter reads the delimiter word that follows a heredoc operator. The
// word runs to the next whitespace, so pieces glued together (EOF.txt, E'O'F)
// join into one delimiter. Any quoting of it — <<'EOF', <<"EOF", or <<\EOF —
// marks the body as literal, and is stripped from the delimiter itself.
func (l *Lexer) scanDelimiter() (delim string, quoted bool, end int, ok bool) {
	var b strings.Builder
	for first := true; ; first = false {
		save := l.pos
		tok := l.NextToken()
		if tok.Type == token.EOF || tok.PrecededByNewline {
			l.pos = save
			break
		}
		if !first && tok.PrecededByWhitespace {
			l.pos = save
			break
		}
		// A leading backslash quotes the delimiter (<<\EOF) without being part
		// of it. The lexer has no escape token, so it arrives as ILLEGAL.
		if first && tok.Type == token.ILLEGAL && tok.Literal == `\` {
			quoted = true
			end = tok.End
			continue
		}
		if tok.Type == token.STRING {
			quoted = true
			b.WriteString(tok.Literal)
		} else {
			b.WriteString(l.input[tok.Offset:tok.End])
		}
		end = tok.End
	}
	if b.Len() == 0 {
		return "", false, 0, false
	}
	return b.String(), quoted, end, true
}

// collectBody gathers heredoc lines from start until a line that is exactly the
// delimiter, returning the body, the offset just past the delimiter line, and
// whether that delimiter was actually found. Every body line is newline
// terminated, including the last one, matching how a shell feeds a heredoc to
// its command.
func collectBody(input string, start int, delim string, stripTabs bool) (body string, end int, terminated bool) {
	var b strings.Builder
	i := start
	for i < len(input) {
		line, next := input[i:], len(input)
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line, next = line[:j], i+j+1
		}
		if stripTabs {
			line = strings.TrimLeft(line, "\t")
		}
		if strings.TrimSuffix(line, "\r") == delim {
			return b.String(), next, true
		}
		b.WriteString(line)
		b.WriteByte('\n')
		i = next
	}
	return b.String(), len(input), false
}

// lineStartAfter returns the offset of the first character on the line
// following the one containing pos.
func lineStartAfter(input string, pos int) int {
	if j := strings.IndexByte(input[pos:], '\n'); j >= 0 {
		return pos + j + 1
	}
	return len(input)
}

// heredocSkipAt reports the offset to jump to when pos falls inside a collected
// heredoc body.
func (l *Lexer) heredocSkipAt(pos int) (int, bool) {
	for _, s := range l.skips {
		if pos >= s.start && pos < s.end {
			return s.end, true
		}
	}
	return 0, false
}

// HeredocAt returns the heredoc collected for the operator at the given byte
// offset, or nil if that operator does not open one.
func (l *Lexer) HeredocAt(offset int) *Heredoc {
	return l.heredocs[offset]
}

// UnterminatedHeredoc reports whether the input opens a heredoc whose delimiter
// line never arrived. The REPL uses it to keep prompting for the body instead
// of running a half-written command.
func (l *Lexer) UnterminatedHeredoc() bool {
	for _, h := range l.heredocs {
		if !h.Terminated {
			return true
		}
	}
	return false
}
