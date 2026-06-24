package evaluator

import (
	"fmt"
	"time"

	"ravenshell/completion"
)

const ravenCompletionsUsage = `raven-completions [update [--deep]|clear|path]

Generate fish-style tab completions for system commands — the equivalent of
fish's fish_update_completions. Flags come from man pages; subcommands come from
each tool's --help. Both are cached on disk and reused until the underlying man
page or binary changes.

  update          Parse every installed man page and cache command flags.
  update --deep   Also run '<cmd> --help' on every user command to harvest
                  subcommands (kubectl, gh, docker, ...). This executes those
                  programs, so it is opt-in.
  clear           Delete all cached completions.
  path            Print the completion cache directory.

With no argument, report the cache location and how many commands are cached.
Completions are also filled in automatically: installing a package with a known
manager (brew, apt, cargo install, npm i -g, ...) pre-builds the new commands'
completions and drops any that were uninstalled, and tabbing a command for the
first time scrapes it on demand. 'update' just pre-warms everything at once.
('update' alone does flags for everything; subcommands are scraped lazily per
command, or in bulk with --deep.)`

// execRavenCompletions manages the completion cache that backs fish-style flag
// and subcommand completion for arbitrary commands.
func (e *Evaluator) execRavenCompletions(args []string) (string, error) {
	action := ""
	deep := false
	for _, a := range args {
		switch a {
		case "--help", "-h":
			fmt.Fprintln(e.stdout, ravenCompletionsUsage)
			return "", nil
		case "--deep":
			deep = true
		case "update", "clear", "path", "status":
			if action != "" {
				return "", fmt.Errorf("raven-completions: only one action at a time (got %q and %q)", action, a)
			}
			action = a
		default:
			return "", fmt.Errorf("raven-completions: unknown argument %q (try raven-completions --help)", a)
		}
	}
	if deep && action != "update" {
		return "", fmt.Errorf("raven-completions: --deep is only valid with 'update'")
	}

	switch action {
	case "path":
		fmt.Fprintln(e.stdout, completion.DefaultCacheDir())
		return "", nil

	case "clear":
		n, err := completion.ClearCache()
		if err != nil {
			return "", fmt.Errorf("raven-completions: %v", err)
		}
		fmt.Fprintf(e.stdout, "Removed %d cached completion(s) from %s\n", n, completion.DefaultCacheDir())
		return "", nil

	case "update":
		fmt.Fprintf(e.stdout, "Scanning man pages into %s ...\n", completion.DefaultCacheDir())
		stats, err := completion.GenerateAll(e.stdout, deep)
		if err != nil {
			return "", fmt.Errorf("raven-completions: %v", err)
		}
		fmt.Fprintf(e.stdout, "Done: %d commands scanned, %d with flags, %d (re)built",
			stats.Scanned, stats.WithFlags, stats.Updated)
		if deep {
			fmt.Fprintf(e.stdout, ", %d with subcommands", stats.WithSubcommands)
		}
		fmt.Fprintf(e.stdout, " (%s)\n", stats.Elapsed.Round(time.Millisecond))
		return "", nil

	default: // no action: report status
		fmt.Fprintf(e.stdout, "Completion cache: %s\n", completion.DefaultCacheDir())
		fmt.Fprintf(e.stdout, "%d command(s) cached.\n", completion.CachedCount())
		fmt.Fprintln(e.stdout, "Run 'raven-completions update' to (re)generate from man pages.")
		return "", nil
	}
}
