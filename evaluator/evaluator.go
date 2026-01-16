package evaluator

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"ravenshell/ast"
	"strconv"
	"strings"
)

// Value represents any value in the shell
type Value interface{}

// Evaluator executes AST nodes
type Evaluator struct {
	cwd    string            // Current working directory
	env    map[string]string // Environment variables (for $VAR)
	vars   map[string]Value  // Script variables
	stdout io.Writer         // Standard output (for redirections)
	stdin  io.Reader         // Standard input (for redirections)
}

// New creates a new Evaluator
func New() *Evaluator {
	cwd, _ := os.Getwd()
	return &Evaluator{
		cwd:    cwd,
		env:    make(map[string]string),
		vars:   make(map[string]Value),
		stdout: os.Stdout,
		stdin:  os.Stdin,
	}
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
	case *ast.IfStatement:
		return e.evalIfStatement(s)
	}
	return nil
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
		if val, ok := e.vars[node.Value]; ok {
			return val, nil
		}
		return node.Value, nil
	case *ast.PathExpression:
		return e.resolvePath(node.Value), nil
	case *ast.StringLiteral:
		return node.Value, nil
	case *ast.IntegerLiteral:
		return node.Value, nil
	case *ast.VariableReference:
		return e.expandVariable(node.Name.Value), nil
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
	// Evaluate arguments
	args := make([]string, len(cmd.Arguments))
	for i, arg := range cmd.Arguments {
		val, err := e.evalExpression(arg)
		if err != nil {
			return "", err
		}
		args[i] = val
	}

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
	case ast.CMD_TILDE:
		return e.execHome()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd.Name)
	}
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

func (e *Evaluator) execList(args []string) (string, error) {
	dir := e.cwd
	if len(args) > 0 {
		dir = e.resolvePath(args[0])
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("ls: %v", err)
	}

	var output bytes.Buffer
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		output.WriteString(name + "\n")
	}

	result := output.String()
	fmt.Fprint(e.stdout, result)
	return result, nil
}

func (e *Evaluator) execChangeDir(args []string) (string, error) {
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
	if len(args) == 0 {
		return "", fmt.Errorf("rmdir: missing operand")
	}

	for _, arg := range args {
		path := e.resolvePath(arg)
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("rmdir: %v", err)
		}
	}
	return "", nil
}

func (e *Evaluator) execRemove(args []string) (string, error) {
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
	e.vars[stmt.Name.Value] = val
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
		e.vars[stmt.Variable.Value] = item
		if err := e.evalBlockStatement(stmt.Body); err != nil {
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
	default:
		return nil, fmt.Errorf("unknown function: %s", node.Function)
	}
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
