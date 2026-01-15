package ast

import (
	"bytes"
	"ravenshell/token"
)

// Node is the base interface for all AST nodes
type Node interface {
	TokenLiteral() string // Returns literal value of the token (for debugging)
	String() string       // Pretty-print the node (for debugging/testing)
}

// Statement represents a statement in the shell
type Statement interface {
	Node
	statementNode()
}

// Expression represents a value-producing construct
type Expression interface {
	Node
	expressionNode()
}

// Program is the root node of the AST
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// ExpressionStatement wraps an expression to be used as a statement
type ExpressionStatement struct {
	Token      token.Token // First token of the expression
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// Identifier represents a name (file, path, variable name, etc.)
type Identifier struct {
	Token token.Token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// PathExpression represents a file path (e.g., ./foo, ../bar, /absolute/path)
type PathExpression struct {
	Token token.Token // First token of the path
	Value string      // The complete path string
}

func (pe *PathExpression) expressionNode()      {}
func (pe *PathExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PathExpression) String() string       { return pe.Value }

// IntegerLiteral represents an integer value
type IntegerLiteral struct {
	Token token.Token
	Value int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Token.Literal }

// StringLiteral represents a quoted string
type StringLiteral struct {
	Token token.Token
	Value string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return "\"" + sl.Value + "\"" }

// VariableReference represents $VAR syntax
type VariableReference struct {
	Token token.Token // the DOLLAR token
	Name  *Identifier
}

func (vr *VariableReference) expressionNode()      {}
func (vr *VariableReference) TokenLiteral() string { return vr.Token.Literal }
func (vr *VariableReference) String() string       { return "$" + vr.Name.String() }

// CommandType represents the type of built-in command
type CommandType string

const (
	CMD_LIST       CommandType = "ls"
	CMD_REMOVE     CommandType = "rm"
	CMD_CHANGEDIR  CommandType = "cd"
	CMD_REMOVEDIR  CommandType = "rmdir"
	CMD_MAKEDIR    CommandType = "mkdir"
	CMD_WHOAMI     CommandType = "whoami"
	CMD_CURRENTDIR CommandType = "cwd"
	CMD_MAKEFILE   CommandType = "mkfile"
	CMD_OUTPUT     CommandType = "output"
	CMD_PRINT      CommandType = "print"
	CMD_TILDE      CommandType = "~"
	CMD_EXTERNAL   CommandType = "external"
)

// Command represents a shell command with its arguments
type Command struct {
	Token     token.Token  // The command token
	Type      CommandType  // The command type
	Name      string       // The command name as string
	Arguments []Expression // Command arguments
}

func (c *Command) expressionNode()      {}
func (c *Command) TokenLiteral() string { return c.Token.Literal }
func (c *Command) String() string {
	var out bytes.Buffer
	out.WriteString(c.Name)
	for _, arg := range c.Arguments {
		out.WriteString(" ")
		out.WriteString(arg.String())
	}
	return out.String()
}

// PipeExpression represents a pipe between commands
type PipeExpression struct {
	Token token.Token // The PIPE token '|'
	Left  Expression  // Command on the left
	Right Expression  // Command on the right
}

func (pe *PipeExpression) expressionNode()      {}
func (pe *PipeExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PipeExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(pe.Left.String())
	out.WriteString(" | ")
	out.WriteString(pe.Right.String())
	out.WriteString(")")
	return out.String()
}

// RedirectionType indicates the type of redirection
type RedirectionType string

const (
	REDIR_OUTPUT  RedirectionType = ">"
	REDIR_APPEND  RedirectionType = ">>"
	REDIR_INPUT   RedirectionType = "<"
	REDIR_HEREDOC RedirectionType = "<<"
)

// RedirectionExpression represents I/O redirection
type RedirectionExpression struct {
	Token   token.Token     // The redirection token (>, >>, <)
	Type    RedirectionType // Type of redirection
	Command Expression      // The command being redirected
	Target  Expression      // The file target
}

func (re *RedirectionExpression) expressionNode()      {}
func (re *RedirectionExpression) TokenLiteral() string { return re.Token.Literal }
func (re *RedirectionExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(re.Command.String())
	out.WriteString(" " + string(re.Type) + " ")
	out.WriteString(re.Target.String())
	out.WriteString(")")
	return out.String()
}
