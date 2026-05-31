# RavenShell feature showcase

# --- Functions ---
fn add(a, b) {
    return a + b
}

fn factorial(n) {
    if n <= 1 {
        return 1
    }
    return n * factorial(n - 1)
}

print "add(3, 4) = " + add(3, 4)
print "factorial(5) = " + factorial(5)

# --- While loop with break / continue ---
print "odd numbers below 8:"
i = 0
while i < 100 {
    i = i + 1
    if i % 2 == 0 {
        continue
    }
    if i > 7 {
        break
    }
    print i
}

# --- else-if chains ---
fn grade(score) {
    if score >= 90 {
        return "A"
    } else if score >= 80 {
        return "B"
    } else if score >= 70 {
        return "C"
    } else {
        return "F"
    }
}
print "grade(85) = " + grade(85)

# --- String / collection built-ins ---
parts = split("alpha,beta,gamma", ",")
print "split: " + join(parts, " | ")
print "len: " + len(parts)
print "upper: " + upper("ravenshell")
print "contains: " + contains(parts, "beta")
print "replace: " + replace("a-b-a", "a", "x")

# --- Environment variables ---
export PROJECT RavenShell
print "project is " + $PROJECT

# --- Command substitution ---
here = $(cwd)
print "running in: " + here

# --- External commands ---
git --version
