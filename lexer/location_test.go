package lexer

import "testing"

func TestTokenLocations(t *testing.T) {
	l := NewLexer("first\n  second")
	first := l.NextToken()
	second := l.NextToken()
	if first.Line != 1 || first.Column != 1 || first.Offset != 0 {
		t.Fatalf("first token location = %d:%d offset %d", first.Line, first.Column, first.Offset)
	}
	if second.Line != 2 || second.Column != 3 || second.Offset != 8 {
		t.Fatalf("second token location = %d:%d offset %d", second.Line, second.Column, second.Offset)
	}
}
