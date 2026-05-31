# RavenShell

A command-line interpreter and scripting language written in Go. RavenShell combines traditional shell commands with a Go-like scripting syntax, providing both an interactive REPL and script execution capabilities.

## Features

- **Interactive REPL** - Full-featured command line with line editing and a colored prompt
- **Smart Tab Completion** - Completes built-ins, user-defined functions, `$PATH` executables, and file paths
- **Custom Search Paths** - `raven-add path <dir>` registers extra executable directories (saved to `~/.raven_paths`)
- **Fish-style Autosuggestions** - As you type, the most recent matching command appears dimmed inline; accept it with → / Ctrl-E / End
- **Persistent History** - Command history is saved to `~/.raven_history` and restored across sessions
- **ANSI Colors** - Colorized prompt, `ls` output (directories/executables/symlinks), and error messages (only when attached to a terminal)
- **Script Execution** - Run `.rsh` script files for automation
- **Go-like Syntax** - Variables, arrays, loops, and conditionals with familiar syntax
- **Functions** - Define reusable functions with parameters, return values, and recursion (`fn name(args) { ... }`)
- **Rich Control Flow** - `for`, `while`, `if`/`else if`/`else`, plus `break` and `continue`
- **Built-in Helpers** - String/collection functions: `len`, `split`, `join`, `contains`, `upper`, `lower`, `trim`, `replace`
- **Environment & Substitution** - `export`/`env` for environment variables and `$(command)` substitution
- **Built-in Commands** - File system operations (ls, cd, mkdir, rm, etc.)
- **External Commands** - Run any program on your `PATH` (git, python, cat, ...) with flag support (`-l`, `--all`)
- **Pipes & Redirection** - Chain commands with `|` and redirect with `>`, `>>`, `<`
- **Configuration** - Customize startup behavior with `.ravenrc`

## Quick Start

### Installation

```bash
git clone https://github.com/yourusername/ravenshell.git
cd ravenshell
go build -o ravenshell
```

### Usage

**Interactive mode:**
```bash
./ravenshell
```

**Run a script:**
```bash
./ravenshell script.rsh
```

### Basic Examples

```rsh
# File operations
ls
cd ~/Documents
mkdir new_folder

# Variables and printing
name = "RavenShell"
print name

# Arrays and loops
numbers = [1, 2, 3, 4, 5]
for n in numbers {
    print n
}

# Conditionals (with else-if chains)
count = 10
if count > 50 {
    print "big"
} else if count > 5 {
    print "count is greater than 5"
} else {
    print "count is 5 or less"
}

# While loops with break / continue
i = 0
while i < 10 {
    i = i + 1
    if i % 2 == 0 {
        continue
    }
    print i
}

# Functions (parameters, return values, recursion)
fn factorial(n) {
    if n <= 1 {
        return 1
    }
    return n * factorial(n - 1)
}
print factorial(5)

# String / collection built-ins
parts = split("a,b,c", ",")
print join(parts, "-")
print upper("ravenshell")
print contains(parts, "b")

# Environment variables and command substitution
export PROJECT RavenShell
print $PROJECT
here = $(cwd)
print here

# Pipes and redirection
ls | print
ls > files.txt

# External programs on your PATH (anything that isn't a built-in)
git status
python --version
print "hello" | wc -w

# Register an extra directory to search for executables (persisted)
raven-add path ~/scripts
raven-add path            # list registered search paths
```

> Note: language-level text output uses the built-in `print`. Bare words that
> aren't built-ins (e.g. `git`, `python`) are executed as external programs.

## Documentation

| Document | Description |
|----------|-------------|
| [User Guide](docs/user-guide.md) | Getting started, shell features, keyboard shortcuts |
| [Language Reference](docs/language-reference.md) | Complete scripting syntax reference |
| [Commands Reference](docs/commands.md) | Built-in commands documentation |
| [Examples](docs/examples.md) | Practical examples and tutorials |
| [Architecture](docs/architecture.md) | Technical guide for developers |
| [Contributing](docs/contributing.md) | How to contribute to RavenShell |

## Requirements

- Go 1.21 or later
- `golang.org/x/term` (installed automatically via go modules)

## License

MIT License - see LICENSE file for details.
