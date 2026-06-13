package lexer

import (
	"ravenshell/token"
	"testing"
)

// tokenize collects every token (excluding the trailing EOF) produced for src.
func tokenize(src string) []token.Token {
	l := NewLexer(src)
	var toks []token.Token
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		toks = append(toks, tok)
	}
	return toks
}

// TestNumberAndIdentifierBoundaries documents how the lexer classifies words
// that mix letters and digits. These boundaries are exactly why some filenames
// need quoting (a digit-leading name lexes as INTEGER + IDENT), so the parser's
// path logic is tested against this ground truth.
func TestNumberAndIdentifierBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		want []token.TokenType
	}{
		// Letter-led words absorb digits: one identifier.
		{"report2024", []token.TokenType{token.IDENT}},
		{"v1", []token.TokenType{token.IDENT}},
		// Pure digits are one integer.
		{"2024", []token.TokenType{token.INTEGER}},
		// Digit-led mixed word splits: INTEGER then IDENT (the known quirk).
		{"2024report", []token.TokenType{token.INTEGER, token.IDENT}},
		// Underscores and interior hyphens stay inside one identifier.
		{"my_file", []token.TokenType{token.IDENT}},
		{"docker-compose", []token.TokenType{token.IDENT}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			toks := tokenize(tc.in)
			if len(toks) != len(tc.want) {
				t.Fatalf("%q -> %d tokens, want %d (%v)", tc.in, len(toks), len(tc.want), types(toks))
			}
			for i, w := range tc.want {
				if toks[i].Type != w {
					t.Errorf("%q token[%d] = %s, want %s", tc.in, i, toks[i].Type, w)
				}
			}
		})
	}
}

// TestKeywordVsIdentifier checks that reserved words lex as their keyword token
// even when they appear bare, while non-reserved words are identifiers. This is
// the ground truth behind the parser needing to treat keyword tokens as valid
// path segments (e.g. the 'env' in '.env').
func TestKeywordVsIdentifier(t *testing.T) {
	keywords := map[string]token.TokenType{
		"env":    token.ENV,
		"output": token.OUTPUT,
		"print":  token.PRINT,
		"ls":     token.LIST,
		"rm":     token.REMOVE,
		"read":   token.SHOW,
		"for":    token.FOR,
		"in":     token.IN,
	}
	for word, want := range keywords {
		toks := tokenize(word)
		if len(toks) != 1 || toks[0].Type != want {
			t.Errorf("%q -> %v, want single %s", word, types(toks), want)
		}
	}

	for _, word := range []string{"foobar", "myfile", "configg"} {
		toks := tokenize(word)
		if len(toks) != 1 || toks[0].Type != token.IDENT {
			t.Errorf("%q -> %v, want single IDENT", word, types(toks))
		}
	}
}

// TestPathLikeTokenStream verifies the raw token stream for a tricky path: the
// lexer emits dots, slashes, integers, and keyword tokens as separate pieces —
// it is the parser's job to glue them back into one path. This pins down the
// inputs the parser path tests rely on.
func TestPathLikeTokenStream(t *testing.T) {
	toks := tokenize("dir/output.tar.2.gz")
	want := []token.TokenType{
		token.IDENT,    // dir
		token.FSLASH,   // /
		token.OUTPUT,   // output (reserved word)
		token.FULLSTOP, // .
		token.IDENT,    // tar
		token.FULLSTOP, // .
		token.INTEGER,  // 2
		token.FULLSTOP, // .
		token.IDENT,    // gz
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens (%v), want %d", len(toks), types(toks), len(want))
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("token[%d] = %s, want %s", i, toks[i].Type, w)
		}
	}
}

// TestWhitespaceFlags checks the adjacency flags the parser uses to decide
// whether neighboring tokens join into a path or are separate arguments.
func TestWhitespaceFlags(t *testing.T) {
	// "a/b" — no whitespace between pieces.
	toks := tokenize("a/b")
	if toks[1].PrecededByWhitespace {
		t.Error("'/' in a/b should not be flagged PrecededByWhitespace")
	}
	// "a /b" — the '/' has whitespace before it, so it starts a new argument.
	toks = tokenize("a /b")
	if !toks[1].PrecededByWhitespace {
		t.Error("'/' in 'a /b' should be flagged PrecededByWhitespace")
	}
}

// TestColonAtWordTokens verifies that ':' and '@' are scanned as part of a
// bareword instead of falling through to an ILLEGAL token. This is the lexer
// half of the docker-image-tag parse-error fix.
func TestColonAtWordTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []token.Token
	}{
		// Tags and host:port stay one IDENT.
		{"image:tag", []token.Token{{Type: token.IDENT, Literal: "image:tag"}}},
		{"localhost:8080", []token.Token{{Type: token.IDENT, Literal: "localhost:8080"}}},
		// Digit-led port mapping becomes an IDENT (not an INTEGER).
		{"8080:80", []token.Token{{Type: token.IDENT, Literal: "8080:80"}}},
		// user@host and scope words.
		{"user@host", []token.Token{{Type: token.IDENT, Literal: "user@host"}}},
		// scheme://host splits into IDENT + slashes for the parser to rejoin.
		{"http://x", []token.Token{
			{Type: token.IDENT, Literal: "http:"},
			{Type: token.FSLASH, Literal: "/"},
			{Type: token.FSLASH, Literal: "/"},
			{Type: token.IDENT, Literal: "x"},
		}},
		// A lone ':' no longer lexes as ILLEGAL.
		{":", []token.Token{{Type: token.IDENT, Literal: ":"}}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := tokenize(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("%q -> %v, want %d tokens", tc.in, types(got), len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Type != w.Type || got[i].Literal != w.Literal {
					t.Errorf("%q token[%d] = {%q %q}, want {%q %q}",
						tc.in, i, got[i].Type, got[i].Literal, w.Type, w.Literal)
				}
			}
		})
	}
}

func types(toks []token.Token) []token.TokenType {
	out := make([]token.TokenType, len(toks))
	for i, tk := range toks {
		out[i] = tk.Type
	}
	return out
}
