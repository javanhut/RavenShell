package parser

import (
	"fmt"
	"ravenshell/ast"
	"ravenshell/lexer"
	"ravenshell/token"
	"strconv"
)

// Operator precedence levels (lower = binds looser)
const (
	_ int = iota
	LOWEST
	REDIRECT // >, >>, <
	PIPE     // |
	PREFIX   // $ (variable reference)
	COMMAND  // commands
)

// Precedence table for infix operators
var precedences = map[token.TokenType]int{
	token.PIPE:    PIPE,
	token.GREATER: REDIRECT,
	token.INTO:    REDIRECT,
	token.LESS:    REDIRECT,
	token.OUT:     REDIRECT,
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
	p.registerPrefix(token.DOLLAR, p.parseVariableReference)
	p.registerPrefix(token.FULLSTOP, p.parsePath)
	p.registerPrefix(token.FSLASH, p.parsePath)
	p.registerPrefix(token.TILDE, p.parseTilde)

	// Register infix parse functions
	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PIPE, p.parsePipeExpression)
	p.registerInfix(token.GREATER, p.parseRedirectionExpression)
	p.registerInfix(token.INTO, p.parseRedirectionExpression)
	p.registerInfix(token.LESS, p.parseRedirectionExpression)
	p.registerInfix(token.OUT, p.parseRedirectionExpression)

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
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	return p.parseExpressionStatement()
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)
	return stmt
}

// parseExpression is the core Pratt parser function
func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

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
	// Check if this identifier is a known command
	if cmdType, ok := token.TokenMap[p.curToken.Literal]; ok {
		return p.parseCommand(cmdType)
	}

	// Check if this identifier is followed by path tokens (e.g., file.txt, foo/bar)
	if p.peekTokenIs(token.FSLASH) || p.peekTokenIs(token.FULLSTOP) {
		return p.parsePathFromIdent()
	}

	// Otherwise, it's a regular identifier
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
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

	// Continue while next token is an argument (not an operator)
	for p.isArgumentToken(p.peekToken.Type) {
		p.nextToken()

		if p.curTokenIs(token.DOLLAR) {
			args = append(args, p.parseVariableReference())
		} else if p.curTokenIs(token.STRING) {
			args = append(args, p.parseStringLiteral())
		} else if p.curTokenIs(token.INTEGER) {
			args = append(args, p.parseIntegerLiteral())
		} else if p.curTokenIs(token.FULLSTOP) || p.curTokenIs(token.FSLASH) || p.curTokenIs(token.TILDE) {
			// Path starting with ., /, or ~
			args = append(args, p.parsePath())
		} else if p.curTokenIs(token.IDENT) {
			// Check if this identifier is followed by path tokens (e.g., foo/bar, test.txt)
			if p.peekTokenIs(token.FSLASH) || p.peekTokenIs(token.FULLSTOP) {
				args = append(args, p.parsePathFromIdent())
			} else {
				args = append(args, &ast.Identifier{
					Token: p.curToken,
					Value: p.curToken.Literal,
				})
			}
		}
	}

	return args
}

// isArgumentToken returns true if the token type can be a command argument
func (p *Parser) isArgumentToken(tt token.TokenType) bool {
	switch tt {
	case token.IDENT, token.STRING, token.INTEGER, token.DOLLAR, token.FULLSTOP, token.FSLASH, token.TILDE:
		return true
	default:
		return false
	}
}

// isPathToken returns true if the token type can be part of a path
func (p *Parser) isPathToken(tt token.TokenType) bool {
	switch tt {
	case token.IDENT, token.FULLSTOP, token.FSLASH:
		return true
	default:
		return false
	}
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
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

// parsePath parses a file path (./foo, ../bar, /absolute/path, etc.)
func (p *Parser) parsePath() ast.Expression {
	path := &ast.PathExpression{Token: p.curToken}
	var pathStr string

	// Collect all path tokens
	pathStr += p.curToken.Literal
	lastWasExtension := false

	// Continue while next token is part of a path
	for p.isPathToken(p.peekToken.Type) {
		// After an extension (. + IDENT), only continue if next is FSLASH
		if lastWasExtension && !p.peekTokenIs(token.FSLASH) {
			break
		}

		p.nextToken()
		pathStr += p.curToken.Literal

		// Track if we just parsed an extension (FULLSTOP followed by IDENT)
		lastWasExtension = p.curTokenIs(token.IDENT) && len(pathStr) > 1 && pathStr[len(pathStr)-len(p.curToken.Literal)-1] == '.'
	}

	path.Value = pathStr
	return path
}

// parsePathFromIdent parses a path that starts with an identifier (e.g., foo/bar, test.txt)
func (p *Parser) parsePathFromIdent() ast.Expression {
	path := &ast.PathExpression{Token: p.curToken}
	pathStr := p.curToken.Literal
	lastWasExtension := false

	// Continue while next token is part of a path
	for p.isPathToken(p.peekToken.Type) {
		// After an extension (. + IDENT), only continue if next is FSLASH
		if lastWasExtension && !p.peekTokenIs(token.FSLASH) {
			break
		}

		p.nextToken()
		pathStr += p.curToken.Literal

		// Track if we just parsed an extension (FULLSTOP followed by IDENT)
		lastWasExtension = p.curTokenIs(token.IDENT) && len(pathStr) > 1 && pathStr[len(pathStr)-len(p.curToken.Literal)-1] == '.'
	}

	path.Value = pathStr
	return path
}

// parseTilde handles ~ - either as a path prefix (~/foo) or as a home command
func (p *Parser) parseTilde() ast.Expression {
	// If followed by FSLASH, it's a path like ~/foo
	if p.peekTokenIs(token.FSLASH) {
		return p.parsePath()
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
	vr := &ast.VariableReference{Token: p.curToken}

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
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseRedirectionExpression(left ast.Expression) ast.Expression {
	expression := &ast.RedirectionExpression{
		Token:   p.curToken,
		Command: left,
	}

	// Determine redirection type
	switch p.curToken.Type {
	case token.GREATER:
		expression.Type = ast.REDIR_OUTPUT
	case token.INTO:
		expression.Type = ast.REDIR_APPEND
	case token.LESS:
		expression.Type = ast.REDIR_INPUT
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
	case token.IDENT:
		// Check if followed by path tokens (e.g., output.txt, foo/bar)
		if p.peekTokenIs(token.FSLASH) || p.peekTokenIs(token.FULLSTOP) {
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
	default:
		return ast.CMD_EXTERNAL
	}
}
