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

		// URLs — the ':' must glue onto the surrounding word so the whole URL
		// stays a single argument (regression: ':' used to lex as ILLEGAL).
		"https://github.com/javanhut/RavenShell.git",
		"http://example.com",
		"https://example.com:8080/path",

		// scp-style remotes — the '@' must glue too (regression: '@' used to lex
		// as ILLEGAL).
		"git@github.com:javanhut/RavenShell.git",
		"ssh://git@host.example.com/repo.git",
		"user@host:8080/path",

		// Path segments that begin with '-' (the lexer reads them as FLAG tokens;
		// glued after a '/' they are ordinary directory names).
		"/tmp/cache-1000/-home-user/x",
		"dir/-seg/file.txt",
		"./-out/log.txt",
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

// TestDigitLeadingFilename verifies that a filename starting with a digit
// (e.g. 2024report.md) stays a single argument whether quoted or not. Word
// coalescing glues the leading integer token to the following path tokens, so
// the old "must quote digit-leading names" limitation no longer applies.
func TestDigitLeadingFilename(t *testing.T) {
	// Quoted form: exactly one argument with the full name.
	args := parseArgs(t, `"2024report.md"`)
	if len(args) != 1 {
		t.Fatalf("quoted digit-leading name parsed into %d arguments, want 1: %v",
			len(args), argStrings(args))
	}
	if got := args[0].String(); got != `"2024report.md"` && got != "2024report.md" {
		t.Errorf("quoted name value = %q, want it to contain 2024report.md", got)
	}

	// Unquoted form: also exactly one argument now.
	unquoted := parseArgs(t, "2024report.md")
	if len(unquoted) != 1 {
		t.Fatalf("unquoted digit-leading name parsed into %d arguments, want 1: %v",
			len(unquoted), argStrings(unquoted))
	}
	if got := unquoted[0].String(); got != "2024report.md" {
		t.Errorf("unquoted name value = %q, want 2024report.md", got)
	}
}

// TestColonAndAtWords is the regression guard for the bug where ':' (and '@')
// lexed as an ILLEGAL token, so words like image:tag or a docker reference like
// ravenlinux-build-minimal:latest aborted parsing with "no prefix parse
// function for ILLEGAL found". Each input must stay a single argument whose
// value is unchanged.
func TestColonAndAtWords(t *testing.T) {
	cases := []string{
		// Image references / tags.
		"image:tag",
		"nginx:latest",
		"ravenlinux-build-minimal:latest", // the originally reported case
		// Host:port (the host segment starts with a letter).
		"localhost:8080",
		"db.local:5432", // dotted host with a colon port
		// URLs (scheme://host/path joins through the path machinery).
		"http://example.com",
		"https://example.com/a/b",
		// scp-style and at-references (the word before '@' anchors the path).
		"git@github.com:user/repo.git",
		"user@host",
		// Volume-mount style with a colon between paths.
		"/host:/container",
		// NOTE: ':' and '@' are their own tokens, so a path only folds them in
		// when it begins with a word, '.', or '/'. Forms that begin with a digit
		// ("8080:80", a bare IP "127.0.0.1:5432") or with '@'/':' ("@scope/pkg")
		// split into separate tokens and must be quoted — see
		// TestDigitLeadingFilenameQuotedWorkaround.
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			args := parseArgs(t, in)
			if len(args) != 1 {
				t.Fatalf("%q parsed into %d arguments, want 1: %v", in, len(args), argStrings(args))
			}
			if got := args[0].String(); got != in {
				t.Errorf("arg value = %q, want %q", got, in)
			}
		})
	}
}

// TestSymbolLeadingWords is the regression guard for the bug where an argument
// led by (or containing) an operator character — chmod's +x, glued symbolic
// modes like g+w, date's +%Y-%m-%d, tail's +10 line offset, globs like *.txt —
// was misparsed as arithmetic and collapsed (e.g. `chmod +x` became the bogus
// command word "chmodx"). Each must now stay a single argument whose printed
// value is the source text unchanged.
func TestSymbolLeadingWords(t *testing.T) {
	cases := []string{
		"+x",
		"+rwx",
		"g+w",
		"u+x",
		"a-x",
		"+%Y-%m-%d",
		"+10",
		"*.txt",
		"-",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			args := parseArgs(t, in)
			if len(args) != 1 {
				t.Fatalf("%q parsed into %d arguments, want 1: %v", in, len(args), argStrings(args))
			}
			// Compare the underlying literal value rather than String(), since a
			// standalone flag (+x) prints quoted as a StringLiteral while paths
			// and identifiers print bare.
			if got := argText(args[0]); got != in {
				t.Errorf("arg value = %q, want %q", got, in)
			}
		})
	}
}

// TestGluedEqualsWord is the regression guard for the bug where a glued
// KEY=value argument ended the command and started a new statement, so
// `printf x a FOO=bar b` ran "b" as its own command. RavenShell assigns with
// the spaced form (x = 5), so a glued '=' is ordinary word text.
func TestGluedEqualsWord(t *testing.T) {
	cases := []string{
		"FOO=bar",
		"CGO_ENABLED=0",
		"FOO=",  // empty value: used to abort with "no prefix parse function for EOF"
		"a=b=c", // second '=': used to abort with "no prefix parse function for ASSIGN"
		"FOO=a/b.txt",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			args := parseArgs(t, in)
			if len(args) != 1 {
				t.Fatalf("%q parsed into %d arguments, want 1: %v", in, len(args), argStrings(args))
			}
			if got := argText(args[0]); got != in {
				t.Errorf("arg value = %q, want %q", got, in)
			}
		})
	}
}

// TestGluedEqualsDoesNotSplitCommand is the reported case: `printf x a FOO=bar b`
// must stay one command with four arguments instead of ending at FOO=bar and
// running "b" as a command of its own.
func TestGluedEqualsDoesNotSplitCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"x a FOO=bar b", []string{"x", "a", "FOO=bar", "b"}},
		{"FOO=bar", []string{"FOO=bar"}},
		{"a FOO=bar", []string{"a", "FOO=bar"}},
		{"FOO=bar BAZ=qux", []string{"FOO=bar", "BAZ=qux"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			args := parseArgs(t, tc.in)
			if len(args) != len(tc.want) {
				t.Fatalf("%q parsed into %d arguments, want %d: %v",
					tc.in, len(args), len(tc.want), argStrings(args))
			}
			for i, want := range tc.want {
				if got := argText(args[i]); got != want {
					t.Errorf("arg[%d] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestDashSegmentRedirectionTarget verifies that a path segment beginning with
// '-' glued straight after a '/' stays part of the redirect target. The lexer
// hands such a segment over as one FLAG token carrying the whole remainder of
// the path, so without this the target was truncated at the last slash and the
// evaluator tried to open a directory.
func TestDashSegmentRedirectionTarget(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ls > /tmp/cache-1000/-home-user/x", "/tmp/cache-1000/-home-user/x"},
		{"ls >> ./-out/log.txt", "./-out/log.txt"},
		{"print < dir/-seg/input.txt", "dir/-seg/input.txt"},
		{"ls > ~/-cache/x", "~/-cache/x"},
		{"ls 2> /var/-log/err.txt", "/var/-log/err.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			l := lexer.NewLexer(tc.in)
			p := New(l)
			program := p.ParseProgram()
			checkParserErrors(t, p)
			stmt := program.Statements[0].(*ast.ExpressionStatement)
			redir, ok := stmt.Expression.(*ast.RedirectionExpression)
			if !ok {
				t.Fatalf("stmt.Expression is not ast.RedirectionExpression. got=%T", stmt.Expression)
			}
			if got := argText(redir.Target); got != tc.want {
				t.Errorf("target = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFlagStaysFlagBesidePath is the other half of the dash-segment rule: a '-'
// word separated from its neighbour by whitespace is still a flag, never path
// text, no matter what sits next to it.
func TestFlagStaysFlagBesidePath(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"-la /tmp/-a/b", []string{"-la", "/tmp/-a/b"}},
		{"--color=auto x", []string{"--color=auto", "x"}},
		{"/tmp/dir -x", []string{"/tmp/dir", "-x"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			args := parseArgs(t, tc.in)
			if len(args) != len(tc.want) {
				t.Fatalf("%q parsed into %d arguments, want %d: %v",
					tc.in, len(args), len(tc.want), argStrings(args))
			}
			for i, want := range tc.want {
				if got := argText(args[i]); got != want {
					t.Errorf("arg[%d] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// argText returns the literal text an argument node carries, independent of how
// its String() renders it.
func argText(exp ast.Expression) string {
	switch a := exp.(type) {
	case *ast.StringLiteral:
		return a.Value
	case *ast.PathExpression:
		return a.Value
	case *ast.Identifier:
		return a.Value
	default:
		return exp.String()
	}
}

func argStrings(args []ast.Expression) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a.String()
	}
	return out
}
