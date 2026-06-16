package lexer

import (
	"ravenshell/token"
	"testing"
)

func TestNextTokenBasics(t *testing.T) {
	input := `x = 10 + 5 * 2 == 20`
	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.IDENT, "x"},
		{token.ASSIGN, "="},
		{token.INTEGER, "10"},
		{token.PLUS, "+"},
		{token.INTEGER, "5"},
		{token.ASTERISK, "*"},
		{token.INTEGER, "2"},
		{token.EQ, "=="},
		{token.INTEGER, "20"},
		{token.EOF, ""},
	}

	l := NewLexer(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - wrong type. got=%q, want=%q", i, tok.Type, tt.expectedType)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - wrong literal. got=%q, want=%q", i, tok.Literal, tt.expectedLiteral)
		}
	}
}

func TestFlagTokens(t *testing.T) {
	input := `ls -l --all --max-count=5`
	want := []struct {
		typ token.TokenType
		lit string
	}{
		{token.LIST, "ls"},
		{token.FLAG, "-l"},
		{token.FLAG, "--all"},
		{token.FLAG, "--max-count=5"},
		{token.EOF, ""},
	}

	l := NewLexer(input)
	for i, w := range want {
		tok := l.NextToken()
		if tok.Type != w.typ || tok.Literal != w.lit {
			t.Fatalf("tests[%d] - got {%q %q}, want {%q %q}", i, tok.Type, tok.Literal, w.typ, w.lit)
		}
	}
}

func TestMinusVsFlag(t *testing.T) {
	// Spaces around '-' keep it as subtraction; '-' glued to a letter is a flag.
	l := NewLexer("5 - 3")
	l.NextToken() // 5
	tok := l.NextToken()
	if tok.Type != token.MINUS {
		t.Fatalf("expected MINUS for spaced '-', got %q", tok.Type)
	}

	l2 := NewLexer("-rf")
	tok2 := l2.NextToken()
	if tok2.Type != token.FLAG || tok2.Literal != "-rf" {
		t.Fatalf("expected FLAG -rf, got {%q %q}", tok2.Type, tok2.Literal)
	}
}

func TestHyphenatedIdentifiers(t *testing.T) {
	// Hyphens join words inside a command name, but a spaced flag stays a flag.
	l := NewLexer("docker-compose up")
	tok := l.NextToken()
	if tok.Type != token.IDENT || tok.Literal != "docker-compose" {
		t.Fatalf("got {%q %q}, want IDENT docker-compose", tok.Type, tok.Literal)
	}
	if tok2 := l.NextToken(); tok2.Type != token.IDENT || tok2.Literal != "up" {
		t.Fatalf("got {%q %q}, want IDENT up", tok2.Type, tok2.Literal)
	}

	// raven-add lexes as a single token so it can be a keyword command.
	l2 := NewLexer("raven-add path /opt/bin")
	if tok := l2.NextToken(); tok.Literal != "raven-add" {
		t.Fatalf("got %q, want raven-add", tok.Literal)
	}

	// A flag still lexes as a flag after a space.
	l3 := NewLexer("tool -l")
	l3.NextToken() // tool
	if tok := l3.NextToken(); tok.Type != token.FLAG || tok.Literal != "-l" {
		t.Fatalf("got {%q %q}, want FLAG -l", tok.Type, tok.Literal)
	}
}

func TestPrecededByNewline(t *testing.T) {
	input := "print x\ngit status"
	types := []token.TokenType{token.PRINT, token.IDENT, token.IDENT, token.IDENT, token.EOF}
	wantNL := []bool{false, false, true, false, false}

	l := NewLexer(input)
	for i := range types {
		tok := l.NextToken()
		if tok.Type != types[i] {
			t.Fatalf("tests[%d] - wrong type. got=%q, want=%q", i, tok.Type, types[i])
		}
		if tok.PrecededByNewline != wantNL[i] {
			t.Fatalf("tests[%d] (%q) - PrecededByNewline got=%v, want=%v", i, tok.Literal, tok.PrecededByNewline, wantNL[i])
		}
	}
}

func TestCommentsAndNewlines(t *testing.T) {
	input := "x = 1 # a comment\ny = 2"
	l := NewLexer(input)
	var got []token.TokenType
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		got = append(got, tok.Type)
	}
	want := []token.TokenType{
		token.IDENT, token.ASSIGN, token.INTEGER,
		token.IDENT, token.ASSIGN, token.INTEGER,
	}
	if len(got) != len(want) {
		t.Fatalf("token count got=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tests[%d] got=%q want=%q", i, got[i], want[i])
		}
	}
}

// TestURLTokens guards against the bug where ':' lexed as ILLEGAL, breaking
// commands like `git clone https://...`. The colon must produce a COLON token.
func TestURLTokens(t *testing.T) {
	input := `git clone https://github.com/javanhut/RavenShell.git`
	want := []token.TokenType{
		token.IDENT,  // git
		token.IDENT,  // clone
		token.IDENT,  // https
		token.COLON,  // :
		token.FSLASH, // /
		token.FSLASH, // /
		token.IDENT,  // github
		token.FULLSTOP,
		token.IDENT, // com
		token.FSLASH,
		token.IDENT, // javanhut
		token.FSLASH,
		token.IDENT, // RavenShell
		token.FULLSTOP,
		token.IDENT, // git
	}
	l := NewLexer(input)
	for i, w := range want {
		tok := l.NextToken()
		if tok.Type == token.ILLEGAL {
			t.Fatalf("tests[%d] - got ILLEGAL (%q); ':' must not lex as ILLEGAL", i, tok.Literal)
		}
		if tok.Type != w {
			t.Fatalf("tests[%d] - got=%q (%q), want=%q", i, tok.Type, tok.Literal, w)
		}
	}
}

// TestScpRemoteTokens guards the scp-style remote form git@host:repo.git: both
// '@' and ':' must lex as real tokens (regression: each used to be ILLEGAL).
func TestScpRemoteTokens(t *testing.T) {
	input := `git@github.com:repo.git`
	want := []token.TokenType{
		token.IDENT, // git
		token.AT,    // @
		token.IDENT, // github
		token.FULLSTOP,
		token.IDENT, // com
		token.COLON, // :
		token.IDENT, // repo
		token.FULLSTOP,
		token.IDENT, // git
	}
	l := NewLexer(input)
	for i, w := range want {
		tok := l.NextToken()
		if tok.Type == token.ILLEGAL {
			t.Fatalf("tests[%d] - got ILLEGAL (%q)", i, tok.Literal)
		}
		if tok.Type != w {
			t.Fatalf("tests[%d] - got=%q (%q), want=%q", i, tok.Type, tok.Literal, w)
		}
	}
}

// TestLineContinuation verifies that a backslash before a newline splices the
// lines into one logical line: the tokens on both sides are produced and the
// token after the continuation is NOT marked as starting a new line (so it stays
// part of the same command's argument list).
func TestLineContinuation(t *testing.T) {
	l := NewLexer("echo a \\\n  b")
	var got []string
	var bTok token.Token
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		if tok.Type == token.ILLEGAL {
			t.Fatalf("backslash continuation produced ILLEGAL (%q)", tok.Literal)
		}
		got = append(got, tok.Literal)
		if tok.Literal == "b" {
			bTok = tok
		}
	}
	want := []string{"echo", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got tokens %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	// The spliced continuation must not look like a line boundary.
	if bTok.PrecededByNewline {
		t.Errorf("token after line continuation marked PrecededByNewline; it should be spliced onto the same line")
	}
}
