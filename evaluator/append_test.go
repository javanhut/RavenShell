package evaluator

import "testing"

// append(arr, val) grows the named variable in place and also returns the
// grown array, so both the bare call and the assignment form work.
func TestAppendGrowsVariableInPlace(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bare call in loop", "arr = []\nfor _ in range(3) { append(arr, \"s\") }\nprint arr\n", "s s s\n"},
		{"assignment form", "arr = []\narr = append(arr, \"a\")\narr = append(arr, \"b\")\nprint arr\n", "a b\n"},
		{"dollar form", "arr = [1]\nappend($arr, 2)\nprint arr\n", "1 2\n"},
		{"copy is untouched", "a = [1]\nb = a\nappend(a, 2)\nprint a\nprint b\n", "1 2\n1\n"},
		{"non-variable argument", "print append(range(2), 7)\n", "0 1 7\n"},
		{"function parameter is local", "fn grow(x) { append(x, 1); return x }\narr = []\ny = grow(arr)\nprint len(arr)\nprint y\n", "0\n1\n"},
	}
	for _, c := range cases {
		_, out := run(t, c.src)
		if out != c.want {
			t.Errorf("%s: output = %q, want %q", c.name, out, c.want)
		}
	}
}
