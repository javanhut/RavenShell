# Test all new language features

# Phase 1: Division
print "=== Phase 1: Division ==="
x = 10 / 2
print x

# Phase 2: Logical operators
print "=== Phase 2: Logical Operators ==="
print (true && false)
print (true || false)
print (!true)
print (!false)

# Phase 3: Break/Continue
print "=== Phase 3: Break/Continue ==="
print "Break test:"
for i in range(10) {
    if i == 5 { break }
    print i
}
print "Continue test (skip 3):"
for i in range(6) {
    if i == 3 { continue }
    print i
}

# Phase 4: User-defined Functions
print "=== Phase 4: Functions ==="
fn add(a, b) {
    return a + b
}
fn greet(name) {
    print "Hello, " + name
}
print add(3, 4)
greet("World")

# Phase 5: String Functions
print "=== Phase 5: String Functions ==="
print len("hello")
print upper("hello")
print lower("HELLO")
print trim("  trimmed  ")
print contains("hello world", "world")
print replace("hello", "l", "L")
parts = split("a,b,c", ",")
print parts

# Phase 6: Array Functions
print "=== Phase 6: Array Functions ==="
arr = [1, 2, 3, 4, 5]
print first(arr)
print last(arr)
print join(["a", "b", "c"], "-")
print slice(arr, 1, 3)

# Phase 7: Dictionary
print "=== Phase 7: Dictionary ==="
d = {"name": "Alice", "age": 30}
print d["name"]
print d["age"]

# Phase 8: Switch/Match
print "=== Phase 8: Switch/Match ==="
x = 2
switch x {
    case 1: { print "one" }
    case 2: { print "two" }
    case 3: { print "three" }
    default { print "other" }
}

# Phase 9: Regex
print "=== Phase 9: Regex ==="
if ("test@example.com" =~ "[a-z]+@[a-z]+[.][a-z]+") {
    print "valid email pattern"
}
matches = regex_find("The year is 2024 and month is 12", "[0-9]+")
print matches
replaced = regex_replace("hello123world456", "[0-9]+", "#")
print replaced

print "=== All tests completed ==="
