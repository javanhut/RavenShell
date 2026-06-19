package main

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestOscWorkingDir(t *testing.T) {
	// Non-absolute or empty paths report nothing.
	if got := oscWorkingDir("relative/dir"); got != "" {
		t.Errorf("non-absolute dir should yield empty, got %q", got)
	}
	if got := oscWorkingDir(""); got != "" {
		t.Errorf("empty dir should yield empty, got %q", got)
	}

	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}

	// A plain absolute path produces a well-formed OSC 7 sequence:
	// ESC ] 7 ; file://<host>/<path> BEL.
	got := oscWorkingDir("/usr/local/bin")
	if !strings.HasPrefix(got, "\x1b]7;file://") {
		t.Fatalf("missing OSC 7 prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\x07") {
		t.Fatalf("missing BEL terminator: %q", got)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]7;"), "\x07")
	if want := "file://" + host + "/usr/local/bin"; body != want {
		t.Errorf("OSC 7 body = %q, want %q", body, want)
	}

	// Spaces (and other URL-significant chars) in the path must be
	// percent-encoded so the file:// URL stays well-formed, and round-trip back
	// to the original path.
	got = oscWorkingDir("/Users/a b/proj")
	if !strings.Contains(got, "/Users/a%20b/proj") {
		t.Errorf("path not percent-encoded: %q", got)
	}
	body = strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]7;"), "\x07")
	u, err := url.Parse(body)
	if err != nil {
		t.Fatalf("emitted URL does not parse: %v", err)
	}
	if u.Scheme != "file" || u.Host != host || u.Path != "/Users/a b/proj" {
		t.Errorf("round-trip mismatch: scheme=%q host=%q path=%q", u.Scheme, u.Host, u.Path)
	}
}
