package parser

import (
	"ravenshell/ast"
	"ravenshell/lexer"
	"testing"
)

func TestSimpleCommand(t *testing.T) {
	input := "ls"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program has wrong number of statements. got=%d",
			len(program.Statements))
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0])
	}

	cmd, ok := stmt.Expression.(*ast.Command)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.Command. got=%T", stmt.Expression)
	}

	if cmd.Type != ast.CMD_LIST {
		t.Errorf("cmd.Type is not CMD_LIST. got=%s", cmd.Type)
	}

	if cmd.Name != "ls" {
		t.Errorf("cmd.Name is not 'ls'. got=%s", cmd.Name)
	}
}

func TestAllCommands(t *testing.T) {
	tests := []struct {
		input       string
		cmdType     ast.CommandType
		commandName string
	}{
		{"ls", ast.CMD_LIST, "ls"},
		{"rm", ast.CMD_REMOVE, "rm"},
		{"cd", ast.CMD_CHANGEDIR, "cd"},
		{"mkdir", ast.CMD_MAKEDIR, "mkdir"},
		{"rmdir", ast.CMD_REMOVEDIR, "rmdir"},
		{"whoami", ast.CMD_WHOAMI, "whoami"},
		{"cwd", ast.CMD_CURRENTDIR, "cwd"},
		{"mkfile", ast.CMD_MAKEFILE, "mkfile"},
		{"output", ast.CMD_OUTPUT, "output"},
		{"print", ast.CMD_PRINT, "print"},
	}

	for _, tt := range tests {
		l := lexer.NewLexer(tt.input)
		p := New(l)
		program := p.ParseProgram()
		checkParserErrors(t, p)

		stmt := program.Statements[0].(*ast.ExpressionStatement)
		cmd := stmt.Expression.(*ast.Command)

		if cmd.Type != tt.cmdType {
			t.Errorf("for input %q: cmd.Type wrong. expected=%s, got=%s",
				tt.input, tt.cmdType, cmd.Type)
		}

		if cmd.Name != tt.commandName {
			t.Errorf("for input %q: cmd.Name wrong. expected=%s, got=%s",
				tt.input, tt.commandName, cmd.Name)
		}
	}
}

func TestCommandWithArguments(t *testing.T) {
	input := "rm file1 file2"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd, ok := stmt.Expression.(*ast.Command)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.Command. got=%T", stmt.Expression)
	}

	if len(cmd.Arguments) != 2 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testIdentifier(t, cmd.Arguments[0], "file1")
	testIdentifier(t, cmd.Arguments[1], "file2")
}

func TestFileWithExtension(t *testing.T) {
	input := "rm test.txt"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. expected=1, got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "test.txt")
}

func TestHiddenFile(t *testing.T) {
	input := "rm .gitignore"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. expected=1, got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], ".gitignore")
}

func TestPathWithFileExtension(t *testing.T) {
	input := "rm ./src/main.go"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. expected=1, got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "./src/main.go")
}

func TestMultipleFilesWithExtensions(t *testing.T) {
	input := "rm file1.txt file2.go"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 2 {
		t.Fatalf("wrong number of arguments. expected=2, got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "file1.txt")
	testPath(t, cmd.Arguments[1], "file2.go")
}

func TestCommandWithStringArgument(t *testing.T) {
	input := `print "hello world"`
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	strLit, ok := cmd.Arguments[0].(*ast.StringLiteral)
	if !ok {
		t.Fatalf("argument is not StringLiteral. got=%T", cmd.Arguments[0])
	}

	if strLit.Value != "hello world" {
		t.Errorf("string value wrong. expected=%s, got=%s", "hello world", strLit.Value)
	}
}

func TestPipeExpression(t *testing.T) {
	input := "ls | print"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	pipe, ok := stmt.Expression.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.PipeExpression. got=%T",
			stmt.Expression)
	}

	testCommand(t, pipe.Left, ast.CMD_LIST)
	testCommand(t, pipe.Right, ast.CMD_PRINT)
}

func TestChainedPipes(t *testing.T) {
	input := "ls | print | output"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	pipe, ok := stmt.Expression.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.PipeExpression. got=%T",
			stmt.Expression)
	}

	// Should parse as (ls | print) | output due to left associativity
	leftPipe, ok := pipe.Left.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("pipe.Left is not PipeExpression. got=%T", pipe.Left)
	}

	testCommand(t, leftPipe.Left, ast.CMD_LIST)
	testCommand(t, leftPipe.Right, ast.CMD_PRINT)
	testCommand(t, pipe.Right, ast.CMD_OUTPUT)
}

func TestRedirectionOutput(t *testing.T) {
	input := "ls > out.txt"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	redir, ok := stmt.Expression.(*ast.RedirectionExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.RedirectionExpression. got=%T",
			stmt.Expression)
	}

	if redir.Type != ast.REDIR_OUTPUT {
		t.Errorf("wrong redirection type. expected=%s, got=%s",
			ast.REDIR_OUTPUT, redir.Type)
	}

	testCommand(t, redir.Command, ast.CMD_LIST)
	testPath(t, redir.Target, "out.txt")
}

func TestRedirectionAppend(t *testing.T) {
	input := "ls >> out.txt"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	redir, ok := stmt.Expression.(*ast.RedirectionExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.RedirectionExpression. got=%T",
			stmt.Expression)
	}

	if redir.Type != ast.REDIR_APPEND {
		t.Errorf("wrong redirection type. expected=%s, got=%s",
			ast.REDIR_APPEND, redir.Type)
	}
}

func TestRedirectionInput(t *testing.T) {
	input := "print < input.txt"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	redir, ok := stmt.Expression.(*ast.RedirectionExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.RedirectionExpression. got=%T",
			stmt.Expression)
	}

	if redir.Type != ast.REDIR_INPUT {
		t.Errorf("wrong redirection type. expected=%s, got=%s",
			ast.REDIR_INPUT, redir.Type)
	}
}

func TestRedirectionHeredoc(t *testing.T) {
	input := "print << EOF"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	redir, ok := stmt.Expression.(*ast.RedirectionExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not ast.RedirectionExpression. got=%T",
			stmt.Expression)
	}

	if redir.Type != ast.REDIR_HEREDOC {
		t.Errorf("wrong redirection type. expected=%s, got=%s",
			ast.REDIR_HEREDOC, redir.Type)
	}

	testCommand(t, redir.Command, ast.CMD_PRINT)
	testIdentifier(t, redir.Target, "EOF")
}

func TestVariableReference(t *testing.T) {
	input := "cd $HOME"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	varRef, ok := cmd.Arguments[0].(*ast.VariableReference)
	if !ok {
		t.Fatalf("argument is not VariableReference. got=%T", cmd.Arguments[0])
	}

	if varRef.Name.Value != "HOME" {
		t.Errorf("variable name wrong. got=%s", varRef.Name.Value)
	}
}

func TestPipeWithRedirection(t *testing.T) {
	input := "ls | print > output.txt"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)

	// Due to precedence, pipe binds tighter than redirection
	// So this should parse as (ls | print) > output.txt
	redir, ok := stmt.Expression.(*ast.RedirectionExpression)
	if !ok {
		t.Fatalf("stmt.Expression is not RedirectionExpression. got=%T",
			stmt.Expression)
	}

	pipe, ok := redir.Command.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("redir.Command is not PipeExpression. got=%T", redir.Command)
	}

	testCommand(t, pipe.Left, ast.CMD_LIST)
	testCommand(t, pipe.Right, ast.CMD_PRINT)
}

func TestCommandWithDot(t *testing.T) {
	input := "cd ."
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], ".")
}

func TestRelativePath(t *testing.T) {
	input := "cd ./foo"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "./foo")
}

func TestParentPath(t *testing.T) {
	input := "cd ../bar"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "../bar")
}

func TestAbsolutePath(t *testing.T) {
	input := "cd /home/user"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "/home/user")
}

func TestPathWithIdentifier(t *testing.T) {
	input := "cd foo/bar/baz"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd := stmt.Expression.(*ast.Command)

	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong number of arguments. got=%d", len(cmd.Arguments))
	}

	testPath(t, cmd.Arguments[0], "foo/bar/baz")
}

func TestIdentifierAsExpression(t *testing.T) {
	input := "somefile"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	ident, ok := stmt.Expression.(*ast.Identifier)
	if !ok {
		t.Fatalf("stmt.Expression is not Identifier. got=%T", stmt.Expression)
	}

	if ident.Value != "somefile" {
		t.Errorf("identifier value wrong. got=%s", ident.Value)
	}
}

func TestIntegerLiteral(t *testing.T) {
	input := "123"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	intLit, ok := stmt.Expression.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("stmt.Expression is not IntegerLiteral. got=%T", stmt.Expression)
	}

	if intLit.Value != 123 {
		t.Errorf("integer value wrong. got=%d", intLit.Value)
	}
}

// Helper functions

func checkParserErrors(t *testing.T, p *Parser) {
	errors := p.Errors()
	if len(errors) == 0 {
		return
	}

	t.Errorf("parser has %d errors", len(errors))
	for _, msg := range errors {
		t.Errorf("parser error: %q", msg)
	}
	t.FailNow()
}

func testCommand(t *testing.T, exp ast.Expression, expectedType ast.CommandType) {
	t.Helper()
	cmd, ok := exp.(*ast.Command)
	if !ok {
		t.Errorf("exp not *ast.Command. got=%T", exp)
		return
	}
	if cmd.Type != expectedType {
		t.Errorf("cmd.Type not %s. got=%s", expectedType, cmd.Type)
	}
}

func testIdentifier(t *testing.T, exp ast.Expression, expectedValue string) {
	t.Helper()
	ident, ok := exp.(*ast.Identifier)
	if !ok {
		t.Errorf("exp not *ast.Identifier. got=%T", exp)
		return
	}
	if ident.Value != expectedValue {
		t.Errorf("ident.Value not %s. got=%s", expectedValue, ident.Value)
	}
}

func testPath(t *testing.T, exp ast.Expression, expectedValue string) {
	t.Helper()
	path, ok := exp.(*ast.PathExpression)
	if !ok {
		t.Errorf("exp not *ast.PathExpression. got=%T", exp)
		return
	}
	if path.Value != expectedValue {
		t.Errorf("path.Value not %s. got=%s", expectedValue, path.Value)
	}
}
