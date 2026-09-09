package parser

import (
	"ravenshell/lexer"
	"strings"
	"testing"
)

// Parse errors name what the user typed, not the parser's token vocabulary.
func TestParseErrorsAreReadable(t *testing.T) {
	cases := []struct{ input, want string }{
		{"for i in range(3): print(i)", "expected '{', found ':'"},
		{`print "hi".upper()`, "unexpected ')'; expected a value or expression"},
		{"x = 2 ** 3", "unexpected '*'; expected a value or expression"},
		{"x = 5; print x > 3", "expected a file name after the redirection, found number 3"},
		{"if x > 1 {", "expected '}' to close the block, found end of input"},
		{"echo $", "expected a variable name after '$', found end of input"},
		{"print(1, 2)", "expected ')', found ','"},
	}
	for _, c := range cases {
		p := New(lexer.NewLexer(c.input))
		p.ParseProgram()
		errs := p.Errors()
		if len(errs) == 0 {
			t.Errorf("input %q: expected a parse error, got none", c.input)
			continue
		}
		if !strings.Contains(errs[0], c.want) {
			t.Errorf("input %q: first error = %q, want it to contain %q", c.input, errs[0], c.want)
		}
		for _, e := range errs {
			for _, leak := range []string{"prefix parse function", "LBRACE", "RPAREN", "INTEGER", "EOF", "COLON"} {
				if strings.Contains(e, leak) {
					t.Errorf("input %q: error %q leaks token name %q", c.input, e, leak)
				}
			}
		}
	}
}

// An empty array literal takes a type hint only when it is glued ([]string);
// a name on the next line is a new statement.
func TestEmptyArrayHintMustBeGlued(t *testing.T) {
	for _, in := range []string{"arr = []\narr = 1\nprint arr\n", "arr = []\nprint arr\n", "a = [] \nb = 2\n"} {
		p := New(lexer.NewLexer(in))
		p.ParseProgram()
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("input %q: unexpected parse errors %v", in, errs)
		}
	}
	p := New(lexer.NewLexer("items = []string\nprint len(items)\n"))
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("glued hint: unexpected parse errors %v", errs)
	}
	if len(prog.Statements) != 2 {
		t.Errorf("glued hint: got %d statements, want 2", len(prog.Statements))
	}
}
