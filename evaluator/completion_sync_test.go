package evaluator

import "testing"

func TestIsPackageOp(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		// Dedicated managers: any invocation counts.
		{"brew", []string{"install", "ripgrep"}, true},
		{"brew", []string{"uninstall", "ripgrep"}, true},
		{"brew", []string{"list"}, true},
		{"apt-get", []string{"install", "-y", "jq"}, true},
		{"/usr/bin/brew", []string{"install", "x"}, true},
		{"pacman", []string{"-S", "ripgrep"}, true},

		// Looking through sudo/doas/env wrappers.
		{"sudo", []string{"apt", "install", "jq"}, true},
		{"sudo", []string{"-H", "apt-get", "remove", "jq"}, true},
		{"doas", []string{"pacman", "-S", "ripgrep"}, true},
		{"env", []string{"FOO=1", "dnf", "install", "x"}, true},

		// Conditional managers: only on an install/remove verb.
		{"go", []string{"install", "golang.org/x/tools/gopls@latest"}, true},
		{"go", []string{"build", "./..."}, false},
		{"go", []string{"test", "./..."}, false},
		{"cargo", []string{"install", "ripgrep"}, true},
		{"cargo", []string{"build"}, false},
		{"npm", []string{"install", "-g", "typescript"}, true},
		{"npm", []string{"ci"}, true},
		{"npm", []string{"run", "test"}, false},

		// Not package operations.
		{"git", []string{"clone", "https://example.com/x"}, false},
		{"ls", []string{"-la"}, false},
		{"sudo", nil, false},
		{"sudo", []string{"-H"}, false},
	}
	for _, c := range cases {
		if got := isPackageOp(c.name, c.args); got != c.want {
			t.Errorf("isPackageOp(%q, %v) = %v, want %v", c.name, c.args, got, c.want)
		}
	}
}
