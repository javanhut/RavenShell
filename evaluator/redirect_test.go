package evaluator

import (
	"os"
	"path/filepath"
	"testing"
)

// Redirections nest with the last one outermost, so applying them in AST order
// would let the leftmost win. bash gives the last redirection of a stream the
// final say; the losing file is still created and truncated.
func TestStackedRedirectionLastWins(t *testing.T) {
	cases := []struct {
		src  string
		want map[string]string
		note string
	}{
		{`print "hi" > a.txt > b.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, "> then >"},
		{`print "hi" > a.txt >> b.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, "> then >>"},
		{`print "hi" >> a.txt > b.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, ">> then >"},
		{`print "hi" > a.txt 2> b.txt`, map[string]string{"a.txt": "hi\n", "b.txt": ""}, "stdout and stderr both apply"},
		{`print "hi" 2> a.txt > b.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, "stderr first, stdout still lands"},
		// An fd above 2 is created but never written to. Treating it as another
		// name for stdout would let it win the last-one-wins race and steal the
		// output away from the real `>` target.
		{`print "hi" > b.txt 3> a.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, "fd 3 does not capture stdout"},
		{`print "hi" > b.txt 5>> a.txt`, map[string]string{"a.txt": "", "b.txt": "hi\n"}, "appending fd 5 does not capture stdout"},
		{`print "hi" 3> a.txt`, map[string]string{"a.txt": ""}, "lone fd 3 creates the file but takes no output"},
	}
	for _, c := range cases {
		t.Run(c.note, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := evalScript(t, dir, c.src); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for name, want := range c.want {
				// Reading also asserts the losing file was created.
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if string(got) != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestStackedInputRedirectionLastWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.txt", "first\n")
	writeFile(t, dir, "two.txt", "second\n")

	out, err := evalScript(t, dir, "print < one.txt < two.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "second\n" {
		t.Errorf("stacked input redirection = %q, want %q", out, "second\n")
	}
}

// `2>&1` copies whatever stdout points at *when it is reached*, so an earlier
// `> file` must already be in effect. The old evaluator applied the outermost
// redirection first and sent stderr to the terminal instead.
func TestDupFollowsEarlierStdoutRedirection(t *testing.T) {
	dir := t.TempDir()
	if _, err := evalScript(t, dir, `/bin/sh -c "echo OUT; echo ERR 1>&2" > a.txt 2>&1`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OUT\nERR\n" {
		t.Errorf("a.txt = %q, want %q", got, "OUT\nERR\n")
	}
}
