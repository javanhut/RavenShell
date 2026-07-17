package evaluator

import (
	"fmt"
	"os"
	"path/filepath"
	"ravenshell/ast"
	"ravenshell/lexer"
	"ravenshell/parser"
	"sort"
	"strings"
)

// execRavenAlias defines or lists interactive aliases. Definitions are plain
// RavenScript and can therefore live in .ravenrc:
//
//	raven-alias ll ls -la
//
// Arguments after the target command are retained as individual arguments,
// including quoted strings.
func (e *Evaluator) execRavenAlias(args []string) (string, error) {
	if len(args) == 0 {
		names := make([]string, 0, len(e.aliases))
		for name := range e.aliases {
			names = append(names, name)
		}
		sort.Strings(names)
		var out strings.Builder
		for _, name := range names {
			fmt.Fprintf(&out, "%s = %s\n", name, formatAlias(e.aliases[name]))
		}
		result := out.String()
		fmt.Fprint(e.stdout, result)
		return result, nil
	}
	if len(args) < 2 {
		return "", fmt.Errorf("raven-alias: usage: raven-alias <name> <command> [arguments...]")
	}
	name := args[0]
	if strings.ContainsAny(name, " \t\r\n") {
		return "", fmt.Errorf("raven-alias: invalid alias name %q", name)
	}
	expansion := append([]string(nil), args[1:]...)
	e.aliases[name] = expansion
	e.execCacheValid = false
	return "", nil
}

func formatAlias(parts []string) string {
	quoted := make([]string, len(parts))
	for i, part := range parts {
		if strings.ContainsAny(part, " \t\"'") {
			quoted[i] = fmt.Sprintf("%q", part)
		} else {
			quoted[i] = part
		}
	}
	return strings.Join(quoted, " ")
}

func (e *Evaluator) execRavenUnalias(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("raven-unalias: usage: raven-unalias <name>...")
	}
	for _, name := range args {
		if _, ok := e.aliases[name]; !ok {
			return "", fmt.Errorf("raven-unalias: alias %q is not defined", name)
		}
		delete(e.aliases, name)
	}
	e.execCacheValid = false
	return "", nil
}

// execAlias invokes an alias without re-parsing generated source. This keeps
// argument boundaries intact and avoids command-injection surprises.
func (e *Evaluator) execAlias(alias string, expansion, callArgs []string) (string, error) {
	if e.aliasDepth >= 32 {
		return "", fmt.Errorf("raven-alias: recursive alias involving %q", alias)
	}
	if len(expansion) == 0 {
		return "", fmt.Errorf("raven-alias: alias %q has no command", alias)
	}
	e.aliasDepth++
	defer func() { e.aliasDepth-- }()

	name := expansion[0]
	args := append(append([]string(nil), expansion[1:]...), callArgs...)
	if next, ok := e.aliases[name]; ok {
		return e.execAlias(name, next, args)
	}
	cmdType := commandTypeForName(name)
	result, err := e.dispatchCommand(&ast.Command{Name: name, Type: cmdType}, args)
	if cmdType != ast.CMD_EXTERNAL {
		if err != nil {
			e.lastStatus = 1
		} else {
			e.lastStatus = 0
		}
	}
	return result, err
}

func commandTypeForName(name string) ast.CommandType {
	switch name {
	case "ls":
		return ast.CMD_LIST
	case "rm", "remove", "delete":
		return ast.CMD_REMOVE
	case "cd":
		return ast.CMD_CHANGEDIR
	case "rmdir":
		return ast.CMD_REMOVEDIR
	case "mkdir", "makedir":
		return ast.CMD_MAKEDIR
	case "whoami":
		return ast.CMD_WHOAMI
	case "cwd", "whereami", "wai":
		return ast.CMD_CURRENTDIR
	case "mkfile", "makefile", "newfile", "touch":
		return ast.CMD_MAKEFILE
	case "output":
		return ast.CMD_OUTPUT
	case "print":
		return ast.CMD_PRINT
	case "show", "read", "view":
		return ast.CMD_SHOW
	case "clear":
		return ast.CMD_CLEAR
	case "export":
		return ast.CMD_EXPORT
	case "env":
		return ast.CMD_ENV
	case "raven-add":
		return ast.CMD_RAVENADD
	case "raven-help", "help":
		return ast.CMD_RAVENHELP
	case "raven-update":
		return ast.CMD_RAVENUPDATE
	case "raven-completions":
		return ast.CMD_RAVENCOMPLETIONS
	case "raven-alias":
		return ast.CMD_RAVENALIAS
	case "raven-unalias":
		return ast.CMD_RAVENUNALIAS
	case "raven-source":
		return ast.CMD_RAVENSOURCE
	case "raven-unset":
		return ast.CMD_RAVENUNSET
	case "raven-type":
		return ast.CMD_RAVENTYPE
	case "ps":
		return ast.CMD_PS
	case "kill":
		return ast.CMD_KILL
	case "killall":
		return ast.CMD_KILLALL
	case "jobs":
		return ast.CMD_JOBS
	default:
		return ast.CMD_EXTERNAL
	}
}

// raven-source evaluates another RavenScript file in the current evaluator, so
// functions, aliases, variables, and environment changes remain available.
func (e *Evaluator) execRavenSource(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("raven-source: usage: raven-source <file>")
	}
	if e.sourceDepth >= 32 {
		return "", fmt.Errorf("raven-source: maximum include depth exceeded")
	}
	path := e.resolvePath(args[0])
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("raven-source: %v", err)
	}
	l := lexer.NewLexer(string(content))
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return "", fmt.Errorf("raven-source: %s: %s", filepath.Base(path), strings.Join(errs, "; "))
	}
	e.sourceDepth++
	defer func() { e.sourceDepth-- }()
	if err := e.Eval(program); err != nil {
		return "", fmt.Errorf("raven-source: %s: %w", filepath.Base(path), err)
	}
	return "", nil
}

func (e *Evaluator) execRavenUnset(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("raven-unset: usage: raven-unset <name>...")
	}
	for _, name := range args {
		for i := len(e.scopes) - 1; i >= 0; i-- {
			if _, ok := e.scopes[i][name]; ok {
				delete(e.scopes[i], name)
				break
			}
		}
		delete(e.env, name)
	}
	return "", nil
}

func (e *Evaluator) execRavenType(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("raven-type: usage: raven-type <name>...")
	}
	var out strings.Builder
	for _, name := range args {
		switch {
		case e.aliases[name] != nil:
			fmt.Fprintf(&out, "%s is an alias for %s\n", name, formatAlias(e.aliases[name]))
		case e.funcs[name] != nil:
			fmt.Fprintf(&out, "%s is a RavenScript function\n", name)
		case commandTypeForName(name) != ast.CMD_EXTERNAL:
			fmt.Fprintf(&out, "%s is a RavenShell built-in\n", name)
		default:
			path, err := e.lookPath(name)
			if err != nil {
				return "", fmt.Errorf("raven-type: %s not found", name)
			}
			fmt.Fprintf(&out, "%s is %s\n", name, path)
		}
	}
	result := out.String()
	fmt.Fprint(e.stdout, result)
	return result, nil
}
