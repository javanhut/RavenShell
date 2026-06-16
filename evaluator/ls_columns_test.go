package evaluator

import (
	"fmt"
	"strings"
	"testing"
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

// TestFormatColumnsColumnMajor verifies entries fill top-to-bottom, 10 per
// column, with extra columns appended (25 -> columns of 10, 10, 5).
func TestFormatColumnsColumnMajor(t *testing.T) {
	names := seqNames(25)
	out := formatColumns(names, names, 10)
	lines := gridLines(out)

	if len(lines) != 10 {
		t.Fatalf("expected 10 rows, got %d:\n%s", len(lines), out)
	}

	// Reconstruct reading column-major (down each column, then across); it must
	// equal the original order.
	rows := make([][]string, len(lines))
	maxCols := 0
	for i, ln := range lines {
		rows[i] = strings.Fields(ln)
		if len(rows[i]) > maxCols {
			maxCols = len(rows[i])
		}
	}
	if maxCols != 3 {
		t.Fatalf("expected 3 columns for 25 entries, got %d:\n%s", maxCols, out)
	}

	var got []string
	for c := 0; c < maxCols; c++ {
		for r := range rows {
			if c < len(rows[r]) {
				got = append(got, rows[r][c])
			}
		}
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Errorf("column-major order mismatch.\n got: %v\nwant: %v", got, names)
	}

	// Row 0 should be entries 0, 10, 20.
	if r0 := strings.Fields(lines[0]); r0[0] != "e00" || r0[1] != "e10" || r0[2] != "e20" {
		t.Errorf("row 0 = %v, want [e00 e10 e20]", r0)
	}
}

// TestFormatColumnsSingleColumn checks that <= perCol entries form one column,
// one entry per line.
func TestFormatColumnsSingleColumn(t *testing.T) {
	for _, n := range []int{1, 5, 10} {
		names := seqNames(n)
		lines := gridLines(formatColumns(names, names, 10))
		if len(lines) != n {
			t.Errorf("n=%d: expected %d lines, got %d", n, n, len(lines))
		}
		for i, ln := range lines {
			if strings.TrimSpace(ln) != names[i] {
				t.Errorf("n=%d line %d = %q, want %q", n, i, ln, names[i])
			}
		}
	}
}

// TestFormatColumnsEleven checks the first overflow into a second column.
func TestFormatColumnsEleven(t *testing.T) {
	names := seqNames(11)
	lines := gridLines(formatColumns(names, names, 10))
	if len(lines) != 10 {
		t.Fatalf("expected 10 rows, got %d", len(lines))
	}
	// Row 0 holds e00 and e10; rows 1..9 hold a single entry each.
	if r0 := strings.Fields(lines[0]); len(r0) != 2 || r0[0] != "e00" || r0[1] != "e10" {
		t.Errorf("row 0 = %v, want [e00 e10]", r0)
	}
	for i := 1; i < 10; i++ {
		if f := strings.Fields(lines[i]); len(f) != 1 {
			t.Errorf("row %d = %v, want a single entry", i, f)
		}
	}
}

// TestFormatColumnsEmpty checks the empty directory case.
func TestFormatColumnsEmpty(t *testing.T) {
	if got := formatColumns(nil, nil, 10); got != "" {
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

	plainLayout := formatColumns(names, names, 10)
	colorLayout := stripANSI(formatColumns(names, colored, 10))

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
