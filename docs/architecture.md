# RavenShell Architecture Guide

This document provides a technical overview of RavenShell's architecture for developers and contributors.

## Overview

RavenShell implements a classic interpreter design pattern with a three-stage pipeline:

```
Input → Lexer → Parser → AST → Evaluator → Output
```

1. **Lexer**: Converts raw input text into tokens
2. **Parser**: Builds an Abstract Syntax Tree (AST) from tokens
3. **Evaluator**: Executes the AST and produces results

## Project Structure

```
ravenshell/
├── main.go              # Entry point, REPL, script runner
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
│
├── token/
│   └── token.go         # Token types and definitions
│
├── lexer/
│   └── lexer.go         # Tokenization
│
├── ast/
│   ├── ast.go           # AST node definitions
│   └── ast_test.go      # AST tests
│
├── parser/
│   ├── parser.go        # Pratt parser implementation
│   └── parser_test.go   # Parser tests
│
├── evaluator/
│   └── evaluator.go     # AST execution engine
│
├── readline/
│   └── readline.go      # Interactive line editing
│
└── examples/
    └── *.rsh            # Example scripts
```

## Token Package

**Location:** `token/token.go`

Defines all token types used by the lexer and parser.

### Token Structure

```go
type TokenType string

type Token struct {
    Type    TokenType
    Literal string
}
```

### Token Categories

| Category | Examples |
|----------|----------|
| Keywords | `LIST`, `REMOVE`, `CHANGEDIR`, `FOR`, `IF`, `ELSE` |
| Operators | `PIPE`, `PLUS`, `MINUS`, `EQ`, `NOT_EQ`, `LT`, `GT` |
| Delimiters | `LBRACE`, `RBRACE`, `LPAREN`, `RPAREN`, `LBRACKET` |
| Literals | `INTEGER`, `STRING`, `IDENT` |
| Special | `EOF`, `ILLEGAL`, `DOLLAR`, `TILDE` |

### TokenMap

Maps keyword strings to their token types:

```go
var TokenMap = map[string]TokenType{
    "ls":     LIST,
    "rm":     REMOVE,
    "mkdir":  MAKEDIR,
    "for":    FOR,
    "if":     IF,
    // ...
}
```

## Lexer Package

**Location:** `lexer/lexer.go`

Converts input strings into a stream of tokens.

### Key Methods

| Method | Description |
|--------|-------------|
| `NewLexer(input string)` | Creates a new lexer |
| `NextToken()` | Returns the next token |
| `peek()` | Look at next character without consuming |
| `advance()` | Move to next character |

### Tokenization Rules

1. **Whitespace**: Skipped (except in strings)
2. **Comments**: `#` to end of line, skipped
3. **Strings**: `"..."` or `'...'`
4. **Numbers**: Sequence of digits
5. **Identifiers**: Start with letter, contain letters/numbers/underscores
6. **Keywords**: Identifiers checked against `TokenMap`
7. **Multi-character operators**: `==`, `!=`, `>>`, `<<`, `>=`, `<=`

## AST Package

**Location:** `ast/ast.go`

Defines the Abstract Syntax Tree node types.

### Node Interface Hierarchy

```
Node (interface)
├── Statement (interface)
│   ├── Program
│   ├── ExpressionStatement
│   ├── AssignmentStatement
│   ├── ForStatement
│   ├── IfStatement
│   └── BlockStatement
│
└── Expression (interface)
    ├── Identifier
    ├── PathExpression
    ├── IntegerLiteral
    ├── StringLiteral
    ├── VariableReference
    ├── Command
    ├── PipeExpression
    ├── RedirectionExpression
    ├── InfixExpression
    ├── CallExpression
    ├── ArrayLiteral
    └── IndexExpression
```

### Statement Types

| Type | Description | Example |
|------|-------------|---------|
| `Program` | Root node containing all statements | - |
| `ExpressionStatement` | Wraps an expression as a statement | `ls` |
| `AssignmentStatement` | Variable assignment | `x = 5` |
| `ForStatement` | For loop | `for i in range(10) { }` |
| `IfStatement` | Conditional | `if x > 5 { }` |
| `BlockStatement` | Block of statements | `{ stmt1; stmt2 }` |

### Expression Types

| Type | Description | Example |
|------|-------------|---------|
| `Identifier` | Variable or name | `x`, `filename` |
| `PathExpression` | File path | `./file`, `/home/user` |
| `IntegerLiteral` | Integer value | `42` |
| `StringLiteral` | String value | `"hello"` |
| `VariableReference` | Environment variable | `$HOME` |
| `Command` | Built-in command | `ls`, `cd ~` |
| `PipeExpression` | Pipe operation | `ls \| print` |
| `RedirectionExpression` | I/O redirection | `ls > file.txt` |
| `InfixExpression` | Binary operation | `x + 5` |
| `CallExpression` | Function call | `range(10)` |
| `ArrayLiteral` | Array | `[1, 2, 3]` |
| `IndexExpression` | Array access | `arr[0]` |

## Parser Package

**Location:** `parser/parser.go`

Implements a Pratt parser (top-down operator precedence parsing).

### Precedence Levels

From lowest to highest:

```go
const (
    LOWEST      // default
    REDIRECT    // >, >>, <
    PIPE        // |
    EQUALS      // ==, !=
    LESSGREATER // <, >, <=, >=
    SUM         // +, -
    PRODUCT     // *, /, %
    PREFIX      // $ (variable reference)
    INDEX       // array[index]
    COMMAND     // commands
)
```

### Parse Functions

The parser uses two types of parse functions:

- **Prefix parse functions**: Handle tokens at the start of expressions
- **Infix parse functions**: Handle tokens in the middle of expressions

```go
type prefixParseFn func() ast.Expression
type infixParseFn  func(ast.Expression) ast.Expression
```

### Key Methods

| Method | Description |
|--------|-------------|
| `New(lexer)` | Creates parser, registers parse functions |
| `ParseProgram()` | Entry point, returns complete AST |
| `parseStatement()` | Dispatches to specific statement parsers |
| `parseExpression(precedence)` | Core Pratt algorithm |

### Registration Pattern

```go
// Prefix: token at start of expression
p.registerPrefix(token.INTEGER, p.parseIntegerLiteral)
p.registerPrefix(token.LIST, p.parseCommandKeyword)

// Infix: token between expressions
p.registerInfix(token.PLUS, p.parseInfixExpression)
p.registerInfix(token.PIPE, p.parsePipeExpression)
```

## Evaluator Package

**Location:** `evaluator/evaluator.go`

Executes the AST and maintains shell state.

### Evaluator Structure

```go
type Evaluator struct {
    cwd    string            // Current working directory
    env    map[string]string // Environment variables (for $VAR)
    vars   map[string]Value  // Script variables
    stdout io.Writer         // Standard output (for redirections)
    stdin  io.Reader         // Standard input (for redirections)
}
```

### Value System

The evaluator uses a dynamic type system:

```go
type Value interface{}
```

Values can be:
- `string`
- `int64`
- `bool`
- `[]Value` (arrays)

### Key Methods

| Method | Description |
|--------|-------------|
| `New()` | Creates evaluator with current directory |
| `Eval(program)` | Entry point for evaluation |
| `evalStatement(stmt)` | Evaluates a statement |
| `evalExpressionValue(expr)` | Evaluates an expression |
| `evalCommand(cmd)` | Executes a built-in command |

### Command Implementations

Each built-in command has an implementation function:

```go
func (e *Evaluator) execList(args []string) (string, error)
func (e *Evaluator) execChangeDir(args []string) (string, error)
func (e *Evaluator) execMakeDir(args []string) (string, error)
// ...
```

### Built-in Functions

```go
func (e *Evaluator) builtinRange(args []ast.Expression) (Value, error)
func (e *Evaluator) builtinAppend(args []ast.Expression) (Value, error)
```

## Readline Package

**Location:** `readline/readline.go`

Provides interactive line editing.

### Features

- Raw terminal mode handling
- Cursor movement
- Command history (arrow keys)
- Tab completion
- Keyboard shortcuts (Ctrl+A, Ctrl+E, etc.)

### Key Methods

| Method | Description |
|--------|-------------|
| `New(prompt)` | Creates readline instance |
| `ReadLine()` | Reads a line with editing support |
| `AddHistory(line)` | Adds to history |
| `SetCwdFunc(fn)` | Sets function for path completion |

## Adding New Features

### Adding a New Command

1. **Add token** in `token/token.go`:
   ```go
   MYCOMMAND TokenType = "MYCOMMAND"
   ```

2. **Add to TokenMap**:
   ```go
   "mycommand": MYCOMMAND,
   ```

3. **Add CommandType** in `ast/ast.go`:
   ```go
   CMD_MYCOMMAND CommandType = "mycommand"
   ```

4. **Register prefix** in `parser/parser.go`:
   ```go
   p.registerPrefix(token.MYCOMMAND, p.parseCommandKeyword)
   ```

5. **Add mapping** in parser's `tokenTypeToCommandType`:
   ```go
   token.MYCOMMAND: ast.CMD_MYCOMMAND,
   ```

6. **Implement** in `evaluator/evaluator.go`:
   ```go
   case ast.CMD_MYCOMMAND:
       return e.execMyCommand(args)
   ```

7. **Add to readline** command list for completion.

### Adding a New Operator

1. **Add token** in `token/token.go`
2. **Update lexer** to recognize the operator
3. **Add precedence** in parser's `precedences` map
4. **Register infix** parse function
5. **Implement** in evaluator's `evalInfixExpression`

### Adding a Built-in Function

1. **Add token** (optional, for keyword recognition)
2. **Register** in parser for `CallExpression`
3. **Implement** in evaluator's `evalCallExpression`:
   ```go
   case "myfunc":
       return e.builtinMyFunc(node.Arguments)
   ```

## Testing

### Running Tests

```bash
go test ./...
```

### Test Organization

- `ast/ast_test.go`: AST node string representations
- `parser/parser_test.go`: Parser correctness tests

### Writing Tests

Use table-driven tests:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"test input", "expected output"},
    }

    for _, tt := range tests {
        // Test logic
    }
}
```

## Data Flow Example

For the input `x = 5 + 3`:

1. **Lexer** produces tokens:
   - `IDENT("x")`, `ASSIGN("=")`, `INTEGER("5")`, `PLUS("+")`, `INTEGER("3")`

2. **Parser** builds AST:
   ```
   AssignmentStatement
   ├── Name: Identifier("x")
   └── Value: InfixExpression
       ├── Left: IntegerLiteral(5)
       ├── Operator: "+"
       └── Right: IntegerLiteral(3)
   ```

3. **Evaluator** executes:
   - Evaluates `5 + 3` → `8`
   - Stores `"x"` → `8` in `vars` map

## Dependencies

- `golang.org/x/term`: Terminal control for raw mode in readline
