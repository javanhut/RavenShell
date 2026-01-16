# Test script for new features

# Test variable assignment
print "Testing variable assignment..."
x = 5
print x

# Test array
print "Testing arrays..."
arr = []string
arr = append(arr, "hello")
arr = append(arr, "world")
print arr

# Test for loop with range
print "Testing for loop..."
for i in range(5) {
    print i
}

# Test if statement
print "Testing conditionals..."
n = 10
if n > 5 {
    print "n is greater than 5"
}

# Test arithmetic
print "Testing arithmetic..."
result = 3 + 4 * 2
print result

# Test modulo (for even/odd check)
print "Testing modulo..."
for i in range(6) {
    if i % 2 == 0 {
        print i
    }
}

print "All tests completed!"
