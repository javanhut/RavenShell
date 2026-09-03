package parser

import (
	"ravenshell/ast"
	"ravenshell/lexer"
	"strings"
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
		{"raven-add", ast.CMD_RAVENADD, "raven-add"},
		{"raven-update", ast.CMD_RAVENUPDATE, "raven-update"},
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

func TestBareWordIsExternalCommand(t *testing.T) {
	// A bare identifier at the start of a statement is run as an external
	// command (e.g. typing `git` or `somefile` at the prompt).
	input := "somefile"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd, ok := stmt.Expression.(*ast.Command)
	if !ok {
		t.Fatalf("stmt.Expression is not Command. got=%T", stmt.Expression)
	}

	if cmd.Type != ast.CMD_EXTERNAL {
		t.Errorf("command type wrong. got=%s, want=%s", cmd.Type, ast.CMD_EXTERNAL)
	}
	if cmd.Name != "somefile" {
		t.Errorf("command name wrong. got=%s", cmd.Name)
	}
}

func TestIdentifierAsValue(t *testing.T) {
	// An identifier used as a value (not in command position) stays an
	// Identifier, e.g. the right-hand side of an assignment.
	input := "x = somevar"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.AssignmentStatement)
	ident, ok := stmt.Value.(*ast.Identifier)
	if !ok {
		t.Fatalf("stmt.Value is not Identifier. got=%T", stmt.Value)
	}

	if ident.Value != "somevar" {
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

// TestUnclosedBlockReportsError checks that a block left open at EOF is a parse
// error rather than being silently accepted (which would absorb any following
// statements into the body).
func TestUnclosedBlockReportsError(t *testing.T) {
	inputs := []string{
		"for i in range(3) { print i",
		"if 1 == 1 { print HI",
		"while 1 == 1 { print x",
		"fn greet() { print hi",
		"if 1 == 1 { print a } else { print b",      // unclosed else block
		"for i in range(2) {\nprint i\nprint after", // following stmt absorbed
	}
	for _, in := range inputs {
		p := New(lexer.NewLexer(in))
		p.ParseProgram()
		if len(p.Errors()) == 0 {
			t.Errorf("input %q: expected a parse error for the unclosed block, got none", in)
		}
	}
}

func TestParserErrorsCarrySourceLocation(t *testing.T) {
	p := New(lexer.NewLexer("print ok\nif true {"))
	p.ParseProgram()
	if len(p.Errors()) == 0 || !strings.HasPrefix(p.Errors()[0], "2:") {
		t.Fatalf("parser errors = %v, want a line 2 location", p.Errors())
	}
}

// TestClosedBlocksParseClean is the regression guard: well-formed blocks must
// still parse without errors after the unclosed-block check was added.
func TestClosedBlocksParseClean(t *testing.T) {
	inputs := []string{
		"for i in range(3) { print i }",
		"if 1 == 1 { print HI }",
		"if 1 == 1 { print a } else { print b }",
		"while 1 == 1 { break }",
		"fn greet() { print hi }",
	}
	for _, in := range inputs {
		p := New(lexer.NewLexer(in))
		p.ParseProgram()
		if errs := p.Errors(); len(errs) != 0 {
			t.Errorf("input %q parsed with errors: %v", in, errs)
		}
	}
}

// TestSeparatorSplitsAfterFlag verifies the lexer fix at the parser level: a
// command separator glued to a flag still ends the command.
func TestSeparatorSplitsAfterFlag(t *testing.T) {
	p := New(lexer.NewLexer("echo -y;echo SECOND"))
	program := p.ParseProgram()
	checkParserErrors(t, p)
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(program.Statements))
	}

	// A flag glued to '&' backgrounds the command rather than joining the flag.
	p2 := New(lexer.NewLexer("sleep 1 -x&"))
	prog2 := p2.ParseProgram()
	checkParserErrors(t, p2)
	stmt := prog2.Statements[0].(*ast.ExpressionStatement)
	if _, ok := stmt.Expression.(*ast.BackgroundExpression); !ok {
		t.Fatalf("expected BackgroundExpression, got %T", stmt.Expression)
	}
}

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

func TestExternalCommandWithFlags(t *testing.T) {
	input := "git commit -m message"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd, ok := stmt.Expression.(*ast.Command)
	if !ok {
		t.Fatalf("expression is not Command. got=%T", stmt.Expression)
	}
	if cmd.Type != ast.CMD_EXTERNAL || cmd.Name != "git" {
		t.Fatalf("got command {%s %s}, want external git", cmd.Type, cmd.Name)
	}
	if len(cmd.Arguments) != 3 {
		t.Fatalf("wrong arg count. got=%d, want 3 (%s)", len(cmd.Arguments), cmd.String())
	}
	// The flag should be preserved verbatim.
	if cmd.Arguments[1].String() != `"-m"` {
		t.Errorf("flag arg = %s, want \"-m\"", cmd.Arguments[1].String())
	}
}

func TestNewlineSeparatesCommands(t *testing.T) {
	// A newline must end the first command's argument list so the second line
	// is parsed as its own command rather than being absorbed as an argument.
	input := "print x\ngit status"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("got %d statements, want 2", len(program.Statements))
	}
	cmd := program.Statements[1].(*ast.ExpressionStatement).Expression.(*ast.Command)
	if cmd.Type != ast.CMD_EXTERNAL || cmd.Name != "git" {
		t.Errorf("second statement = {%s %s}, want external git", cmd.Type, cmd.Name)
	}
}

func TestExternalCommandWithKeywordArgument(t *testing.T) {
	// A reserved keyword (ls) used as an argument to an external command must be
	// taken as a literal word, not split off into a second command. Previously
	// `podman ls` parsed as two separate commands.
	input := "podman ls"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("got %d statements, want 1 (keyword arg was split into a command)", len(program.Statements))
	}
	cmd := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.Command)
	if cmd.Type != ast.CMD_EXTERNAL || cmd.Name != "podman" {
		t.Fatalf("got command {%s %s}, want external podman", cmd.Type, cmd.Name)
	}
	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong arg count. got=%d, want 1 (%s)", len(cmd.Arguments), cmd.String())
	}
	testIdentifier(t, cmd.Arguments[0], "ls")
}

func TestExternalCommandWithKeywordArgsAndFlags(t *testing.T) {
	// Both `rm` (a keyword) and the following flag/path must attach to sudo.
	input := "sudo rm -rf cache"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(program.Statements))
	}
	cmd := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.Command)
	if cmd.Type != ast.CMD_EXTERNAL || cmd.Name != "sudo" {
		t.Fatalf("got command {%s %s}, want external sudo", cmd.Type, cmd.Name)
	}
	if len(cmd.Arguments) != 3 {
		t.Fatalf("wrong arg count. got=%d, want 3 (%s)", len(cmd.Arguments), cmd.String())
	}
	testIdentifier(t, cmd.Arguments[0], "rm")
	if cmd.Arguments[1].String() != `"-rf"` {
		t.Errorf("flag arg = %s, want \"-rf\"", cmd.Arguments[1].String())
	}
	testIdentifier(t, cmd.Arguments[2], "cache")
}

func TestKeywordArgumentWithExtensionStaysOnePath(t *testing.T) {
	// A keyword glued to an extension (env.local) is a single path argument, not
	// a keyword word followed by a separate ".local".
	input := "cat env.local"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	cmd := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.Command)
	if len(cmd.Arguments) != 1 {
		t.Fatalf("wrong arg count. got=%d, want 1 (%s)", len(cmd.Arguments), cmd.String())
	}
	testPath(t, cmd.Arguments[0], "env.local")
}

func TestPipeRightSideIsCommand(t *testing.T) {
	// The right side of a pipe is in command position, so `wc` is an external
	// command, not a bare identifier value.
	input := `print "hi" | wc -l`
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	pipe, ok := stmt.Expression.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("expression is not PipeExpression. got=%T", stmt.Expression)
	}
	right, ok := pipe.Right.(*ast.Command)
	if !ok {
		t.Fatalf("pipe right is not Command. got=%T", pipe.Right)
	}
	if right.Type != ast.CMD_EXTERNAL || right.Name != "wc" {
		t.Errorf("pipe right = {%s %s}, want external wc", right.Type, right.Name)
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

// A newline ends a statement, so an operator-led line is its own statement
// rather than an infix continuation of the previous one: `print 10 - 4` followed
// by `/bin/echo hi` used to parse as `4 / bin` and fail with "unknown operator".
func TestOperatorLedLineStartsNewStatement(t *testing.T) {
	for _, in := range []string{"print 10 - 4\n/bin/echo hi", "x = 2\n/usr/bin/true"} {
		l := lexer.NewLexer(in)
		p := New(l)
		program := p.ParseProgram()
		if n := len(program.Statements); n != 2 {
			t.Errorf("%q parsed into %d statements, want 2", in, n)
		}
	}
}

// TestFindExecPlaceholderArguments is the regression for
// `find /lib/firmware -name '*.zst' -exec zstd -d --rm -q {} +` failing with
// "no prefix parse function for LBRACE found": the literal `{}` placeholder and
// the trailing `+` must stay arguments of find, not fall out as expressions.
func TestFindExecPlaceholderArguments(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"find x -name '*.zst' -exec zstd -d --rm -q {} +", []string{"x", "-name", "*.zst", "-exec", "zstd", "-d", "--rm", "-q", "{}", "+"}},
		{`find x -exec rm {} \;`, []string{"x", "-exec", "rm", "{}", ";"}},
		{"xargs -I {} ls {}", []string{"-I", "{}", "ls", "{}"}},
	}
	for _, c := range cases {
		p := New(lexer.NewLexer(c.input))
		program := p.ParseProgram()
		checkParserErrors(t, p)
		if len(program.Statements) != 1 {
			t.Fatalf("%q: got %d statements, want 1", c.input, len(program.Statements))
		}
		cmd, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.Command)
		if !ok || cmd.Type != ast.CMD_EXTERNAL {
			t.Fatalf("%q: not an external command: %T", c.input, program.Statements[0].(*ast.ExpressionStatement).Expression)
		}
		if len(cmd.Arguments) != len(c.want) {
			t.Fatalf("%q: got %d args, want %d (%s)", c.input, len(cmd.Arguments), len(c.want), cmd.String())
		}
		for i, w := range c.want {
			var got string
			switch a := cmd.Arguments[i].(type) {
			case *ast.BraceExpression:
				got = a.Word.String()
			case *ast.StringLiteral:
				got = a.Value
			default:
				got = a.String()
			}
			if got != w {
				t.Errorf("%q: arg %d = %q (%T), want %q", c.input, i, got, cmd.Arguments[i], w)
			}
		}
	}
}

func TestExternalCommandNameWithOperatorCharacters(t *testing.T) {
	// The lexer splits `g++` into IDENT + PLUS + PLUS. In command position the
	// glued tokens must join back into a single command name, the same way
	// argument words absorb glued operator characters. Previously this ran a
	// command named "g" with "++" as its first argument.
	tests := []struct {
		input   string
		name    string
		argsLen int
	}{
		{"g++ main.cpp -o main", "g++", 3},
		{"g++ --version", "g++", 1},
		{"c++ x.cpp", "c++", 1},
		{"clang++ x.cpp", "clang++", 1},
		{"g++-14 x.cpp", "g++-14", 1},
		{"/usr/bin/g++ x.cpp", "/usr/bin/g++", 1},
		{"./g++ x.cpp", "./g++", 1},
		{"~/bin/g++ x.cpp", "~/bin/g++", 1},
		{"g++", "g++", 0},
	}
	for _, tt := range tests {
		l := lexer.NewLexer(tt.input)
		p := New(l)
		program := p.ParseProgram()
		checkParserErrors(t, p)

		if len(program.Statements) != 1 {
			t.Fatalf("%q: got %d statements, want 1", tt.input, len(program.Statements))
		}
		cmd, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.Command)
		if !ok {
			t.Fatalf("%q: expression is not Command. got=%T", tt.input, program.Statements[0].(*ast.ExpressionStatement).Expression)
		}
		if cmd.Type != ast.CMD_EXTERNAL || cmd.Name != tt.name {
			t.Errorf("%q: got command {%s %q}, want external %q", tt.input, cmd.Type, cmd.Name, tt.name)
		}
		if len(cmd.Arguments) != tt.argsLen {
			t.Errorf("%q: wrong arg count. got=%d, want %d (%s)", tt.input, len(cmd.Arguments), tt.argsLen, cmd.String())
		}
	}
}

func TestExternalCommandNameStopsAtBoundaries(t *testing.T) {
	// A command name must not swallow a glued pipe: the boundary rule for
	// argument words applies to the name too.
	input := "g++ x.cpp|wc"
	l := lexer.NewLexer(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)
	if len(program.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(program.Statements))
	}
	pipe, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("expression is not PipeExpression. got=%T", program.Statements[0].(*ast.ExpressionStatement).Expression)
	}
	left := pipe.Left.(*ast.Command)
	if left.Name != "g++" || len(left.Arguments) != 1 {
		t.Errorf("left of pipe = %s, want g++ with one argument", left.String())
	}
}
