package evaluator

import (
	"bytes"
	"maps"
	"slices"
	"strings"
)

// helpTopic documents one part of the RavenScript language for `raven-help`.
// Topics sit beside the built-in commands so the language is discoverable from
// inside the shell: `raven-help syntax`, `raven-help strings`, ...
type helpTopic struct {
	name    string
	aliases []string
	summary string
	body    string // pre-formatted text; every line is indented on output
}

// helpTopics lists the language topics in the order the overview shows them.
var helpTopics = []helpTopic{
	{
		name:    "syntax",
		aliases: []string{"language", "script", "overview"},
		summary: "How RavenScript statements, commands, and blocks fit together.",
		body: `A program is a sequence of statements, one per line or separated by ';'.
A statement is either a command or an expression:

  ls ~/Documents            command: a name and its argument words
  count = len(args)         expression: assignment with a function call
  print "hello $USER"       command whose arguments are evaluated

Blocks use braces and are always whitespace-separated from what precedes
them. A block may span lines; the interactive prompt keeps reading until the
braces balance:

  if count > 3 {
      print "many"
  } else {
      print "few"
  }

Comments start with '#' and run to the end of the line.

Script files use the .rsh extension: 'ravenshell build.rsh debug'. Arguments
after the file name are the 'args' array. 'raven-source file.rsh' evaluates
a file in the current session.

Topics: variables, strings, arrays, operators, control-flow, functions,
commands, expansion. Run 'raven-help <topic>' for one of them.`,
	},
	{
		name:    "variables",
		aliases: []string{"vars", "assignment", "environment"},
		summary: "Assignment, $name references, exported environment, args, and $?.",
		body: `Assign with '=' and no keyword. A variable holds an integer, a string,
a boolean, or an array:

  count = 5
  name = "raven"
  done = count == 5
  items = [1, 2, 3]

Read a variable as '$name', or as a bare name in expression position:

  print $count              5
  total = count + 1         6

In command arguments a bare name is substituted as a word, but it does not
start arithmetic; use '$name' or parentheses when the value should compute:

  print count + 10          5 + 10   (three words)
  print $count + 10         15
  print (count + 10)        15

'export NAME=value' (or 'export NAME value...') sets an environment
variable that '$NAME' can read and external commands inherit. 'env' lists
the environment. 'raven-unset name' removes a variable.

'args' is the array of arguments given to a script. '$?' is the exit
status of the most recent command; lastStatus() is the same as a function.`,
	},
	{
		name:    "strings",
		aliases: []string{"quoting", "interpolation", "quotes"},
		summary: "Quoting rules, escapes, interpolation, and multi-line strings.",
		body: `Double quotes interpolate; single quotes are literal:

  user = "raven"
  print "hi $user"          hi raven
  print "home: ${HOME}"     home: /home/you
  print 'literal $user'     literal $user

Escapes \n \t \\ \" and \' work inside quotes. Triple quotes span lines:

  message = """Build started
  Build complete"""

Command substitution '$(cmd)' is a value, not text, so assign it first
rather than writing it inside a string:

  year = $(date +%Y)
  print "year $year"

'+' joins strings, converting numbers as needed:

  path = $HOME + "/documents"
  label = "count: " + count

Functions: len(s), upper(s), lower(s), trim(s), split(s, sep),
join(arr, sep), contains(s, sub), replace(s, old, new), repeat_str(s, n).`,
	},
	{
		name:    "arrays",
		aliases: []string{"lists", "indexing", "index"},
		summary: "Array literals, indexing, append, len, and iteration.",
		body: `Arrays are ordered and may mix types. An empty array may carry a type hint:

  numbers = [10, 20, 30]
  mixed = ["text", 42]
  items = []
  names = []string

Index from zero, in expressions and in command arguments alike:

  first = numbers[0]        10
  print numbers[1]          20
  grid = [[1, 2], [3, 4]]
  print grid[1][0]          3

append(arr, value) grows the variable in place and also returns the grown
array, so both forms below work:

  append(items, "a")
  items = append(items, "b")

Other functions: len(arr), contains(arr, value), join(arr, sep),
range(n) / range(start, stop), glob(pattern), split(s, sep).

Iterate with for-in; an array argument to a command is splatted into words:

  for n in numbers { print n }
  print numbers             10 20 30`,
	},
	{
		name:    "operators",
		aliases: []string{"arithmetic", "math", "comparison", "expressions"},
		summary: "Integer arithmetic, comparisons, precedence, and the spacing rule.",
		body: `Arithmetic is 64-bit integer: + - * / %  ('/' truncates toward zero).
'*' and '/' and '%' bind tighter than '+' and '-'; parentheses group.

  total = 10 + 5 * 2        20
  half = (10 + 5) / 2       7

Comparisons yield booleans: == != < > <= >=

  if count >= 10 { print "ten or more" }

Put spaces around operators. A '-' glued to a word is a flag (-l, --all)
and a '-' between word characters is part of a name (docker-compose), so
write 'a - b', not 'a-b'.

In command arguments only 'print' and 'output' evaluate spaced arithmetic,
and only when the left side is a number, a $name, an index like n[0], a
function call, or a parenthesised expression. Everything else stays words:

  print 10 - 4              6
  print $count * 2          10
  print (count + 1) * 2     12
  print count * 2           5 * 2   (a bare name is a word)
  echo 10 - 4               10 - 4
  print Done - all good     Done - all good

'+' with a string on either side concatenates. A spaced '*' after a word
or string is not multiplication; it is a glob (see 'raven-help expansion').`,
	},
	{
		name:    "control-flow",
		aliases: []string{"if", "for", "while", "loops", "conditionals", "flow"},
		summary: "if / else if / else, for-in over arrays and range, while, break, continue.",
		body: `Conditionals:

  if x > 10 {
      print "big"
  } else if x > 5 {
      print "medium"
  } else {
      print "small"
  }

Loops:

  for i in range(3) { print i }          0 1 2
  for i in range(1, 4) { print i }       1 2 3
  for item in items { print item }
  for f in glob("*.go") { print f }

  n = 0
  while n < 3 {
      n = n + 1
  }

'break' leaves the innermost loop; 'continue' skips to its next iteration.
Loop bodies share the surrounding scope, so a variable assigned in a loop
is visible after it. Conditions are expressions: a comparison, a boolean
variable, or a function returning one.`,
	},
	{
		name:    "functions",
		aliases: []string{"fn", "return", "builtins", "calls"},
		summary: "Defining functions with fn, calling them, scope, and the built-in functions.",
		body: `Define with 'fn'. Parameters are local; 'return' yields a value:

  fn add(a, b) {
      return a + b
  }
  fn factorial(n) {
      if n <= 1 { return 1 }
      return n * factorial(n - 1)
  }
  print add(3, 4)           7

Call a function with its name glued to '(' anywhere an expression is
allowed, including inside command arguments. At the start of a statement a
space before '(' is also a call; if no such function exists the name runs
as an external command with the values as its arguments:

  total = add(1, 2)
  print upper("hi")
  add (1, 2)
  echo (1 + 2)              runs echo with 3

Each call has its own scope: parameters and new variables do not leak out,
outer variables stay readable. append() on a parameter changes only the
local copy.

Built-in functions:
  len(x)  split(s, sep)  join(arr, sep)  contains(x, v)  upper(s)
  lower(s)  trim(s)  replace(s, old, new)  repeat_str(s, n)  glob(pat)
  range(n) / range(start, stop)  append(arr, v)  exit([status])
  lastStatus()`,
	},
	{
		name:    "commands",
		aliases: []string{"pipes", "redirection", "sequencing", "status", "external"},
		summary: "Running commands, pipes, redirection, && || ;, background jobs, and $?.",
		body: `A statement that starts with a name runs that command with the following
words as arguments. Built-ins (ls, cd, print, ...) and programs on PATH are
written the same way; 'raven-type name' says which one a name resolves to.

  git status --short
  ls ~/Documents

Sequencing and status:

  a ; b                     run a, then b
  a && b                    run b only if a succeeded ($? == 0)
  a || b                    run b only if a failed
  a &                       run a in the background ('jobs', 'kill %1')
  print $?                  status of the most recent command

A built-in that fails at its job (cd to a missing directory, rm of a
missing file) reports on stderr, sets $? to 1, and the program continues,
so '||' and 'if' can react. A misuse of the language itself (len() with no
argument, an index past the end) stops the program with a line:column.

Pipes and redirection:

  ls | grep raven
  cmd > out.txt             stdout to a file (>> appends)
  cmd < in.txt              stdin from a file
  cmd 2> err.txt            stderr to a file
  cmd 2>&1 | less           stderr follows stdout
  cmd &> all.txt            both streams to a file

'$(cmd)' captures a command's output as a value. 'raven-alias name cmd
args...' defines an interactive alias. Arguments containing braces or glob
characters are expanded first (see 'raven-help expansion').`,
	},
	{
		name:    "expansion",
		aliases: []string{"globbing", "glob", "braces", "paths", "wildcards"},
		summary: "Globs (* ? [..]), brace expansion, ~ and path words.",
		body: `An unquoted argument word containing * ? or [ expands to its sorted
matches. Names starting with '.' match only a pattern that starts with '.',
and a pattern that matches nothing is passed through as written:

  ls *.go
  rm *                      everything except dotfiles
  print "*.go"              the literal text *.go

glob(pattern) does the same explicitly and returns an array:

  for f in glob("src/*.go") { print f }

Brace groups expand textually, in argument position only:

  mkdir -p s01/{ep1,ep2}    s01/ep1 s01/ep2
  touch file{1,2}.txt       file1.txt file2.txt
  print {1..5}              1 2 3 4 5
  print {01..03}            01 02 03
  print {1..9..2}           1 3 5 7 9
  print {a..e}              a b c d e
  cp report{,.bak}          report report.bak

Paths are words: /abs/path, ./relative, ../parent, ~ and ~/dir. A leading
'~' expands to the home directory, in arguments and in export values.
Join paths with '+':

  full = $HOME + "/documents/file.txt"`,
	},
}

// findTopic looks up a language topic by name or alias.
func findTopic(name string) (helpTopic, bool) {
	for _, t := range helpTopics {
		if t.name == name || slices.Contains(t.aliases, name) {
			return t, true
		}
	}
	return helpTopic{}, false
}

// HelpTopicSummaries maps every topic name and alias to its one-line summary,
// for tab completion after `raven-help`.
func HelpTopicSummaries() map[string]string {
	m := make(map[string]string, len(helpTopics)*3)
	for _, t := range helpTopics {
		m[t.name] = t.summary
		for _, a := range t.aliases {
			m[a] = t.summary
		}
	}
	return m
}

// HelpSummaries merges the built-in command summaries with the language topic
// summaries: everything `raven-help` accepts, for tab completion.
func HelpSummaries() map[string]string {
	m := BuiltinSummaries()
	maps.Copy(m, HelpTopicSummaries())
	return m
}

// renderTopicDetail builds the text for one language topic.
func renderTopicDetail(t helpTopic, color bool) string {
	var out bytes.Buffer
	out.WriteString(bold(t.name, color) + " — " + t.summary + "\n")
	if len(t.aliases) > 0 {
		out.WriteString(dim("  also: "+strings.Join(t.aliases, ", "), color) + "\n")
	}
	out.WriteString("\n")
	for line := range strings.SplitSeq(t.body, "\n") {
		if line == "" {
			out.WriteString("\n")
			continue
		}
		out.WriteString("  " + line + "\n")
	}
	return out.String()
}

// topicNameList returns the topic names in display order.
func topicNameList() []string {
	names := make([]string, 0, len(helpTopics))
	for _, t := range helpTopics {
		names = append(names, t.name)
	}
	return names
}
