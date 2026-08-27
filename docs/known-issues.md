# Known Issues

Behaviour that is wrong and not yet fixed, recorded so it is not rediscovered
from scratch each time. Each entry has a repro that can be pasted into
`ravenshell -c`.

Last reviewed: 2026-08-27.

## Indexing does not work in command-argument position

`arr[i]` evaluates correctly in expression context, but as an argument to a
command it is parsed as two separate things: the bare name, and then a stray
index applied to the result. The command runs with the whole array, and the
leftover `[i]` is evaluated against a string.

```rsh
n = [10, 20, 30]

s = n[1]
print s          # 20 -- correct

print n[1]       # prints "10 20 30", then:
                 # error: index operator not supported on string
```

`echo n[1]` fails the same way, so this is the argument path rather than
anything specific to `print`. Parenthesising does not help — `print (n[1])`
prints an empty line and no error, which is a third wrong answer.

Workaround: index into a variable first, then pass the variable.

```rsh
second = n[1]
print second
```

## A failing builtin ignores redirection and aborts the script

A builtin that fails raises a script-level error. That error is reported before
redirection is applied, so `2>/dev/null` does not suppress it, and it ends the
script rather than setting a status and carrying on. An external command that
fails does neither of those things.

```rsh
/bin/ls /nonexistent 2>/dev/null   # suppressed
echo after-external                # reached

ls /nonexistent 2>/dev/null        # NOT suppressed:
                                   # error: ls: stat /nonexistent: ...
echo after-builtin                 # NOT reached
```

The difference is visible without redirection too: `false; echo reached`
prints `reached`, while `ls /nonexistent; echo reached` does not.

Workaround: call the external binary by path (`/bin/ls`) where the failure needs
to be suppressed or survived.
