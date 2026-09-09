# Known Issues

Behaviour that is wrong and not yet fixed, recorded so it is not rediscovered
from scratch each time. Each entry has a repro that can be pasted into
`ravenshell -c`.

Last reviewed: 2026-09-09.

No open issues.

Fixed since the last review, kept here so the repro stays paired with the
behaviour it guards (each has a test in `evaluator/known_issues_test.go`):

- **Indexing in command-argument position.** `print n[1]` printed the whole
  array and then failed; it now prints the element. `print $n[1]`, nested
  `m[0][1]`, and `print (n[1])` work too.
- **A failing builtin aborted the script and ignored redirection.** A builtin
  that fails at its job (`ls /nonexistent`, `cd /missing`, `rm /missing`) now
  reports on the shell's stderr, so `2>/dev/null` silences it, sets `$?` to 1,
  and the program carries on; `||`, `&&`, and `if` can react to it.
- **`print (expr)` printed an empty line.** A parenthesised expression is now
  one argument evaluated as a value: `print (2 + 3)` prints 5.
