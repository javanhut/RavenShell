package evaluator

import (
	"fmt"
	"os"
	"path/filepath"
)

// refuseSystemRemoval rejects removing the root filesystem, any directory
// directly under it (/usr, /home, /etc, ...), or a home directory (the
// user's own, or any directory directly under /home). The top-level check is
// what catches `rm -rf /*`, which expands past GNU rm's own --preserve-root.
// There is deliberately no override: --no-preserve-root is not honored.
//
// Every operand is checked before anything is removed, so a protected path
// anywhere in the list stops the whole command.
func (e *Evaluator) refuseSystemRemoval(cmd string, operands []string) error {
	for _, arg := range operands {
		if why := protectedPath(e.resolvePath(arg)); why != "" {
			return fmt.Errorf("%s: refusing to remove '%s': it is %s", cmd, arg, why)
		}
	}
	return nil
}

// protectedPath reports why path must not be removed, or "" if it may be.
// Symlinks in the parent are resolved (so /usr/bin/.. or a link to / as a
// parent is caught), but not the final component: removing a symlink that
// points at /usr only removes the link.
func protectedPath(path string) string {
	p := canonical(path)
	if p == "/" {
		return "the root filesystem"
	}
	// Only directories (and links to them, like /lib -> usr/lib) are
	// protected; plain files such as /swapfile or ~/.bashrc are not.
	if info, err := os.Stat(p); err != nil || !info.IsDir() {
		return ""
	}
	if filepath.Dir(p) == "/" {
		return "a top-level system directory"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && p == canonical(home) {
		return "your home directory"
	}
	if filepath.Dir(p) == "/home" {
		return "a home directory"
	}
	return ""
}

// canonical cleans path and resolves symlinks in its parent directory.
func canonical(path string) string {
	p := filepath.Clean(path)
	if parent, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(parent, filepath.Base(p))
	}
	return p
}
