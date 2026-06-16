# RavenShell

A command-line interpreter and scripting language written in Go. RavenShell combines traditional shell commands with a Go-like scripting syntax, providing both an interactive REPL and script execution capabilities.

## Features

- **Interactive REPL** - Full-featured command line with line editing and a colored prompt
- **Smart Tab Completion** - Fish-style completion of built-ins, user functions, `$PATH` executables, and file paths, plus subcommands and flags (with descriptions) for tools like `git`, `go`, `npm`, `docker`, and `make` — including dynamic candidates such as git branches and Makefile targets, user-defined completion specs, and file candidates color-coded by type
- **Custom Search Paths** - `raven-add path <dir>` registers extra executable directories (saved to `~/.raven_paths`)
- **Fish-style Autosuggestions** - As you type, the most recent matching command appears dimmed inline; accept it with → / Ctrl-E / End
- **Persistent History** - Command history is saved to `~/.raven_history` and restored across sessions
- **ANSI Colors** - Colorized prompt, `ls` output (directories/executables/symlinks), and error messages (only when attached to a terminal)
- **Script Execution** - Run `.rsh` script files for automation
- **Go-like Syntax** - Variables, arrays, loops, and conditionals with familiar syntax
- **Functions** - Define reusable functions with parameters, return values, and recursion (`fn name(args) { ... }`)
- **Rich Control Flow** - `for`, `while`, `if`/`else if`/`else`, plus `break` and `continue`
- **Command Sequencing** - `;` separators, `&&`/`||` short-circuit chaining, and `$?` exit status
- **Multi-line Editing** - Type loops, conditionals, and functions across lines with a continuation prompt
- **Interruptible** - `Ctrl-C` stops a running command or loop without killing the shell; `Ctrl-R` searches history
- **Process Management** - `ps`, `kill`, `killall`, background jobs (`&`), and `jobs`
- **Built-in Helpers** - String/collection functions: `len`, `split`, `join`, `contains`, `upper`, `lower`, `trim`, `replace`, `glob`
- **Environment & Substitution** - `export`/`env`, `$(command)` substitution, and `$VAR` interpolation in double quotes
- **Built-in Commands** - File system operations (ls, cd, mkdir, rm, etc.)
- **Human-Readable Aliases** - Natural-language names for common actions: `whereami`/`wai` (cwd), `read`/`view` (show a file), `remove`/`delete` (rm), `makefile`/`newfile`/`touch` (mkfile), `makedir` (mkdir)
- **Built-in Help** - `raven-help` lists every built-in; `raven-help <command>` shows usage and aliases
- **External Commands** - Run any program on your `PATH` (git, python, cat, ...) with flag support (`-l`, `--all`)
- **Pipes & Redirection** - Chain commands with `|` and redirect with `>`, `>>`, `<`
- **Configuration** - Customize startup behavior with `.ravenrc`

## Quick Start

### Installation

Clone the repo, then install with the provided script or `make`:

```bash
git clone https://github.com/yourusername/ravenshell.git
cd ravenshell

# Option 1: one-step install script (uses sudo for /usr/local/bin if needed)
./install.sh

# Option 2: Makefile (override location with PREFIX=...)
make install                 # installs to /usr/local/bin
make install PREFIX=~/.local # installs to ~/.local/bin (no sudo)

# Option 3: Go toolchain
go install .                 # installs to $(go env GOPATH)/bin
```

To install to a directory without root, set `PREFIX` to a location you own
(e.g. `~/.local`) and make sure its `bin` is on your `PATH`.

For a quick local build without installing:

```bash
go build -o ravenshell .
```

### Usage

**Interactive mode:**
```bash
ravenshell
```

**Run a script:**
```bash
ravenshell script.rsh
```

**Run a one-off command:**
```bash
ravenshell -c 'print "hello"'
```

**From a pipe or redirect:**
```bash
echo 'print 6 * 7' | ravenshell
ravenshell < script.rsh
```

### Set as Your Default Shell

Once installed, RavenShell can be used as your login shell like any other shell:

```bash
make register-shell   # adds the binary to /etc/shells (needs sudo)
make set-default      # chsh -s to RavenShell
```

Then log out and back in. (To do it manually: add the binary's full path to
`/etc/shells`, then run `chsh -s /path/to/ravenshell`.)

### Basic Examples

```rsh
# File operations
ls
cd ~/Documents
mkdir new_folder

# Human-readable command names (aliases of the classic ones)
whereami                 # like cwd / pwd  (also: wai)
makefile notes.txt       # like mkfile / touch  (also: newfile, touch)
read notes.txt           # like show / cat  (also: view)
remove notes.txt         # like rm  (also: delete)
rmdir old_dir/ --force   # remove a non-empty directory (-f also works)
raven-help               # list all built-ins; `raven-help read` for details

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

# Environment variables, interpolation, and command substitution
export PROJECT RavenShell
print "project is $PROJECT"      # $VAR expands inside double quotes
here = $(cwd)
print here

# Command sequencing and exit status
mkdir build ; cd build           # ; separates commands
git pull && make                 # run make only if git pull succeeds
test -f config || print "missing config"
print $?                         # exit status of the last command

# Globbing (modern, unambiguous): glob() + array splatting
for f in glob("*.txt") { print f }
rm glob("*.tmp")

# Process management
ps chrome                        # list processes matching "chrome"
sleep 60 &                       # run in the background
jobs                             # list background jobs
kill %1                          # kill background job 1
killall node KILL                # signal all matching processes

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
