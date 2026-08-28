package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBraceExpand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"s01/{ep1,ep2}", []string{"s01/ep1", "s01/ep2"}},  // the reported case
		{"{a,b,c}", []string{"a", "b", "c"}},               // bare list
		{"pre{a,b}post", []string{"preapost", "prebpost"}}, // prefix + suffix
		{"file{1,2}.txt", []string{"file1.txt", "file2.txt"}},
		{"{a,b}{c,d}", []string{"ac", "ad", "bc", "bd"}}, // cross product
		{"{a,{b,c}}", []string{"a", "b", "c"}},           // nesting
		{"{1..3}", []string{"1", "2", "3"}},              // numeric sequence
		{"{3..1}", []string{"3", "2", "1"}},              // descending
		{"{01..03}", []string{"01", "02", "03"}},         // zero-padded
		{"{1..9..3}", []string{"1", "4", "7"}},           // step
		{"{a..e..2}", []string{"a", "c", "e"}},           // char sequence + step
		{"plain", []string{"plain"}},                     // no braces
		{"{a}", []string{"{a}"}},                         // single item stays literal
		{"{a,b", []string{"{a,b"}},                       // unbalanced stays literal
		{"a{b}c", []string{"a{b}c"}},                     // no separator stays literal
	}
	for _, c := range cases {
		got := braceExpand(c.in)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("braceExpand(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestMakeDirBraceExpansion is the end-to-end regression for the reported
// `mkdir -p s01/{ep1,ep2}` parse error: the command must now create both dirs.
func TestMakeDirBraceExpansion(t *testing.T) {
	dir := t.TempDir()
	if _, err := evalScript(t, dir, "mkdir -p s01/{ep1,ep2}"); err != nil {
		t.Fatalf("mkdir with brace expansion failed: %v", err)
	}
	for _, rel := range []string{"s01/ep1", "s01/ep2"} {
		if info, err := os.Stat(filepath.Join(dir, rel)); err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist", rel)
		}
	}
}

func TestEmptyBracesStayLiteral(t *testing.T) {
	if got := braceExpand("{}"); len(got) != 1 || got[0] != "{}" {
		t.Errorf("braceExpand(\"{}\") = %v, want [{}]", got)
	}
}
