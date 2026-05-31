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
├── main.go              # Entry point, REPL, script runner, colored prompt
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
│
├── token/
│   └── token.go         # Token types and definitions
│
├── lexer/
│   ├── lexer.go         # Tokenization
│   └── lexer_test.go    # Lexer tests
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
│   ├── evaluator.go     # AST execution engine
│   └── evaluator_test.go # Evaluator tests
│
├── readline/
│   ├── readline.go      # Interactive line editing, completion, autosuggestions
│   └── readline_test.go # Readline tests
│
├── ansi/
│   └── ansi.go          # ANSI color escape codes and helpers
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
    // PrecededByNewline marks the first token on a line; the parser uses it to
    // end a command's argument list at a line break.
    PrecededByNewline bool
    // PrecededByWhitespace marks any token with whitespace/comment before it,
    // so adjacent tokens only join into a path when there is no space between
    // them (foo/bar joins, but `foo /bar` is two arguments).
    PrecededByWhitespace bool
}
```

### Token Categories

| Category | Examples |
|----------|----------|
| Command keywords | `LIST`, `REMOVE`, `CHANGEDIR`, `EXPORT`, `ENV`, `RAVENADD` |
| Control keywords | `FOR`, `WHILE`, `IF`, `ELSE`, `BREAK`, `CONTINUE`, `FN`, `RETURN` |
| Operators | `PIPE`, `PLUS`, `MINUS`, `EQ`, `NOT_EQ`, `LT`, `GT` |
| Delimiters | `LBRACE`, `RBRACE`, `LPAREN`, `RPAREN`, `LBRACKET` |
| Literals | `INTEGER`, `STRING`, `IDENT`, `FLAG` |
| Special | `EOF`, `ILLEGAL`, `DOLLAR`, `TILDE` |

`FLAG` represents a command flag such as `-l`, `--all`, or `--max-count=5`.

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

1. **Whitespace**: Skipped (except in strings), but the lexer records whether
   whitespace and/or a newline preceded each token (`PrecededByWhitespace`,
   `PrecededByNewline`).
2. **Comments**: `#` to end of line, skipped
3. **Strings**: `"..."` or `'...'`
4. **Numbers**: Sequence of digits
5. **Identifiers**: Start with a letter/underscore; contain letters, numbers,
   underscores, and interior hyphens (`docker-compose`, `raven-add`).
6. **Keywords**: Identifiers checked against `TokenMap`
7. **Flags**: A `-` glued to a letter or another `-` starts a `FLAG` token
   (`-l`, `--all`); a spaced `-` is `MINUS` (subtraction).
8. **Multi-character operators**: `==`, `!=`, `>>`, `<<`, `>=`, `<=`

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
│   ├── WhileStatement
│   ├── IfStatement
│   ├── BlockStatement
│   ├── BreakStatement
│   ├── ContinueStatement
│   ├── FunctionStatement
│   └── ReturnStatement
│
└── Expression (interface)
    ├── Identifier
    ├── PathExpression
    ├── IntegerLiteral
    ├── StringLiteral
    ├── VariableReference
    ├── CommandSubstitution
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
| `WhileStatement` | While loop | `while x < 5 { }` |
| `IfStatement` | Conditional (with else-if chains) | `if x > 5 { } else if ... { }` |
| `BlockStatement` | Block of statements | `{ stmt1; stmt2 }` |
| `BreakStatement` | Break out of a loop | `break` |
| `ContinueStatement` | Continue a loop | `continue` |
| `FunctionStatement` | Function definition | `fn add(a, b) { }` |
| `ReturnStatement` | Return from a function | `return x` |

### Expression Types

| Type | Description | Example |
|------|-------------|---------|
| `Identifier` | Variable or name | `x`, `filename` |
| `PathExpression` | File path | `./file`, `/home/user` |
| `IntegerLiteral` | Integer value | `42` |
| `StringLiteral` | String value | `"hello"` |
| `VariableReference` | Environment variable | `$HOME` |
| `CommandSubstitution` | Capture command output | `$(cwd)` |
| `Command` | Built-in or external command | `ls`, `cd ~`, `git status` |
| `PipeExpression` | Pipe operation | `ls \| print` |
| `RedirectionExpression` | I/O redirection | `ls > file.txt` |
| `InfixExpression` | Binary operation | `x + 5` |
| `CallExpression` | Function call (built-in or user) | `range(10)`, `add(1, 2)` |
| `ArrayLiteral` | Array | `[1, 2, 3]` |
| `IndexExpression` | Array access | `arr[0]` |

`Command` covers both built-ins and external programs; external commands use
the `CMD_EXTERNAL` command type and are detected when a bare word or path
appears in *command position* (the start of a statement, or the right side of a
pipe).

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
    cwd         string                            // Current working directory
    env         map[string]string                 // Environment variables (for $VAR)
    vars        map[string]Value                  // Global script variables (== scopes[0])
    scopes      []map[string]Value                // Variable scope chain; innermost is last
    funcs       map[string]*ast.FunctionStatement // User-defined functions
    searchPaths []string                          // Extra executable search dirs (raven-add path)
    stdout      io.Writer                         // Standard output (for redirections)
    stdin       io.Reader                         // Standard input (for redirections)
    // ... plus an executable-name cache for tab completion
}
```

### Scopes

Variables live in a stack of scopes (`scopes`), with the global scope at index
0. A function call pushes a new scope holding its parameters and locals; lookups
walk from the innermost scope outward, so functions can read outer variables but
their own bindings don't leak. `getVar`, `setVar`, and `setLocal` manage this.

### Control-Flow Signals

`break`, `continue`, and `return` are implemented as a `controlSignal` value
returned through the normal `error` channel. Loops intercept `ctrlBreak` /
`ctrlContinue`; `callFunction` intercepts `ctrlReturn` and yields its value. A
signal that escapes its construct surfaces as a descriptive error (e.g.
"break outside of loop").

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
func (e *Evaluator) execExport(args []string) (string, error)
func (e *Evaluator) execEnv() (string, error)
func (e *Evaluator) execRavenAdd(args []string) (string, error)
// ...
```

### External Commands

`CMD_EXTERNAL` commands run real programs:

```go
func (e *Evaluator) execExternal(name string, args []string) (string, error)
func (e *Evaluator) lookPath(name string) (string, error)   // searchPaths, then system PATH
func (e *Evaluator) buildEnv() []string                     // env + searchPaths prepended to PATH
```

`execExternal` wires the process to the evaluator's `stdin`/`stdout`, runs it in
`cwd`, and inherits the merged environment, so external commands participate in
pipes and redirection. If a bare name with no args matches a defined variable,
it resolves to that value instead of running a program.

### Built-in Functions

Built-in functions are dispatched in `evalCallExpression`:

```go
func (e *Evaluator) builtinRange(args []ast.Expression) (Value, error)
func (e *Evaluator) builtinAppend(args []ast.Expression) (Value, error)
// String/collection helpers: len, split, join, contains, upper, lower, trim, replace
```

User-defined functions are looked up in `funcs` and invoked via `callFunction`.

## Readline Package

**Location:** `readline/readline.go`

Provides interactive line editing.

### Features

- Raw terminal mode handling
- Cursor movement
- Command history (arrow keys), persisted to `~/.raven_history`
- Tab completion (built-ins, user functions, PATH executables, file paths)
- Fish-style inline autosuggestions from history (dim text, accepted with
  →/Ctrl+E/End)
- Keyboard shortcuts (Ctrl+A, Ctrl+E, etc.)

### Key Methods

| Method | Description |
|--------|-------------|
| `New(prompt)` | Creates readline instance, loads persistent history |
| `ReadLine()` | Reads a line with editing support |
| `AddHistory(line)` | Adds to history (memory + file) |
| `SetCwdFunc(fn)` | Sets function for path completion |
| `SetCommandProvider(fn)` | Supplies dynamic command names (functions, PATH executables) |
| `suggestionFor(line)` | Computes the inline autosuggestion |

The colored prompt itself is built in `main.go` (`makePrompt`), refreshed each
iteration so it tracks the current directory. ANSI codes come from the `ansi`
package.

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
- `lexer/lexer_test.go`: Tokenization (flags, hyphenated identifiers, newline tracking)
- `parser/parser_test.go`: Parser correctness tests
- `evaluator/evaluator_test.go`: Execution behavior (arithmetic, control flow, functions, env, builtins)
- `readline/readline_test.go`: History persistence, autosuggestions, completion merging

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
