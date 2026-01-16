# RavenShell

A command-line interpreter and scripting language written in Go. RavenShell combines traditional shell commands with a Go-like scripting syntax, providing both an interactive REPL and script execution capabilities.

## Features

- **Interactive REPL** - Full-featured command line with tab completion, history, and line editing
- **Script Execution** - Run `.rsh` script files for automation
- **Go-like Syntax** - Variables, arrays, loops, and conditionals with familiar syntax
- **Built-in Commands** - File system operations (ls, cd, mkdir, rm, etc.)
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

# Conditionals
count = 10
if count > 5 {
    print "count is greater than 5"
} else {
    print "count is 5 or less"
}

# Pipes and redirection
ls | print
ls > files.txt
```

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
