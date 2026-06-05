package parser

import (
	"ravenshell/ast"
	"ravenshell/lexer"
	"testing"
)

// parseArgs parses `rm <rest>` and returns the command's argument expressions.
// `rm` is used because it is a built-in that collects path-style arguments.
func parseArgs(t *testing.T, rest string) []ast.Expression {
	t.Helper()
	l := lexer.NewLexer("rm " + rest)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	cmd, ok := stmt.Expression.(*ast.Command)
	if !ok {
		t.Fatalf("expression is not *ast.Command, got %T", stmt.Expression)
	}
	return cmd.Arguments
}

// TestSinglePathArgument verifies that each input parses into exactly one path
// argument whose value is the input unchanged. This is the regression guard for
// the multi-dot / dotfile path-splitting bug: filenames like archive.tar.gz and
// .env.local must not be chopped at the second dot.
func TestSinglePathArgument(t *testing.T) {
	cases := []string{
		// Simple names with a single extension.
		"file.txt",
		"main.go",
		"report2024.md", // digits allowed inside a letter-led identifier

		// Multi-dot filenames — the classic failure case.
		"archive.tar.gz",
		"bundle.min.js",
		"a.b.c.d.e",
		"v1.2.3.tgz",

		// Dotfiles (leading dot) and dotfiles WITH extensions.
		".gitignore",
		".env",
		".env.local",
		".config.json",

		// Relative, parent, and absolute paths.
		"./main.go",
		"./src/main.go",
		"../parent/file.txt",
		"/usr/local/bin/tool",
		"/etc/nginx/nginx.conf",

		// Directory paths and nested multi-dot segments.
		"foo/bar/baz",
		"dir/.hidden",
		"dir/.hidden.md",
		"src/a.b/c.d.e",
		"build/output.tar.gz",

		// Hyphenated identifiers (joined inside a word by the lexer).
		"my-file.txt",
		"docker-compose.yml",
		"some-dir/another-file.tar.gz",
	}

	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			args := parseArgs(t, in)
			if len(args) != 1 {
				t.Fatalf("%q parsed into %d arguments, want 1: %v", in, len(args), argStrings(args))
			}
			if got := args[0].String(); got != in {
				t.Errorf("path value = %q, want %q", got, in)
			}
		})
	}
}

// TestTildePathArgument checks ~-rooted paths stay a single path argument.
func TestTildePathArgument(t *testing.T) {
	cases := []string{"~/notes.md", "~/dir/archive.tar.gz", "~/.config.json"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			args := parseArgs(t, in)
			if len(args) != 1 {
				t.Fatalf("%q parsed into %d arguments, want 1: %v", in, len(args), argStrings(args))
			}
			if got := args[0].String(); got != in {
				t.Errorf("path value = %q, want %q", got, in)
			}
		})
	}
}

// TestWhitespaceSeparatesPaths verifies that whitespace — and only whitespace —
// splits adjacent path arguments. Each input lists the files and the count of
// arguments expected.
func TestWhitespaceSeparatesPaths(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a.txt b.txt", []string{"a.txt", "b.txt"}},
		{"archive.tar.gz notes.md", []string{"archive.tar.gz", "notes.md"}},
		{".env.local .gitignore", []string{".env.local", ".gitignore"}},
		{"dir/a.b.c dir/d.e", []string{"dir/a.b.c", "dir/d.e"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			args := parseArgs(t, tc.in)
			if len(args) != len(tc.want) {
				t.Fatalf("%q parsed into %d arguments, want %d: %v",
					tc.in, len(args), len(tc.want), argStrings(args))
			}
			for i, want := range tc.want {
				if got := args[i].String(); got != want {
					t.Errorf("arg[%d] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestDigitLeadingFilenameQuotedWorkaround documents a known limitation: a
// filename that STARTS with a digit (e.g. 2024report.md) is tokenized as an
// integer followed by a path, so unquoted it splits into two arguments. The
// supported way to name such a file is to quote it, which parses as one string
// argument. If unquoted digit-leading names are ever fixed, update this test.
func TestDigitLeadingFilenameQuotedWorkaround(t *testing.T) {
	// Quoted form: exactly one argument with the full name.
	args := parseArgs(t, `"2024report.md"`)
	if len(args) != 1 {
		t.Fatalf("quoted digit-leading name parsed into %d arguments, want 1: %v",
			len(args), argStrings(args))
	}
	if got := args[0].String(); got != `"2024report.md"` && got != "2024report.md" {
		t.Errorf("quoted name value = %q, want it to contain 2024report.md", got)
	}

	// Unquoted form: currently splits (documents the limitation). When this
	// stops being true the name was fixed — flip this assertion then.
	unquoted := parseArgs(t, "2024report.md")
	if len(unquoted) == 1 {
		t.Errorf("unquoted digit-leading name now parses as one argument — " +
			"the limitation was fixed; update this test to expect 1 argument")
	}
}

func argStrings(args []ast.Expression) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a.String()
	}
	return out
}
