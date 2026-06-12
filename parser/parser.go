package parser

import (
	"fmt"
	"ravenshell/ast"
	"ravenshell/lexer"
	"ravenshell/token"
	"strconv"
	"strings"
)

// Operator precedence levels (lower = binds looser)
const (
	_ int = iota
	LOWEST
	ANDOR       // &&, || (loosest binding command operators)
	REDIRECT    // >, >>, <
	PIPE        // |
	EQUALS      // ==, !=
	LESSGREATER // <, >
	SUM         // +, -
	PRODUCT     // *, /, %
	PREFIX      // $ (variable reference)
	INDEX       // array[index]
	COMMAND     // commands
)

// Precedence table for infix operators
var precedences = map[token.TokenType]int{
	token.AND:      ANDOR,
	token.OR:       ANDOR,
	token.PIPE:     PIPE,
	token.INTO:     REDIRECT,
	token.OUT:      REDIRECT,
	token.EQ:       EQUALS,
	token.NOT_EQ:   EQUALS,
	token.LT:       REDIRECT, // Low precedence so redirections bind looser than pipes
	token.GT:       REDIRECT, // Low precedence so redirections bind looser than pipes
	token.LTE:      LESSGREATER,
	token.GTE:      LESSGREATER,
	token.PLUS:     SUM,
	token.MINUS:    SUM,
	token.ASTERISK: PRODUCT,
	token.PERCENT:  PRODUCT,
	token.LBRACKET: INDEX,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

// Parser parses tokens from the lexer into an AST
type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  token.Token
	peekToken token.Token

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn

	// cmdPos is true when the next expression begins in "command position"
	// (the start of an expression statement), where a bare identifier or path
	// is interpreted as an external command rather than a value.
	cmdPos bool
	// prefixCmdPos carries cmdPos to the prefix parse function for the current
	// expression. Prefix functions that can produce a command capture and clear
	// it at entry so nested expressions are not treated as commands.
	prefixCmdPos bool
}

// New creates a new Parser
func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	// Register prefix parse functions
	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifierOrCommand)
	p.registerPrefix(token.INTEGER, p.parseIntegerLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.FLAG, p.parseFlagLiteral)
	p.registerPrefix(token.LASTSTATUS, p.parseLastStatus)
	p.registerPrefix(token.DOLLAR, p.parseVariableReference)
	p.registerPrefix(token.FULLSTOP, p.parsePath)
	p.registerPrefix(token.FSLASH, p.parsePath)
	p.registerPrefix(token.TILDE, p.parseTilde)
	p.registerPrefix(token.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.RANGE, p.parseCallExpression)
	p.registerPrefix(token.APPEND, p.parseCallExpression)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)

	// Register command keywords as prefix parse functions
	p.registerPrefix(token.LIST, p.parseCommandKeyword)
	p.registerPrefix(token.REMOVE, p.parseCommandKeyword)
	p.registerPrefix(token.CHANGEDIR, p.parseCommandKeyword)
	p.registerPrefix(token.REMOVEDIR, p.parseCommandKeyword)
	p.registerPrefix(token.MAKEDIR, p.parseCommandKeyword)
	p.registerPrefix(token.WHOAMI, p.parseCommandKeyword)
	p.registerPrefix(token.CURRENTDIR, p.parseCommandKeyword)
	p.registerPrefix(token.MAKEFILE, p.parseCommandKeyword)
	p.registerPrefix(token.OUTPUT, p.parseCommandKeyword)
	p.registerPrefix(token.PRINT, p.parseCommandKeyword)
	p.registerPrefix(token.SHOW, p.parseCommandKeyword)
	p.registerPrefix(token.CLEAR, p.parseCommandKeyword)
	p.registerPrefix(token.EXPORT, p.parseCommandKeyword)
	p.registerPrefix(token.ENV, p.parseCommandKeyword)
	p.registerPrefix(token.RAVENADD, p.parseCommandKeyword)
	p.registerPrefix(token.RAVENHELP, p.parseHelpCommand)
	p.registerPrefix(token.PS, p.parseCommandKeyword)
	p.registerPrefix(token.KILL, p.parseCommandKeyword)
	p.registerPrefix(token.KILLALL, p.parseCommandKeyword)
	p.registerPrefix(token.JOBS, p.parseCommandKeyword)

	// Register infix parse functions
	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PIPE, p.parsePipeExpression)
	p.registerInfix(token.INTO, p.parseRedirectionExpression)
	p.registerInfix(token.OUT, p.parseRedirectionExpression)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.PERCENT, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseComparisonOrRedirection)
	p.registerInfix(token.GT, p.parseComparisonOrRedirection)
	p.registerInfix(token.LTE, p.parseInfixExpression)
	p.registerInfix(token.GTE, p.parseInfixExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.AND, p.parseLogicalExpression)
	p.registerInfix(token.OR, p.parseLogicalExpression)

	// Read two tokens to initialize curToken and peekToken
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t token.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if prec, ok := precedences[p.curToken.Type]; ok {
		return prec
	}
	return LOWEST
}

// Errors returns the list of parsing errors
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t token.TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead",
		t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.errors = append(p.errors, msg)
}

// ParseProgram is the main entry point
func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}

	for !p.curTokenIs(token.EOF) {
		// ';' separates statements on a line.
		if p.curTokenIs(token.SEMICOLON) {
			p.nextToken()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.FOR:
		return p.parseForStatement()
	case token.WHILE:
		return p.parseWhileStatement()
	case token.IF:
		return p.parseIfStatement()
	case token.BREAK:
		return &ast.BreakStatement{Token: p.curToken}
	case token.CONTINUE:
		return &ast.ContinueStatement{Token: p.curToken}
	case token.FN:
		return p.parseFunctionStatement()
	case token.RETURN:
		return p.parseReturnStatement()
	case token.IDENT:
		// Check if this is an assignment (IDENT = value)
		if p.peekTokenIs(token.ASSIGN) {
			return p.parseAssignmentStatement()
		}
		return p.parseExpressionStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	// The first token of a statement is in command position: a bare identifier
	// or path here is run as an external command.
	p.cmdPos = true
	stmt.Expression = p.parseExpression(LOWEST)

	// A trailing '&' runs the command in the background.
	if p.peekTokenIs(token.AMP) {
		amp := p.peekToken
		p.nextToken()
		stmt.Expression = &ast.BackgroundExpression{Token: amp, Command: stmt.Expression}
	}

	return stmt
}

// parseLastStatus parses $? (the exit status of the last command).
func (p *Parser) parseLastStatus() ast.Expression {
	p.prefixCmdPos = false
	return &ast.LastStatus{Token: p.curToken}
}

// parseLogicalExpression parses left && right or left || right.
func (p *Parser) parseLogicalExpression(left ast.Expression) ast.Expression {
	exp := &ast.LogicalExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	// The right side is a command, like the right side of a pipe.
	p.cmdPos = true
	exp.Right = p.parseExpression(precedence)
	return exp
}

// parseExpression is the core Pratt parser function
func (p *Parser) parseExpression(precedence int) ast.Expression {
	// Hand command-position status to the prefix function for this expression,
	// then clear it so nested sub-expressions are parsed as values.
	p.prefixCmdPos = p.cmdPos
	p.cmdPos = false

	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()
	p.prefixCmdPos = false

	// Continue parsing infix expressions while precedence allows
	for !p.peekTokenIs(token.EOF) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

// parseIdentifierOrCommand handles IDENT tokens
func (p *Parser) parseIdentifierOrCommand() ast.Expression {
	cmdPos := p.prefixCmdPos
	p.prefixCmdPos = false

	// Check if this identifier is a known command
	if cmdType, ok := token.TokenMap[p.curToken.Literal]; ok {
		return p.parseCommand(cmdType)
	}

	// An identifier immediately followed by '(' is a function call, e.g. foo(x).
	if p.peekTokenIs(token.LPAREN) {
		return p.parseCallExpression()
	}

	// Check if this identifier is directly followed (no space) by path tokens
	// (e.g., file.txt, foo/bar). A space means a separate argument.
	if (p.peekTokenIs(token.FSLASH) || p.peekTokenIs(token.FULLSTOP)) && !p.peekToken.PrecededByWhitespace {
		path := p.parsePathFromIdent()
		if cmdPos {
			// e.g. ./script.sh or bin/tool at the start of a statement
			return p.finishExternalCommand(path.String())
		}
		return path
	}

	// A bare identifier at the start of a statement is an external command.
	if cmdPos {
		return p.finishExternalCommand(p.curToken.Literal)
	}

	// Otherwise, it's a regular identifier (a value/variable reference)
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

// parseFlagLiteral handles FLAG tokens (-l, --all, ...) as string arguments.
func (p *Parser) parseFlagLiteral() ast.Expression {
	p.prefixCmdPos = false
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

// finishExternalCommand builds a CMD_EXTERNAL command node with the given name
// and collects its arguments. curToken must be the command's name token.
func (p *Parser) finishExternalCommand(name string) ast.Expression {
	cmd := &ast.Command{
		Token: p.curToken,
		Name:  name,
		Type:  ast.CMD_EXTERNAL,
	}
	cmd.Arguments = p.parseExternalArguments()
	return cmd
}

// parseExternalArguments collects arguments for an external command. Unlike
// built-in commands, external commands also accept FLAG tokens.
func (p *Parser) parseExternalArguments() []ast.Expression {
	args := []ast.Expression{}

	for p.isArgumentToken(p.peekToken.Type) || p.peekTokenIs(token.FLAG) {
		// A newline ends the command's argument list.
		if p.peekToken.PrecededByNewline {
			break
		}
		// Stop if we see IDENT followed by ASSIGN - that's a new statement.
		if p.peekTokenIs(token.IDENT) && p.isNextAssignment() {
			break
		}

		p.nextToken()
		// Use PIPE precedence so pipes and redirects are left for the caller.
		args = append(args, p.parseExpression(PIPE))
	}

	return args
}

// parseCommandKeyword handles command keyword tokens (LIST, REMOVE, etc.)
func (p *Parser) parseCommandKeyword() ast.Expression {
	return p.parseCommand(p.curToken.Type)
}

// parseHelpCommand handles `raven-help [command]`. Its operands are command
// names, which may themselves be command keywords (read, remove, rmdir, ...),
// so it collects following same-line words by their literal rather than parsing
// them as nested commands.
func (p *Parser) parseHelpCommand() ast.Expression {
	cmd := &ast.Command{
		Token: p.curToken,
		Name:  p.curToken.Literal,
		Type:  ast.CMD_RAVENHELP,
	}
	for !p.peekToken.PrecededByNewline && p.peekIsCommandWord() {
		p.nextToken()
		cmd.Arguments = append(cmd.Arguments, &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		})
	}
	return cmd
}

// peekIsCommandWord reports whether the peek token can name a command: a bare
// identifier, or a keyword whose literal is the keyword itself (so command
// keywords can be passed to raven-help by name).
func (p *Parser) peekIsCommandWord() bool {
	if p.peekTokenIs(token.IDENT) {
		return true
	}
	if t, ok := token.TokenMap[p.peekToken.Literal]; ok && t == p.peekToken.Type {
		return true
	}
	return false
}

func (p *Parser) parseCommand(cmdTokenType token.TokenType) ast.Expression {
	cmd := &ast.Command{
		Token: p.curToken,
		Name:  p.curToken.Literal,
		Type:  tokenTypeToCommandType(cmdTokenType),
	}

	// Parse arguments until we hit an operator or EOF
	cmd.Arguments = p.parseCommandArguments()

	return cmd
}

func (p *Parser) parseCommandArguments() []ast.Expression {
	args := []ast.Expression{}

	// Continue while next token can start an argument expression
	for p.isArgumentToken(p.peekToken.Type) {
		// A newline ends the command's argument list.
		if p.peekToken.PrecededByNewline {
			break
		}
		// Stop if we see IDENT followed by ASSIGN - that's a new assignment statement
		if p.peekTokenIs(token.IDENT) && p.isNextAssignment() {
			break
		}

		p.nextToken()
		// Parse a full expression (handles operators like +)
		// Use PIPE precedence so pipes and redirects are not consumed
		args = append(args, p.parseExpression(PIPE))
	}

	return args
}

// isNextAssignment checks if peek token is IDENT and the token after that is ASSIGN
func (p *Parser) isNextAssignment() bool {
	// We need to look two tokens ahead: peek is IDENT, and the one after is ASSIGN
	// Save current state
	savedPos := p.l.GetPos()
	savedCur := p.curToken
	savedPeek := p.peekToken

	// Advance to check
	p.nextToken() // now curToken is the IDENT
	isAssign := p.peekTokenIs(token.ASSIGN)

	// Restore state
	p.l.SetPos(savedPos)
	p.curToken = savedCur
	p.peekToken = savedPeek

	return isAssign
}

// isArgumentToken returns true if the token type can be a command argument
func (p *Parser) isArgumentToken(tt token.TokenType) bool {
	switch tt {
	case token.IDENT, token.STRING, token.INTEGER, token.DOLLAR, token.LASTSTATUS,
		token.FULLSTOP, token.FSLASH, token.TILDE, token.FLAG:
		return true
	default:
		return false
	}
}

// isPathContinuation reports whether tok can extend a path it is glued to with
// no intervening whitespace. Beyond identifiers, dots, and slashes, this also
// covers integer segments (so v1.2.3.tgz stays whole) and reserved words (so
// path segments may legitimately be named env, output, print, in, ...: e.g.
// dir/output.go, .env.local, lib/print.txt). Without this, any path segment
// that happens to be a keyword or a number would split the path.
func (p *Parser) isPathContinuation(tok token.Token) bool {
	switch tok.Type {
	case token.IDENT, token.INTEGER, token.FULLSTOP, token.FSLASH:
		return true
	}
	// A token whose literal maps back to its own type is a reserved word
	// (ls, env, output, for, ...); treat it as plain path text here.
	if t, ok := token.TokenMap[tok.Literal]; ok && t == tok.Type {
		return true
	}
	return false
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer", p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{
		Token:       p.curToken,
		Value:       p.curToken.Literal,
		Interpolate: !p.curToken.SingleQuoted,
	}
}

// parsePath parses a file path (./foo, ../bar, /absolute/path, etc.)
func (p *Parser) parsePath() ast.Expression {
	cmdPos := p.prefixCmdPos
	p.prefixCmdPos = false

	path := &ast.PathExpression{Token: p.curToken}

	// Collect all path tokens. Path tokens (IDENT, '.', '/') that are adjacent
	// with no intervening whitespace form a single path, so multi-dot names
	// (archive.tar.gz), dotfiles (.gitignore), and dotfiles with extensions
	// (.env.local) all stay intact. Whitespace is the only delimiter between
	// separate path arguments.
	var pathStr strings.Builder
	pathStr.WriteString(p.curToken.Literal)
	for p.isPathContinuation(p.peekToken) {
		if p.peekToken.PrecededByWhitespace {
			break
		}
		p.nextToken()
		pathStr.WriteString(p.curToken.Literal)
	}

	path.Value = pathStr.String()
	// A path at the start of a statement (./a.out, /usr/bin/tool) runs as an
	// external command.
	if cmdPos {
		return p.finishExternalCommand(pathStr.String())
	}
	return path
}

// parsePathFromIdent parses a path that starts with an identifier (e.g.,
// foo/bar, test.txt, archive.tar.gz). Adjacent path tokens with no whitespace
// between them join into a single path; whitespace ends the path.
func (p *Parser) parsePathFromIdent() ast.Expression {
	path := &ast.PathExpression{Token: p.curToken}
	var pathStr strings.Builder
	pathStr.WriteString(p.curToken.Literal)

	for p.isPathContinuation(p.peekToken) {
		if p.peekToken.PrecededByWhitespace {
			break
		}
		p.nextToken()
		pathStr.WriteString(p.curToken.Literal)
	}

	path.Value = pathStr.String()
	return path
}

// parseTilde handles ~ - either as a path prefix (~/foo) or as a home command
func (p *Parser) parseTilde() ast.Expression {
	cmdPos := p.prefixCmdPos
	p.prefixCmdPos = false

	// If directly followed by FSLASH (no space), it's a path like ~/foo
	if p.peekTokenIs(token.FSLASH) && !p.peekToken.PrecededByWhitespace {
		path := p.parsePath()
		if cmdPos {
			// e.g. ~/bin/tool at the start of a statement
			return p.finishExternalCommand(path.String())
		}
		return path
	}

	// Standalone ~ is a command to print/go to home directory
	cmd := &ast.Command{
		Token:     p.curToken,
		Name:      p.curToken.Literal,
		Type:      ast.CMD_TILDE,
		Arguments: []ast.Expression{},
	}
	return cmd
}

func (p *Parser) parseVariableReference() ast.Expression {
	dollar := p.curToken
	p.prefixCmdPos = false

	// $(command) is command substitution - capture the command's output.
	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // curToken = '('
		p.nextToken() // curToken = first token of the inner command
		p.cmdPos = true
		cmd := p.parseExpression(LOWEST)
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
		return &ast.CommandSubstitution{Token: dollar, Command: cmd}
	}

	vr := &ast.VariableReference{Token: dollar}

	if !p.peekTokenIs(token.IDENT) {
		p.errors = append(p.errors, "expected identifier after $")
		return nil
	}

	p.nextToken()
	vr.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	return vr
}

func (p *Parser) parsePipeExpression(left ast.Expression) ast.Expression {
	expression := &ast.PipeExpression{
		Token: p.curToken,
		Left:  left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	// The right side of a pipe is in command position so bare words there are
	// run as commands (e.g. the `cat` in `echo hi | cat`).
	p.cmdPos = true
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseRedirectionExpression(left ast.Expression) ast.Expression {
	expression := &ast.RedirectionExpression{
		Token:   p.curToken,
		Command: left,
	}

	// Determine redirection type (INTO = >>, OUT = <<)
	switch p.curToken.Type {
	case token.INTO:
		expression.Type = ast.REDIR_APPEND
	case token.OUT:
		expression.Type = ast.REDIR_HEREDOC
	}

	p.nextToken()
	// Parse target as a path/identifier, not as a command
	expression.Target = p.parseRedirectionTarget()

	return expression
}

// parseRedirectionTarget parses the target of a redirection (always a path/identifier, never a command)
func (p *Parser) parseRedirectionTarget() ast.Expression {
	switch p.curToken.Type {
	case token.IDENT,
		// Allow keyword tokens to be used as filenames
		token.LIST, token.REMOVE, token.CHANGEDIR, token.REMOVEDIR, token.MAKEDIR,
		token.WHOAMI, token.CURRENTDIR, token.MAKEFILE, token.OUTPUT, token.PRINT,
		token.SHOW, token.CLEAR, token.FOR, token.IN, token.IF, token.ELSE,
		token.RANGE, token.APPEND:
		// Check if directly followed by path tokens (e.g., output.txt, foo/bar)
		if (p.peekTokenIs(token.FSLASH) || p.peekTokenIs(token.FULLSTOP)) && !p.peekToken.PrecededByWhitespace {
			return p.parsePathFromIdent()
		}
		// Plain identifier
		return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	case token.FULLSTOP, token.FSLASH, token.TILDE:
		// Path starting with ., /, or ~
		return p.parsePath()

	case token.STRING:
		return p.parseStringLiteral()

	case token.DOLLAR:
		return p.parseVariableReference()

	default:
		p.errors = append(p.errors, fmt.Sprintf("unexpected token %s in redirection target", p.curToken.Type))
		return nil
	}
}

func tokenTypeToCommandType(tt token.TokenType) ast.CommandType {
	switch tt {
	case token.LIST:
		return ast.CMD_LIST
	case token.REMOVE:
		return ast.CMD_REMOVE
	case token.CHANGEDIR:
		return ast.CMD_CHANGEDIR
	case token.REMOVEDIR:
		return ast.CMD_REMOVEDIR
	case token.MAKEDIR:
		return ast.CMD_MAKEDIR
	case token.WHOAMI:
		return ast.CMD_WHOAMI
	case token.CURRENTDIR:
		return ast.CMD_CURRENTDIR
	case token.MAKEFILE:
		return ast.CMD_MAKEFILE
	case token.OUTPUT:
		return ast.CMD_OUTPUT
	case token.PRINT:
		return ast.CMD_PRINT
	case token.SHOW:
		return ast.CMD_SHOW
	case token.CLEAR:
		return ast.CMD_CLEAR
	case token.EXPORT:
		return ast.CMD_EXPORT
	case token.ENV:
		return ast.CMD_ENV
	case token.RAVENADD:
		return ast.CMD_RAVENADD
	case token.RAVENHELP:
		return ast.CMD_RAVENHELP
	case token.PS:
		return ast.CMD_PS
	case token.KILL:
		return ast.CMD_KILL
	case token.KILLALL:
		return ast.CMD_KILLALL
	case token.JOBS:
		return ast.CMD_JOBS
	default:
		return ast.CMD_EXTERNAL
	}
}

// parseAssignmentStatement parses: identifier = expression
func (p *Parser) parseAssignmentStatement() *ast.AssignmentStatement {
	stmt := &ast.AssignmentStatement{Token: p.curToken}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

// parseForStatement parses: for identifier in expression { block }
func (p *Parser) parseForStatement() *ast.ForStatement {
	stmt := &ast.ForStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.IN) {
		return nil
	}

	p.nextToken()
	stmt.Iterable = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

// parseFunctionStatement parses: fn name(p1, p2) { block }
func (p *Parser) parseFunctionStatement() *ast.FunctionStatement {
	stmt := &ast.FunctionStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	stmt.Parameters = p.parseFunctionParameters()

	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

// parseFunctionParameters parses a comma-separated list of identifier
// parameters terminated by ')'. curToken must be '(' on entry.
func (p *Parser) parseFunctionParameters() []*ast.Identifier {
	params := []*ast.Identifier{}

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		return params
	}

	p.nextToken()
	params = append(params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		params = append(params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return params
}

// parseReturnStatement parses: return [expression]
func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.curToken}

	// A bare `return` (end of line, end of block, or EOF) has no value.
	if p.peekTokenIs(token.EOF) || p.peekTokenIs(token.RBRACE) || p.peekToken.PrecededByNewline {
		return stmt
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

// parseWhileStatement parses: while expression { block }
func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	stmt := &ast.WhileStatement{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

// parseIfStatement parses: if expr { block } [else if expr { block }]... [else { block }]
func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken()

		// "else if" chains: parse the nested if and wrap it in a block so the
		// IfStatement.Alternative shape stays uniform.
		if p.peekTokenIs(token.IF) {
			p.nextToken()
			nested := p.parseIfStatement()
			stmt.Alternative = &ast.BlockStatement{
				Token:      nested.Token,
				Statements: []ast.Statement{nested},
			}
			return stmt
		}

		if !p.expectPeek(token.LBRACE) {
			return nil
		}

		stmt.Alternative = p.parseBlockStatement()
	}

	return stmt
}

// parseBlockStatement parses: { statements }
func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		// ';' separates statements within a block.
		if p.curTokenIs(token.SEMICOLON) {
			p.nextToken()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// parseComparisonOrRedirection handles > and < which can be either comparison or redirection
func (p *Parser) parseComparisonOrRedirection(left ast.Expression) ast.Expression {
	// If left is a command or pipe expression, treat as redirection
	switch left.(type) {
	case *ast.Command, *ast.PipeExpression:
		return p.parseRedirectionFromGTLT(left)
	}
	// Otherwise treat as comparison
	return p.parseInfixExpression(left)
}

// parseRedirectionFromGTLT handles redirection when using GT/LT tokens
func (p *Parser) parseRedirectionFromGTLT(left ast.Expression) ast.Expression {
	expression := &ast.RedirectionExpression{
		Token:   p.curToken,
		Command: left,
	}

	// Determine redirection type based on GT or LT
	switch p.curToken.Type {
	case token.GT:
		expression.Type = ast.REDIR_OUTPUT
	case token.LT:
		expression.Type = ast.REDIR_INPUT
	}

	p.nextToken()
	expression.Target = p.parseRedirectionTarget()

	return expression
}

// parseInfixExpression parses: left operator right
func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

// parseCallExpression parses: function(args)
func (p *Parser) parseCallExpression() ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: p.curToken.Literal}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	exp.Arguments = p.parseExpressionList(token.RPAREN)

	return exp
}

// parseExpressionList parses a comma-separated list of expressions
func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := []ast.Expression{}

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

// parseArrayLiteral parses: [elements] or []type
func (p *Parser) parseArrayLiteral() ast.Expression {
	array := &ast.ArrayLiteral{Token: p.curToken}

	// Check for []type syntax (empty array with type hint)
	if p.peekTokenIs(token.RBRACKET) {
		p.nextToken()
		// Check if followed by a type identifier
		if p.peekTokenIs(token.IDENT) {
			p.nextToken()
			array.TypeHint = p.curToken.Literal
			array.Elements = []ast.Expression{}
			return array
		}
		// Empty array without type
		array.Elements = []ast.Expression{}
		return array
	}

	array.Elements = p.parseExpressionList(token.RBRACKET)

	return array
}

// parseIndexExpression parses: expression[index]
func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}

// parseGroupedExpression parses: (expression)
func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return exp
}

// expectPeek checks if the next token is the expected type and advances
func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}
