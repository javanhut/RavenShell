package evaluator

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Build metadata for `raven-update`. main sets these at startup so the running
// binary knows its own version and where it was built from. BuildSourceDir is
// stamped in by the install script / Makefile via
// -ldflags "-X main.sourceDir=<abs path>" (forwarded here from main).
var (
	BuildVersion   = "dev"
	BuildSourceDir = ""
)

const ravenUpdateUsage = `raven-update [--check]

Rebuild RavenShell from source and replace the running binary in place,
so an outdated installed binary is cleaned up and replaced.

  --check   Report the source location and any shadowing binaries, but
            make no changes.

The source tree is found via $RAVENSHELL_SRC, the path embedded at build
time, or a few common locations. Set RAVENSHELL_SRC if it lives elsewhere.`

// execRavenUpdate rebuilds RavenShell from its source tree and atomically
// replaces the currently running binary. This exists so a stale binary (built
// before a fix landed) can be refreshed without hunting down the source by
// hand — the failure mode that made a relative path get mangled into an
// absolute one before reaching an external tool.
func (e *Evaluator) execRavenUpdate(args []string) (string, error) {
	checkOnly := false
	for _, a := range args {
		switch a {
		case "--check", "-n", "check":
			checkOnly = true
		case "--help", "-h":
			fmt.Fprintln(e.stdout, ravenUpdateUsage)
			return "", nil
		default:
			return "", fmt.Errorf("raven-update: unknown argument %q (try raven-update --help)", a)
		}
	}

	target, err := currentBinaryPath()
	if err != nil {
		return "", fmt.Errorf("raven-update: cannot locate the running binary: %v", err)
	}

	src, err := resolveSourceDir()
	if err != nil {
		return "", err
	}

	fmt.Fprintf(e.stdout, "RavenShell %s\n", BuildVersion)
	fmt.Fprintf(e.stdout, "  binary: %s\n", target)
	fmt.Fprintf(e.stdout, "  source: %s\n", src)

	// Surface the exact problem that motivated this command: another
	// ravenshell earlier on PATH that would keep shadowing the updated one.
	warnShadowingBinaries(e.stdout, target)

	if checkOnly {
		return "", nil
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("raven-update: Go is required to rebuild RavenShell (https://go.dev/dl/)")
	}

	// Best-effort refresh of the source tree before building. A dirty tree,
	// detached HEAD, or no network is fine — build whatever is checked out.
	if isGitRepo(src) {
		fmt.Fprintln(e.stdout, "Updating source (git pull --ff-only)...")
		if out, err := runCaptured(src, "git", "pull", "--ff-only"); err != nil {
			fmt.Fprintf(e.stdout, "  skipped pull: %s\n", firstLine(out, err))
		} else if s := strings.TrimSpace(out); s != "" {
			fmt.Fprintln(e.stdout, "  "+strings.ReplaceAll(s, "\n", "\n  "))
		}
	}

	version := gitDescribe(src)

	// Build into the (writable) source tree, then move the result onto the
	// target. Building straight into the install dir would need write access
	// there just to create the temp file.
	built := filepath.Join(src, "ravenshell.update")
	_ = os.Remove(built)
	fmt.Fprintln(e.stdout, "Building...")
	ldflags := fmt.Sprintf("-X main.version=%s -X main.sourceDir=%s", version, src)
	if out, err := runCaptured(src, "go", "build", "-ldflags", ldflags, "-o", built, "."); err != nil {
		_ = os.Remove(built)
		return "", fmt.Errorf("raven-update: build failed: %v\n%s", err, out)
	}
	defer os.Remove(built)

	if err := replaceBinary(e.stdout, src, built, target); err != nil {
		return "", err
	}

	newVer := binaryVersion(target)
	fmt.Fprintf(e.stdout, "Updated %s\n  %s -> %s\n", target, BuildVersion, newVer)
	fmt.Fprintln(e.stdout, "Restart RavenShell to run the new binary.")
	return "", nil
}

// currentBinaryPath returns the absolute, symlink-resolved path of the running
// executable — the binary that raven-update should replace.
func currentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

// resolveSourceDir locates the RavenShell source checkout to rebuild from.
func resolveSourceDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("RAVENSHELL_SRC")); d != "" {
		if isRavenSource(d) {
			return d, nil
		}
		return "", fmt.Errorf("raven-update: RAVENSHELL_SRC=%q is not a RavenShell source tree", d)
	}
	if BuildSourceDir != "" && isRavenSource(BuildSourceDir) {
		return BuildSourceDir, nil
	}

	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, "Development", "RavenShell"),
			filepath.Join(home, "Development", "ToolsForRaven", "RavenShell"),
			filepath.Join(home, "RavenShell"),
			filepath.Join(home, "src", "RavenShell"),
			filepath.Join(home, "Projects", "RavenShell"),
		)
	}
	for _, d := range candidates {
		if isRavenSource(d) {
			return d, nil
		}
	}
	return "", fmt.Errorf("raven-update: could not locate the RavenShell source tree.\n" +
		"  Set RAVENSHELL_SRC to its path, for example:\n" +
		"    export RAVENSHELL_SRC=~/Development/RavenShell")
}

// isRavenSource reports whether dir is a RavenShell source checkout, identified
// by a go.mod that declares the ravenshell module.
func isRavenSource(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == "module ravenshell" {
			return true
		}
	}
	return false
}

// replaceBinary moves the freshly built binary onto target, escalating to sudo
// only when the install directory is not writable (e.g. /usr/local/bin).
func replaceBinary(w io.Writer, src, built, target string) error {
	dstDir := filepath.Dir(target)
	if isWritableDir(dstDir) {
		// Same-filesystem rename is atomic; fall back to install(1) for the
		// rare cross-device case (source tree on a different mount).
		if err := os.Rename(built, target); err == nil {
			return nil
		}
		if out, err := runCaptured(src, "install", "-m", "0755", built, target); err != nil {
			return fmt.Errorf("raven-update: could not replace %s: %v\n%s", target, err, out)
		}
		return nil
	}

	fmt.Fprintf(w, "Replacing %s requires elevated permissions; using sudo...\n", target)
	if err := runInteractive(w, src, "sudo", "install", "-m", "0755", built, target); err != nil {
		return fmt.Errorf("raven-update: could not replace %s with sudo: %v", target, err)
	}
	return nil
}

// warnShadowingBinaries reports other ravenshell executables on PATH. If a
// different one is found before target, it would keep being run instead of the
// binary we just updated, so the user is told to remove it.
func warnShadowingBinaries(w io.Writer, target string) {
	targetResolved := resolve(target)

	var found []string
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "ravenshell")
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		rp := resolve(p)
		if seen[rp] {
			continue
		}
		seen[rp] = true
		found = append(found, p)
	}

	if len(found) <= 1 {
		return
	}

	fmt.Fprintln(w, "warning: multiple 'ravenshell' binaries found on PATH:")
	for _, p := range found {
		marker := ""
		if resolve(p) == targetResolved {
			marker = "  <- updated here"
		}
		fmt.Fprintf(w, "  %s%s\n", p, marker)
	}
	if first := found[0]; resolve(first) != targetResolved {
		fmt.Fprintf(w, "  '%s' shadows the updated binary; remove it so the new one is used.\n", first)
	}
}

// isWritableDir reports whether a new file can be created in dir.
func isWritableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".raven-update-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// gitDescribe mirrors install.sh's version stamping.
func gitDescribe(dir string) string {
	out, err := runCaptured(dir, "git", "describe", "--tags", "--always", "--dirty")
	if err != nil {
		return "dev"
	}
	if v := strings.TrimSpace(out); v != "" {
		return v
	}
	return "dev"
}

// binaryVersion asks an installed ravenshell binary for its version string.
func binaryVersion(path string) string {
	out, err := exec.Command(path, "-v").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "RavenShell "))
}

func resolve(path string) string {
	if rp, err := filepath.EvalSymlinks(path); err == nil {
		return rp
	}
	return path
}

func runCaptured(dir, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

// runInteractive runs a command wired to the terminal so prompts (e.g. sudo's
// password prompt) work.
func runInteractive(w io.Writer, dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = w
	c.Stderr = os.Stderr
	return c.Run()
}

func firstLine(out string, err error) string {
	s := strings.TrimSpace(out)
	if s == "" {
		return err.Error()
	}
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}
