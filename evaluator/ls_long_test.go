package evaluator

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// TestListLongShowsOwnerAndPermissions covers `ls -l`: every entry reports its
// mode, owner and size, and symlinks show their target.
func TestListLongShowsOwnerAndPermissions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "script.sh", "echo hi\n")
	if err := os.Chmod(filepath.Join(dir, "script.sh"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("script.sh", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	out, err := evalScript(t, dir, "ls -l")
	if err != nil {
		t.Fatal(err)
	}
	lines := gridLines(out)
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "total ") {
		t.Fatalf("ls -l = %q, want a total line and 3 entries", out)
	}

	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"link":      "lrwxrwxrwx",
		"script.sh": "-rwxr-x---",
		"sub/":      "drwxr-xr-x",
	}
	for _, ln := range lines[1:] {
		f := strings.Fields(ln)
		name := f[8]
		if f[0] != want[name] {
			t.Errorf("%s: mode %s, want %s", name, f[0], want[name])
		}
		if f[2] != me.Username {
			t.Errorf("%s: owner %s, want %s", name, f[2], me.Username)
		}
		if name == "script.sh" && f[4] != "8" {
			t.Errorf("script.sh: size %s, want 8", f[4])
		}
		if name == "link" && !strings.HasSuffix(ln, "link -> script.sh") {
			t.Errorf("symlink line %q lacks target", ln)
		}
	}
}

// TestListHiddenFiles checks dotfiles are hidden by default, shown by -A, and
// shown along with . and .. by -a.
func TestListHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "")
	writeFile(t, dir, ".env", "")
	cases := map[string]string{
		"ls":     "a.txt",
		"ls -A":  ".env,a.txt",
		"ls -a":  "./,../,.env,a.txt",
		"ls -la": "./,../,.env,a.txt",
	}
	for src, want := range cases {
		out, err := evalScript(t, dir, src)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, ln := range gridLines(out) {
			if strings.HasPrefix(ln, "total ") {
				continue
			}
			f := strings.Fields(ln)
			names = append(names, f[len(f)-1])
		}
		if got := strings.Join(names, ","); got != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
}

func TestPermString(t *testing.T) {
	cases := map[os.FileMode]string{
		0o644:                              "-rw-r--r--",
		os.ModeDir | 0o1777&^0o1000:        "drwxrwxrwx",
		os.ModeDir | os.ModeSticky | 0o777: "drwxrwxrwt",
		os.ModeSetuid | 0o755:              "-rwsr-xr-x",
		os.ModeSetgid | 0o644:              "-rw-r-Sr--",
	}
	for m, want := range cases {
		if got := permString(m); got != want {
			t.Errorf("permString(%v) = %s, want %s", m, got, want)
		}
	}
}

func TestSizeStringHuman(t *testing.T) {
	cases := map[int64]string{0: "0", 1023: "1023", 1536: "1.5K", 20 << 20: "20M"}
	for n, want := range cases {
		if got := sizeString(n, true); got != want {
			t.Errorf("sizeString(%d) = %s, want %s", n, got, want)
		}
	}
}
