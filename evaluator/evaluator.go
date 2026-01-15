package evaluator

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"ravenshell/ast"
	"strconv"
)

// Evaluator executes AST nodes
type Evaluator struct {
	cwd    string            // Current working directory
	env    map[string]string // Environment variables
	stdout io.Writer         // Standard output (for redirections)
	stdin  io.Reader         // Standard input (for redirections)
}

// New creates a new Evaluator
func New() *Evaluator {
	cwd, _ := os.Getwd()
	return &Evaluator{
		cwd:    cwd,
		env:    make(map[string]string),
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
		_, err := e.evalExpression(s.Expression)
		return err
	}
	return nil
}

func (e *Evaluator) evalExpression(expr ast.Expression) (string, error) {
	switch node := expr.(type) {
	case *ast.Command:
		return e.evalCommand(node)
	case *ast.PipeExpression:
		return e.evalPipe(node)
	case *ast.RedirectionExpression:
		return e.evalRedirection(node)
	case *ast.Identifier:
		return node.Value, nil
	case *ast.PathExpression:
		return node.Value, nil
	case *ast.StringLiteral:
		return node.Value, nil
	case *ast.IntegerLiteral:
		return strconv.FormatInt(node.Value, 10), nil
	case *ast.VariableReference:
		return e.expandVariable(node.Name.Value), nil
	}
	return "", fmt.Errorf("unknown expression type: %T", expr)
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

	// Otherwise print arguments
	output := ""
	for i, arg := range args {
		if i > 0 {
			output += " "
		}
		output += arg
	}
	fmt.Fprintln(e.stdout, output)
	return output, nil
}

func (e *Evaluator) execOutput(args []string) (string, error) {
	// Same as print for now
	return e.execPrint(args)
}

// Helper functions

func (e *Evaluator) resolvePath(path string) string {
	if len(path) == 0 {
		return e.cwd
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
