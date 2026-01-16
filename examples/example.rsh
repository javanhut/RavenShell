# Raven Shell Script Example (.rsh)
# This file demonstrates the Raven Shell scripting syntax

# Variable assignment with empty array
x = []string

# For loop with range - collects even numbers
for i in range(10) {
    if i % 2 == 0 {
        x = append(x, i)
    }
    print x
}

print "This is a raven shell script"

# String concatenation with tilde expansion and variable
print ~ + "RavenShell"

# More examples:

# Simple variable assignment
name = "RavenShell"
version = 1

# Arithmetic operations
result = 10 + 5 * 2
print result

# Array operations
numbers = [1, 2, 3, 4, 5]
first = numbers[0]
print first

# Conditional with else
count = 5
if count > 3 {
    print "count is greater than 3"
} else {
    print "count is 3 or less"
}

# Built-in shell commands
ls
cwd
whoami
