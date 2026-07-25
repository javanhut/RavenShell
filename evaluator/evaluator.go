package evaluator

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"ravenshell/ansi"
	"ravenshell/ast"
	"ravenshell/token"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// Value represents any value in the shell
type Value any

// controlKind identifies a non-local control-flow transfer.
type controlKind int

const (
	ctrlBreak controlKind = iota
	ctrlContinue
	ctrlReturn
)

// controlSignal is propagated as an error to unwind loops and functions for
// break, continue, and return. It is intercepted by the relevant evaluator and
// never surfaces to the user as a real error.
type controlSignal struct {
	kind  controlKind
	value Value
}

func (c *controlSignal) Error() string {
	switch c.kind {
	case ctrlBreak:
		return "break outside of loop"
	case ctrlContinue:
		return "continue outside of loop"
	default:
		return "return outside of function"
	}
}

func asControl(err error) (*controlSignal, bool) {
	sig, ok := err.(*controlSignal)
	return sig, ok
}

// Evaluator executes AST nodes
type Evaluator struct {
	cwd          string                            // Current working directory
	env          map[string]string                 // Environment variables (for $VAR)
	vars         map[string]Value                  // Global script variables (== scopes[0])
	scopes       []map[string]Value                // Variable scope chain; innermost is last
	funcs        map[string]*ast.FunctionStatement // User-defined functions
	aliases      map[string][]string               // Interactive command aliases
	aliasDepth   int                               // Guards recursive aliases
	sourceDepth  int                               // Guards recursive raven-source calls
	searchPaths  []string                          // Extra executable search dirs (raven-add path)
	defaultPaths []string                          // Standard system executable dirs for this OS
	stdout       io.Writer                         // Standard output (for redirections)
	stderr       io.Writer                         // Standard error (for redirections, e.g. 2>file, 2>&1)
	stdin        io.Reader                         // Standard input (for redirections)

	lastStatus int // exit status of the last command ($?)

	jobs      []*job // background jobs
	nextJobID int
	jobsMu    sync.Mutex

	interrupted int32 // set by Interrupt() (SIGINT); checked by loops

	execCache      []string // cached executable names from search/system PATH
	execCacheValid bool
}

// ErrInterrupted is returned when evaluation is interrupted by SIGINT (Ctrl-C).
var ErrInterrupted = errors.New("interrupted")

// ExitRequest is the structured control signal produced by exit(). Hosts can
// distinguish it from a runtime failure and terminate with the requested code.
type ExitRequest struct{ Status int }

func (e *ExitRequest) Error() string { return fmt.Sprintf("exit requested with status %d", e.Status) }

// RuntimeError attaches source coordinates to an evaluation failure.
type RuntimeError struct {
	Line   int
	Column int
	Cause  error
}

func (e *RuntimeError) Error() string { return fmt.Sprintf("%d:%d: %v", e.Line, e.Column, e.Cause) }
func (e *RuntimeError) Unwrap() error { return e.Cause }

// New creates a new Evaluator
func New() *Evaluator {
	cwd, _ := os.Getwd()
	global := make(map[string]Value)
	e := &Evaluator{
		cwd:       cwd,
		env:       make(map[string]string),
		vars:      global,
		scopes:    []map[string]Value{global},
		funcs:     make(map[string]*ast.FunctionStatement),
		aliases:   make(map[string][]string),
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		stdin:     os.Stdin,
		nextJobID: 1,
	}
	e.defaultPaths = defaultExecPaths()
	e.loadSearchPaths()
	return e
}

// NewWithArgs creates an evaluator and exposes script arguments through the
// global RavenScript `args` array. The script filename is intentionally not
// included: RavenScript treats arguments as language data rather than emulating
// a POSIX shell's numbered variables.
func NewWithArgs(args []string) *Evaluator {
	e := New()
	e.SetScriptArgs(args)
	return e
}

// SetScriptArgs replaces the global `args` array.
func (e *Evaluator) SetScriptArgs(args []string) {
	values := make([]Value, len(args))
	for i, arg := range args {
		values[i] = arg
	}
	e.scopes[0]["args"] = values
}

// LastStatus returns the status produced by the most recently evaluated
// command. It is used by non-interactive entry points as their process status.
func (e *Evaluator) LastStatus() int { return e.lastStatus }

// defaultExecPaths returns the standard system executable directories for the
// host OS, plus common per-user bin dirs that exist. They are always searched
// (and merged into a child's PATH) so basic tools resolve without the user
// having to `raven-add path` standard locations like /usr/bin or
// /opt/homebrew/bin. Only directories that exist are returned.
func defaultExecPaths() []string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/opt/homebrew/bin", "/opt/homebrew/sbin",
			"/usr/local/bin", "/usr/local/sbin",
			"/usr/bin", "/bin", "/usr/sbin", "/sbin",
		}
	default: // linux and other unix-likes
		candidates = []string{
			"/usr/local/sbin", "/usr/local/bin",
			"/usr/sbin", "/usr/bin",
			"/sbin", "/bin",
		}
	}
	// Common per-user tool directories.
	if home, err := os.UserHomeDir(); err == nil {
		for _, sub := range []string{".local/bin", "go/bin", ".cargo/bin", ".bun/bin"} {
			candidates = append(candidates, filepath.Join(home, sub))
		}
	}

	var dirs []string
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// composePath joins ordered groups of directories into a single PATH value,
// dropping empty entries and duplicates while preserving first-seen order.
func composePath(groups ...[]string) string {
	seen := make(map[string]struct{})
	var out []string
	for _, group := range groups {
		for _, dir := range group {
			if dir == "" {
				continue
			}
			if _, ok := seen[dir]; ok {
				continue
			}
			seen[dir] = struct{}{}
			out = append(out, dir)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// Interrupt marks evaluation as interrupted (called from a SIGINT handler).
// Running RavenShell loops will unwind with ErrInterrupted.
func (e *Evaluator) Interrupt() {
	atomic.StoreInt32(&e.interrupted, 1)
}

// ClearInterrupt resets the interrupt flag (called before each REPL line).
func (e *Evaluator) ClearInterrupt() {
	atomic.StoreInt32(&e.interrupted, 0)
}

func (e *Evaluator) checkInterrupt() bool {
	return atomic.LoadInt32(&e.interrupted) == 1
}

// configPath returns the path to a RavenShell config file in the user's home
// directory, or "" if the home directory cannot be determined.
func configPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, name)
}

// searchPathsFile is where extra executable search directories are persisted.
const searchPathsFile = ".raven_paths"

// loadSearchPaths reads persisted extra search directories into the evaluator.
func (e *Evaluator) loadSearchPaths() {
	path := configPath(searchPathsFile)
	if path == "" {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		dir := strings.TrimSpace(line)
		if dir != "" {
			e.searchPaths = append(e.searchPaths, dir)
		}
	}
}

// pushScope adds a new innermost variable scope (e.g. a function call frame).
func (e *Evaluator) pushScope() {
	e.scopes = append(e.scopes, make(map[string]Value))
}

// popScope removes the innermost scope.
func (e *Evaluator) popScope() {
	e.scopes = e.scopes[:len(e.scopes)-1]
}

// getVar looks up a variable from the innermost scope outward.
func (e *Evaluator) getVar(name string) (Value, bool) {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if v, ok := e.scopes[i][name]; ok {
			return v, true
		}
	}
	return nil, false
}

// setVar assigns to an existing binding wherever it lives, or creates a new
// binding in the innermost scope.
func (e *Evaluator) setVar(name string, val Value) {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if _, ok := e.scopes[i][name]; ok {
			e.scopes[i][name] = val
			return
		}
	}
	e.scopes[len(e.scopes)-1][name] = val
}

// setLocal binds a variable in the innermost scope unconditionally (used for
// function parameters).
func (e *Evaluator) setLocal(name string, val Value) {
	e.scopes[len(e.scopes)-1][name] = val
}

// Eval evaluates a program and returns the result
func (e *Evaluator) Eval(program *ast.Program) error {
	for _, stmt := range program.Statements {
		if err := e.evalStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) evalStatement(stmt ast.Statement) error {
	err := e.evalStatementRaw(stmt)
	if err == nil {
		return nil
	}
	if _, ok := asControl(err); ok || errors.Is(err, ErrInterrupted) {
		return err
	}
	var exit *ExitRequest
	var runtimeErr *RuntimeError
	if errors.As(err, &exit) || errors.As(err, &runtimeErr) {
		return err
	}
	tok := statementToken(stmt)
	return &RuntimeError{Line: tok.Line, Column: tok.Column, Cause: err}
}

func (e *Evaluator) evalStatementRaw(stmt ast.Statement) (err error) {
	// A bug in the evaluator must not kill the shell. Recover the panic as an
	// ordinary evaluation error (evalStatement stamps line:column onto it) and
	// restore the streams a redirection or command substitution was unwound
	// past, since those swap e.stdout/stderr/stdin back without a defer.
	// Control flow (break/continue/return, exit, ^C) travels by error, never by
	// panic, so nothing here can swallow it.
	out, errOut, in := e.stdout, e.stderr, e.stdin
	defer func() {
		if r := recover(); r != nil {
			e.stdout, e.stderr, e.stdin = out, errOut, in
			e.lastStatus = 1
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	switch s := stmt.(type) {
	case *ast.ExpressionStatement:
		_, err := e.evalExpressionValue(s.Expression)
		return err
	case *ast.AssignmentStatement:
		return e.evalAssignment(s)
	case *ast.ForStatement:
		return e.evalForStatement(s)
	case *ast.WhileStatement:
		return e.evalWhileStatement(s)
	case *ast.IfStatement:
		return e.evalIfStatement(s)
	case *ast.BreakStatement:
		return &controlSignal{kind: ctrlBreak}
	case *ast.ContinueStatement:
		return &controlSignal{kind: ctrlContinue}
	case *ast.FunctionStatement:
		e.funcs[s.Name.Value] = s
		return nil
	case *ast.ReturnStatement:
		return e.evalReturnStatement(s)
	}
	return nil
}

func statementToken(stmt ast.Statement) token.Token {
	switch node := stmt.(type) {
	case *ast.ExpressionStatement:
		return node.Token
	case *ast.AssignmentStatement:
		return node.Token
	case *ast.ForStatement:
		return node.Token
	case *ast.WhileStatement:
		return node.Token
	case *ast.IfStatement:
		return node.Token
	case *ast.BlockStatement:
		return node.Token
	case *ast.BreakStatement:
		return node.Token
	case *ast.ContinueStatement:
		return node.Token
	case *ast.FunctionStatement:
		return node.Token
	case *ast.ReturnStatement:
		return node.Token
	default:
		return token.Token{Line: 1, Column: 1}
	}
}

// evalReturnStatement evaluates the return value (if any) and raises a return
// control signal to unwind to the calling function.
func (e *Evaluator) evalReturnStatement(stmt *ast.ReturnStatement) error {
	var val Value
	if stmt.Value != nil {
		v, err := e.evalExpressionValue(stmt.Value)
		if err != nil {
			return err
		}
		val = v
	}
	return &controlSignal{kind: ctrlReturn, value: val}
}

// callFunction invokes a user-defined function with already-evaluated args.
func (e *Evaluator) callFunction(fn *ast.FunctionStatement, args []Value) (Value, error) {
	if len(args) != len(fn.Parameters) {
		return nil, fmt.Errorf("%s() expects %d argument(s), got %d",
			fn.Name.Value, len(fn.Parameters), len(args))
	}

	e.pushScope()
	defer e.popScope()
	for i, param := range fn.Parameters {
		e.setLocal(param.Value, args[i])
	}

	if err := e.evalBlockStatement(fn.Body); err != nil {
		if sig, ok := asControl(err); ok && sig.kind == ctrlReturn {
			return sig.value, nil
		}
		return nil, err
	}
	// Functions with no explicit return yield nil.
	return nil, nil
}

// evalExpressionValue evaluates an expression and returns a Value
func (e *Evaluator) evalExpressionValue(expr ast.Expression) (Value, error) {
	switch node := expr.(type) {
	case *ast.Command:
		result, err := e.evalCommand(node)
		return result, err
	case *ast.PipeExpression:
		result, err := e.evalPipe(node)
		return result, err
	case *ast.RedirectionExpression:
		result, err := e.evalRedirection(node)
		return result, err
	case *ast.Identifier:
		// Check if it's a variable first
		if val, ok := e.getVar(node.Value); ok {
			return val, nil
		}
		if node.Value == "true" {
			return true, nil
		}
		if node.Value == "false" {
			return false, nil
		}
		return node.Value, nil
	case *ast.PathExpression:
		// A path used as a command argument is passed through verbatim, with
		// only ~ expanded. Rewriting "." or a relative path to an absolute one
		// here breaks external commands that treat "." specially (e.g. tools
		// whose "stage everything" path differs from staging a named dir).
		// Builtins re-resolve their own args against e.cwd via resolvePath.
		return e.expandHome(node.Value), nil
	case *ast.StringLiteral:
		if node.Interpolate {
			return e.interpolate(node.Value), nil
		}
		return node.Value, nil
	case *ast.IntegerLiteral:
		return node.Value, nil
	case *ast.VariableReference:
		// A shell variable keeps its type, so `$x - 1` is arithmetic rather
		// than a failed string subtraction. Names that only exist in the
		// environment stay strings.
		if val, ok := e.getVar(node.Name.Value); ok {
			return val, nil
		}
		return e.expandVariable(node.Name.Value), nil
	case *ast.LastStatus:
		return int64(e.lastStatus), nil
	case *ast.LogicalExpression:
		return e.evalLogical(node)
	case *ast.BackgroundExpression:
		return e.evalBackground(node)
	case *ast.CommandSubstitution:
		return e.evalCommandSubstitution(node)
	case *ast.WordExpression:
		// A composite word: concatenate the string value of each part (literal
		// runs and $-expansions) into a single argument.
		var sb strings.Builder
		for _, part := range node.Parts {
			val, err := e.evalExpressionValue(part)
			if err != nil {
				return nil, err
			}
			sb.WriteString(e.valueToString(val))
		}
		word := sb.String()
		// A word that began with a bare '~' token (e.g. ~/Downloads/"a b.mp4")
		// still gets tilde expansion — the '~' is unquoted, only a later part is
		// quoted. Words led by a quoted "~..." keep the token type of the string,
		// so their tilde stays literal, matching shell semantics.
		if node.Token.Type == token.TILDE {
			word = e.expandHome(word)
		}
		return word, nil
	case *ast.BraceExpression:
		// Expand the literal brace group into a list of argument strings. The
		// []Value result is splatted into multiple arguments by evalArgs.
		inner, err := e.evalExpressionValue(node.Word)
		if err != nil {
			return nil, err
		}
		words := braceExpand(e.valueToString(inner))
		vals := make([]Value, len(words))
		for i, w := range words {
			vals[i] = w
		}
		return vals, nil
	case *ast.InfixExpression:
		return e.evalInfixExpression(node)
	case *ast.CallExpression:
		return e.evalCallExpression(node)
	case *ast.ArrayLiteral:
		return e.evalArrayLiteral(node)
	case *ast.IndexExpression:
		return e.evalIndexExpression(node)
	}
	return nil, fmt.Errorf("unknown expression type: %T", expr)
}

// evalExpression evaluates an expression and returns a string (for backwards compatibility)
func (e *Evaluator) evalExpression(expr ast.Expression) (string, error) {
	val, err := e.evalExpressionValue(expr)
	if err != nil {
		return "", err
	}
	return e.valueToString(val), nil
}

// valueToString converts a Value to a string
func (e *Evaluator) valueToString(val Value) string {
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []Value:
		strs := make([]string, len(v))
		for i, elem := range v {
			strs[i] = e.valueToString(elem)
		}
		return "[" + strings.Join(strs, ", ") + "]"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// valueToInt64 converts a Value to int64
func (e *Evaluator) valueToInt64(val Value) (int64, error) {
	switch v := val.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to integer", val)
	}
}

// valueToBool converts a Value to bool
func (e *Evaluator) valueToBool(val Value) bool {
	switch v := val.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case string:
		return v != ""
	case []Value:
		return len(v) > 0
	default:
		return val != nil
	}
}

func (e *Evaluator) evalCommand(cmd *ast.Command) (string, error) {
	// Evaluate arguments (array-valued arguments are splatted into multiple args)
	args, err := e.evalArgs(cmd.Arguments, cmd.Type == ast.CMD_EXTERNAL)
	if err != nil {
		return "", err
	}

	var result string
	if cmd.Type == ast.CMD_EXTERNAL {
		if expansion, ok := e.aliases[cmd.Name]; ok {
			result, err = e.execAlias(cmd.Name, expansion, args)
		} else {
			result, err = e.dispatchCommand(cmd, args)
		}
	} else {
		result, err = e.dispatchCommand(cmd, args)
	}

	// Track exit status for $?. External commands set their own status; other
	// commands are 0 on success and 1 on error.
	if cmd.Type != ast.CMD_EXTERNAL {
		if err != nil {
			e.lastStatus = 1
		} else {
			e.lastStatus = 0
		}
	}
	return result, err
}

// evalArgs evaluates command argument expressions, splatting array values into
// multiple string arguments (so glob(...) and arrays expand naturally).
//
// When external is true the arguments are for an external program: path
// arguments are passed verbatim (only '~' is expanded) rather than being
// resolved to an absolute path. The child process inherits the shell's working
// directory, so a relative path like ./build or a URL stays exactly as written.
func (e *Evaluator) evalArgs(arguments []ast.Expression, external bool) ([]string, error) {
	var args []string
	for _, arg := range arguments {
		val, err := e.evalArg(arg, external)
		if err != nil {
			return nil, err
		}
		if arr, ok := val.([]Value); ok {
			for _, el := range arr {
				args = append(args, e.expandGlob(arg, e.valueToString(el))...)
			}
		} else {
			args = append(args, e.expandGlob(arg, e.valueToString(val))...)
		}
	}
	return args, nil
}

// expandGlob expands an unquoted argument word containing a glob metacharacter
// into its sorted matches. Quoted words keep their StringLiteral shape and are
// never expanded, and a pattern that matches nothing is passed through
// verbatim, the way bash does.
func (e *Evaluator) expandGlob(arg ast.Expression, s string) []string {
	switch arg.(type) {
	case *ast.PathExpression, *ast.WordExpression, *ast.BraceExpression:
	default:
		return []string{s}
	}
	if !strings.ContainsAny(s, "*?[") {
		return []string{s}
	}
	matches, err := e.builtinGlob([]Value{s})
	if err != nil {
		return []string{s} // bad pattern: keep the word text, not an error
	}
	hidden := strings.HasPrefix(filepath.Base(s), ".")
	var out []string
	for _, m := range matches.([]Value) {
		name := e.valueToString(m)
		// filepath.Glob's '*' matches a leading dot but a shell's does not, so
		// `rm *` must not sweep up .git or .env.
		// ponytail: only the last segment is checked (*/.git/* is not);
		// per-segment matching if someone hits it.
		if !hidden && strings.HasPrefix(filepath.Base(name), ".") {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

// evalArg evaluates a single command argument. For external commands a path
// argument is expanded only for a leading '~' and otherwise passed through
// verbatim, so relative paths (./build) and URLs (https://...) reach the
// program exactly as the user wrote them instead of being absolutized.
func (e *Evaluator) evalArg(arg ast.Expression, external bool) (Value, error) {
	if external {
		if pe, ok := arg.(*ast.PathExpression); ok {
			return e.expandHome(pe.Value), nil
		}
	}
	return e.evalExpressionValue(arg)
}

func (e *Evaluator) dispatchCommand(cmd *ast.Command, args []string) (string, error) {
	// Execute command based on type
	switch cmd.Type {
	case ast.CMD_LIST:
		return e.execList(args)
	case ast.CMD_CHANGEDIR:
		return e.execChangeDir(args)
	case ast.CMD_CURRENTDIR:
		return e.execCurrentDir()
	case ast.CMD_MAKEDIR:
		return e.execMakeDir(cmd.Name, args)
	case ast.CMD_REMOVEDIR:
		return e.execRemoveDir(args)
	case ast.CMD_REMOVE:
		return e.execRemove(args)
	case ast.CMD_MAKEFILE:
		return e.execMakeFile(args)
	case ast.CMD_WHOAMI:
		return e.execWhoami()
	case ast.CMD_PRINT:
		return e.execPrint(args)
	case ast.CMD_OUTPUT:
		return e.execOutput(args)
	case ast.CMD_SHOW:
		return e.execShow(args)
	case ast.CMD_CLEAR:
		return e.execClear()
	case ast.CMD_EXPORT:
		return e.execExport(args)
	case ast.CMD_ENV:
		return e.execEnv()
	case ast.CMD_RAVENADD:
		return e.execRavenAdd(args)
	case ast.CMD_RAVENHELP:
		return e.execRavenHelp(args)
	case ast.CMD_RAVENUPDATE:
		return e.execRavenUpdate(args)
	case ast.CMD_RAVENCOMPLETIONS:
		return e.execRavenCompletions(args)
	case ast.CMD_RAVENALIAS:
		return e.execRavenAlias(args)
	case ast.CMD_RAVENUNALIAS:
		return e.execRavenUnalias(args)
	case ast.CMD_RAVENSOURCE:
		return e.execRavenSource(args)
	case ast.CMD_RAVENUNSET:
		return e.execRavenUnset(args)
	case ast.CMD_RAVENTYPE:
		return e.execRavenType(args)
	case ast.CMD_PS:
		return e.execPs(args)
	case ast.CMD_KILL:
		return e.execKill(args)
	case ast.CMD_KILLALL:
		return e.execKillall(args)
	case ast.CMD_JOBS:
		return e.execJobs()
	case ast.CMD_TILDE:
		return e.execHome()
	case ast.CMD_EXTERNAL:
		return e.execExternal(cmd.Name, args)
	default:
		return "", fmt.Errorf("unknown command: %s", cmd.Name)
	}
}

// execExternal runs an external program found on PATH, wiring it to the
// evaluator's current stdin/stdout/cwd/env so pipes and redirections work.
func (e *Evaluator) execExternal(name string, args []string) (string, error) {
	path, err := e.lookPath(name)
	if err != nil {
		// A bare name with no arguments that matches a defined variable is a
		// value reference (e.g. inspecting `x` or using it in an expression),
		// not a command invocation.
		if len(args) == 0 {
			if val, ok := e.getVar(name); ok {
				return e.valueToString(val), nil
			}
		}
		// Like a real shell, an existing but non-executable file and a missing
		// command set distinct non-zero statuses and report to stderr, but do
		// not abort the program.
		if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "%s: permission denied\n", name)
			e.lastStatus = 126
			return "", nil
		}
		fmt.Fprintf(os.Stderr, "%s: command not found\n", name)
		e.lastStatus = 127
		return "", nil
	}

	// Snapshot the installed-command set before a package manager runs so its
	// effect can be reconciled into the completion cache afterward.
	var pkgBefore map[string]bool
	if isPackageOp(name, args) {
		pkgBefore = e.commandNameSet()
	}

	c := exec.Command(path, args...)
	c.Dir = e.cwd
	c.Stdin = e.stdin
	c.Stdout = e.stdout
	c.Stderr = e.stderr
	c.Env = e.buildEnv()

	err = c.Run()
	if errors.Is(err, syscall.ENOEXEC) {
		// Executable scripts with no shebang line: POSIX shells fall back to
		// interpreting the file with /bin/sh.
		c = exec.Command("/bin/sh", append([]string{path}, args...)...)
		c.Dir = e.cwd
		c.Stdin = e.stdin
		c.Stdout = e.stdout
		c.Stderr = e.stderr
		c.Env = e.buildEnv()
		err = c.Run()
	}
	e.lastStatus = exitStatus(err)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			// A real run error (not just a non-zero exit) - report it.
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		}
	}

	// A package manager that finished successfully may have installed or removed
	// commands; reconcile their completions so they appear (or disappear)
	// without a manual `raven-completions update`.
	if pkgBefore != nil && e.lastStatus == 0 {
		e.reconcileCompletionsAfterPkgOp(pkgBefore)
	}
	return "", nil
}

// exitStatus maps a command run error to a numeric exit status.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

// colorOutput reports whether the evaluator's stdout is an interactive
// terminal, so colored output is suppressed for pipes, captures, and files.
func (e *Evaluator) colorOutput() bool {
	f, ok := e.stdout.(*os.File)
	return ok && f == os.Stdout && term.IsTerminal(int(f.Fd()))
}

// lookPath resolves a command name to an executable path. Names containing a
// path separator are resolved against the shell's current directory; bare
// names are searched in the extra search paths first, then the system PATH.
func (e *Evaluator) lookPath(name string) (string, error) {
	if strings.ContainsRune(name, '/') {
		resolved := e.resolvePath(name)
		if isExecutableFile(resolved) {
			return resolved, nil
		}
		if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
			return "", os.ErrPermission
		}
		return "", exec.ErrNotFound
	}

	for _, dir := range e.searchPaths {
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}

	// Try the inherited system PATH next.
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	} else {
		// Fall back to the standard system directories so common tools resolve
		// even when the inherited PATH is minimal or empty (e.g. when the shell
		// is launched from a GUI with a stripped-down environment).
		for _, dir := range e.defaultPaths {
			candidate := filepath.Join(dir, name)
			if isExecutableFile(candidate) {
				return candidate, nil
			}
		}
		return "", err
	}
}

// isExecutableFile reports whether path is a regular file with an execute bit.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0111 != 0
}

// buildEnv merges the process environment with shell-local variables and the
// executable search paths so external commands see both inherited and user-set
// ($VAR) values, any raven-add path directories (prepended to PATH), and the OS
// default directories (appended) so basic tools resolve even when the inherited
// PATH is minimal.
func (e *Evaluator) buildEnv() []string {
	merged := make(map[string]string)
	for _, kv := range os.Environ() {
		if before, after, ok := strings.Cut(kv, "="); ok {
			merged[before] = after
		}
	}
	maps.Copy(merged, e.env)
	// Compose PATH as: raven-add dirs, then the inherited PATH, then the OS
	// default dirs. The defaults guarantee basic tools are reachable even when
	// the inherited PATH is minimal, without the user adding them by hand.
	merged["PATH"] = composePath(e.searchPaths, filepath.SplitList(merged["PATH"]), e.defaultPaths)

	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, k+"="+v)
	}
	return env
}

func (e *Evaluator) evalPipe(pipe *ast.PipeExpression) (string, error) {
	var stages []ast.Expression
	flattenPipeline(pipe, &stages)
	if len(stages) < 2 {
		return e.evalExpression(stages[0])
	}

	// io.Pipe provides bounded, streaming backpressure. Every stage starts
	// concurrently, so infinite producers work with bounded consumers
	// (`yes | head`) and large pipelines do not accumulate in memory.
	readers := make([]*io.PipeReader, len(stages)-1)
	writers := make([]*io.PipeWriter, len(stages)-1)
	for i := range readers {
		readers[i], writers[i] = io.Pipe()
	}

	type stageResult struct {
		index  int
		value  string
		status int
		err    error
	}
	results := make(chan stageResult, len(stages))
	for i, stage := range stages {
		var stdin io.Reader = e.stdin
		var stdout io.Writer = e.stdout
		if i > 0 {
			stdin = readers[i-1]
		}
		if i < len(stages)-1 {
			stdout = writers[i]
		}

		go func(index int, expression ast.Expression, in io.Reader, out io.Writer) {
			// A panic in a goroutine is unrecoverable from the parent, so each
			// stage guards itself. Declared first so it runs after the pipe
			// closes below, and so the parent never blocks forever on results.
			defer func() {
				if r := recover(); r != nil {
					results <- stageResult{index: index, status: 1, err: fmt.Errorf("internal error: %v", r)}
				}
			}()
			if index > 0 {
				defer readers[index-1].Close()
			}
			if index < len(stages)-1 {
				defer writers[index].Close()
			}
			child := e.forkForPipeline(in, out)
			value, err := child.evalExpression(expression)
			status := child.lastStatus
			if err != nil {
				status = 1
			}
			results <- stageResult{index: index, value: value, status: status, err: err}
		}(i, stage, stdin, stdout)
	}

	completed := make([]stageResult, len(stages))
	for range stages {
		result := <-results
		completed[result.index] = result
	}
	last := completed[len(completed)-1]
	e.lastStatus = last.status
	for _, result := range completed {
		if result.err != nil {
			return "", result.err
		}
	}
	return last.value, nil
}

func flattenPipeline(expression ast.Expression, stages *[]ast.Expression) {
	if pipe, ok := expression.(*ast.PipeExpression); ok {
		flattenPipeline(pipe.Left, stages)
		flattenPipeline(pipe.Right, stages)
		return
	}
	*stages = append(*stages, expression)
}

// forkForPipeline creates a subshell-like evaluator for one concurrent stage.
// Mutable language state is copied so assignments or cd in a stage neither race
// with sibling stages nor leak back into the parent evaluator.
func (e *Evaluator) forkForPipeline(stdin io.Reader, stdout io.Writer) *Evaluator {
	scopes := make([]map[string]Value, len(e.scopes))
	for i, scope := range e.scopes {
		scopes[i] = maps.Clone(scope)
	}
	aliases := make(map[string][]string, len(e.aliases))
	for name, expansion := range e.aliases {
		aliases[name] = append([]string(nil), expansion...)
	}
	return &Evaluator{
		cwd:            e.cwd,
		env:            maps.Clone(e.env),
		vars:           scopes[0],
		scopes:         scopes,
		funcs:          maps.Clone(e.funcs),
		aliases:        aliases,
		sourceDepth:    e.sourceDepth,
		searchPaths:    append([]string(nil), e.searchPaths...),
		defaultPaths:   append([]string(nil), e.defaultPaths...),
		stdout:         stdout,
		stderr:         e.stderr,
		stdin:          stdin,
		lastStatus:     e.lastStatus,
		nextJobID:      1,
		execCache:      append([]string(nil), e.execCache...),
		execCacheValid: e.execCacheValid,
	}
}

func (e *Evaluator) evalRedirection(redir *ast.RedirectionExpression) (string, error) {
	// fd duplication (N>&M): point the source fd's sink at the target fd's
	// current sink, e.g. 2>&1 makes stderr follow wherever stdout goes. This is
	// what `cmd 2>&1 | tail` relies on: the pipe has already aimed stdout at its
	// buffer, so stderr now lands there too.
	if redir.IsDup {
		src := redir.SrcFd
		if src == 0 {
			src = 1
		}
		var dup io.Writer
		switch redir.DupFd {
		case 1:
			dup = e.stdout
		case 2:
			dup = e.stderr
		default:
			return "", fmt.Errorf("unsupported fd duplication target %d", redir.DupFd)
		}
		oldOut, oldErr := e.stdout, e.stderr
		if src == 2 {
			e.stderr = dup
		} else {
			e.stdout = dup
		}
		result, err := e.evalExpression(redir.Command)
		e.stdout, e.stderr = oldOut, oldErr
		return result, err
	}

	// Get target filename
	target, err := e.evalExpression(redir.Target)
	if err != nil {
		return "", err
	}

	// Resolve path
	targetPath := e.resolvePath(target)

	switch redir.Type {
	case ast.REDIR_OUTPUT, ast.REDIR_APPEND:
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if redir.Type == ast.REDIR_APPEND {
			flags = os.O_APPEND | os.O_CREATE | os.O_WRONLY
		}
		file, err := os.OpenFile(targetPath, flags, 0644)
		if err != nil {
			return "", fmt.Errorf("cannot create file %s: %v", target, err)
		}
		defer file.Close()

		// &>file sends both streams to the file; N>file targets the chosen fd
		// (default stdout); the rest send stdout.
		oldOut, oldErr := e.stdout, e.stderr
		if redir.Both {
			e.stdout, e.stderr = file, file
		} else if redir.SrcFd == 2 {
			e.stderr = file
		} else {
			e.stdout = file
		}
		result, err := e.evalExpression(redir.Command)
		e.stdout, e.stderr = oldOut, oldErr
		return result, err

	case ast.REDIR_INPUT:
		// Read from file
		file, err := os.Open(targetPath)
		if err != nil {
			return "", fmt.Errorf("cannot open file %s: %v", target, err)
		}
		defer file.Close()

		oldStdin := e.stdin
		e.stdin = file
		result, err := e.evalExpression(redir.Command)
		e.stdin = oldStdin
		return result, err

	case ast.REDIR_HEREDOC:
		// For heredoc, target is the delimiter - not implemented yet
		return "", fmt.Errorf("heredoc not yet implemented")
	}

	return "", nil
}

// Command implementations

// stripFlags drops leading-dash flag arguments. Built-in file commands accept
// flags like -p, -rf, or -la for muscle-memory compatibility but ignore them
// (mkdir always creates parents, rm is always recursive, etc.).
func stripFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// parseFlags splits args into positional operands and a set of flag names.
// Long flags (--force, --max=5) land in the set by their name with any =value
// stripped; bundled short flags (-rf) are split into individual letters ("r",
// "f"). A lone "-" is treated as a positional operand, not a flag.
func parseFlags(args []string) (positionals []string, flags map[string]bool) {
	flags = make(map[string]bool)
	for _, a := range args {
		switch {
		case a == "-" || !strings.HasPrefix(a, "-"):
			positionals = append(positionals, a)
		case strings.HasPrefix(a, "--"):
			name := strings.TrimPrefix(a, "--")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			flags[name] = true
		default:
			for _, c := range a[1:] {
				flags[string(c)] = true
			}
		}
	}
	return positionals, flags
}

// hasFlag reports whether any of the given flag names were present.
func hasFlag(flags map[string]bool, names ...string) bool {
	for _, n := range names {
		if flags[n] {
			return true
		}
	}
	return false
}

// lsRowsPerColumn is how many entries fill one column of `ls` output before the
// next column begins. Columns are added as needed (10, then 20, then 30, ...),
// so a directory with 25 entries lists as columns of 10, 10, and 5.
const lsRowsPerColumn = 10

func (e *Evaluator) execList(args []string) (string, error) {
	operands := stripFlags(args)
	if len(operands) == 0 {
		operands = []string{e.cwd}
	}

	color := e.colorOutput()

	// Build the visible names (with a trailing / for directories), their colored
	// display forms, and the plain one-per-line listing that is returned for
	// pipes and command substitution.
	var names, display []string
	var plain bytes.Buffer
	for _, arg := range operands {
		info, err := os.Stat(e.resolvePath(arg))
		if err != nil {
			return "", fmt.Errorf("ls: %v", err)
		}
		if !info.IsDir() {
			// Operands that are not directories - the file names a glob such as
			// `ls *.txt` expands to - list as themselves.
			// ponytail: no colorizeEntry here, it needs an os.DirEntry.
			names = append(names, arg)
			display = append(display, arg)
			plain.WriteString(arg + "\n")
			continue
		}
		entries, err := os.ReadDir(e.resolvePath(arg))
		if err != nil {
			return "", fmt.Errorf("ls: %v", err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			names = append(names, name)
			display = append(display, e.colorizeEntry(entry, name, color))
			plain.WriteString(name + "\n")
		}
	}

	// To an interactive terminal, lay the entries out in columns. When the
	// output is piped or captured (not a terminal), keep one entry per line so
	// downstream tools and $(ls) can parse it.
	if color {
		fmt.Fprint(e.stdout, formatColumns(names, display, lsRowsPerColumn))
	} else {
		for _, d := range display {
			fmt.Fprintln(e.stdout, d)
		}
	}
	return plain.String(), nil
}

// formatColumns arranges entries into columns filled top-to-bottom, with
// perCol entries per column (the last column may hold fewer). names holds the
// visible text used for width/alignment; display holds the matching (possibly
// colored) text that is actually written. Columns are sized to their widest
// member and separated by two spaces.
func formatColumns(names, display []string, perCol int) string {
	n := len(names)
	if n == 0 {
		return ""
	}

	numCols := (n + perCol - 1) / perCol
	rows := perCol
	if numCols == 1 {
		rows = n
	}

	// Width of each column = widest visible name it contains.
	colWidth := make([]int, numCols)
	for c := range numCols {
		for r := range perCol {
			idx := c*perCol + r
			if idx >= n {
				break
			}
			if w := utf8.RuneCountInString(names[idx]); w > colWidth[c] {
				colWidth[c] = w
			}
		}
	}

	const gap = 2
	var out bytes.Buffer
	for r := 0; r < rows; r++ {
		for c := range numCols {
			idx := c*perCol + r
			if idx >= n {
				continue
			}
			out.WriteString(display[idx])
			// Pad to the column width + gap, unless nothing follows on this row.
			if next := (c+1)*perCol + r; next < n {
				out.WriteString(strings.Repeat(" ", colWidth[c]-utf8.RuneCountInString(names[idx])+gap))
			}
		}
		out.WriteByte('\n')
	}
	return out.String()
}

// colorizeEntry applies a color to a directory listing entry based on its
// type. When color is false the name is returned unchanged.
func (e *Evaluator) colorizeEntry(entry os.DirEntry, name string, color bool) string {
	if !color {
		return name
	}
	switch {
	case entry.IsDir():
		return ansi.Wrap(ansi.Bold+ansi.Blue, name)
	case entry.Type()&os.ModeSymlink != 0:
		return ansi.Wrap(ansi.Cyan, name)
	default:
		if info, err := entry.Info(); err == nil && info.Mode()&0111 != 0 {
			return ansi.Wrap(ansi.Green, name)
		}
		return name
	}
}

func (e *Evaluator) execChangeDir(args []string) (string, error) {
	args = stripFlags(args)
	if len(args) == 0 {
		// Change to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cd: %v", err)
		}
		if err := e.setCwd(home); err != nil {
			return "", fmt.Errorf("cd: %v", err)
		}
		return "", nil
	}

	// An unquoted path containing spaces arrives as multiple args (e.g.
	// `cd Statistical Methods and Data Analysis/` -> ["Statistical",
	// "Methods", "and", "Data", "Analysis/"]). Try the whole thing joined
	// back together first so directories with spaces work without quoting,
	// then fall back to the first arg alone for the `cd dir extra` case.
	candidates := []string{strings.Join(args, " ")}
	if len(args) > 1 {
		candidates = append(candidates, args[0])
	}

	var firstErr error
	for _, cand := range candidates {
		target := e.resolvePath(cand)
		info, err := os.Stat(target)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("cd: %s: no such file or directory", cand)
			}
			continue
		}
		if !info.IsDir() {
			if firstErr == nil {
				firstErr = fmt.Errorf("cd: %s: not a directory", cand)
			}
			continue
		}
		if err := e.setCwd(target); err != nil {
			return "", fmt.Errorf("cd: %v", err)
		}
		return "", nil
	}
	return "", firstErr
}

// setCwd changes the shell's working directory, keeping the evaluator's tracked
// cwd (e.cwd, used to resolve relative paths) and the OS process working
// directory in sync. The process chdir matters for host terminals such as
// RavenTerminal, which read the shell process's cwd (via /proc or lsof) to open
// new tabs and splits in the same directory — without it, cd would move the
// shell but a new tab would still open where the shell first started.
func (e *Evaluator) setCwd(dir string) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	e.cwd = dir
	return nil
}

func (e *Evaluator) execCurrentDir() (string, error) {
	fmt.Fprintln(e.stdout, e.cwd)
	return e.cwd, nil
}

func (e *Evaluator) execMakeDir(commandName string, args []string) (string, error) {
	paths, flags := parseFlags(args)
	if len(paths) == 0 {
		return "", fmt.Errorf("mkdir: missing operand")
	}
	// makedir is RavenShell's intentionally convenient spelling; mkdir retains
	// the conventional requirement for -p when parents are missing.
	parents := commandName == "makedir" || hasFlag(flags, "p", "parents")
	for _, arg := range paths {
		path := e.resolvePath(arg)
		var err error
		if parents {
			err = os.MkdirAll(path, 0755)
		} else {
			err = os.Mkdir(path, 0755)
		}
		if err != nil {
			return "", fmt.Errorf("mkdir: %v", err)
		}
	}
	return "", nil
}

func (e *Evaluator) execRemoveDir(args []string) (string, error) {
	paths, flags := parseFlags(args)
	if len(paths) == 0 {
		return "", fmt.Errorf("rmdir: missing operand")
	}
	force := hasFlag(flags, "f", "force")

	for _, arg := range paths {
		path := e.resolvePath(arg)
		if force {
			if err := os.RemoveAll(path); err != nil {
				return "", fmt.Errorf("rmdir: %v", err)
			}
			continue
		}
		// Without --force, only empty directories are removed (the safe default).
		if err := os.Remove(path); err != nil {
			if strings.Contains(err.Error(), "not empty") {
				return "", fmt.Errorf("rmdir: %v (use -f/--force to remove a non-empty directory)", err)
			}
			return "", fmt.Errorf("rmdir: %v", err)
		}
	}
	return "", nil
}

func (e *Evaluator) execRemove(args []string) (string, error) {
	paths, flags := parseFlags(args)
	if len(paths) == 0 {
		return "", fmt.Errorf("rm: missing operand")
	}
	recursive := hasFlag(flags, "r", "R", "recursive")
	force := hasFlag(flags, "f", "force")
	for _, arg := range paths {
		path := e.resolvePath(arg)
		info, err := os.Lstat(path)
		if err != nil {
			if force && os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("rm: %v", err)
		}
		if info.IsDir() && !recursive {
			return "", fmt.Errorf("rm: %s is a directory (use -r/--recursive)", arg)
		}
		if recursive {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil && !(force && os.IsNotExist(err)) {
			return "", fmt.Errorf("rm: %v", err)
		}
	}
	return "", nil
}

func (e *Evaluator) execMakeFile(args []string) (string, error) {
	args = stripFlags(args)
	if len(args) == 0 {
		return "", fmt.Errorf("mkfile: missing operand")
	}

	for _, arg := range args {
		path := e.resolvePath(arg)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("mkfile: %v", err)
		}
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("mkfile: %v", err)
		}
		now := time.Now()
		if err := os.Chtimes(path, now, now); err != nil {
			return "", fmt.Errorf("mkfile: %v", err)
		}
	}
	return "", nil
}

func (e *Evaluator) execWhoami() (string, error) {
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME") // Windows
	}
	fmt.Fprintln(e.stdout, username)
	return username, nil
}

func (e *Evaluator) execHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home: %v", err)
	}
	fmt.Fprintln(e.stdout, home)
	return home, nil
}

func (e *Evaluator) execPrint(args []string) (string, error) {
	// If we have stdin content (from pipe), print that
	if e.stdin != os.Stdin {
		content, err := io.ReadAll(e.stdin)
		if err != nil {
			return "", err
		}
		fmt.Fprint(e.stdout, string(content))
		return string(content), nil
	}

	// Print arguments as text (like echo)
	result := strings.Join(args, " ") + "\n"
	fmt.Fprint(e.stdout, result)
	return result, nil
}

func (e *Evaluator) execOutput(args []string) (string, error) {
	// Same as print for now
	return e.execPrint(args)
}

func (e *Evaluator) execShow(args []string) (string, error) {
	args = stripFlags(args)
	if len(args) == 0 {
		return "", fmt.Errorf("show: missing file argument")
	}

	var output bytes.Buffer
	for _, arg := range args {
		path := e.resolvePath(arg)
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("show: %v", err)
		}
		output.Write(content)
	}

	result := output.String()
	fmt.Fprint(e.stdout, result)
	return result, nil
}

func (e *Evaluator) execClear() (string, error) {
	// ANSI escape codes to clear screen and move cursor to home position
	fmt.Fprint(e.stdout, "\033[2J\033[H")
	return "", nil
}

// execExport sets a shell environment variable: export NAME [value...].
// Remaining arguments are joined with spaces to form the value.
func (e *Evaluator) execExport(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("export: missing variable name")
	}
	name := args[0]
	value := strings.Join(args[1:], " ")
	e.env[name] = value
	return "", nil
}

// execEnv prints the effective environment (process env with shell-local
// overrides applied), sorted by name.
func (e *Evaluator) execEnv() (string, error) {
	merged := make(map[string]string)
	for _, kv := range os.Environ() {
		if before, after, ok := strings.Cut(kv, "="); ok {
			merged[before] = after
		}
	}
	maps.Copy(merged, e.env)

	names := make([]string, 0, len(merged))
	for k := range merged {
		names = append(names, k)
	}
	sort.Strings(names)

	var out bytes.Buffer
	for _, k := range names {
		out.WriteString(k + "=" + merged[k] + "\n")
	}
	result := out.String()
	fmt.Fprint(e.stdout, result)
	return result, nil
}

// execRavenAdd handles configuration commands:
//
//	raven-add path <dir>   register an extra executable search directory
//	raven-add path         list the registered search directories
func (e *Evaluator) execRavenAdd(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("raven-add: usage: raven-add path <dir>")
	}

	switch args[0] {
	case "path":
		if len(args) == 1 {
			// List current extra search paths.
			var out bytes.Buffer
			for _, dir := range e.searchPaths {
				out.WriteString(dir + "\n")
			}
			result := out.String()
			fmt.Fprint(e.stdout, result)
			return result, nil
		}

		dir := e.resolvePath(args[1])
		info, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("raven-add: %v", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("raven-add: %s is not a directory", dir)
		}

		if e.addSearchPath(dir) {
			fmt.Fprintf(e.stdout, "added %s to search paths\n", dir)
		} else {
			fmt.Fprintf(e.stdout, "%s is already a search path\n", dir)
		}
		return "", nil

	default:
		return "", fmt.Errorf("raven-add: unknown target %q (expected 'path')", args[0])
	}
}

// addSearchPath registers dir as an extra executable search directory and
// persists it. It returns false if dir was already registered.
func (e *Evaluator) addSearchPath(dir string) bool {
	if slices.Contains(e.searchPaths, dir) {
		return false
	}
	// New directories take priority over existing ones.
	e.searchPaths = append([]string{dir}, e.searchPaths...)
	e.execCacheValid = false

	if path := configPath(searchPathsFile); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			fmt.Fprintln(f, dir)
			f.Close()
		}
	}
	return true
}

// FunctionNames returns the names of all user-defined functions.
func (e *Evaluator) FunctionNames() []string {
	names := make([]string, 0, len(e.funcs))
	for name := range e.funcs {
		names = append(names, name)
	}
	return names
}

// AvailableCommands returns the names of all invokable commands for tab
// completion: user-defined functions plus executables found in the extra
// search paths and the system PATH.
func (e *Evaluator) AvailableCommands() []string {
	if !e.execCacheValid {
		e.execCache = e.scanExecutables()
		e.execCacheValid = true
	}

	set := make(map[string]bool)
	for _, name := range e.execCache {
		set[name] = true
	}
	for name := range e.funcs {
		set[name] = true
	}
	for name := range e.aliases {
		set[name] = true
	}
	for _, name := range builtinCommandNames() {
		set[name] = true
	}

	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// scanExecutables collects executable file names from the search paths and the
// system PATH.
func (e *Evaluator) scanExecutables() []string {
	set := make(map[string]bool)
	dirs := append([]string{}, e.searchPaths...)
	dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)
	dirs = append(dirs, e.defaultPaths...)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if info, err := entry.Info(); err == nil && info.Mode()&0111 != 0 {
				set[entry.Name()] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	return names
}

// evalCommandSubstitution runs a command with its stdout captured and returns
// the output as a string with the trailing newline trimmed.
func (e *Evaluator) evalCommandSubstitution(node *ast.CommandSubstitution) (Value, error) {
	var buf bytes.Buffer
	oldStdout := e.stdout
	e.stdout = &buf
	_, err := e.evalExpressionValue(node.Command)
	e.stdout = oldStdout
	if err != nil {
		return nil, err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// Helper functions

// expandHome expands a leading '~' or '~/...' to the user's home directory and
// returns any other path unchanged. Unlike resolvePath it does not join with the
// working directory, so relative paths and URLs are left exactly as written. This
// is the only rewriting a shell performs on a path argument before handing it to
// an external command: ~ is the shell's responsibility, but ".", "..", and
// relative paths must reach the child verbatim so it resolves them against its
// own working directory.
func (e *Evaluator) expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolvePath turns a path argument into an absolute, cleaned path rooted at
// the shell's current directory. Builtins use this because the shell tracks its
// own cwd (e.cwd) instead of calling os.Chdir, so a bare "." or "foo" must be
// joined with e.cwd rather than the process working directory.
func (e *Evaluator) resolvePath(path string) string {
	if len(path) == 0 {
		return e.cwd
	}

	// A URL (scheme://...) is not a filesystem path; return it verbatim so
	// path cleaning does not collapse the "//" (e.g. https://github.com would
	// otherwise become https:/github.com).
	if strings.Contains(path, "://") {
		return path
	}

	// Expand ~ to home directory
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return e.cwd
		}
		return home
	}
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Clean(filepath.Join(e.cwd, path))
		}
		return filepath.Clean(filepath.Join(home, path[2:]))
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	// Relative path - join with cwd and clean
	return filepath.Clean(filepath.Join(e.cwd, path))
}

func (e *Evaluator) expandVariable(name string) string {
	// First check local env
	if val, ok := e.env[name]; ok {
		return val
	}
	// Then check OS env
	return os.Getenv(name)
}

// GetCwd returns the current working directory
func (e *Evaluator) GetCwd() string {
	return e.cwd
}

// CallPrompt invokes a user-defined `prompt` function (if any) and renders its
// return value as the REPL prompt. The function may take no parameters, or one
// parameter that receives the exit status of the last command ($?). It returns
// ok=false when no usable prompt function is defined, the call errors, or it
// produces an empty prompt — callers then fall back to the built-in prompt.
func (e *Evaluator) CallPrompt() (string, bool) {
	fn, exists := e.funcs["prompt"]
	if !exists || len(fn.Parameters) > 1 {
		return "", false
	}
	var args []Value
	if len(fn.Parameters) == 1 {
		args = []Value{int64(e.lastStatus)}
	}
	// Commands run inside the prompt function must not clobber the $? seen by
	// the user's next command.
	saved := e.lastStatus
	val, err := e.callFunction(fn, args)
	e.lastStatus = saved
	if err != nil || val == nil {
		return "", false
	}
	s := e.valueToString(val)
	if s == "" {
		return "", false
	}
	return s, true
}

// SetEnv sets an environment variable
func (e *Evaluator) SetEnv(name, value string) {
	e.env[name] = value
}

// evalAssignment handles variable assignment: x = value
func (e *Evaluator) evalAssignment(stmt *ast.AssignmentStatement) error {
	val, err := e.evalExpressionValue(stmt.Value)
	if err != nil {
		return err
	}
	e.setVar(stmt.Name.Value, val)
	return nil
}

// evalForStatement handles for loops: for i in range(n) { ... }
func (e *Evaluator) evalForStatement(stmt *ast.ForStatement) error {
	iterable, err := e.evalExpressionValue(stmt.Iterable)
	if err != nil {
		return err
	}

	// Convert iterable to a slice
	var items []Value
	switch v := iterable.(type) {
	case []Value:
		items = v
	case []int64:
		items = make([]Value, len(v))
		for i, item := range v {
			items[i] = item
		}
	default:
		return fmt.Errorf("cannot iterate over %T", iterable)
	}

	// Iterate
	for _, item := range items {
		if e.checkInterrupt() {
			return ErrInterrupted
		}
		e.setVar(stmt.Variable.Value, item)
		if err := e.evalBlockStatement(stmt.Body); err != nil {
			if sig, ok := asControl(err); ok {
				if sig.kind == ctrlBreak {
					break
				}
				if sig.kind == ctrlContinue {
					continue
				}
			}
			return err
		}
	}

	return nil
}

// evalWhileStatement handles while loops: while cond { ... }
func (e *Evaluator) evalWhileStatement(stmt *ast.WhileStatement) error {
	for {
		if e.checkInterrupt() {
			return ErrInterrupted
		}
		cond, err := e.evalExpressionValue(stmt.Condition)
		if err != nil {
			return err
		}
		if !e.valueToBool(cond) {
			break
		}

		if err := e.evalBlockStatement(stmt.Body); err != nil {
			if sig, ok := asControl(err); ok {
				if sig.kind == ctrlBreak {
					break
				}
				if sig.kind == ctrlContinue {
					continue
				}
			}
			return err
		}
	}

	return nil
}

// evalIfStatement handles conditionals: if cond { ... } else { ... }
func (e *Evaluator) evalIfStatement(stmt *ast.IfStatement) error {
	condition, err := e.evalExpressionValue(stmt.Condition)
	if err != nil {
		return err
	}

	if e.valueToBool(condition) {
		return e.evalBlockStatement(stmt.Consequence)
	} else if stmt.Alternative != nil {
		return e.evalBlockStatement(stmt.Alternative)
	}

	return nil
}

// evalBlockStatement evaluates a block of statements
func (e *Evaluator) evalBlockStatement(block *ast.BlockStatement) error {
	for _, stmt := range block.Statements {
		if err := e.evalStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

// evalInfixExpression handles binary operations: left op right
func (e *Evaluator) evalInfixExpression(node *ast.InfixExpression) (Value, error) {
	left, err := e.evalExpressionValue(node.Left)
	if err != nil {
		return nil, err
	}

	right, err := e.evalExpressionValue(node.Right)
	if err != nil {
		return nil, err
	}

	// String concatenation
	if node.Operator == "+" {
		// Check if either is a string
		_, leftIsString := left.(string)
		_, rightIsString := right.(string)
		if leftIsString || rightIsString {
			return e.valueToString(left) + e.valueToString(right), nil
		}
	}

	// Numeric operations
	leftNum, leftErr := e.valueToInt64(left)
	rightNum, rightErr := e.valueToInt64(right)

	// If both can be converted to numbers, do numeric operation
	if leftErr == nil && rightErr == nil {
		switch node.Operator {
		case "+":
			return leftNum + rightNum, nil
		case "-":
			return leftNum - rightNum, nil
		case "*":
			return leftNum * rightNum, nil
		case "/":
			if rightNum == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return leftNum / rightNum, nil
		case "%":
			if rightNum == 0 {
				return nil, fmt.Errorf("modulo by zero")
			}
			return leftNum % rightNum, nil
		case "==":
			return leftNum == rightNum, nil
		case "!=":
			return leftNum != rightNum, nil
		case "<":
			return leftNum < rightNum, nil
		case ">":
			return leftNum > rightNum, nil
		case "<=":
			return leftNum <= rightNum, nil
		case ">=":
			return leftNum >= rightNum, nil
		}
	}

	// String comparison
	leftStr := e.valueToString(left)
	rightStr := e.valueToString(right)
	switch node.Operator {
	case "==":
		return leftStr == rightStr, nil
	case "!=":
		return leftStr != rightStr, nil
	case "+":
		return leftStr + rightStr, nil
	}

	return nil, fmt.Errorf("unknown operator: %s", node.Operator)
}

// evalCallExpression handles function calls: range(n), append(arr, val)
func (e *Evaluator) evalCallExpression(node *ast.CallExpression) (Value, error) {
	switch node.Function {
	case "range":
		return e.builtinRange(node.Arguments)
	case "append":
		return e.builtinAppend(node.Arguments)
	}

	// Evaluate arguments once for the remaining builtins and user functions.
	args := make([]Value, len(node.Arguments))
	for i, arg := range node.Arguments {
		val, err := e.evalExpressionValue(arg)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}

	switch node.Function {
	case "exit":
		status := int64(0)
		if len(args) > 1 {
			return nil, fmt.Errorf("exit() takes zero or one argument")
		}
		if len(args) == 1 {
			var err error
			status, err = e.valueToInt64(args[0])
			if err != nil || status < 0 || status > 255 {
				return nil, fmt.Errorf("exit() status must be an integer from 0 to 255")
			}
		}
		return nil, &ExitRequest{Status: int(status)}
	case "lastStatus":
		if len(args) != 0 {
			return nil, fmt.Errorf("lastStatus() takes no arguments")
		}
		return int64(e.lastStatus), nil
	case "len":
		return e.builtinLen(args)
	case "split":
		return e.builtinSplit(args)
	case "join":
		return e.builtinJoin(args)
	case "contains":
		return e.builtinContains(args)
	case "upper":
		return e.builtinStr1(args, "upper", strings.ToUpper)
	case "lower":
		return e.builtinStr1(args, "lower", strings.ToLower)
	case "trim":
		return e.builtinStr1(args, "trim", strings.TrimSpace)
	case "replace":
		return e.builtinReplace(args)
	case "glob":
		return e.builtinGlob(args)
	}

	// User-defined function.
	if fn, ok := e.funcs[node.Function]; ok {
		return e.callFunction(fn, args)
	}

	return nil, fmt.Errorf("unknown function: %s", node.Function)
}

// builtinLen returns the length of a string (in runes) or array.
func (e *Evaluator) builtinLen(args []Value) (Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("len() takes exactly 1 argument")
	}
	switch v := args[0].(type) {
	case string:
		return int64(utf8.RuneCountInString(v)), nil
	case []Value:
		return int64(len(v)), nil
	default:
		return nil, fmt.Errorf("len() not supported on %T", args[0])
	}
}

// builtinSplit splits a string by a separator into an array of strings.
func (e *Evaluator) builtinSplit(args []Value) (Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("split() takes exactly 2 arguments (string, sep)")
	}
	s := e.valueToString(args[0])
	sep := e.valueToString(args[1])
	parts := strings.Split(s, sep)
	result := make([]Value, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result, nil
}

// builtinJoin joins an array's elements into a string with a separator.
func (e *Evaluator) builtinJoin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("join() takes exactly 2 arguments (array, sep)")
	}
	arr, ok := args[0].([]Value)
	if !ok {
		return nil, fmt.Errorf("join() first argument must be an array")
	}
	sep := e.valueToString(args[1])
	parts := make([]string, len(arr))
	for i, el := range arr {
		parts[i] = e.valueToString(el)
	}
	return strings.Join(parts, sep), nil
}

// builtinContains reports substring membership for strings, or element
// membership for arrays.
func (e *Evaluator) builtinContains(args []Value) (Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("contains() takes exactly 2 arguments")
	}
	switch container := args[0].(type) {
	case string:
		return strings.Contains(container, e.valueToString(args[1])), nil
	case []Value:
		target := e.valueToString(args[1])
		for _, el := range container {
			if e.valueToString(el) == target {
				return true, nil
			}
		}
		return false, nil
	default:
		return nil, fmt.Errorf("contains() not supported on %T", args[0])
	}
}

// builtinStr1 applies a single-argument string transform (upper/lower/trim).
func (e *Evaluator) builtinStr1(args []Value, name string, fn func(string) string) (Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s() takes exactly 1 argument", name)
	}
	return fn(e.valueToString(args[0])), nil
}

// builtinReplace replaces all occurrences of old with new in a string.
func (e *Evaluator) builtinReplace(args []Value) (Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("replace() takes exactly 3 arguments (string, old, new)")
	}
	s := e.valueToString(args[0])
	old := e.valueToString(args[1])
	newStr := e.valueToString(args[2])
	return strings.ReplaceAll(s, old, newStr), nil
}

// braceExpand performs shell brace expansion on one word: {a,b} comma lists,
// {1..5}/{a..e} sequences (with an optional {1..9..2} step and zero-padding),
// nesting ({a,{b,c}}), and cross products ({a,b}{c,d}). A word with no valid
// brace group expands to just itself.
func braceExpand(s string) []string {
	pre, body, post, ok := splitFirstBraceGroup(s)
	if !ok {
		return []string{s}
	}

	var alts []string
	if seq := expandBraceSequence(body); seq != nil {
		alts = seq
	} else if parts := splitTopLevelCommas(body); len(parts) > 1 {
		alts = parts
	} else {
		// A single element with no ',' or valid '..' is not an expansion: keep
		// the braces literal and expand only what follows them.
		out := []string{}
		for _, tail := range braceExpand(post) {
			out = append(out, pre+"{"+body+"}"+tail)
		}
		return out
	}

	// Cross the alternatives (each possibly nested) with the expansions of the
	// text that follows the group.
	tails := braceExpand(post)
	out := []string{}
	for _, alt := range alts {
		for _, sub := range braceExpand(alt) {
			for _, tail := range tails {
				out = append(out, pre+sub+tail)
			}
		}
	}
	return out
}

// splitFirstBraceGroup splits s around the first balanced { ... } group into the
// text before it, the content between the braces, and the text after the close.
// ok is false when there is no '{' or it never balances.
func splitFirstBraceGroup(s string) (pre, body, post string, ok bool) {
	open := strings.IndexByte(s, '{')
	if open < 0 {
		return "", "", "", false
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:open], s[open+1 : i], s[i+1:], true
			}
		}
	}
	return "", "", "", false
}

// splitTopLevelCommas splits s on commas that are not nested inside inner braces.
func splitTopLevelCommas(s string) []string {
	parts := []string{}
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// expandBraceSequence expands a numeric or single-character sequence body
// ("1..5", "a..e", "1..9..2") into its elements, or returns nil when body is not
// a valid sequence.
func expandBraceSequence(body string) []string {
	f := strings.Split(body, "..")
	if len(f) != 2 && len(f) != 3 {
		return nil
	}
	step := 1
	if len(f) == 3 {
		n, err := strconv.Atoi(f[2])
		if err != nil || n == 0 {
			return nil
		}
		step = n
		if step < 0 {
			step = -step
		}
	}

	if lo, loErr := strconv.Atoi(f[0]); loErr == nil {
		if hi, hiErr := strconv.Atoi(f[1]); hiErr == nil {
			width := 0
			if isZeroPadded(f[0]) || isZeroPadded(f[1]) {
				width = max(len(f[0]), len(f[1]))
			}
			return intSequence(lo, hi, step, width)
		}
	}
	if len(f[0]) == 1 && len(f[1]) == 1 && isAlpha(f[0][0]) && isAlpha(f[1][0]) {
		return charSequence(int(f[0][0]), int(f[1][0]), step)
	}
	return nil
}

func intSequence(lo, hi, step, width int) []string {
	out := []string{}
	if lo <= hi {
		for v := lo; v <= hi; v += step {
			out = append(out, padInt(v, width))
		}
	} else {
		for v := lo; v >= hi; v -= step {
			out = append(out, padInt(v, width))
		}
	}
	return out
}

func charSequence(lo, hi, step int) []string {
	out := []string{}
	if lo <= hi {
		for c := lo; c <= hi; c += step {
			out = append(out, string(rune(c)))
		}
	} else {
		for c := lo; c >= hi; c -= step {
			out = append(out, string(rune(c)))
		}
	}
	return out
}

// padInt formats v left-padded with zeros to width (keeping any leading '-').
func padInt(v, width int) string {
	s := strconv.Itoa(v)
	if width == 0 {
		return s
	}
	sign := ""
	if v < 0 {
		sign, s = "-", s[1:]
	}
	for len(sign)+len(s) < width {
		s = "0" + s
	}
	return sign + s
}

func isZeroPadded(s string) bool {
	s = strings.TrimPrefix(s, "-")
	return len(s) > 1 && s[0] == '0'
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// builtinGlob expands a shell glob pattern against the current directory and
// returns a sorted array of matching paths (relative when the pattern is
// relative). Returns an empty array when nothing matches.
func (e *Evaluator) builtinGlob(args []Value) (Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("glob() takes exactly 1 argument")
	}
	pattern := e.valueToString(args[0])
	relative := !filepath.IsAbs(pattern) && !strings.HasPrefix(pattern, "~")

	matches, err := filepath.Glob(e.resolvePath(pattern))
	if err != nil {
		return nil, fmt.Errorf("glob: %v", err)
	}

	result := make([]Value, 0, len(matches))
	for _, m := range matches {
		if relative {
			if rel, err := filepath.Rel(e.cwd, m); err == nil {
				m = rel
			}
		}
		result = append(result, m)
	}
	return result, nil
}

// interpolate expands $VAR, ${VAR}, and $? references inside a double-quoted
// string. A literal dollar sign is written as $$.
func (e *Evaluator) interpolate(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i == len(s)-1 {
			out.WriteByte(s[i])
			continue
		}
		next := s[i+1]
		switch {
		case next == '$':
			out.WriteByte('$')
			i++
		case next == '?':
			out.WriteString(strconv.Itoa(e.lastStatus))
			i++
		case next == '{':
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				out.WriteByte('$') // unterminated - leave literal
				continue
			}
			name := s[i+2 : i+2+end]
			out.WriteString(e.lookupName(name))
			i += 2 + end
		case isNameStart(next):
			j := i + 1
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			out.WriteString(e.lookupName(s[i+1 : j]))
			i = j - 1
		default:
			out.WriteByte('$')
		}
	}
	return out.String()
}

// lookupName resolves a name to its shell variable value, falling back to the
// environment.
func (e *Evaluator) lookupName(name string) string {
	if val, ok := e.getVar(name); ok {
		return e.valueToString(val)
	}
	return e.expandVariable(name)
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

// evalLogical evaluates && and || with short-circuiting based on exit status.
func (e *Evaluator) evalLogical(node *ast.LogicalExpression) (Value, error) {
	_, err := e.evalExpressionValue(node.Left)
	if err != nil {
		// Treat an evaluation error on the left as failure for chaining.
		e.lastStatus = 1
	}
	leftOK := err == nil && e.lastStatus == 0

	switch node.Operator {
	case "&&":
		if leftOK {
			return e.evalExpressionValue(node.Right)
		}
	case "||":
		if !leftOK {
			return e.evalExpressionValue(node.Right)
		}
	}
	return nil, nil
}

// maxRangeLen bounds range() so a typo can't OOM the shell.
const maxRangeLen = 10_000_000

// builtinRange implements range(stop) and range(start, stop) - returns
// [start, start+1, ..., stop-1]; an empty range when stop <= start.
func (e *Evaluator) builtinRange(args []ast.Expression) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nil, fmt.Errorf("range() takes 1 or 2 arguments")
	}

	nums := make([]int64, len(args))
	for i, arg := range args {
		val, err := e.evalExpressionValue(arg)
		if err != nil {
			return nil, err
		}
		if nums[i], err = e.valueToInt64(val); err != nil {
			return nil, fmt.Errorf("range() arguments must be integers")
		}
	}

	start, stop := int64(0), nums[0]
	if len(nums) == 2 {
		start, stop = nums[0], nums[1]
	}
	if stop <= start {
		return []Value{}, nil
	}
	// ponytail: fixed cap, make it configurable if anyone ever needs a bigger literal array
	if n := stop - start; n > maxRangeLen || n < 0 { // n < 0 means the subtraction overflowed
		return nil, fmt.Errorf("range() is too large (max %d elements)", maxRangeLen)
	}

	result := make([]Value, 0, stop-start)
	for i := start; i < stop; i++ {
		result = append(result, i)
	}
	return result, nil
}

// builtinAppend implements append(arr, val) - returns new array with val appended
func (e *Evaluator) builtinAppend(args []ast.Expression) (Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("append() takes exactly 2 arguments")
	}

	arrVal, err := e.evalExpressionValue(args[0])
	if err != nil {
		return nil, err
	}

	arr, ok := arrVal.([]Value)
	if !ok {
		return nil, fmt.Errorf("append() first argument must be an array")
	}

	val, err := e.evalExpressionValue(args[1])
	if err != nil {
		return nil, err
	}

	// Create a new array with the value appended
	result := make([]Value, len(arr)+1)
	copy(result, arr)
	result[len(arr)] = val
	return result, nil
}

// evalArrayLiteral handles array literals: [1, 2, 3] or []string
func (e *Evaluator) evalArrayLiteral(node *ast.ArrayLiteral) (Value, error) {
	// Empty array with type hint
	if node.TypeHint != "" {
		return []Value{}, nil
	}

	elements := make([]Value, len(node.Elements))
	for i, elem := range node.Elements {
		val, err := e.evalExpressionValue(elem)
		if err != nil {
			return nil, err
		}
		elements[i] = val
	}
	return elements, nil
}

// evalIndexExpression handles array indexing: arr[0]
func (e *Evaluator) evalIndexExpression(node *ast.IndexExpression) (Value, error) {
	left, err := e.evalExpressionValue(node.Left)
	if err != nil {
		return nil, err
	}

	index, err := e.evalExpressionValue(node.Index)
	if err != nil {
		return nil, err
	}

	arr, ok := left.([]Value)
	if !ok {
		return nil, fmt.Errorf("index operator not supported on %T", left)
	}

	idx, err := e.valueToInt64(index)
	if err != nil {
		return nil, fmt.Errorf("array index must be an integer")
	}

	if idx < 0 || idx >= int64(len(arr)) {
		return nil, fmt.Errorf("array index out of bounds: %d", idx)
	}

	return arr[idx], nil
}
