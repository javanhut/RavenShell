package evaluator

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"ravenshell/ansi"
	"ravenshell/ast"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/term"
)

// Value represents any value in the shell
type Value interface{}

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
	cwd         string                            // Current working directory
	env         map[string]string                 // Environment variables (for $VAR)
	vars        map[string]Value                  // Global script variables (== scopes[0])
	scopes      []map[string]Value                // Variable scope chain; innermost is last
	funcs       map[string]*ast.FunctionStatement // User-defined functions
	searchPaths []string                          // Extra executable search dirs (raven-add path)
	stdout      io.Writer                         // Standard output (for redirections)
	stdin       io.Reader                         // Standard input (for redirections)

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
		stdout:    os.Stdout,
		stdin:     os.Stdin,
		nextJobID: 1,
	}
	e.loadSearchPaths()
	return e
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
	for _, line := range strings.Split(string(content), "\n") {
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
		return node.Value, nil
	case *ast.PathExpression:
		return e.resolvePath(node.Value), nil
	case *ast.StringLiteral:
		if node.Interpolate {
			return e.interpolate(node.Value), nil
		}
		return node.Value, nil
	case *ast.IntegerLiteral:
		return node.Value, nil
	case *ast.VariableReference:
		return e.expandVariable(node.Name.Value), nil
	case *ast.LastStatus:
		return int64(e.lastStatus), nil
	case *ast.LogicalExpression:
		return e.evalLogical(node)
	case *ast.BackgroundExpression:
		return e.evalBackground(node)
	case *ast.CommandSubstitution:
		return e.evalCommandSubstitution(node)
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
	args, err := e.evalArgs(cmd.Arguments)
	if err != nil {
		return "", err
	}

	result, err := e.dispatchCommand(cmd, args)

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
func (e *Evaluator) evalArgs(arguments []ast.Expression) ([]string, error) {
	var args []string
	for _, arg := range arguments {
		val, err := e.evalExpressionValue(arg)
		if err != nil {
			return nil, err
		}
		if arr, ok := val.([]Value); ok {
			for _, el := range arr {
				args = append(args, e.valueToString(el))
			}
		} else {
			args = append(args, e.valueToString(val))
		}
	}
	return args, nil
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
		return e.execMakeDir(args)
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
		// Like a real shell, a missing command sets a non-zero status and
		// reports to stderr, but does not abort the program.
		fmt.Fprintf(os.Stderr, "%s: command not found\n", name)
		e.lastStatus = 127
		return "", nil
	}

	c := exec.Command(path, args...)
	c.Dir = e.cwd
	c.Stdin = e.stdin
	c.Stdout = e.stdout
	c.Stderr = os.Stderr
	c.Env = e.buildEnv()

	err = c.Run()
	e.lastStatus = exitStatus(err)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			// A real run error (not just a non-zero exit) - report it.
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		}
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
		return "", exec.ErrNotFound
	}

	for _, dir := range e.searchPaths {
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return exec.LookPath(name)
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
// extra search paths (prepended to PATH) so external commands see both
// inherited and user-set ($VAR) values plus any raven-add path directories.
func (e *Evaluator) buildEnv() []string {
	merged := make(map[string]string)
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			merged[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range e.env {
		merged[k] = v
	}
	if len(e.searchPaths) > 0 {
		extra := strings.Join(e.searchPaths, string(os.PathListSeparator))
		if existing := merged["PATH"]; existing != "" {
			merged["PATH"] = extra + string(os.PathListSeparator) + existing
		} else {
			merged["PATH"] = extra
		}
	}

	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, k+"="+v)
	}
	return env
}

func (e *Evaluator) evalPipe(pipe *ast.PipeExpression) (string, error) {
	// Capture output from left command
	var leftOutput bytes.Buffer
	oldStdout := e.stdout
	e.stdout = &leftOutput

	_, err := e.evalExpression(pipe.Left)
	e.stdout = oldStdout
	if err != nil {
		return "", err
	}

	// Use left output as input for right command
	oldStdin := e.stdin
	e.stdin = &leftOutput

	result, err := e.evalExpression(pipe.Right)
	e.stdin = oldStdin

	return result, err
}

func (e *Evaluator) evalRedirection(redir *ast.RedirectionExpression) (string, error) {
	// Get target filename
	target, err := e.evalExpression(redir.Target)
	if err != nil {
		return "", err
	}

	// Resolve path
	targetPath := e.resolvePath(target)

	switch redir.Type {
	case ast.REDIR_OUTPUT:
		// Overwrite file
		file, err := os.Create(targetPath)
		if err != nil {
			return "", fmt.Errorf("cannot create file %s: %v", target, err)
		}
		defer file.Close()

		oldStdout := e.stdout
		e.stdout = file
		result, err := e.evalExpression(redir.Command)
		e.stdout = oldStdout
		return result, err

	case ast.REDIR_APPEND:
		// Append to file
		file, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("cannot open file %s: %v", target, err)
		}
		defer file.Close()

		oldStdout := e.stdout
		e.stdout = file
		result, err := e.evalExpression(redir.Command)
		e.stdout = oldStdout
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

func (e *Evaluator) execList(args []string) (string, error) {
	args = stripFlags(args)
	dir := e.cwd
	if len(args) > 0 {
		dir = e.resolvePath(args[0])
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("ls: %v", err)
	}

	color := e.colorOutput()

	// Plain (uncolored) listing is also returned so it works in pipes/captures.
	var plain bytes.Buffer
	var display bytes.Buffer
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		plain.WriteString(name + "\n")
		display.WriteString(e.colorizeEntry(entry, name, color) + "\n")
	}

	fmt.Fprint(e.stdout, display.String())
	return plain.String(), nil
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
		e.cwd = home
		return "", nil
	}

	target := e.resolvePath(args[0])
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("cd: %v", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cd: %s: not a directory", args[0])
	}

	e.cwd = target
	return "", nil
}

func (e *Evaluator) execCurrentDir() (string, error) {
	fmt.Fprintln(e.stdout, e.cwd)
	return e.cwd, nil
}

func (e *Evaluator) execMakeDir(args []string) (string, error) {
	args = stripFlags(args)
	if len(args) == 0 {
		return "", fmt.Errorf("mkdir: missing operand")
	}

	for _, arg := range args {
		path := e.resolvePath(arg)
		if err := os.MkdirAll(path, 0755); err != nil {
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
	args = stripFlags(args)
	if len(args) == 0 {
		return "", fmt.Errorf("rm: missing operand")
	}

	for _, arg := range args {
		path := e.resolvePath(arg)
		if err := os.RemoveAll(path); err != nil {
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
		file, err := os.Create(path)
		if err != nil {
			return "", fmt.Errorf("mkfile: %v", err)
		}
		file.Close()
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
		if i := strings.IndexByte(kv, '='); i >= 0 {
			merged[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range e.env {
		merged[k] = v
	}

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
	for _, existing := range e.searchPaths {
		if existing == dir {
			return false
		}
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

func (e *Evaluator) resolvePath(path string) string {
	if len(path) == 0 {
		return e.cwd
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

	// Absolute path
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

// builtinRange implements range(n) - returns [0, 1, 2, ..., n-1]
func (e *Evaluator) builtinRange(args []ast.Expression) (Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("range() takes exactly 1 argument")
	}

	val, err := e.evalExpressionValue(args[0])
	if err != nil {
		return nil, err
	}

	n, err := e.valueToInt64(val)
	if err != nil {
		return nil, fmt.Errorf("range() argument must be an integer")
	}

	result := make([]Value, n)
	for i := int64(0); i < n; i++ {
		result[i] = i
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
