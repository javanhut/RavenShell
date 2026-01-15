package ast

import (
	"ravenshell/token"
	"testing"
)

func TestProgramString(t *testing.T) {
	program := &Program{
		Statements: []Statement{
			&ExpressionStatement{
				Token: token.Token{Type: token.IDENT, Literal: "ls"},
				Expression: &Command{
					Token: token.Token{Type: token.IDENT, Literal: "ls"},
					Type:  CMD_LIST,
					Name:  "ls",
					Arguments: []Expression{
						&Identifier{
							Token: token.Token{Type: token.IDENT, Literal: "dir"},
							Value: "dir",
						},
					},
				},
			},
		},
	}

	if program.String() != "ls dir" {
		t.Errorf("program.String() wrong. got=%q", program.String())
	}
}

func TestCommandString(t *testing.T) {
	cmd := &Command{
		Token: token.Token{Type: token.IDENT, Literal: "rm"},
		Type:  CMD_REMOVE,
		Name:  "rm",
		Arguments: []Expression{
			&Identifier{Token: token.Token{Type: token.IDENT, Literal: "file1"}, Value: "file1"},
			&Identifier{Token: token.Token{Type: token.IDENT, Literal: "file2"}, Value: "file2"},
		},
	}

	expected := "rm file1 file2"
	if cmd.String() != expected {
		t.Errorf("cmd.String() wrong. expected=%q, got=%q", expected, cmd.String())
	}
}

func TestPipeExpressionString(t *testing.T) {
	pipe := &PipeExpression{
		Token: token.Token{Type: token.PIPE, Literal: "|"},
		Left: &Command{
			Token: token.Token{Type: token.IDENT, Literal: "ls"},
			Type:  CMD_LIST,
			Name:  "ls",
		},
		Right: &Command{
			Token: token.Token{Type: token.IDENT, Literal: "print"},
			Type:  CMD_PRINT,
			Name:  "print",
		},
	}

	expected := "(ls | print)"
	if pipe.String() != expected {
		t.Errorf("pipe.String() wrong. expected=%q, got=%q", expected, pipe.String())
	}
}

func TestRedirectionExpressionString(t *testing.T) {
	tests := []struct {
		redirType RedirectionType
		expected  string
	}{
		{REDIR_OUTPUT, "(ls > out.txt)"},
		{REDIR_APPEND, "(ls >> out.txt)"},
		{REDIR_INPUT, "(ls < out.txt)"},
		{REDIR_HEREDOC, "(ls << out.txt)"},
	}

	for _, tt := range tests {
		redir := &RedirectionExpression{
			Token: token.Token{Type: token.GREATER, Literal: string(tt.redirType)},
			Type:  tt.redirType,
			Command: &Command{
				Token: token.Token{Type: token.IDENT, Literal: "ls"},
				Type:  CMD_LIST,
				Name:  "ls",
			},
			Target: &Identifier{
				Token: token.Token{Type: token.IDENT, Literal: "out.txt"},
				Value: "out.txt",
			},
		}

		if redir.String() != tt.expected {
			t.Errorf("redir.String() wrong. expected=%q, got=%q", tt.expected, redir.String())
		}
	}
}

func TestPathExpressionString(t *testing.T) {
	tests := []struct {
		value    string
		expected string
	}{
		{".", "."},
		{"./foo", "./foo"},
		{"../bar", "../bar"},
		{"/home/user", "/home/user"},
		{"foo/bar/baz", "foo/bar/baz"},
	}

	for _, tt := range tests {
		path := &PathExpression{
			Token: token.Token{Type: token.FULLSTOP, Literal: "."},
			Value: tt.value,
		}

		if path.String() != tt.expected {
			t.Errorf("path.String() wrong. expected=%q, got=%q", tt.expected, path.String())
		}
	}
}

func TestVariableReferenceString(t *testing.T) {
	varRef := &VariableReference{
		Token: token.Token{Type: token.DOLLAR, Literal: "$"},
		Name: &Identifier{
			Token: token.Token{Type: token.IDENT, Literal: "HOME"},
			Value: "HOME",
		},
	}

	expected := "$HOME"
	if varRef.String() != expected {
		t.Errorf("varRef.String() wrong. expected=%q, got=%q", expected, varRef.String())
	}
}

func TestStringLiteralString(t *testing.T) {
	strLit := &StringLiteral{
		Token: token.Token{Type: token.STRING, Literal: "hello world"},
		Value: "hello world",
	}

	expected := `"hello world"`
	if strLit.String() != expected {
		t.Errorf("strLit.String() wrong. expected=%q, got=%q", expected, strLit.String())
	}
}

func TestIntegerLiteralString(t *testing.T) {
	intLit := &IntegerLiteral{
		Token: token.Token{Type: token.INTEGER, Literal: "42"},
		Value: 42,
	}

	expected := "42"
	if intLit.String() != expected {
		t.Errorf("intLit.String() wrong. expected=%q, got=%q", expected, intLit.String())
	}
}

func TestIdentifierString(t *testing.T) {
	ident := &Identifier{
		Token: token.Token{Type: token.IDENT, Literal: "filename"},
		Value: "filename",
	}

	expected := "filename"
	if ident.String() != expected {
		t.Errorf("ident.String() wrong. expected=%q, got=%q", expected, ident.String())
	}
}

func TestTokenLiteralMethods(t *testing.T) {
	// Test that TokenLiteral returns the correct values
	cmd := &Command{
		Token: token.Token{Type: token.IDENT, Literal: "ls"},
		Type:  CMD_LIST,
		Name:  "ls",
	}
	if cmd.TokenLiteral() != "ls" {
		t.Errorf("cmd.TokenLiteral() wrong. got=%q", cmd.TokenLiteral())
	}

	ident := &Identifier{
		Token: token.Token{Type: token.IDENT, Literal: "file"},
		Value: "file",
	}
	if ident.TokenLiteral() != "file" {
		t.Errorf("ident.TokenLiteral() wrong. got=%q", ident.TokenLiteral())
	}

	pipe := &PipeExpression{
		Token: token.Token{Type: token.PIPE, Literal: "|"},
	}
	if pipe.TokenLiteral() != "|" {
		t.Errorf("pipe.TokenLiteral() wrong. got=%q", pipe.TokenLiteral())
	}
}

func TestEmptyProgram(t *testing.T) {
	program := &Program{
		Statements: []Statement{},
	}

	if program.TokenLiteral() != "" {
		t.Errorf("empty program TokenLiteral should be empty. got=%q", program.TokenLiteral())
	}

	if program.String() != "" {
		t.Errorf("empty program String should be empty. got=%q", program.String())
	}
}

func TestExpressionStatementWithNilExpression(t *testing.T) {
	stmt := &ExpressionStatement{
		Token:      token.Token{Type: token.IDENT, Literal: "test"},
		Expression: nil,
	}

	if stmt.String() != "" {
		t.Errorf("ExpressionStatement with nil Expression should return empty string. got=%q", stmt.String())
	}
}
