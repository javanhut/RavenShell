package evaluator

import (
	"path/filepath"
	"strings"

	"ravenshell/completion"
)

// After a package manager runs, RavenShell reconciles its completion cache: it
// pre-builds completions for newly installed commands (so the first Tab is
// instant, with no manual `raven-completions update`) and removes them for
// commands whose package was uninstalled. This keeps tab completion in step
// with what is installed, the way fish refreshes on package changes.

// alwaysPkgManagers are dedicated package managers: any invocation may add or
// remove executables, and they are not run often enough for the reconcile to be
// a noticeable cost.
var alwaysPkgManagers = map[string]bool{
	"brew": true, "port": true, "apt": true, "apt-get": true, "dpkg": true,
	"dnf": true, "yum": true, "zypper": true, "pacman": true, "apk": true,
	"snap": true, "flatpak": true, "pipx": true, "mas": true, "nix-env": true,
	"asdf": true,
}

// conditionalPkgManagers install packages only for certain subcommands and are
// otherwise run constantly (builds, tests, scripts), so they trigger a
// reconcile only when an install/remove verb is present.
var conditionalPkgManagers = map[string]bool{
	"go": true, "cargo": true, "npm": true, "pnpm": true, "yarn": true,
	"pip": true, "pip3": true, "gem": true, "conda": true,
}

// pkgOpVerbs mark a conditional manager's invocation as an install/remove.
var pkgOpVerbs = map[string]bool{
	"install": true, "uninstall": true, "remove": true, "rm": true,
	"add": true, "i": true, "un": true, "reinstall": true, "ci": true,
}

// pkgWrappers are transparent prefixes (sudo apt …, doas pacman …); the real
// command follows the wrapper and its options.
var pkgWrappers = map[string]bool{"sudo": true, "doas": true, "env": true}

// isPackageOp reports whether running name with args may change the set of
// installed executables, looking through sudo/doas/env wrappers first.
func isPackageOp(name string, args []string) bool {
	cmd := filepath.Base(name)
	for pkgWrappers[cmd] {
		i := 0
		for i < len(args) && (strings.HasPrefix(args[i], "-") ||
			(cmd == "env" && strings.Contains(args[i], "="))) {
			i++
		}
		if i >= len(args) {
			return false
		}
		cmd = filepath.Base(args[i])
		args = args[i+1:]
	}

	if alwaysPkgManagers[cmd] {
		return true
	}
	if conditionalPkgManagers[cmd] {
		for _, a := range args {
			if pkgOpVerbs[a] {
				return true
			}
		}
	}
	return false
}

// commandNameSet returns the current invokable command names as a set.
func (e *Evaluator) commandNameSet() map[string]bool {
	names := e.AvailableCommands()
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// reconcileCompletionsAfterPkgOp refreshes the completion cache after a package
// manager runs successfully: it rescans PATH, then pre-builds completions for
// commands that appeared and removes them for commands that went away. The
// man/--help work runs in the background so the prompt returns immediately.
func (e *Evaluator) reconcileCompletionsAfterPkgOp(before map[string]bool) {
	e.execCacheValid = false // a package op changed PATH; rescan to see it
	after := e.commandNameSet()

	var added, removed []string
	for name := range after {
		if !before[name] {
			added = append(added, name)
		}
	}
	for name := range before {
		if !after[name] {
			removed = append(removed, name)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return
	}

	// Guard against a pathological install dumping a huge number of binaries;
	// any not pre-built here are still scraped lazily on first Tab.
	const maxBuild = 200
	if len(added) > maxBuild {
		added = added[:maxBuild]
	}

	go func() {
		for _, c := range removed {
			completion.RemoveCached(c)
		}
		for _, c := range added {
			completion.BuildCached(c)
		}
	}()
}
