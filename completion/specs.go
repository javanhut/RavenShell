package completion

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// gitBranches lists local branch names; gitRefs adds tags; gitRemotesAndBranches
// is used where either a remote or a ref makes sense (push/pull/fetch).
const (
	gitBranches           = `git branch --format='%(refname:short)' 2>/dev/null`
	gitRefs               = `git for-each-ref --format='%(refname:short)' refs/heads refs/tags 2>/dev/null`
	gitRemotesAndBranches = `{ git remote; git branch --format='%(refname:short)'; } 2>/dev/null`
)

// builtinSpecs builds the registry of completion specs that ship with the
// shell. It takes the engine so specs can close over engine state (e.g. the
// built-in command summaries used by `help`).
func builtinSpecs(e *Engine) map[string]*Spec {
	branches := &ArgSpec{Command: gitBranches, NoFiles: true}

	git := &Spec{
		Flags: []Candidate{
			{Text: "-C", Desc: "Run as if started in the given directory"},
			{Text: "--help", Desc: "Show help"},
			{Text: "--version", Desc: "Show version"},
		},
		Subcommands: []SubSpec{
			{Name: "status", Desc: "Show the working tree status", Flags: []Candidate{
				{Text: "-s", Desc: "Short format"}, {Text: "--short", Desc: "Short format"}, {Text: "-b", Desc: "Show branch info"}}},
			{Name: "add", Desc: "Add file contents to the index", Flags: []Candidate{
				{Text: "-A", Desc: "Add all changes"}, {Text: "--all", Desc: "Add all changes"},
				{Text: "-p", Desc: "Interactively choose hunks"}, {Text: "--patch", Desc: "Interactively choose hunks"},
				{Text: "-u", Desc: "Update tracked files only"}}},
			{Name: "commit", Desc: "Record changes to the repository", Flags: []Candidate{
				{Text: "-m", Desc: "Commit message"}, {Text: "--message", Desc: "Commit message"},
				{Text: "-a", Desc: "Stage modified and deleted files"}, {Text: "--all", Desc: "Stage modified and deleted files"},
				{Text: "--amend", Desc: "Amend the previous commit"},
				{Text: "--no-verify", Desc: "Skip pre-commit hooks"}}},
			{Name: "push", Desc: "Update remote refs", Flags: []Candidate{
				{Text: "-u", Desc: "Set upstream"}, {Text: "--set-upstream", Desc: "Set upstream"},
				{Text: "--force", Desc: "Force update"}, {Text: "--force-with-lease", Desc: "Force if remote is unchanged"},
				{Text: "--tags", Desc: "Push tags"}, {Text: "--dry-run", Desc: "Show what would be pushed"}},
				Args: &ArgSpec{Command: gitRemotesAndBranches, NoFiles: true}},
			{Name: "pull", Desc: "Fetch and integrate from a remote", Flags: []Candidate{
				{Text: "--rebase", Desc: "Rebase instead of merge"}, {Text: "--ff-only", Desc: "Fast-forward only"}},
				Args: &ArgSpec{Command: gitRemotesAndBranches, NoFiles: true}},
			{Name: "fetch", Desc: "Download objects and refs", Flags: []Candidate{
				{Text: "--all", Desc: "Fetch all remotes"}, {Text: "--prune", Desc: "Remove stale remote refs"}},
				Args: &ArgSpec{Command: `git remote 2>/dev/null`, NoFiles: true}},
			{Name: "checkout", Desc: "Switch branches or restore files", Flags: []Candidate{
				{Text: "-b", Desc: "Create and switch to a new branch"}, {Text: "--track", Desc: "Set upstream"}},
				Args: &ArgSpec{Command: gitRefs}},
			{Name: "switch", Desc: "Switch branches", Flags: []Candidate{
				{Text: "-c", Desc: "Create and switch to a new branch"}, {Text: "--detach", Desc: "Detach HEAD"}},
				Args: branches},
			{Name: "branch", Desc: "List, create, or delete branches", Flags: []Candidate{
				{Text: "-d", Desc: "Delete a merged branch"}, {Text: "-D", Desc: "Force delete a branch"},
				{Text: "-m", Desc: "Rename a branch"}, {Text: "-a", Desc: "List remote branches too"},
				{Text: "--list", Desc: "List branches"}},
				Args: branches},
			{Name: "merge", Desc: "Join development histories", Flags: []Candidate{
				{Text: "--no-ff", Desc: "Always create a merge commit"}, {Text: "--squash", Desc: "Squash commits"},
				{Text: "--abort", Desc: "Abort the current merge"}},
				Args: branches},
			{Name: "rebase", Desc: "Reapply commits on another base", Flags: []Candidate{
				{Text: "--continue", Desc: "Continue after resolving conflicts"}, {Text: "--abort", Desc: "Abort the rebase"},
				{Text: "--onto", Desc: "Rebase onto the given base"}},
				Args: branches},
			{Name: "log", Desc: "Show commit logs", Flags: []Candidate{
				{Text: "--oneline", Desc: "One line per commit"}, {Text: "--graph", Desc: "Draw the commit graph"},
				{Text: "--all", Desc: "All refs"}, {Text: "-p", Desc: "Show patches"}, {Text: "--stat", Desc: "Show diffstats"}}},
			{Name: "diff", Desc: "Show changes", Flags: []Candidate{
				{Text: "--staged", Desc: "Changes staged for commit"}, {Text: "--stat", Desc: "Summary of changes"},
				{Text: "--name-only", Desc: "Changed file names only"}}},
			{Name: "stash", Desc: "Stash working tree changes",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "push", Desc: "Save changes to a new stash"}, {Text: "pop", Desc: "Apply and drop the latest stash"},
					{Text: "apply", Desc: "Apply a stash"}, {Text: "drop", Desc: "Delete a stash"},
					{Text: "list", Desc: "List stashes"}, {Text: "show", Desc: "Show a stash as a diff"}}, NoFiles: true}},
			{Name: "restore", Desc: "Restore working tree files", Flags: []Candidate{
				{Text: "--staged", Desc: "Unstage instead of discarding"}, {Text: "--source", Desc: "Restore from the given ref"}}},
			{Name: "reset", Desc: "Reset HEAD to a state", Flags: []Candidate{
				{Text: "--soft", Desc: "Keep index and working tree"}, {Text: "--hard", Desc: "Discard index and working tree"},
				{Text: "--mixed", Desc: "Keep working tree only"}}},
			{Name: "clone", Desc: "Clone a repository", Flags: []Candidate{
				{Text: "--depth", Desc: "Shallow clone depth"}, {Text: "--branch", Desc: "Checkout the given branch"}}},
			{Name: "init", Desc: "Create an empty repository"},
			{Name: "remote", Desc: "Manage remotes",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "add", Desc: "Add a remote"}, {Text: "remove", Desc: "Remove a remote"},
					{Text: "rename", Desc: "Rename a remote"}, {Text: "-v", Desc: "List remotes with URLs"},
					{Text: "show", Desc: "Show a remote"}, {Text: "set-url", Desc: "Change a remote's URL"}}, NoFiles: true}},
			{Name: "tag", Desc: "Create, list, or delete tags", Flags: []Candidate{
				{Text: "-a", Desc: "Annotated tag"}, {Text: "-d", Desc: "Delete a tag"}, {Text: "-l", Desc: "List tags"},
				{Text: "-m", Desc: "Tag message"}}},
			{Name: "show", Desc: "Show objects (commits, tags, ...)"},
			{Name: "blame", Desc: "Show who last modified each line"},
			{Name: "cherry-pick", Desc: "Apply an existing commit", Flags: []Candidate{
				{Text: "--continue", Desc: "Continue after resolving conflicts"}, {Text: "--abort", Desc: "Abort the cherry-pick"}}},
			{Name: "worktree", Desc: "Manage worktrees",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "add", Desc: "Create a worktree"}, {Text: "list", Desc: "List worktrees"},
					{Text: "remove", Desc: "Remove a worktree"}, {Text: "prune", Desc: "Clean up worktree data"}}, NoFiles: true}},
			{Name: "grep", Desc: "Search tracked files"},
		},
	}

	goTestFlags := []Candidate{
		{Text: "-v", Desc: "Verbose output"}, {Text: "-run", Desc: "Run tests matching a pattern"},
		{Text: "-race", Desc: "Enable the race detector"}, {Text: "-count", Desc: "Run each test n times"},
		{Text: "-cover", Desc: "Enable coverage analysis"}, {Text: "-bench", Desc: "Run benchmarks matching a pattern"},
		{Text: "-timeout", Desc: "Per-test timeout"},
	}
	goBuildFlags := []Candidate{
		{Text: "-o", Desc: "Output file"}, {Text: "-v", Desc: "Print package names"},
		{Text: "-race", Desc: "Enable the race detector"}, {Text: "-tags", Desc: "Build tags"},
		{Text: "-ldflags", Desc: "Linker flags"},
	}
	golang := &Spec{
		Subcommands: []SubSpec{
			{Name: "build", Desc: "Compile packages", Flags: goBuildFlags},
			{Name: "run", Desc: "Compile and run a package", Flags: goBuildFlags},
			{Name: "test", Desc: "Run tests", Flags: goTestFlags},
			{Name: "vet", Desc: "Report likely mistakes"},
			{Name: "fmt", Desc: "Format source files"},
			{Name: "get", Desc: "Add a dependency"},
			{Name: "install", Desc: "Compile and install packages"},
			{Name: "mod", Desc: "Module maintenance",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "tidy", Desc: "Add missing and remove unused modules"}, {Text: "download", Desc: "Download modules"},
					{Text: "init", Desc: "Create a new module"}, {Text: "vendor", Desc: "Vendor dependencies"},
					{Text: "why", Desc: "Explain why a module is needed"}, {Text: "graph", Desc: "Print the module graph"},
					{Text: "edit", Desc: "Edit go.mod"}}, NoFiles: true}},
			{Name: "generate", Desc: "Run code generators"},
			{Name: "clean", Desc: "Remove build artifacts"},
			{Name: "doc", Desc: "Show documentation"},
			{Name: "env", Desc: "Print Go environment"},
			{Name: "list", Desc: "List packages or modules"},
			{Name: "version", Desc: "Print Go version"},
			{Name: "work", Desc: "Workspace maintenance"},
			{Name: "tool", Desc: "Run a Go tool"},
		},
	}

	npm := &Spec{
		Subcommands: []SubSpec{
			{Name: "install", Desc: "Install dependencies", Flags: []Candidate{
				{Text: "-D", Desc: "Save as dev dependency"}, {Text: "--save-dev", Desc: "Save as dev dependency"},
				{Text: "-g", Desc: "Install globally"}, {Text: "--global", Desc: "Install globally"}}},
			{Name: "uninstall", Desc: "Remove a package"},
			{Name: "run", Desc: "Run a package.json script",
				Args: &ArgSpec{Generate: npmScripts, NoFiles: true}},
			{Name: "test", Desc: "Run the test script"},
			{Name: "start", Desc: "Run the start script"},
			{Name: "init", Desc: "Create a package.json"},
			{Name: "publish", Desc: "Publish the package"},
			{Name: "update", Desc: "Update packages"},
			{Name: "audit", Desc: "Scan for vulnerabilities"},
			{Name: "ci", Desc: "Clean install from the lockfile"},
			{Name: "exec", Desc: "Run a command from a package"},
			{Name: "ls", Desc: "List installed packages"},
		},
	}

	dockerNames := &ArgSpec{
		Command: `docker ps --format '{{.Names}}' 2>/dev/null`,
		NoFiles: true,
	}
	docker := &Spec{
		Subcommands: []SubSpec{
			{Name: "ps", Desc: "List containers", Flags: []Candidate{
				{Text: "-a", Desc: "Include stopped containers"}, {Text: "-q", Desc: "IDs only"}}},
			{Name: "images", Desc: "List images"},
			{Name: "run", Desc: "Run a command in a new container", Flags: []Candidate{
				{Text: "-it", Desc: "Interactive with a TTY"}, {Text: "--rm", Desc: "Remove on exit"},
				{Text: "-d", Desc: "Run detached"}, {Text: "-p", Desc: "Publish a port"},
				{Text: "-v", Desc: "Bind mount a volume"}, {Text: "--name", Desc: "Container name"},
				{Text: "-e", Desc: "Set an environment variable"}}},
			{Name: "exec", Desc: "Run a command in a running container", Flags: []Candidate{
				{Text: "-it", Desc: "Interactive with a TTY"}}, Args: dockerNames},
			{Name: "build", Desc: "Build an image", Flags: []Candidate{
				{Text: "-t", Desc: "Name and tag the image"}, {Text: "-f", Desc: "Dockerfile path"},
				{Text: "--no-cache", Desc: "Build without cache"}}},
			{Name: "pull", Desc: "Download an image"},
			{Name: "push", Desc: "Upload an image"},
			{Name: "stop", Desc: "Stop containers", Args: dockerNames},
			{Name: "start", Desc: "Start containers", Args: dockerNames},
			{Name: "restart", Desc: "Restart containers", Args: dockerNames},
			{Name: "rm", Desc: "Remove containers", Args: dockerNames},
			{Name: "rmi", Desc: "Remove images"},
			{Name: "logs", Desc: "Fetch container logs", Flags: []Candidate{
				{Text: "-f", Desc: "Follow output"}, {Text: "--tail", Desc: "Last n lines"}}, Args: dockerNames},
			{Name: "inspect", Desc: "Show detailed object info", Args: dockerNames},
			{Name: "compose", Desc: "Multi-container applications",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "up", Desc: "Create and start services"}, {Text: "down", Desc: "Stop and remove services"},
					{Text: "logs", Desc: "Show service logs"}, {Text: "ps", Desc: "List service containers"},
					{Text: "build", Desc: "Build service images"}, {Text: "restart", Desc: "Restart services"},
					{Text: "exec", Desc: "Run a command in a service"}, {Text: "pull", Desc: "Pull service images"}}, NoFiles: true}},
			{Name: "volume", Desc: "Manage volumes",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "ls", Desc: "List volumes"}, {Text: "rm", Desc: "Remove volumes"},
					{Text: "create", Desc: "Create a volume"}, {Text: "prune", Desc: "Remove unused volumes"}}, NoFiles: true}},
			{Name: "network", Desc: "Manage networks",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "ls", Desc: "List networks"}, {Text: "rm", Desc: "Remove networks"},
					{Text: "create", Desc: "Create a network"}, {Text: "inspect", Desc: "Show network details"}}, NoFiles: true}},
		},
	}

	makeSpec := &Spec{
		Flags: []Candidate{
			{Text: "-j", Desc: "Number of parallel jobs"}, {Text: "-B", Desc: "Rebuild unconditionally"},
			{Text: "-n", Desc: "Print commands without running them"}, {Text: "-C", Desc: "Change directory first"},
			{Text: "-f", Desc: "Use the given makefile"},
		},
		Args: &ArgSpec{Generate: makefileTargets, NoFiles: true},
	}

	cargo := &Spec{
		Subcommands: []SubSpec{
			{Name: "build", Desc: "Compile the package", Flags: []Candidate{
				{Text: "--release", Desc: "Optimized build"}}},
			{Name: "run", Desc: "Build and run", Flags: []Candidate{
				{Text: "--release", Desc: "Optimized build"}}},
			{Name: "test", Desc: "Run tests"},
			{Name: "check", Desc: "Type-check without building"},
			{Name: "fmt", Desc: "Format source files"},
			{Name: "clippy", Desc: "Run lints"},
			{Name: "add", Desc: "Add a dependency"},
			{Name: "new", Desc: "Create a new package"},
			{Name: "init", Desc: "Create a package in this directory"},
			{Name: "update", Desc: "Update dependencies"},
			{Name: "doc", Desc: "Build documentation"},
			{Name: "clean", Desc: "Remove build artifacts"},
			{Name: "publish", Desc: "Publish to the registry"},
			{Name: "install", Desc: "Install a binary crate"},
			{Name: "bench", Desc: "Run benchmarks"},
		},
	}

	brew := &Spec{
		Subcommands: []SubSpec{
			{Name: "install", Desc: "Install a formula or cask"},
			{Name: "uninstall", Desc: "Remove a formula or cask"},
			{Name: "upgrade", Desc: "Upgrade outdated packages"},
			{Name: "update", Desc: "Fetch the newest Homebrew"},
			{Name: "list", Desc: "List installed packages"},
			{Name: "search", Desc: "Search for packages"},
			{Name: "info", Desc: "Show package info"},
			{Name: "doctor", Desc: "Check for problems"},
			{Name: "outdated", Desc: "List outdated packages"},
			{Name: "cleanup", Desc: "Remove stale files"},
			{Name: "tap", Desc: "Add a repository of formulae"},
			{Name: "services", Desc: "Manage background services",
				Args: &ArgSpec{Static: []Candidate{
					{Text: "start", Desc: "Start a service"}, {Text: "stop", Desc: "Stop a service"},
					{Text: "restart", Desc: "Restart a service"}, {Text: "list", Desc: "List services"}}, NoFiles: true}},
		},
	}

	specs := map[string]*Spec{
		"git":    git,
		"go":     golang,
		"npm":    npm,
		"docker": docker,
		"make":   makeSpec,
		"cargo":  cargo,
		"brew":   brew,

		// Raven built-ins whose arguments are more specific than "any file".
		"cd":    {Args: &ArgSpec{DirsOnly: true}},
		"tldr":  {Args: &ArgSpec{Generate: tldrPages, NoFiles: true}},
		"rmdir": {Flags: []Candidate{{Text: "-f", Desc: "Remove non-empty directories"}, {Text: "--force", Desc: "Remove non-empty directories"}}, Args: &ArgSpec{DirsOnly: true}},
		"raven-add": {Args: &ArgSpec{Static: []Candidate{
			{Text: "path", Desc: "Register an extra executable search directory"}}}},
		"raven-completions": {
			Flags: []Candidate{{Text: "--deep", Desc: "With update: scrape --help for subcommands too"}},
			Args: &ArgSpec{Static: []Candidate{
				{Text: "update", Desc: "Parse man pages and cache completions"},
				{Text: "clear", Desc: "Delete cached completions"},
				{Text: "path", Desc: "Print the cache directory"},
			}, NoFiles: true}},
	}

	// help/raven-help complete the built-in command names with their summaries.
	helpArgs := &ArgSpec{
		Generate: func(string) []Candidate {
			names := make([]string, 0, len(e.summaries))
			for name := range e.summaries {
				names = append(names, name)
			}
			sort.Strings(names)
			out := make([]Candidate, 0, len(names))
			for _, name := range names {
				out = append(out, Candidate{Text: name, Desc: e.summaries[name]})
			}
			return out
		},
		NoFiles: true,
	}
	specs["help"] = &Spec{Args: helpArgs}
	specs["raven-help"] = &Spec{Args: helpArgs}

	return specs
}

// makefileTargets parses the Makefile in cwd (also makefile / GNUmakefile) and
// returns its target names, fish-style dynamic completion for `make <Tab>`.
func makefileTargets(cwd string) []Candidate {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		data, err := os.ReadFile(filepath.Join(cwd, name))
		if err != nil {
			continue
		}
		seen := make(map[string]bool)
		var out []Candidate
		for line := range strings.SplitSeq(string(data), "\n") {
			// Targets start at column 1; recipes, comments, and special
			// targets (.PHONY etc.) do not produce candidates.
			if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' || line[0] == '.' {
				continue
			}
			i := strings.IndexByte(line, ':')
			if i <= 0 || strings.HasPrefix(line[i:], ":=") {
				continue
			}
			target := strings.TrimSpace(line[:i])
			if target == "" || strings.ContainsAny(target, " \t$%=") || seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, Candidate{Text: target, Desc: "make target"})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// tldrPages lists the available tldr page names via `tldr --list`, the dynamic
// argument source for `tldr <Tab>` (tldr's argument is a page name, not a file).
// tldr prints one page per line under colorized "Pages for <platform>" section
// headers, so ANSI escapes are stripped and header lines skipped. It runs the
// command itself (bounded by generatorTimeout) rather than going through the
// engine so it can fit the Generate signature.
func tldrPages(string) []Candidate {
	ctx, cancel := context.WithTimeout(context.Background(), generatorTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tldr", "--list").Output()
	if err != nil && len(out) == 0 {
		return nil
	}

	var cands []Candidate
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(ansiSGR.ReplaceAllString(line, ""))
		// Skip blanks, the "Pages for <platform>" headers, and any multi-word
		// line that is clearly not a single page name.
		if line == "" || strings.HasPrefix(line, "Pages for") || strings.ContainsAny(line, " \t") {
			continue
		}
		cands = append(cands, Candidate{Text: line, Desc: "tldr page"})
	}
	return cands
}

// npmScripts reads package.json in cwd and returns its script names, each
// described by the command it runs.
func npmScripts(cwd string) []Candidate {
	data, err := os.ReadFile(filepath.Join(cwd, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Candidate, 0, len(names))
	for _, name := range names {
		desc := pkg.Scripts[name]
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		out = append(out, Candidate{Text: name, Desc: desc})
	}
	return out
}
