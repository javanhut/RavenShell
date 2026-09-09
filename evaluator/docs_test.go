package evaluator

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ravenshell/lexer"
	"ravenshell/parser"
)

// The docs are executable. Every ```rsh block that states an expectation is
// run, statement by statement, in a fresh evaluator and a temporary directory,
// and its output is compared with what the prose promises. A block with no
// expectation is not run, so examples that only show a command shape cost
// nothing. See docs/contributing.md, "Executable examples".
//
// Expectations, all compared with whitespace collapsed:
//
//	print x            # 5              the statement's output is "5"
//	print 10 / 0       # error: text    the statement fails; text is in the error
//	print n[1]         # 20 -- prose    text after " -- " is commentary
//	print $HOME        # e.g. /home/you illustrative only, not checked
//	# Output: e.g. /home/you            likewise for a marker
//	# Output: 0 1 2                     everything printed since the previous
//	                                    marker (or the block start) is "0 1 2"
//	# Output:                           the same, listed one line per comment
//	# 0
//	# 1
//	# ravenshell build.rsh debug app    the block runs with args = [debug, app]
//
// Only lines starting with print, output, or echo carry a trailing
// expectation; comments on other statements are documentation.
func TestDocsAreExecutable(t *testing.T) {
	for _, f := range []string{"language-reference.md", "examples.md", "user-guide.md", "commands.md"} {
		t.Run(f, func(t *testing.T) { checkDocExamples(t, filepath.Join("..", "docs", f)) })
	}
}

type docBlock struct {
	file    string
	docLine int // line in the file of the block's first source line
	lines   []string
}

type docExpect struct {
	line int // line within the block, 1-based
	kind string
	want string
}

func checkDocExamples(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	blocks := extractRshBlocks(path, string(data))
	ran := 0
	for _, b := range blocks {
		if runDocBlock(t, b) {
			ran++
		}
	}
	if ran == 0 {
		t.Fatalf("%s: no example block carries an expectation", path)
	}
	t.Logf("%s: %d of %d example blocks checked", path, ran, len(blocks))
}

func extractRshBlocks(file, text string) []docBlock {
	var blocks []docBlock
	var cur *docBlock
	for i, line := range strings.Split(text, "\n") {
		switch {
		case cur == nil && strings.HasPrefix(strings.TrimSpace(line), "```rsh"):
			cur = &docBlock{file: file, docLine: i + 2}
		case cur != nil && strings.HasPrefix(strings.TrimSpace(line), "```"):
			blocks = append(blocks, *cur)
			cur = nil
		case cur != nil:
			cur.lines = append(cur.lines, line)
		}
	}
	return blocks
}

// commentStart returns the index of a '#' that begins a comment (outside
// quotes), or -1.
func commentStart(line string) int {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return i
			}
		}
	}
	return -1
}

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

// docExpectations reads the expectations and the args directive out of a block.
func docExpectations(b docBlock) (exps []docExpect, args []string, hasArgs bool) {
	for i := 0; i < len(b.lines); i++ {
		line := b.lines[i]
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "# Output:"); ok {
			at := i + 1
			// A bare "# Output:" lists the output on the comment lines below it.
			if strings.TrimSpace(rest) == "" {
				var lines []string
				for i+1 < len(b.lines) {
					next := strings.TrimSpace(b.lines[i+1])
					if next != "#" && !strings.HasPrefix(next, "# ") {
						break
					}
					lines = append(lines, strings.TrimPrefix(next, "#"))
					i++
				}
				rest = strings.Join(lines, "\n")
			}
			if strings.HasPrefix(strings.TrimSpace(rest), "e.g.") {
				continue // illustrative, not checked
			}
			exps = append(exps, docExpect{line: at, kind: "output", want: normalize(rest)})
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "# ravenshell "); ok {
			f := strings.Fields(rest)
			if len(f) > 0 {
				args, hasArgs = f[1:], true
			}
			continue
		}
		first, _, _ := strings.Cut(trimmed, " ")
		if first != "print" && first != "output" && first != "echo" {
			continue
		}
		at := commentStart(line)
		if at < 0 {
			continue
		}
		text := strings.TrimSpace(line[at+1:])
		if strings.HasPrefix(text, "e.g.") {
			continue
		}
		text, _, _ = strings.Cut(text, " -- ")
		if rest, ok := strings.CutPrefix(text, "error:"); ok {
			exps = append(exps, docExpect{line: i + 1, kind: "error", want: strings.TrimSpace(rest)})
		} else {
			exps = append(exps, docExpect{line: i + 1, kind: "stmt", want: normalize(text)})
		}
	}
	return exps, args, hasArgs
}

// runDocBlock runs one block and reports whether it carried expectations.
func runDocBlock(t *testing.T, b docBlock) bool {
	t.Helper()
	exps, args, hasArgs := docExpectations(b)
	if len(exps) == 0 {
		return false
	}
	where := func(blockLine int) string {
		return b.file + ":" + itoa(b.docLine+blockLine-1)
	}

	src := strings.Join(b.lines, "\n") + "\n"
	p := parser.New(lexer.NewLexer(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("%s: example does not parse: %v", where(1), errs)
		return true
	}

	var e *Evaluator
	if hasArgs {
		e = NewWithArgs(args)
	} else {
		e = New()
	}
	e.cwd = t.TempDir()
	var out bytes.Buffer
	e.stdout, e.stderr = &out, &bytes.Buffer{}
	// The shell's cd moves the process too; put it back after the block.
	if wd, err := os.Getwd(); err == nil {
		defer os.Chdir(wd)
	}

	byLine := map[int]docExpect{}
	var markers []docExpect
	for _, x := range exps {
		if x.kind == "output" {
			markers = append(markers, x)
		} else {
			byLine[x.line] = x
		}
	}

	var segment strings.Builder
	stmts := program.Statements
	for i, stmt := range stmts {
		start := statementToken(stmt).Line
		next := len(b.lines) + 1
		if i+1 < len(stmts) {
			next = statementToken(stmts[i+1]).Line
		}

		out.Reset()
		err := e.evalStatement(stmt)
		got := out.String()
		segment.WriteString(got)

		if x, ok := byLine[start]; ok {
			switch x.kind {
			case "stmt":
				if err != nil {
					t.Errorf("%s: %q failed: %v", where(start), strings.TrimSpace(b.lines[start-1]), err)
				} else if normalize(got) != x.want {
					t.Errorf("%s: %q printed %q, doc says %q", where(start), strings.TrimSpace(b.lines[start-1]), normalize(got), x.want)
				}
			case "error":
				if err == nil || !strings.Contains(err.Error(), x.want) {
					t.Errorf("%s: %q: error = %v, doc says error containing %q", where(start), strings.TrimSpace(b.lines[start-1]), err, x.want)
				}
			}
		} else if err != nil {
			t.Errorf("%s: %q failed: %v", where(start), strings.TrimSpace(b.lines[start-1]), err)
		}

		for _, m := range markers {
			if m.line > start && m.line < next {
				if got := normalize(segment.String()); got != m.want {
					t.Errorf("%s: output so far is %q, doc says %q", where(m.line), got, m.want)
				}
				segment.Reset()
			}
		}
	}
	return true
}

func itoa(n int) string { return strconv.Itoa(n) }
