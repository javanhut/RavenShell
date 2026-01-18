package ast

import (
	"bytes"
	"ravenshell/token"
	"strings"
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

// BooleanLiteral represents true/false values
type BooleanLiteral struct {
	Token token.Token
	Value bool
}

func (bl *BooleanLiteral) expressionNode()      {}
func (bl *BooleanLiteral) TokenLiteral() string { return bl.Token.Literal }
func (bl *BooleanLiteral) String() string       { return bl.Token.Literal }

// PrefixExpression represents unary operators: !expr
type PrefixExpression struct {
	Token    token.Token // the prefix token, e.g. !
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	return "(" + pe.Operator + pe.Right.String() + ")"
}

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
	CMD_SHOW       CommandType = "show"
	CMD_CLEAR      CommandType = "clear"
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

// AssignmentStatement represents variable assignment: x = value
type AssignmentStatement struct {
	Token token.Token // the ASSIGN token
	Name  *Identifier
	Value Expression
}

func (as *AssignmentStatement) statementNode()       {}
func (as *AssignmentStatement) TokenLiteral() string { return as.Token.Literal }
func (as *AssignmentStatement) String() string {
	var out bytes.Buffer
	out.WriteString(as.Name.String())
	out.WriteString(" = ")
	if as.Value != nil {
		out.WriteString(as.Value.String())
	}
	return out.String()
}

// BlockStatement represents a block of statements: { ... }
type BlockStatement struct {
	Token      token.Token // the LBRACE token
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {}
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{ ")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

// ForStatement represents: for i in range(n) { ... }
type ForStatement struct {
	Token    token.Token     // the FOR token
	Variable *Identifier     // loop variable
	Iterable Expression      // the range/array to iterate over
	Body     *BlockStatement // the loop body
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for ")
	out.WriteString(fs.Variable.String())
	out.WriteString(" in ")
	out.WriteString(fs.Iterable.String())
	out.WriteString(" ")
	out.WriteString(fs.Body.String())
	return out.String()
}

// IfStatement represents: if condition { ... } else { ... }
type IfStatement struct {
	Token       token.Token     // the IF token
	Condition   Expression      // the condition
	Consequence *BlockStatement // the if body
	Alternative *BlockStatement // optional else body
}

func (is *IfStatement) statementNode()       {}
func (is *IfStatement) TokenLiteral() string { return is.Token.Literal }
func (is *IfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("if ")
	out.WriteString(is.Condition.String())
	out.WriteString(" ")
	out.WriteString(is.Consequence.String())
	if is.Alternative != nil {
		out.WriteString(" else ")
		out.WriteString(is.Alternative.String())
	}
	return out.String()
}

// BreakStatement represents the break keyword
type BreakStatement struct {
	Token token.Token
}

func (bs *BreakStatement) statementNode()       {}
func (bs *BreakStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BreakStatement) String() string       { return "break" }

// ContinueStatement represents the continue keyword
type ContinueStatement struct {
	Token token.Token
}

func (cs *ContinueStatement) statementNode()       {}
func (cs *ContinueStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ContinueStatement) String() string       { return "continue" }

// FunctionStatement represents: fn name(params) { body }
type FunctionStatement struct {
	Token      token.Token     // the FUNCTION token
	Name       *Identifier     // function name
	Parameters []*Identifier   // parameter names
	Body       *BlockStatement // function body
}

func (fs *FunctionStatement) statementNode()       {}
func (fs *FunctionStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *FunctionStatement) String() string {
	var out bytes.Buffer
	out.WriteString("fn ")
	out.WriteString(fs.Name.String())
	out.WriteString("(")
	for i, p := range fs.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
	}
	out.WriteString(") ")
	out.WriteString(fs.Body.String())
	return out.String()
}

// ReturnStatement represents: return [value]
type ReturnStatement struct {
	Token token.Token // the RETURN token
	Value Expression  // optional return value
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString("return")
	if rs.Value != nil {
		out.WriteString(" ")
		out.WriteString(rs.Value.String())
	}
	return out.String()
}

// CaseClause represents a single case in a switch statement
type CaseClause struct {
	Token      token.Token     // the CASE token
	Values     []Expression    // values to match (can be multiple: case 1, 2, 3:)
	Body       *BlockStatement // case body
}

func (cc *CaseClause) statementNode()       {}
func (cc *CaseClause) TokenLiteral() string { return cc.Token.Literal }
func (cc *CaseClause) String() string {
	var out bytes.Buffer
	out.WriteString("case ")
	for i, v := range cc.Values {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(v.String())
	}
	out.WriteString(": ")
	out.WriteString(cc.Body.String())
	return out.String()
}

// SwitchStatement represents: switch expr { case val: { ... } default { ... } }
type SwitchStatement struct {
	Token   token.Token     // the SWITCH token
	Value   Expression      // expression to switch on
	Cases   []*CaseClause   // case clauses
	Default *BlockStatement // optional default clause
}

func (ss *SwitchStatement) statementNode()       {}
func (ss *SwitchStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *SwitchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("switch ")
	out.WriteString(ss.Value.String())
	out.WriteString(" { ")
	for _, c := range ss.Cases {
		out.WriteString(c.String())
		out.WriteString(" ")
	}
	if ss.Default != nil {
		out.WriteString("default ")
		out.WriteString(ss.Default.String())
	}
	out.WriteString("}")
	return out.String()
}

// InfixExpression represents binary operations: left op right
type InfixExpression struct {
	Token    token.Token // the operator token
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode()      {}
func (ie *InfixExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ie.Left.String())
	out.WriteString(" " + ie.Operator + " ")
	out.WriteString(ie.Right.String())
	out.WriteString(")")
	return out.String()
}

// CallExpression represents function calls: range(10), append(x, y)
type CallExpression struct {
	Token     token.Token  // the function name token
	Function  string       // function name (range, append, etc.)
	Arguments []Expression // function arguments
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ce.Function)
	out.WriteString("(")
	for i, arg := range ce.Arguments {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(arg.String())
	}
	out.WriteString(")")
	return out.String()
}

// ArrayLiteral represents array literals: []string, [1, 2, 3]
type ArrayLiteral struct {
	Token    token.Token  // the LBRACKET token
	Elements []Expression // array elements
	TypeHint string       // optional type hint like "string", "int"
}

func (al *ArrayLiteral) expressionNode()      {}
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("[")
	if al.TypeHint != "" {
		out.WriteString("]" + al.TypeHint)
	} else {
		for i, el := range al.Elements {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(el.String())
		}
		out.WriteString("]")
	}
	return out.String()
}

// DictLiteral represents dictionary literals: {"key": value}
type DictLiteral struct {
	Token token.Token         // the LBRACE token
	Pairs map[Expression]Expression
}

func (dl *DictLiteral) expressionNode()      {}
func (dl *DictLiteral) TokenLiteral() string { return dl.Token.Literal }
func (dl *DictLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("{")
	pairs := []string{}
	for key, value := range dl.Pairs {
		pairs = append(pairs, key.String()+": "+value.String())
	}
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// IndexExpression represents array indexing: arr[0]
type IndexExpression struct {
	Token token.Token // the LBRACKET token
	Left  Expression  // the array
	Index Expression  // the index
}

func (ie *IndexExpression) expressionNode()      {}
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ie.Left.String())
	out.WriteString("[")
	out.WriteString(ie.Index.String())
	out.WriteString("])")
	return out.String()
}
