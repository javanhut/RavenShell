# RavenShell Commands Reference

This document provides a complete reference for all built-in commands in RavenShell.

## File System Commands

Many commands have human-readable aliases so the same action can be written the
way that reads most naturally. Aliases behave identically to the canonical
command.

| Canonical | Aliases | Action |
|-----------|---------|--------|
| `cwd`     | `whereami`, `wai` | Print the current directory |
| `show`    | `read`, `view` | Print a file's contents |
| `mkfile`  | `makefile`, `newfile`, `touch` | Create empty files |
| `mkdir`   | `makedir` | Create directories |
| `rm`      | `remove`, `delete` | Remove files or directories |

Run `raven-help` to list every built-in, or `raven-help <command>` for details.

### ls - List Directory Contents

Lists files and directories in the specified location.

**Syntax:**
```
ls [path]
```

**Arguments:**
- `path` (optional): Directory to list. Defaults to current directory.

**Output:** File and directory names, with directories marked with a trailing `/`.

**Examples:**
```rsh
ls                  # List current directory
ls ~/Documents      # List Documents folder
ls /tmp             # List /tmp directory
```

---

### cd - Change Directory

Changes the current working directory.

**Syntax:**
```
cd [path]
```

**Arguments:**
- `path` (optional): Target directory. Defaults to home directory if omitted.

**Path types supported:**
- Absolute: `/home/user/folder`
- Relative: `./subdir`, `../parent`
- Home: `~`, `~/folder`

**Examples:**
```rsh
cd                  # Go to home directory
cd ~/Documents      # Go to Documents
cd ..               # Go to parent directory
cd /tmp             # Go to /tmp
```

---

### cwd - Current Working Directory

Prints the current working directory.

**Aliases:** `whereami`, `wai`

**Syntax:**
```
cwd
```

**Arguments:** None

**Example:**
```rsh
cwd
whereami            # same thing, reads more naturally
# Output: /home/user/projects
```

---

### mkdir - Make Directory

Creates one or more directories (including any missing parent directories).

**Aliases:** `makedir`

**Syntax:**
```
mkdir path [path...]
```

**Arguments:**
- `path`: One or more directory paths to create.

**Notes:** Creates parent directories if they don't exist.

**Examples:**
```rsh
mkdir new_folder
mkdir dir1 dir2 dir3
mkdir ~/projects/new_project
```

---

### rmdir - Remove Directory

Removes directories. By default only empty directories are removed (the safe
choice); pass `-f` / `--force` to remove a directory and everything inside it.

**Syntax:**
```
rmdir [-f|--force] path [path...]
```

**Arguments:**
- `path`: One or more directory paths to remove.

**Flags:**
- `-f`, `--force`: Remove non-empty directories recursively (like `rm`).

**Notes:** Without `--force`, removing a non-empty directory fails with a hint
to use `--force`.

**Examples:**
```rsh
rmdir empty_folder            # only works if empty
rmdir dir1 dir2
rmdir project/ --force        # remove a non-empty directory
rmdir build -f                # short flag form
```

---

### rm - Remove

Removes files or directories (including contents).

**Aliases:** `remove`, `delete`

**Syntax:**
```
rm path [path...]
```

**Arguments:**
- `path`: One or more file or directory paths to remove.

**Warning:** Removes directories recursively with all contents.

**Examples:**
```rsh
rm file.txt
rm file1.txt file2.txt
rm old_folder
```

---

### mkfile - Make File

Creates empty files.

**Aliases:** `makefile`, `newfile`, `touch`

**Syntax:**
```
mkfile path [path...]
```

**Arguments:**
- `path`: One or more file paths to create.

**Examples:**
```rsh
mkfile newfile.txt
mkfile file1.txt file2.txt
mkfile ~/notes.txt
```

---

### show - Show File Contents

Displays the contents of one or more files (like `cat`).

**Aliases:** `read`, `view`

**Syntax:**
```
show path [path...]
```

**Arguments:**
- `path`: One or more file paths to display.

**Examples:**
```rsh
show file.txt
read config.txt           # alias, reads naturally
view ~/notes.txt          # alias
show file1.txt file2.txt
```

---

## Output Commands

### print - Print Text

Prints text to standard output.

**Syntax:**
```
print [arguments...]
```

**Arguments:**
- `arguments`: Values to print (strings, variables, expressions).

**Behavior:**
- Prints arguments joined by spaces, followed by a newline.
- When used with a pipe, prints the piped input.

**Examples:**
```rsh
print "Hello, World!"
print name
print 1 + 2
print "Value:" count
ls | print
```

---

### output - Output Text

Alias for the `print` command. Behaves identically.

**Syntax:**
```
output [arguments...]
```

**Examples:**
```rsh
output "Hello"
output result
```

---

## Utility Commands

### whoami - Current User

Displays the current username.

**Syntax:**
```
whoami
```

**Arguments:** None

**Example:**
```rsh
whoami
# Output: username
```

---

### clear - Clear Screen

Clears the terminal screen.

**Syntax:**
```
clear
```

**Arguments:** None

---

### ~ - Home Directory

When used alone, prints the home directory path.

**Syntax:**
```
~
```

**Example:**
```rsh
~
# Output: /home/username
```

**Note:** Can also be used in path expressions like `~/Documents`.

---

## Environment Commands

### export - Set Environment Variable

Sets a shell environment variable. The value is the remaining arguments joined
by spaces. Exported variables are visible via `$NAME` and are passed to external
commands.

**Syntax:**
```
export NAME [value...]
```

**Examples:**
```rsh
export EDITOR vim
print $EDITOR               # vim

export GREETING hello world
print $GREETING             # hello world
```

---

### env - List Environment

Prints the effective environment (process environment with shell-local
`export` overrides applied), sorted by name.

**Syntax:**
```
env
```

**Example:**
```rsh
env
env | grep PATH             # combine with an external command
```

---

## Configuration Commands

### raven-add - Manage Shell Configuration

Registers extra configuration for the shell. Currently supports adding
executable search directories.

**Syntax:**
```
raven-add path <dir>        # add an executable search directory
raven-add path              # list registered search directories
```

**Behavior:**
- `<dir>` must exist and be a directory.
- Added directories take priority over the system `PATH` when resolving
  external commands.
- Entries are persisted to `~/.raven_paths` and reloaded on startup.

**Examples:**
```rsh
raven-add path ~/scripts    # register ~/scripts
raven-add path /opt/tools/bin
raven-add path              # list current search paths
```

---

### raven-help - Built-in Command Help

Lists all built-in commands grouped by category, or shows detailed help for a
single command. Command names passed to it may be aliases (e.g. `read`), which
resolve to their canonical command.

**Aliases:** `help`

**Syntax:**
```
raven-help                  # list every built-in command
raven-help <command>        # show usage, summary, and aliases for one command
```

**Examples:**
```rsh
raven-help                  # overview of all commands
help                        # same thing
raven-help rmdir            # details for rmdir, including --force
help read                   # resolves to the `show` command
```

---

## External Commands

Any command that is not a built-in is executed as an external program. The shell
searches the directories registered with `raven-add path` first, then the
system `PATH`.

**Syntax:**
```
program [args...] [flags...]
```

**Examples:**
```rsh
git status
python --version
ls -la
grep -n "TODO" notes.txt
print "a b c" | wc -w       # external commands work in pipes
```

**Notes:**
- Flags (`-l`, `--all`, `--max-count=5`) are passed through to the program.
- Command names may contain hyphens (e.g. `docker-compose`).
- The command runs in the shell's current working directory and inherits the
  shell environment (including `export`ed variables).

---

## Process Management Commands

### ps - List Processes

Lists running system processes, optionally filtered by a name substring
(case-insensitive).

**Syntax:**
```
ps [name-filter]
```

**Examples:**
```rsh
ps                  # list all processes
ps chrome           # only processes whose name contains "chrome"
```

---

### kill - Signal a Process

Sends a signal to a process by PID or background job reference (`%N`). Defaults
to `TERM`.

**Syntax:**
```
kill <pid|%job> [signal]
```

**Signals:** name (`TERM`, `KILL`, `HUP`, `INT`, ...) with or without the `SIG`
prefix, or a number (`9`, `15`).

**Examples:**
```rsh
kill 1234           # send SIGTERM to pid 1234
kill 1234 KILL      # send SIGKILL
kill %1             # kill background job 1
```

---

### killall - Signal Processes by Name

Sends a signal to every process whose name contains the given text
(case-insensitive). Defaults to `TERM`.

**Syntax:**
```
killall <name> [signal]
```

**Examples:**
```rsh
killall node        # SIGTERM every process matching "node"
killall chrome KILL # force-kill matching processes
```

---

### jobs - List Background Jobs

Lists background jobs started with `&`, with their job id, PID, and status.
Completed jobs are dropped after being listed.

**Syntax:**
```
jobs
```

**Example:**
```rsh
sleep 60 &          # [1] 12345
jobs                # [1]  12345    running  sleep
```

---

## Session Commands

### exit / quit

Exits the RavenShell session.

**Syntax:**
```
exit
quit
```

**Note:** In interactive mode, you can also press `Ctrl+D` on an empty line.

---

## Operators

### Pipe ( | )

Sends output from one command as input to another.

**Syntax:**
```
command1 | command2
```

**Example:**
```rsh
ls | print
```

---

### Output Redirection ( > )

Writes command output to a file, overwriting existing content.

**Syntax:**
```
command > file
```

**Example:**
```rsh
ls > files.txt
print "Hello" > greeting.txt
```

---

### Append Redirection ( >> )

Appends command output to a file.

**Syntax:**
```
command >> file
```

**Example:**
```rsh
print "Line 1" > log.txt
print "Line 2" >> log.txt
```

---

### Input Redirection ( < )

Uses file contents as command input.

**Syntax:**
```
command < file
```

**Example:**
```rsh
print < input.txt
```

---

### Sequencing ( ; )

Separates multiple commands on one line.

```rsh
mkdir build ; cd build ; ls
```

---

### And ( && ) / Or ( || )

`&&` runs the right command only if the left succeeded (exit status 0); `||`
runs it only if the left failed.

```rsh
git pull && make                 # make only if pull succeeds
test -f config || print "no config"
```

---

### Background ( & )

Runs an external command in the background and returns to the prompt
immediately. The job's id and PID are printed, and it can be managed with
`jobs` and `kill %id`.

```rsh
sleep 60 &
```
