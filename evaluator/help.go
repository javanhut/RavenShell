package evaluator

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"

	"ravenshell/ansi"
)

// helpEntry documents a built-in command for `raven-help` and feeds the set of
// completable built-in names. name is the canonical command; aliases are the
// extra human-readable spellings that resolve to the same behavior.
type helpEntry struct {
	name    string
	aliases []string
	usage   string
	summary string
	group   string
}

// helpEntries describes every built-in command. The order within a group is
// preserved in the overview; groups are listed in groupOrder.
var helpEntries = []helpEntry{
	{name: "ls", usage: "ls [dir]", summary: "List the contents of a directory.", group: "Files & directories"},
	{name: "cd", usage: "cd [dir]", summary: "Change the current directory (no argument goes home).", group: "Files & directories"},
	{name: "cwd", aliases: []string{"whereami", "wai"}, usage: "whereami", summary: "Print the current working directory.", group: "Files & directories"},
	{name: "show", aliases: []string{"read", "view"}, usage: "read <file>...", summary: "Print the contents of one or more files.", group: "Files & directories"},
	{name: "mkfile", aliases: []string{"makefile", "newfile", "touch"}, usage: "mkfile <file>...", summary: "Create files without truncating existing content.", group: "Files & directories"},
	{name: "mkdir", aliases: []string{"makedir"}, usage: "mkdir [-p|--parents] <dir>...", summary: "Create directories; makedir creates missing parents automatically.", group: "Files & directories"},
	{name: "rm", aliases: []string{"remove", "delete"}, usage: "rm [-r|--recursive] [-f|--force] <path>...", summary: "Remove files; recursive removal must be requested explicitly.", group: "Files & directories"},
	{name: "rmdir", usage: "rmdir [-f|--force] <dir>...", summary: "Remove directories; only empty ones unless --force is given.", group: "Files & directories"},

	{name: "print", aliases: []string{"output"}, usage: "print [text...]", summary: "Print text, or echo piped input, to standard output.", group: "Output"},
	{name: "clear", usage: "clear", summary: "Clear the terminal screen.", group: "Output"},

	{name: "whoami", usage: "whoami", summary: "Print the current user name.", group: "Environment"},
	{name: "export", usage: "export NAME [value...]", summary: "Set a shell environment variable.", group: "Environment"},
	{name: "env", usage: "env", summary: "Print the effective environment, sorted by name.", group: "Environment"},
	{name: "raven-add", usage: "raven-add path [dir]", summary: "Register an extra executable search directory, or list them.", group: "Environment"},

	{name: "raven-alias", usage: "raven-alias [name command [arguments...]]", summary: "Define or list scriptable interactive aliases.", group: "Configuration"},
	{name: "raven-unalias", usage: "raven-unalias <name>...", summary: "Remove interactive aliases.", group: "Configuration"},
	{name: "raven-source", usage: "raven-source <file>", summary: "Evaluate a RavenScript file in the current session.", group: "Configuration"},
	{name: "raven-unset", usage: "raven-unset <name>...", summary: "Remove variables or environment overrides.", group: "Configuration"},
	{name: "raven-type", usage: "raven-type <name>...", summary: "Explain how command names resolve.", group: "Configuration"},

	{name: "ps", usage: "ps [pattern]", summary: "List running processes, optionally filtered by name.", group: "Processes"},
	{name: "kill", usage: "kill <pid|%job> [signal]", summary: "Send a signal (default TERM) to a process or background job.", group: "Processes"},
	{name: "killall", usage: "killall <name> [signal]", summary: "Signal every process whose name matches.", group: "Processes"},
	{name: "jobs", usage: "jobs", summary: "List background jobs started with '&'.", group: "Processes"},

	{name: "raven-update", usage: "raven-update [--check]", summary: "Rebuild RavenShell from source and replace the running binary in place.", group: "Maintenance"},
	{name: "raven-completions", usage: "raven-completions [update|clear|path]", summary: "Generate tab completions for commands from their man pages (fish-style).", group: "Maintenance"},

	{name: "raven-help", aliases: []string{"help"}, usage: "raven-help [command]", summary: "List built-in commands, or show detailed help for one.", group: "Help"},
}

// groupOrder fixes the display order of command groups in the overview.
var groupOrder = []string{
	"Files & directories",
	"Output",
	"Environment",
	"Configuration",
	"Processes",
	"Maintenance",
	"Help",
}

// builtinCommandNames returns every built-in command name and alias, for tab
// completion.
func builtinCommandNames() []string {
	names := make([]string, 0, len(helpEntries)*2)
	for _, h := range helpEntries {
		names = append(names, h.name)
		names = append(names, h.aliases...)
	}
	return names
}

// BuiltinSummaries returns every built-in command name and alias mapped to
// its one-line summary, used by tab completion to describe candidates.
func BuiltinSummaries() map[string]string {
	m := make(map[string]string, len(helpEntries)*2)
	for _, h := range helpEntries {
		m[h.name] = h.summary
		for _, a := range h.aliases {
			m[a] = h.summary
		}
	}
	return m
}

// findHelp looks up a help entry by canonical name or any of its aliases.
func findHelp(name string) (helpEntry, bool) {
	for _, h := range helpEntries {
		if h.name == name {
			return h, true
		}
		if slices.Contains(h.aliases, name) {
			return h, true
		}
	}
	return helpEntry{}, false
}

// execRavenHelp implements `raven-help` / `help`. With no operand it prints an
// overview of every built-in; with a command name it prints that command's
// usage, summary, and aliases.
func (e *Evaluator) execRavenHelp(args []string) (string, error) {
	args = stripFlags(args)
	color := e.colorOutput()

	if len(args) > 0 {
		entry, ok := findHelp(args[0])
		if !ok {
			return "", fmt.Errorf("raven-help: no built-in command %q (run raven-help to list them)", args[0])
		}
		out := renderHelpDetail(entry, color)
		fmt.Fprint(e.stdout, out)
		return out, nil
	}

	out := renderHelpOverview(color)
	fmt.Fprint(e.stdout, out)
	return out, nil
}

// bold styles s when color output is enabled.
func bold(s string, color bool) string {
	if color {
		return ansi.Wrap(ansi.Bold, s)
	}
	return s
}

// dim styles s as secondary text when color output is enabled.
func dim(s string, color bool) string {
	if color {
		return ansi.Wrap(ansi.Dim, s)
	}
	return s
}

// renderHelpOverview builds the full built-in command listing, grouped.
func renderHelpOverview(color bool) string {
	var out bytes.Buffer
	out.WriteString(bold("RavenShell built-in commands", color) + "\n")
	out.WriteString(dim("Run 'raven-help <command>' for details on one command.", color) + "\n")

	// Widest "name" column so summaries align.
	width := 0
	for _, h := range helpEntries {
		if len(h.name) > width {
			width = len(h.name)
		}
	}

	for _, group := range groupOrder {
		out.WriteString("\n" + bold(group, color) + "\n")
		for _, h := range helpEntries {
			if h.group != group {
				continue
			}
			line := fmt.Sprintf("  %-*s  %s", width, h.name, h.summary)
			out.WriteString(line + "\n")
			if len(h.aliases) > 0 {
				alias := fmt.Sprintf("  %-*s  aliases: %s", width, "", strings.Join(h.aliases, ", "))
				out.WriteString(dim(alias, color) + "\n")
			}
		}
	}
	return out.String()
}

// renderHelpDetail builds the detailed help for a single command.
func renderHelpDetail(h helpEntry, color bool) string {
	var out bytes.Buffer
	out.WriteString(bold(h.name, color) + " — " + h.summary + "\n")
	out.WriteString("  usage:   " + h.usage + "\n")
	if len(h.aliases) > 0 {
		out.WriteString("  aliases: " + strings.Join(h.aliases, ", ") + "\n")
	}
	out.WriteString("  group:   " + h.group + "\n")
	return out.String()
}

// helpCommandList returns the sorted canonical names of documented commands
// (used by tests and tooling).
func helpCommandList() []string {
	names := make([]string, 0, len(helpEntries))
	for _, h := range helpEntries {
		names = append(names, h.name)
	}
	sort.Strings(names)
	return names
}
