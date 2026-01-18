# RavenShell Examples

This document provides practical examples demonstrating RavenShell features.

## Basic Usage

### Hello World

```rsh
print "Hello, World!"
```

### Working with Directories

```rsh
# Show where you are
cwd

# List contents
ls

# Create a new directory
mkdir test_dir

# Move into it
cd test_dir

# Create some files
mkfile hello.txt
mkfile notes.txt

# List to verify
ls

# Go back
cd ..

# Clean up
rm test_dir
```

### System Information

```rsh
# Display current user
whoami

# Display home directory
~

# Display current directory
cwd

# Show environment variable
print $HOME
print $USER
```

## Variables and Data Types

### Integer Variables

```rsh
x = 10
y = 5
sum = x + y
print sum           # 15

difference = x - y
print difference    # 5

product = x * y
print product       # 50

quotient = x / y
print quotient      # 2

remainder = x % 3
print remainder     # 1
```

### String Variables

```rsh
first_name = "Raven"
last_name = "Shell"

# Concatenation
full_name = first_name + " " + last_name
print full_name     # Raven Shell

# With numbers
version = 1
info = "Version: " + version
print info          # Version: 1
```

### Arrays

```rsh
# Create an array
numbers = [1, 2, 3, 4, 5]
print numbers       # [1, 2, 3, 4, 5]

# Access elements
first = numbers[0]
print first         # 1

third = numbers[2]
print third         # 3

# Create empty array and build it
items = []string
items = append(items, "apple")
items = append(items, "banana")
items = append(items, "cherry")
print items         # [apple, banana, cherry]
```

## Control Flow

### Simple Conditionals

```rsh
age = 25

if age >= 18 {
    print "Adult"
} else {
    print "Minor"
}
```

### Nested Conditionals

```rsh
score = 85

if score >= 90 {
    print "Grade: A"
} else {
    if score >= 80 {
        print "Grade: B"
    } else {
        if score >= 70 {
            print "Grade: C"
        } else {
            print "Grade: F"
        }
    }
}
```

### Counting Loop

```rsh
# Count from 0 to 4
for i in range(5) {
    print i
}
# Output:
# 0
# 1
# 2
# 3
# 4
```

### Iterating Over Arrays

```rsh
fruits = ["apple", "banana", "cherry", "date"]

for fruit in fruits {
    print "I like " + fruit
}
# Output:
# I like apple
# I like banana
# I like cherry
# I like date
```

### Filtering with Loops

```rsh
# Collect even numbers from 0-9
evens = []int

for i in range(10) {
    if i % 2 == 0 {
        evens = append(evens, i)
    }
}

print evens
# Output: [0, 2, 4, 6, 8]
```

### Collecting Odd Numbers

```rsh
odds = []int

for i in range(10) {
    if i % 2 != 0 {
        odds = append(odds, i)
    }
}

print odds
# Output: [1, 3, 5, 7, 9]
```

## Pipes and Redirection

### Basic Pipe

```rsh
# Pipe ls output to print
ls | print
```

### Output Redirection

```rsh
# Save directory listing to file
ls > listing.txt

# View the file
show listing.txt

# Append more content
print "---" >> listing.txt
cwd >> listing.txt

# View updated file
show listing.txt
```

### Creating Log Files

```rsh
# Start fresh log
print "Log started" > log.txt

# Add entries
print "Entry 1" >> log.txt
print "Entry 2" >> log.txt
print "Entry 3" >> log.txt

# View log
show log.txt
```

## Practical Scripts

### FizzBuzz

Classic programming exercise:

```rsh
# fizzbuzz.rsh

for i in range(20) {
    if i % 15 == 0 {
        print "FizzBuzz"
    } else {
        if i % 3 == 0 {
            print "Fizz"
        } else {
            if i % 5 == 0 {
                print "Buzz"
            } else {
                print i
            }
        }
    }
}
```

**With logical operators:**

```rsh
# fizzbuzz_v2.rsh - using logical operators

for i in range(20) {
    divisible_by_3 = (i % 3 == 0)
    divisible_by_5 = (i % 5 == 0)

    if divisible_by_3 && divisible_by_5 {
        print "FizzBuzz"
    } else {
        if divisible_by_3 {
            print "Fizz"
        } else {
            if divisible_by_5 {
                print "Buzz"
            } else {
                print i
            }
        }
    }
}
```

### Sum of Numbers

```rsh
# Calculate sum of 1 to 100
total = 0

for i in range(101) {
    total = total + i
}

print "Sum of 0-100: " + total
# Output: Sum of 0-100: 5050
```

### Squares Table

```rsh
# Generate squares of numbers
print "Number | Square"
print "-------|-------"

for i in range(10) {
    square = i * i
    print i + "      | " + square
}
```

### Directory Backup List

```rsh
# backup.rsh - Create a backup manifest

print "Creating backup list..."

# Get current directory
cwd

# Save file listing
ls > backup_manifest.txt

# Add timestamp (manual)
print "---" >> backup_manifest.txt
print "Backup created" >> backup_manifest.txt

print "Done! See backup_manifest.txt"
```

### File Organizer Script

```rsh
# organizer.rsh - Create organized directory structure

print "Setting up project directories..."

mkdir docs
mkdir src
mkdir tests
mkdir data

print "Creating placeholder files..."

mkfile docs/README.md
mkfile src/main.rsh
mkfile tests/test.rsh
mkfile data/sample.txt

print "Project structure created:"
ls
```

### Number Classifier

```rsh
# classifier.rsh - Classify numbers

numbers = [12, 7, 25, 3, 18, 42, 9, 15]

small = []int
medium = []int
large = []int

for n in numbers {
    if n < 10 {
        small = append(small, n)
    } else {
        if n < 20 {
            medium = append(medium, n)
        } else {
            large = append(large, n)
        }
    }
}

print "Small (< 10):"
print small

print "Medium (10-19):"
print medium

print "Large (>= 20):"
print large
```

## User-Defined Functions

### Basic Function

```rsh
# Define a function to calculate factorial
fn factorial(n) {
    if n <= 1 {
        return 1
    }
    return n * factorial(n - 1)
}

print factorial(5)    # 120
print factorial(10)   # 3628800
```

### Utility Functions

```rsh
# Helper functions for common tasks
fn is_even(n) {
    return n % 2 == 0
}

fn is_positive(n) {
    return n > 0
}

fn clamp(value, min, max) {
    if value < min {
        return min
    }
    if value > max {
        return max
    }
    return value
}

print is_even(4)          # true
print is_positive(-5)     # false
print clamp(15, 0, 10)    # 10
```

## Logical Operators

### Boolean Expressions

```rsh
# Using logical AND, OR, NOT
x = 5
y = 10

if x > 0 && y > 0 {
    print "both positive"
}

if x < 0 || y < 0 {
    print "at least one negative"
}

enabled = true
if !enabled {
    print "disabled"
}
```

### Complex Conditions

```rsh
age = 25
has_license = true
has_car = false

if age >= 18 && has_license && has_car {
    print "can drive own car"
} else {
    if age >= 18 && has_license {
        print "can drive, but needs a car"
    } else {
        print "cannot drive"
    }
}
```

## Break and Continue

### Early Exit with Break

```rsh
# Find first number divisible by 7
for i in range(100) {
    if i > 0 && i % 7 == 0 {
        print "First number divisible by 7: " + i
        break
    }
}
```

### Skip Iterations with Continue

```rsh
# Print only odd numbers
for i in range(10) {
    if i % 2 == 0 {
        continue
    }
    print i
}
# Output: 1 3 5 7 9
```

## Switch/Match Statements

### Basic Switch

```rsh
day = 3
switch day {
    case 1: { print "Monday" }
    case 2: { print "Tuesday" }
    case 3: { print "Wednesday" }
    case 4: { print "Thursday" }
    case 5: { print "Friday" }
    default { print "Weekend" }
}
```

### HTTP Status Codes

```rsh
fn status_message(code) {
    switch code {
        case 200: { return "OK" }
        case 404: { return "Not Found" }
        case 500: { return "Internal Server Error" }
        default { return "Unknown Status" }
    }
}

print status_message(200)   # OK
print status_message(404)   # Not Found
print status_message(418)   # Unknown Status
```

## String Manipulation

### String Functions

```rsh
text = "  Hello, World!  "

# Transform case
print upper("hello")        # HELLO
print lower("HELLO")        # hello

# Trim whitespace
print trim(text)            # Hello, World!

# Check contents
print contains("hello world", "world")  # true

# Replace text
print replace("banana", "a", "o")       # bonono

# Split into array
csv = "apple,banana,cherry"
fruits = split(csv, ",")
print fruits                # [apple, banana, cherry]

# Get length
print len("hello")          # 5
```

### Building Strings

```rsh
words = ["Hello", "World", "from", "RavenShell"]
sentence = join(words, " ")
print sentence      # Hello World from RavenShell
```

## Array Functions

### Array Manipulation

```rsh
numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

# Get first and last elements
print first(numbers)        # 1
print last(numbers)         # 10

# Get a slice
middle = slice(numbers, 3, 7)
print middle                # [4, 5, 6, 7]

# Join into string
digits = ["1", "2", "3"]
print join(digits, "-")     # 1-2-3
```

## Dictionaries

### Basic Dictionary Usage

```rsh
# Create a dictionary
person = {"name": "Alice", "age": 30, "city": "Boston"}

# Access values
print person["name"]    # Alice
print person["age"]     # 30
```

### Configuration Dictionary

```rsh
config = {
    "debug": "true",
    "max_retries": "3",
    "timeout": "30"
}

print "Debug mode: " + config["debug"]
print "Timeout: " + config["timeout"] + " seconds"
```

## Regular Expressions

### Pattern Matching

```rsh
# Validate email pattern
email = "user@example.com"
if email =~ "[a-z]+@[a-z]+[.][a-z]+" {
    print "Valid email format"
}

# Check file extension
filename = "document.pdf"
if filename =~ ".*[.]pdf$" {
    print "PDF file detected"
}
```

### Finding and Replacing

```rsh
# Find all numbers in text
text = "Order #123 shipped on 2024-01-15, tracking: 456789"
numbers = regex_find(text, "[0-9]+")
print numbers   # [123, 2024, 01, 15, 456789]

# Redact sensitive data
message = "Call me at 555-1234 or 555-5678"
redacted = regex_replace(message, "[0-9]{3}-[0-9]{4}", "XXX-XXXX")
print redacted  # Call me at XXX-XXXX or XXX-XXXX
```

### Log Parsing

```rsh
# Extract timestamps from log entries
log = "2024-01-15 10:30:45 INFO User logged in"
timestamps = regex_find(log, "[0-9]{2}:[0-9]{2}:[0-9]{2}")
print timestamps    # [10:30:45]
```

## Configuration Examples

### Sample .ravenrc

```rsh
# ~/.ravenrc - RavenShell startup configuration

# Welcome message
print "Welcome to RavenShell!"

# Show current location
print "You are in:"
cwd

# Define helpful variables
projects = "~/projects"
downloads = "~/Downloads"

print "Ready!"
```

### Project Setup Script

```rsh
# setup.rsh - Initialize a new project

# Create project structure
mkdir src
mkdir tests
mkdir docs
mkdir build

# Create initial files
mkfile src/main.rsh
mkfile tests/test.rsh
mkfile docs/README.md
mkfile .gitignore

# Verify structure
print "Project structure:"
ls

print "Setup complete!"
```

## Tips and Tricks

### Path Building

```rsh
# Build paths dynamically
base = ~
project = "myproject"
full_path = base + "/" + project

print full_path
cd full_path
```

### Conditional File Operations

```rsh
# Process numbers and save results
results = []int

for i in range(20) {
    if i % 3 == 0 {
        results = append(results, i)
    }
}

# Save to file
print results > multiples_of_3.txt
```

### Array Processing

```rsh
# Double each number in array
original = [1, 2, 3, 4, 5]
doubled = []int

for n in original {
    doubled = append(doubled, n * 2)
}

print "Original:"
print original
print "Doubled:"
print doubled
```

### Running Multiple Commands

```rsh
# Setup and report
mkdir test_folder
cd test_folder
mkfile file1.txt
mkfile file2.txt
mkfile file3.txt
print "Files created:"
ls
cd ..
```
