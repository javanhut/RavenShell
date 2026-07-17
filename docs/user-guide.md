# RavenShell User Guide

This guide covers everything you need to know to use RavenShell effectively.

## Installation

### Prerequisites

- Go 1.21 or later

### Installing

```bash
git clone https://github.com/yourusername/ravenshell.git
cd ravenshell

# Easiest: install script (uses sudo for /usr/local/bin if needed)
./install.sh

# Or with make (override the location with PREFIX=...)
make install                 # /usr/local/bin
make install PREFIX=~/.local # ~/.local/bin, no sudo

# Or via the Go toolchain
go install .                 # installs to $(go env GOPATH)/bin
```

For a quick local build without installing, run `go build -o ravenshell .`.

### Verifying Installation

```bash
ravenshell --version
ravenshell
# You should see: Welcome to Raven Shell.
```

### Setting RavenShell as Your Default Shell

After installing, you can use RavenShell as your login shell:

```bash
make register-shell   # add the binary to /etc/shells (needs sudo)
make set-default      # chsh to RavenShell
```

Log out and back in for the change to take effect. To revert, run
`chsh -s /bin/zsh` (or your previous shell).

### Uninstalling

```bash
make uninstall        # removes the installed binary
```

## Getting Started

### Interactive Mode (REPL)

Start RavenShell without arguments to enter interactive mode:

```bash
./ravenshell
```

You'll see the welcome message and a colored prompt showing your current
directory (the home directory is abbreviated to `~`):

```
Welcome to Raven Shell.
~/projects ❯
```

Type commands and press Enter to execute them. Type `exit` or `quit` to leave
(or press `Ctrl+D` on an empty line).

When connected to a terminal, RavenShell colorizes the prompt, `ls` output
(directories, executables, and symlinks), and error messages. Colors are
automatically disabled when output is piped or redirected.

### Script Mode

Run a `.rsh` script file:

```bash
ravenshell myscript.rsh input.txt --verbose
```

Run a one-off command, or feed a program in via stdin:

```bash
ravenshell -c 'print "hello"'
echo 'print 6 * 7' | ravenshell
ravenshell < myscript.rsh
```

### Creating Scripts

RavenShell scripts use the `.rsh` extension. Create a file with your commands:

```rsh
# myscript.rsh
print "Hello from RavenShell!"
ls
cwd
```

Comments start with `#` and continue to the end of the line.

Arguments after the filename are exposed as the regular RavenScript `args`
array. The filename itself is not included:

```rsh
for arg in args {
    print arg
}
```

## Shell Features

### Navigation

Use `cd` to change directories and `cwd` to show your current location:

```rsh
cwd                 # Show current directory
cd ~/Documents      # Change to Documents
cd ..               # Go up one level
cd                  # Go to home directory
```

### Tab Completion

Press `Tab` to complete the word at the cursor:

- **Command names** (first word): built-in commands, your user-defined
  functions, and executables found on the search/system `PATH`.
  - Type `mk` then `Tab` to see `mkdir`, `mkfile`
  - Type `gi` then `Tab` to complete external programs like `git`
- **Subcommands and flags** (fish-style): RavenShell knows the subcommands and
  common flags of popular tools — `git`, `go`, `npm`, `docker`, `make`,
  `cargo`, and `brew` — and shows each candidate with a short description.
  - `git ch` then `Tab` completes to `che`, and `Tab` again lists `checkout`
    and `cherry-pick` with descriptions
  - `git commit --` then `Tab` lists `--all`, `--amend`, `--message`, ...
- **Dynamic arguments**, generated at the moment you press `Tab`:
  - `git checkout <Tab>` lists your actual branches and tags
  - `git push <Tab>` lists your remotes and branches
  - `make <Tab>` lists the targets of the Makefile in the current directory
  - `npm run <Tab>` lists the scripts from `package.json`
- **Flags of any other command** (fish-style, from man pages): when a command
  has no built-in spec, the first `Tab` on a `-` word reads the command's **man
  page**, scrapes its flags and their descriptions, and caches them to disk
  (`command --help` is used as a fallback when there is no man page).
  - `curl --<Tab>`, `rsync --<Tab>`, `ssh -<Tab>` all work with descriptions
- **Subcommands of any other command** (from `--help`): the first `Tab` at the
  subcommand position of a spec-less tool runs `command --help`, parses its
  `Commands:` section, and caches the result.
  - `kubectl <Tab>`, `gh <Tab>`, `terraform <Tab>` list their subcommands
  - This is filled in lazily per command; see
    [Generating completions in bulk](#generating-completions-in-bulk) to
    pre-build them all at once.
- **File paths** (later words): files and directories relative to the current
  directory, including `~/`, `/absolute`, and relative paths. Dotfiles are
  hidden unless the word starts with `.`, and commands that only take
  directories (like `cd`) only offer directories.
  - Type `~/Doc` then `Tab` to complete `~/Documents/`

When a single completion matches it is inserted automatically. When several
match, `Tab` first inserts their longest common prefix; pressing `Tab` again
lists all options, with descriptions shown dimmed. File candidates in the
listing are color-coded by type, using the same scheme as `ls`: directories
**bold blue**, symlinks **cyan**, executables **green**.

#### Custom completions

You can teach RavenShell to complete your own tools by dropping a JSON spec in
`~/.config/ravenshell/completions/<command>.json`. The file is loaded the
first time you complete that command:

```json
{
  "flags": [{ "text": "--verbose", "desc": "Verbose output" }],
  "subcommands": [
    {
      "name": "serve",
      "desc": "Start the server",
      "flags": [{ "text": "--port", "desc": "Port to listen on" }],
      "args": {
        "static": [{ "text": "dev" }, { "text": "prod" }],
        "command": "mytool list-environments",
        "noFiles": true
      }
    }
  ],
  "args": { "dirsOnly": false, "noFiles": false }
}
```

- `flags` — flags offered anywhere on the command line.
- `subcommands` — each with its own `flags` and `args`.
- `args` — where positional-argument candidates come from: `static` is a fixed
  list, `command` is a shell command run at completion time (one candidate per
  output line; text and description separated by a tab), `noFiles` suppresses
  the file fallback, and `dirsOnly` restricts the file fallback to directories.

#### Generating completions in bulk

By default, flags and subcommands for tools without a built-in spec are scraped
the first time you tab them and cached on disk
(`~/.cache/ravenshell/completions`, honoring `$XDG_CACHE_HOME`). The cache is
keyed by the man page / binary modification time, so it refreshes automatically
when a tool is upgraded.

The `raven-completions` command — RavenShell's equivalent of fish's
`fish_update_completions` — pre-builds the cache so completions are instant from
the first tab:

```
raven-completions               # show the cache location and how many are cached
raven-completions update        # parse every installed man page for flags
raven-completions update --deep # also run '<cmd> --help' for subcommands
raven-completions clear         # delete the cache
raven-completions path          # print the cache directory
```

`update` only reads man pages, so it is safe and passive. `update --deep`
additionally runs `<command> --help` on every user command to harvest
subcommands (kubectl, gh, docker, …); because that executes those programs, it
is opt-in. You rarely need either — tabbing a command fills its completions in
on demand — but `--deep` is the way to get subcommands for *every* installed
tool at once.

### Autosuggestions

As you type, RavenShell shows a dimmed inline suggestion drawn from your most
recent matching history entry (fish-style). Accept the full suggestion with:

- **→ (Right Arrow)** at the end of the line
- **Ctrl+E** or **End**

Keep typing to refine the suggestion, or ignore it.

### Command History

Use arrow keys to navigate through previously entered commands:

- **Up Arrow**: Previous command
- **Down Arrow**: Next command
- **Ctrl+R**: Reverse incremental search — start typing to find a matching past
  command, press `Ctrl+R` again for older matches, `Enter` to run it, or `Esc`
  to cancel.

History is **persistent**: it is saved to `~/.raven_history` and restored across
sessions, so suggestions and recall work for commands from previous runs too.

### Multi-line Input

When you type a line with an open `{`, `(`, or `[` (for example the start of a
loop, conditional, or function), the prompt switches to a `…` continuation
prompt and keeps reading until everything is balanced, then runs it as one
unit:

```
~/projects ❯ for i in range(3) {
…     print i
… }
0
1
2
```

Press `Ctrl+C` at any point to discard the partially-entered block.

### Interrupting Commands

Press `Ctrl+C` to stop a running command (such as a long-running external
program) or a runaway loop. The command is interrupted but the shell itself
keeps running.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+A` | Move cursor to beginning of line |
| `Ctrl+E` | Move to end of line / accept autosuggestion |
| `Ctrl+U` | Clear line before cursor |
| `Ctrl+K` | Clear line after cursor |
| `Ctrl+W` | Delete word before cursor |
| `Ctrl+L` | Clear screen |
| `Ctrl+R` | Reverse incremental history search |
| `Ctrl+C` | Cancel current input, or interrupt a running command/loop |
| `Ctrl+D` | Exit (on empty line) / Delete character |
| `Left Arrow` | Move cursor left |
| `Up Arrow` | Previous history entry |
| `Down Arrow` | Next history entry |
| `Home` | Move to beginning of line |
| `End` | Move to end of line / accept autosuggestion |
| `Right Arrow` | Move right / accept autosuggestion (at end of line) |
| `Delete` | Delete character under cursor |
| `Backspace` | Delete character before cursor |
| `Tab` | Auto-complete command or path |

## Configuration

### The .ravenrc File

RavenShell loads `~/.ravenrc` on startup. Use it to run commands automatically when starting the shell.

**Location:** `~/.ravenrc` (in your home directory)

**Example .ravenrc:**

```rsh
# ~/.ravenrc - RavenShell startup configuration

# Display a welcome message
print "Welcome back!"

# Show current directory
cwd

# Set up commonly used variables
workspace = "~/projects"
```

**Notes:**
- The file is parsed and run as a complete program, so multi-line constructs
  (loops, conditionals, functions) work just like in a script.
- Comments (`#`) and blank lines are ignored.
- Errors in `.ravenrc` are displayed but don't prevent shell startup.

### Custom Executable Search Paths

Register extra directories to search for external commands with `raven-add path`:

```rsh
raven-add path ~/scripts        # add a directory
raven-add path                  # list registered directories
```

Added directories take priority over the system `PATH` and are saved to
`~/.raven_paths`, so they persist across sessions. Putting `raven-add path`
lines in `.ravenrc` works too.

### Configuration Files

| File | Purpose |
|------|---------|
| `~/.ravenrc` | Startup script, run as a program on launch |
| `~/.raven_history` | Persistent command history |
| `~/.raven_paths` | Extra executable search directories (`raven-add path`) |
| `~/.config/ravenshell/completions/` | Your own completion specs (`<command>.json`) |
| `~/.cache/ravenshell/completions/` | Auto-generated completions from man pages / `--help` (`raven-completions`; honors `$XDG_CACHE_HOME`) |

## Working with Files and Directories

### Listing Contents

```rsh
ls                  # Current directory
ls ~/Documents      # Specific directory
ls /var/log         # Absolute path
```

Output shows files and directories, with directories marked by a trailing `/`.

### Creating Files and Directories

```rsh
mkdir new_folder            # Create directory
mkdir dir1 dir2 dir3        # Create multiple directories
mkfile notes.txt            # Create empty file
mkfile a.txt b.txt c.txt    # Create multiple files
```

### Removing Files and Directories

```rsh
rm file.txt                 # Remove file
rm --recursive old_folder   # Remove directory and contents explicitly
rmdir empty_folder          # Remove empty directory only
```

### Viewing File Contents

```rsh
show file.txt               # Display file contents
show file1.txt file2.txt    # Display multiple files
```

## Input and Output

### Printing Output

```rsh
print "Hello, World!"       # Print a string
print variable_name         # Print variable value
print 1 + 2                 # Print expression result
print "Count:" count        # Print multiple values
```

### Pipes

Chain commands together using `|`:

```rsh
ls | print                  # Pipe ls output to print
```

The output of the left command becomes the input to the right command.

### Output Redirection

Write output to files:

```rsh
ls > files.txt              # Overwrite file
ls >> files.txt             # Append to file
```

### Input Redirection

Read input from files:

```rsh
print < input.txt           # Read and print file contents
```

## Running External Programs

Any command that isn't a built-in runs as an external program found on your
search/system `PATH`:

```rsh
git status                  # run git
python --version            # run python with a flag
ls -la                      # flags are passed through
print "a b c" | wc -w       # external programs work in pipes
```

Use `print` for the shell's own text output; bare words are for invoking
programs. Register extra search directories with `raven-add path`.

You can chain commands by exit status and run them in the background:

```rsh
git pull && make            # make only if pull succeeds
test -f x || print "no x"   # print only if the test fails
build ; deploy              # run one after the other
sleep 60 &                  # run in the background
print $?                    # exit status of the last command
```

## Managing Processes

RavenShell has built-in, ergonomic process tools — no `ps aux | grep | awk`
pipelines required:

```rsh
ps                  # list all processes
ps chrome           # filter by name

kill 1234           # send SIGTERM to a pid
kill 1234 KILL      # send a named signal (or a number)
kill %1             # kill background job 1

killall node        # signal every process matching "node"
killall chrome KILL

sleep 60 &          # start a background job  -> [1] 12345
jobs                # list background jobs
kill %1             # stop it
```

## Environment Variables

Read environment variables with `$` and set them with `export`:

```rsh
print $HOME                 # Print home directory
print $USER                 # Print username
cd $HOME                    # Change to home directory

export EDITOR vim           # Set a variable
print $EDITOR               # vim
env                         # List the environment
```

Capture a command's output into a value with `$(...)`:

```rsh
here = $(cwd)
print "you are in " + here
```

## Path Handling

RavenShell understands various path formats:

| Path Type | Example | Description |
|-----------|---------|-------------|
| Absolute | `/home/user/file` | Full path from root |
| Relative | `./file`, `../dir` | Relative to current directory |
| Home | `~`, `~/Documents` | Relative to home directory |
| Current | `.` | Current directory |
| Parent | `..` | Parent directory |

## Error Messages

Common error messages and their meanings:

| Error | Cause |
|-------|-------|
| `cd: no such file or directory` | Directory doesn't exist |
| `cd: not a directory` | Path exists but isn't a directory |
| `rm: missing operand` | No file/directory specified |
| `show: missing file argument` | No file specified to show |
| `cannot create file` | Permission denied or invalid path |

## Tips

1. **Use tab completion** to avoid typing mistakes
2. **Check your location** with `cwd` if you're unsure where you are
3. **Use `~`** as a shortcut for your home directory
4. **Create a `.ravenrc`** to customize your shell startup
5. **Use pipes** to combine commands efficiently
