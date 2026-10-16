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

func TestCompoundAssignment(t *testing.T) {
	cases := []struct{ src, want string }{
		{"count = 0 for i in range(15){count += 1} print count\n", "15\n"},
		{"x = 10\nx -= 3\nx *= 2\nx %= 5\nprint x\n", "4\n"},
		{"s = \"a\"\ns += \"b\"\nprint s\n", "ab\n"},
		{"x = 20\nx /= 4\nprint x\n", "5\n"},
		{"x = 1;\nx += 2;\nx /= 3; print x;\n", "1\n"},
		{"c = 0; for i in range(3){c += 1;} print c\n", "3\n"},
	}
	for _, c := range cases {
		if _, out := run(t, c.src); out != c.want {
			t.Errorf("%q: output = %q, want %q", c.src, out, c.want)
		}
	}
}
