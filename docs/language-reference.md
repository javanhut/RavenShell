# RavenShell Language Reference

This document provides a complete reference for the RavenShell scripting language syntax.

## Overview

RavenShell uses a Go-like syntax that combines shell commands with scripting capabilities. Scripts are stored in `.rsh` files and executed line by line.

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

Boolean literals `true` and `false` can be assigned directly:

```rsh
enabled = true
disabled = false
```

Boolean values also result from comparison and logical operations:

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
- `false` is false, `true` is true

### Dictionaries

Key-value collections (also called maps or hashes):

```rsh
# Dictionary literal
person = {"name": "Alice", "age": 30}

# Access values by key
print person["name"]    # Alice
print person["age"]     # 30
```

Keys must be strings. Values can be any type.

## Variables

### Assignment

Use `=` to assign values to variables:

```rsh
x = 5
name = "RavenShell"
numbers = [1, 2, 3]
empty = []string
```

Variable names must start with a letter and can contain letters, numbers, and underscores.

### Using Variables

Reference variables by name:

```rsh
x = 10
y = x + 5       # y is 15
print x         # Prints 10
```

### Environment Variables

Access system environment variables with `$`:

```rsh
print $HOME     # Prints home directory
print $USER     # Prints username
path = $HOME + "/documents"
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

### Logical Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `&&` | Logical AND | `x > 0 && x < 10` |
| `\|\|` | Logical OR | `x < 0 \|\| x > 10` |
| `!` | Logical NOT | `!condition` |

```rsh
if x > 0 && x < 100 {
    print "x is between 0 and 100"
}

if status == "error" || status == "failed" {
    print "something went wrong"
}

if !enabled {
    print "feature is disabled"
}

# Boolean expressions
print (true && false)   # false
print (true || false)   # true
print (!true)           # false
```

### Regex Match Operator

The `=~` operator tests if a string matches a regular expression:

```rsh
if "test@example.com" =~ "[a-z]+@[a-z]+[.][a-z]+" {
    print "valid email pattern"
}

if filename =~ ".*[.]txt$" {
    print "text file"
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

1. `[]` - Array/dictionary indexing
2. `!` - Logical NOT
3. `*`, `/`, `%` - Multiplication, division, modulo
4. `+`, `-` - Addition, subtraction
5. `<`, `>`, `<=`, `>=` - Comparison
6. `==`, `!=`, `=~` - Equality and regex match
7. `&&` - Logical AND
8. `||` - Logical OR
9. `|` - Pipe
10. `>`, `>>`, `<` - Redirection

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

Nested conditionals:

```rsh
if x > 10 {
    print "large"
} else {
    if x > 5 {
        print "medium"
    } else {
        print "small"
    }
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

### Break and Continue

Use `break` to exit a loop early:

```rsh
for i in range(10) {
    if i == 5 {
        break
    }
    print i
}
# Output: 0 1 2 3 4
```

Use `continue` to skip to the next iteration:

```rsh
for i in range(6) {
    if i == 3 {
        continue
    }
    print i
}
# Output: 0 1 2 4 5
```

### Switch/Match Statements

Match a value against multiple cases:

```rsh
switch value {
    case 1: { print "one" }
    case 2: { print "two" }
    case 3: { print "three" }
    default { print "other" }
}
```

The `match` keyword is an alias for `switch`:

```rsh
match status {
    case "ok": { print "success" }
    case "error": { print "failure" }
    default { print "unknown" }
}
```

## User-Defined Functions

Define custom functions using the `fn` keyword (or `func`):

```rsh
fn add(a, b) {
    return a + b
}

result = add(3, 4)
print result    # 7
```

### Function Syntax

```rsh
fn function_name(param1, param2, ...) {
    # function body
    return value    # optional
}
```

### Examples

```rsh
# Function with no return value
fn greet(name) {
    print "Hello, " + name
}
greet("World")

# Function with return value
fn square(x) {
    return x * x
}
print square(5)     # 25

# Function with multiple parameters
fn max(a, b) {
    if a > b {
        return a
    }
    return b
}
print max(10, 20)   # 20
```

## Built-in Functions

### range(n)

Returns an array of integers from 0 to n-1.

**Syntax:** `range(n)`

**Arguments:**
- `n`: Integer specifying the count

**Returns:** Array `[0, 1, 2, ..., n-1]`

**Example:**

```rsh
for i in range(5) {
    print i
}
# Output: 0 1 2 3 4

numbers = range(3)
print numbers
# Output: [0, 1, 2]
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

### len(value)

Returns the length of a string or array.

**Syntax:** `len(value)`

**Example:**

```rsh
print len("hello")      # 5
print len([1, 2, 3])    # 3
```

## String Functions

### upper(string)

Converts a string to uppercase.

**Syntax:** `upper(string)`

**Example:**

```rsh
print upper("hello")    # HELLO
```

### lower(string)

Converts a string to lowercase.

**Syntax:** `lower(string)`

**Example:**

```rsh
print lower("HELLO")    # hello
```

### trim(string)

Removes leading and trailing whitespace.

**Syntax:** `trim(string)`

**Example:**

```rsh
print trim("  hello  ")     # hello
```

### contains(string, substring)

Checks if a string contains a substring.

**Syntax:** `contains(string, substring)`

**Returns:** Boolean

**Example:**

```rsh
print contains("hello world", "world")  # true
print contains("hello", "xyz")          # false
```

### replace(string, old, new)

Replaces all occurrences of a substring.

**Syntax:** `replace(string, old, new)`

**Example:**

```rsh
print replace("hello", "l", "L")    # heLLo
```

### split(string, delimiter)

Splits a string into an array.

**Syntax:** `split(string, delimiter)`

**Returns:** Array of strings

**Example:**

```rsh
parts = split("a,b,c", ",")
print parts     # [a, b, c]
```

## Array Functions

### first(array)

Returns the first element of an array.

**Syntax:** `first(array)`

**Example:**

```rsh
arr = [1, 2, 3, 4, 5]
print first(arr)    # 1
```

### last(array)

Returns the last element of an array.

**Syntax:** `last(array)`

**Example:**

```rsh
arr = [1, 2, 3, 4, 5]
print last(arr)     # 5
```

### join(array, delimiter)

Joins array elements into a string.

**Syntax:** `join(array, delimiter)`

**Example:**

```rsh
print join(["a", "b", "c"], "-")    # a-b-c
```

### slice(array, start, end)

Returns a portion of an array.

**Syntax:** `slice(array, start, end)`

**Arguments:**
- `start`: Starting index (inclusive)
- `end`: Ending index (exclusive)

**Example:**

```rsh
arr = [1, 2, 3, 4, 5]
print slice(arr, 1, 3)  # [2, 3]
```

## Regex Functions

### regex_find(string, pattern)

Finds all matches of a regex pattern.

**Syntax:** `regex_find(string, pattern)`

**Returns:** Array of matches

**Example:**

```rsh
matches = regex_find("The year is 2024 and month is 12", "[0-9]+")
print matches   # [2024, 12]
```

### regex_replace(string, pattern, replacement)

Replaces all regex matches with a string.

**Syntax:** `regex_replace(string, pattern, replacement)`

**Example:**

```rsh
result = regex_replace("hello123world456", "[0-9]+", "#")
print result    # hello#world#
```

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
