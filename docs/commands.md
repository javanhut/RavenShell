# RavenShell Commands Reference

This document provides a complete reference for all built-in commands in RavenShell.

## File System Commands

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

**Syntax:**
```
cwd
```

**Arguments:** None

**Example:**
```rsh
cwd
# Output: /home/user/projects
```

---

### mkdir - Make Directory

Creates one or more directories.

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

Removes empty directories.

**Syntax:**
```
rmdir path [path...]
```

**Arguments:**
- `path`: One or more empty directory paths to remove.

**Notes:** Directories must be empty. Use `rm` for non-empty directories.

**Examples:**
```rsh
rmdir empty_folder
rmdir dir1 dir2
```

---

### rm - Remove

Removes files or directories (including contents).

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

Displays the contents of one or more files.

**Syntax:**
```
show path [path...]
```

**Arguments:**
- `path`: One or more file paths to display.

**Examples:**
```rsh
show file.txt
show file1.txt file2.txt
show ~/config.txt
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
