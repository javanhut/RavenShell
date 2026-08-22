# RavenShell Language Reference

This document provides a complete reference for the RavenShell scripting language syntax.

## Overview

RavenShell uses a Go-like syntax that combines shell commands with scripting capabilities. Scripts are stored in `.rsh` files and parsed as a whole program (multi-line constructs such as loops, conditionals, and functions are fully supported).

Statements are separated by newlines: a newline ends a command's argument list, so each line is its own statement. Within a `{ ... }` block you can place one statement per line.

## Comments

Single-line comments start with `#`:

```rsh
# This is a comment
print "Hello"  # Inline comment
```

There is no multi-line comment syntax.

## Data Types

### Integers

64-bit signed integers:

```rsh
x = 42
y = -10
z = 0
```

### Strings

Text enclosed in double or single quotes:

```rsh
name = "Hello, World!"
path = 'single quotes work too'
```

Triple-quoted strings can span lines, which is the preferred RavenScript way to
hold multi-line text in a variable (shell-style `<<` heredocs are also supported
for feeding text straight to a command):

```rsh
message = """Build started
Processing files...
Build complete"""
print message
```

Common escapes such as `\n`, `\t`, `\\`, `\"`, and `\'` are supported.

**Interpolation:** double-quoted strings expand `$VAR` and `${VAR}` references
(and `$?`); single-quoted strings are literal:

```rsh
user = "raven"
print "hi $user"        # hi raven
print "home: ${HOME}"   # home: /Users/you
print 'literal $user'   # literal $user
```

### Arrays

Ordered collections of values:

```rsh
# Empty array with type hint
items = []string

# Array literal
numbers = [1, 2, 3, 4, 5]
mixed = ["text", 42, "more"]
```

### Booleans

Boolean values result from comparison operations. The literals `true` and
`false` can also be assigned and passed to functions:

```rsh
if x > 5 {      # Comparison produces boolean
    print "yes"
}
```

**Truthiness rules:**
- `0` is false, non-zero integers are true
- Empty string `""` is false, non-empty strings are true
- Empty arrays are false, non-empty arrays are true
- `nil` is false

## Variables

### Script arguments

Arguments passed after a script filename are available as the global `args`
array. The filename is not inserted into the array and there are no numbered
shell variables:

```rsh
# ravenshell build.rsh debug app
print len(args)     # 2
print args[0]       # debug
print args[1]       # app
```

Because `args` is ordinary language data, it can be indexed, iterated, passed
to functions, or supplied to an external command.

### Assignment

Use `=` to assign values to variables:

```rsh
x = 5
name = "RavenShell"
numbers = [1, 2, 3]
empty = []string
```

Variable names must start with a letter and can contain letters, numbers, and underscores.

Assignment is the *spaced* form: the `=` must have whitespace around it. A glued
`KEY=value` carries no assignment meaning and is an ordinary argument word, so
`printf x a FOO=bar b` passes `FOO=bar` through to `printf` unchanged.

### Program exit

`exit()` ends a script or interactive session successfully. Pass an integer
from 0 through 255 to choose another process status:

```rsh
if len(args) == 0 {
    print "an input is required"
    exit(2)
}
```

`lastStatus()` returns the most recent command's process status as an integer.
It is the language-oriented equivalent of the interactive `$?` shorthand.

### Using Variables

Reference variables by name:

```rsh
x = 10
y = x + 5       # y is 15
print x         # Prints 10
```

### Environment Variables

Access environment variables with `$`. Lookups check shell-local variables set
with `export` first, then the process environment:

```rsh
print $HOME     # Prints home directory
print $USER     # Prints username
path = $HOME + "/documents"
```

Set a shell environment variable with `export` (the value is the remaining
arguments joined by spaces). Exported variables are visible to `$NAME` and are
passed to external commands:

```rsh
export EDITOR vim
print $EDITOR           # vim

export GREETING hello world
print $GREETING         # hello world
```

The shell spelling `NAME=value` works too, including several at once:

```rsh
export EDITOR=vim
export A=1 B=2
```

Use `env` to list the effective environment.

### Per-command Environment

A `NAME=value` written in front of a command sets that variable for the command
only. It does not persist afterwards, which makes it the right way to vary one
setting for a single run:

```rsh
DEBUG=1 ./build.sh          # DEBUG is set only while build.sh runs
print $DEBUG                # empty again

A=1 B=2 make install        # several at once
```

A `NAME=value` with no command after it is an ordinary assignment and does
persist. Note that RavenScript's own assignment is the *spaced* form (`x = 5`);
the glued form is the environment spelling, and a glued `KEY=value` anywhere
else on the line is just an argument word:

```rsh
x = 5                       # RavenScript variable
CGO_ENABLED=0 go build      # environment for one command
printf "%s\n" FOO=bar       # an ordinary argument
```

A `~` directly after the `=` expands to your home directory, so
`GOPATH=~/go` and `GOPATH=/home/you/go` mean the same thing.

### Command Substitution

`$(command)` runs a command and substitutes its captured output (with the
trailing newline trimmed) as a string value:

```rsh
here = $(cwd)
print "running in " + here

user = $(whoami)
print "hello " + user
```

## Operators

### Arithmetic Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Addition | `5 + 3` → `8` |
| `-` | Subtraction | `5 - 3` → `2` |
| `*` | Multiplication | `5 * 3` → `15` |
| `/` | Division (integer) | `7 / 2` → `3` |
| `%` | Modulo | `7 % 3` → `1` |

```rsh
result = 10 + 5 * 2     # 20 (multiplication first)
remainder = 17 % 5       # 2
```

All arithmetic is 64-bit signed integer — there are no floating point literals
in the language. `/` truncates toward zero, `%` is the remainder, and dividing
or taking the modulo of zero is an error.

```rsh
print 10 / 4       # 2
print -7 / 2       # -3
print 10 % 4       # 2
print 10 / 0       # error: division by zero
```

> **Spacing matters for `-`.** Put spaces around arithmetic operators
> (`a - b`). A `-` glued to a word (`-l`, `--all`) is parsed as a command
> *flag*, and a `-` between word characters (`docker-compose`) is part of an
> identifier/command name. So write subtraction as `a - b`, not `a-b`.

### Comparison Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equal | `x == 5` |
| `!=` | Not equal | `x != 5` |
| `<` | Less than | `x < 10` |
| `>` | Greater than | `x > 10` |
| `<=` | Less than or equal | `x <= 10` |
| `>=` | Greater than or equal | `x >= 10` |

```rsh
if x == 5 {
    print "x is 5"
}

if count >= 10 {
    print "count is at least 10"
}
```

### String Concatenation

Use `+` to concatenate strings:

```rsh
greeting = "Hello, " + "World!"
path = ~ + "/documents"
message = "Count: " + count    # Converts count to string
```

When either operand is a string, `+` performs concatenation.

## Operator Precedence

From highest to lowest precedence:

1. `[]` - Array indexing
2. `*`, `/`, `%` - Multiplication, division, modulo
3. `+`, `-` - Addition, subtraction
4. `<`, `>`, `<=`, `>=` - Comparison
5. `==`, `!=` - Equality
6. `|` - Pipe
7. `>`, `>>`, `<`, `<<` - Redirection

Use parentheses to override precedence:

```rsh
result = (1 + 2) * 3    # 9
```

## Control Flow

### If Statements

```rsh
if condition {
    # statements
}
```

With else:

```rsh
if condition {
    # statements if true
} else {
    # statements if false
}
```

**Examples:**

```rsh
count = 10

if count > 5 {
    print "count is greater than 5"
}

if count == 0 {
    print "count is zero"
} else {
    print "count is not zero"
}
```

**Else-if chains:**

```rsh
if x > 10 {
    print "large"
} else if x > 5 {
    print "medium"
} else {
    print "small"
}
```

### For Loops

Iterate over a range or array:

```rsh
for variable in iterable {
    # statements
}
```

**With range:**

```rsh
for i in range(5) {
    print i
}
# Output: 0 1 2 3 4
```

**With array:**

```rsh
fruits = ["apple", "banana", "cherry"]
for fruit in fruits {
    print fruit
}
```

### While Loops

Repeat while a condition is true:

```rsh
while condition {
    # statements
}
```

**Example:**

```rsh
i = 0
while i < 5 {
    print i
    i = i + 1
}
# Output: 0 1 2 3 4
```

### break and continue

`break` exits the nearest enclosing loop; `continue` skips to the next
iteration. Both work in `for` and `while` loops:

```rsh
for i in range(100) {
    if i % 2 == 0 {
        continue        # skip even numbers
    }
    if i > 7 {
        break           # stop once past 7
    }
    print i
}
# Output: 1 3 5 7
```

## Functions

Define reusable functions with `fn`. Functions take parameters, may `return` a
value, and support recursion:

```rsh
fn add(a, b) {
    return a + b
}

fn factorial(n) {
    if n <= 1 {
        return 1
    }
    return n * factorial(n - 1)
}

print add(3, 4)         # 7
print factorial(5)      # 120
```

**Scope:** each call gets its own scope. Parameters and variables created inside
a function are local to that call (they don't leak out), while existing outer
variables remain readable. A bare `return` (no value) exits early and yields an
empty value.

```rsh
x = 100
fn double(x) {          # parameter x shadows the global
    return x * 2
}
print double(5)         # 10
print x                 # 100 (unchanged)
```

## Built-in Functions

### range(stop) / range(start, stop)

Returns an array of integers from `start` (default 0) up to but not including `stop`.

**Syntax:** `range(stop)` or `range(start, stop)`

**Arguments:**
- `start`: Optional integer to start at (default 0)
- `stop`: Integer to stop before

**Returns:** Array `[start, start+1, ..., stop-1]`, or `[]` if `stop <= start`.
Errors if the range would exceed 10,000,000 elements.

**Example:**

```rsh
for i in range(5) {
    print i
}
# Output: 0 1 2 3 4

numbers = range(3)
print numbers
# Output: [0, 1, 2]

numbers = range(1, 5)
print numbers
# Output: [1, 2, 3, 4]
```

### append(array, value)

Returns a new array with the value appended.

**Syntax:** `append(array, value)`

**Arguments:**
- `array`: The array to append to
- `value`: The value to append

**Returns:** New array with value at the end

**Note:** Does not modify the original array.

**Example:**

```rsh
items = []string
items = append(items, "first")
items = append(items, "second")
print items
# Output: [first, second]
```

### String and Collection Functions

| Function | Description | Example | Result |
|----------|-------------|---------|--------|
| `len(x)` | Length of a string (runes) or array | `len("hello")` | `5` |
| `split(s, sep)` | Split a string into an array | `split("a,b,c", ",")` | `[a, b, c]` |
| `join(arr, sep)` | Join an array into a string | `join(["a","b"], "-")` | `a-b` |
| `contains(s, sub)` | Substring test, or array membership | `contains("hello", "ell")` | `true` |
| `upper(s)` | Uppercase a string | `upper("hi")` | `HI` |
| `lower(s)` | Lowercase a string | `lower("HI")` | `hi` |
| `trim(s)` | Trim leading/trailing whitespace | `trim("  hi  ")` | `hi` |
| `replace(s, old, new)` | Replace all occurrences | `replace("a-a", "a", "x")` | `x-x` |
| `glob(pattern)` | Match files against a glob pattern | `glob("*.go")` | `[main.go, ...]` |

```rsh
parts = split("alpha,beta,gamma", ",")
print len(parts)            # 3
print join(parts, " | ")    # alpha | beta | gamma
print contains(parts, "beta")   # true (array membership)
print upper("ravenshell")   # RAVENSHELL
```

## External Commands

Any bare word at the start of a statement that is not a built-in is run as an
external program found on `PATH` (or in a directory added with `raven-add
path`). External commands accept flags and participate in pipes and redirection:

```rsh
git status
python --version
print "one two three" | wc -w
ls -la
```

Command names may contain hyphens (`docker-compose up`). Output produced by the
language itself uses the built-in `print`; bare words are for invoking programs.

### Command Sequencing

| Operator | Meaning |
|----------|---------|
| `a ; b` | Run `a`, then `b` |
| `a && b` | Run `b` only if `a` succeeded (exit status 0) |
| `a \|\| b` | Run `b` only if `a` failed (non-zero exit status) |
| `a &` | Run `a` in the background |

```rsh
mkdir build ; cd build
git pull && make
test -f config || print "no config"
sleep 60 &
```

`$?` holds the exit status of the most recent command:

```rsh
git --version
print $?            # 0 on success, non-zero on failure
```

### Globbing

Globbing is explicit and unambiguous via the `glob()` function (so it never
clashes with the `*` multiplication operator). Array results are splatted into
multiple command arguments:

```rsh
for f in glob("*.go") { print f }   # iterate matches
rm glob("*.tmp")                    # remove all matching files
print glob("src/*.go")              # directory patterns work too
```

`glob()` returns an empty array when nothing matches.

### Brace Expansion

A command argument containing a brace group is expanded into multiple arguments
before the command runs. Expansion is textual and happens on literal braces
only — it is not a filesystem lookup, so the results need not exist.

```rsh
mkdir -p s01/{ep1,ep2}   # -> mkdir -p s01/ep1 s01/ep2
touch file{1,2}.txt      # -> touch file1.txt file2.txt
cp report{,.bak}         # -> cp report report.bak  (empty element allowed)
```

Groups combine and nest, and a prefix/suffix is applied to every element:

| Form | Expands to |
|------|-----------|
| `{a,b,c}` | `a b c` |
| `pre{a,b}post` | `preapost prebpost` |
| `{a,b}{c,d}` | `ac ad bc bd` (cross product) |
| `{a,{b,c}}` | `a b c` (nested) |
| `{1..5}` | `1 2 3 4 5` (numeric sequence) |
| `{5..1}` | `5 4 3 2 1` (descending) |
| `{01..03}` | `01 02 03` (zero-padded) |
| `{1..9..2}` | `1 3 5 7 9` (with step) |
| `{a..e}` | `a b c d e` (character sequence) |

A brace group that would not expand is left untouched as literal text: a group
with no comma or `..` sequence (`{a}`, `a{b}c`) and an unbalanced brace (`{a,b`)
stay exactly as written. Braces inside quotes (`"{a,b}"`) and braces coming from
a variable's value are never expanded.

Brace expansion applies in command-argument position. To build a list in an
expression (e.g. a `for` loop), use an array: `for x in [1, 2, 3] { ... }`.

## Arrays

### Creating Arrays

Empty array with type hint:

```rsh
items = []string
numbers = []int
```

Array literal:

```rsh
numbers = [1, 2, 3, 4, 5]
names = ["Alice", "Bob", "Charlie"]
```

### Array Indexing

Access elements using zero-based indexing:

```rsh
numbers = [10, 20, 30]
first = numbers[0]      # 10
second = numbers[1]     # 20
last = numbers[2]       # 30
```

**Error:** Accessing an out-of-bounds index produces an error.

### Iterating Arrays

Use a for loop:

```rsh
items = ["a", "b", "c"]
for item in items {
    print item
}
```

### Building Arrays

Use append in a loop:

```rsh
evens = []int
for i in range(10) {
    if i % 2 == 0 {
        evens = append(evens, i)
    }
}
print evens
# Output: [0, 2, 4, 6, 8]
```

## Path Expressions

Paths can be used directly in commands:

```rsh
ls /home/user
cd ~/Documents
show ./file.txt
rm ../old_file.txt
```

### Path Types

| Type | Example | Description |
|------|---------|-------------|
| Absolute | `/home/user` | Full path from root |
| Relative | `./dir`, `../parent` | Relative to cwd |
| Home | `~`, `~/folder` | Relative to home |

### Path Concatenation

Combine paths using `+`:

```rsh
base = ~
full = base + "/documents/file.txt"
print full
```

## Expressions in Commands

Command arguments can be expressions:

```rsh
folder = "test"
mkdir folder                # Creates directory named "test"

count = 5
print count                 # Prints 5
print count + 10            # Prints 15
```

## Complete Example

```rsh
# example.rsh - Demonstrates RavenShell syntax

# Variables
name = "RavenShell"
version = 1

# Arrays and loops
numbers = []int
for i in range(10) {
    if i % 2 == 0 {
        numbers = append(numbers, i)
    }
}
print "Even numbers:"
print numbers

# Arithmetic
result = 10 + 5 * 2
print "Result: " + result

# Conditionals
count = 5
if count > 3 {
    print "count is greater than 3"
} else {
    print "count is 3 or less"
}

# Shell commands
print "Current directory:"
cwd
print "Files:"
ls

# String concatenation
greeting = "Hello, " + name + "!"
print greeting
```
