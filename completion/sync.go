package completion

import (
	"os"
	"os/exec"
	"path/filepath"
)

// This file exposes the cache-maintenance hooks the shell calls after a package
// manager runs, so completions track installed packages automatically without a
// manual `raven-completions update`: BuildCached pre-warms a freshly installed
// command, RemoveCached drops an uninstalled one.

// RemoveCached deletes any cached completion for cmd — both the man-page flag
// cache and the scraped --help cache — and reports whether anything was
// removed. It is used to forget a command's completion when its package is
// uninstalled.
func RemoveCached(cmd string) bool {
	dir := DefaultCacheDir()
	if dir == "" || !safeCmdName.MatchString(cmd) {
		return false
	}
	removed := false
	for _, p := range []string{
		filepath.Join(dir, cmd+".json"),
		filepath.Join(dir, "help", cmd+".json"),
	} {
		if err := os.Remove(p); err == nil {
			removed = true
		}
	}
	return removed
}

// BuildCached pre-builds and caches cmd's completion (man-page flags plus the
// flags and subcommands scraped from --help) so the first Tab after a package
// install is instant instead of paying for the scrape then. It runs `man cmd`
// and `cmd --help`, so it is meant only for freshly installed commands; it is a
// no-op when cmd is not on PATH or its name is unsafe. Already-fresh cache
// entries are reused, so re-running it is cheap.
func BuildCached(cmd string) {
	dir := DefaultCacheDir()
	if dir == "" || !safeCmdName.MatchString(cmd) {
		return
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return
	}
	_ = loadOrBuildMan(cmd, dir, ".")
	_ = loadOrScrapeHelp(cmd, dir, ".")
}
