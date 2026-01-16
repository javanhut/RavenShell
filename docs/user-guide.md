# RavenShell User Guide

This guide covers everything you need to know to use RavenShell effectively.

## Installation

### Prerequisites

- Go 1.21 or later

### Building from Source

```bash
git clone https://github.com/yourusername/ravenshell.git
cd ravenshell
go build -o ravenshell
```

### Verifying Installation

```bash
./ravenshell
# You should see: Welcome to Raven Shell.
```

## Getting Started

### Interactive Mode (REPL)

Start RavenShell without arguments to enter interactive mode:

```bash
./ravenshell
```

You'll see the welcome message and a `#` prompt:

```
Welcome to Raven Shell.
#
```

Type commands and press Enter to execute them. Type `exit` or `quit` to leave.

### Script Mode

Run a `.rsh` script file:

```bash
./ravenshell myscript.rsh
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

Press `Tab` to complete commands and file paths:

- Type `mk` then `Tab` to see `mkdir`, `mkfile`
- Type `~/Doc` then `Tab` to complete `~/Documents/`

When multiple completions exist, pressing `Tab` shows all options.

### Command History

Use arrow keys to navigate through previously entered commands:

- **Up Arrow**: Previous command
- **Down Arrow**: Next command

History is maintained for the current session.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+A` | Move cursor to beginning of line |
| `Ctrl+E` | Move cursor to end of line |
| `Ctrl+U` | Clear line before cursor |
| `Ctrl+K` | Clear line after cursor |
| `Ctrl+W` | Delete word before cursor |
| `Ctrl+L` | Clear screen |
| `Ctrl+C` | Cancel current input |
| `Ctrl+D` | Exit (on empty line) / Delete character |
| `Left Arrow` | Move cursor left |
| `Right Arrow` | Move cursor right |
| `Up Arrow` | Previous history entry |
| `Down Arrow` | Next history entry |
| `Home` | Move to beginning of line |
| `End` | Move to end of line |
| `Delete` | Delete character under cursor |
| `Backspace` | Delete character before cursor |
| `Tab` | Auto-complete |

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
- The file is executed line-by-line
- Comments (`#`) and blank lines are ignored
- Errors in `.ravenrc` are displayed but don't prevent shell startup

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
rm old_folder               # Remove directory and contents
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

## Environment Variables

Access system environment variables with `$`:

```rsh
print $HOME                 # Print home directory
print $USER                 # Print username
cd $HOME                    # Change to home directory
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
