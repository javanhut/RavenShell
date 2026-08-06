package evaluator

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// names builds n entries named e00, e01, ... for layout tests.
func seqNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("e%02d", i)
	}
	return out
}

// gridLines splits formatted column output into lines (dropping the trailing
// empty element after the last newline).
func gridLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// TestFormatColumnsFitsWidth is the property that matters: no line is wider
// than the terminal, so nothing wraps and the columns stay aligned.
func TestFormatColumnsFitsWidth(t *testing.T) {
	names := []string{
		".DS_Store", "Ivaldi/", "NoSuchMerge/", "RavenShellTest/", "TestDirectoryOC/",
		"TheRoom/", ".claude/", "IvaldiTest/", "NvCrow/", "RavenTerminal/", "TestHtmx/",
		"concurrency-plan.md", "AWSTUI/", "LocalAgent/", "OllamaTUI/", "ReactVIsland/",
		"TheCarrionLanguage/",
	}
	for _, width := range []int{20, 40, 80, 120} {
		out := formatColumns(names, names, width)
		for _, ln := range gridLines(out) {
			if w := utf8.RuneCountInString(strings.TrimRight(ln, " ")); w > width {
				t.Errorf("width=%d: line %q is %d wide", width, ln, w)
			}
		}
	}
}

// TestFormatColumnsColumnMajor verifies entries fill top-to-bottom and that a
// wider terminal packs them into more columns.
func TestFormatColumnsColumnMajor(t *testing.T) {
	names := seqNames(25) // each entry is 3 wide, so 5 per column at width 25
	out := formatColumns(names, names, 25)
	lines := gridLines(out)

	if len(lines) != 5 {
		t.Fatalf("expected 5 rows, got %d:\n%s", len(lines), out)
	}

	// Reconstruct reading column-major (down each column, then across); it must
	// equal the original order.
	var got []string
	rows := make([][]string, len(lines))
	for i, ln := range lines {
		rows[i] = strings.Fields(ln)
	}
	for c := 0; c < len(rows[0]); c++ {
		for r := range rows {
			if c < len(rows[r]) {
				got = append(got, rows[r][c])
			}
		}
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Errorf("column-major order mismatch.\n got: %v\nwant: %v", got, names)
	}

	// Row 0 should be entries 0, 5, 10, 15, 20.
	if r0 := strings.Fields(lines[0]); strings.Join(r0, ",") != "e00,e05,e10,e15,e20" {
		t.Errorf("row 0 = %v, want [e00 e05 e10 e15 e20]", r0)
	}
}

// TestFormatColumnsNarrow checks that a terminal too narrow for two columns
// falls back to one entry per line.
func TestFormatColumnsNarrow(t *testing.T) {
	names := seqNames(6)
	lines := gridLines(formatColumns(names, names, 6))
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if strings.TrimSpace(ln) != names[i] {
			t.Errorf("line %d = %q, want %q", i, ln, names[i])
		}
	}
}

// TestFormatColumnsOverlongEntry checks that an entry wider than the terminal
// does not hang or drop entries.
func TestFormatColumnsOverlongEntry(t *testing.T) {
	names := []string{strings.Repeat("x", 50), "a", "b"}
	lines := gridLines(formatColumns(names, names, 10))
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), lines)
	}
}

// TestFormatColumnsEmpty checks the empty directory case.
func TestFormatColumnsEmpty(t *testing.T) {
	if got := formatColumns(nil, nil, 80); got != "" {
		t.Errorf("empty listing = %q, want \"\"", got)
	}
}

// TestFormatColumnsColorDoesNotAffectLayout verifies that the colored display
// strings produce the exact same visible layout as plain names — i.e. column
// widths are computed from the visible text, not the ANSI escape codes.
func TestFormatColumnsColorDoesNotAffectLayout(t *testing.T) {
	names := []string{"alpha/", "b", "ccccccc", "d", "ee", "fff", "g", "hh", "iii", "j", "k", "ll"}
	colored := make([]string, len(names))
	for i, n := range names {
		colored[i] = "\033[1;34m" + n + "\033[0m" // pretend everything is a colored dir
	}

	plainLayout := formatColumns(names, names, 40)
	colorLayout := stripANSI(formatColumns(names, colored, 40))

	if plainLayout != colorLayout {
		t.Errorf("color changed the visible layout:\nplain:\n%q\ncolor(stripped):\n%q",
			plainLayout, colorLayout)
	}
}

// TestListPipedStaysOnePerLine locks the contract that captured/piped `ls`
// output is one entry per line (not columnar), so $(ls) and pipes still parse.
func TestListPipedStaysOnePerLine(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		writeFile(t, dir, f, "")
	}
	// Captured via stdout buffer (not a terminal) -> one per line.
	out, err := evalScript(t, dir, "ls")
	if err != nil {
		t.Fatal(err)
	}
	lines := gridLines(out)
	if len(lines) != 3 {
		t.Fatalf("piped ls = %d lines, want 3:\n%s", len(lines), out)
	}
	for _, ln := range lines {
		if len(strings.Fields(ln)) != 1 {
			t.Errorf("piped ls line %q has multiple columns; want one per line", ln)
		}
	}
}

// stripANSI removes ANSI escape sequences (ESC [ ... letter) for layout checks.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			continue // skip the final letter too
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
