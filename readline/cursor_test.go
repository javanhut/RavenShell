package readline

import "testing"

// TestParseCursorColumn covers parsing of the terminal's Cursor Position Report,
// which drives the "always start the prompt on a fresh line" behavior.
func TestParseCursorColumn(t *testing.T) {
	cases := []struct {
		name    string
		resp    string
		wantCol int
		wantOK  bool
	}{
		{"home", "\x1b[1;1R", 1, true},
		{"mid line", "\x1b[12;34R", 34, true},
		{"large", "\x1b[40;120R", 120, true},
		{"leading junk ignored", "garbage\x1b[5;7R", 7, true},
		{"no semicolon", "\x1b[5R", 0, false},
		{"missing R", "\x1b[5;7", 0, false},
		{"empty", "", 0, false},
		{"not a report", "hello", 0, false},
		{"three fields", "\x1b[1;2;3R", 0, false},
		{"non-numeric col", "\x1b[1;xR", 0, false},
		{"zero col rejected", "\x1b[1;0R", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col, ok := parseCursorColumn(tc.resp)
			if ok != tc.wantOK || col != tc.wantCol {
				t.Errorf("parseCursorColumn(%q) = (%d, %v), want (%d, %v)",
					tc.resp, col, ok, tc.wantCol, tc.wantOK)
			}
		})
	}
}
