package evaluator

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// lsEntry is one row of an ls listing.
type lsEntry struct {
	name    string // visible name, with a trailing / for directories
	display string // name as written to the terminal (possibly colored)
	info    os.FileInfo
	target  string // symlink target, empty for non-links
}

func (e *Evaluator) newLsEntry(name, path string, info os.FileInfo, color bool) lsEntry {
	en := lsEntry{name: name, info: info, display: e.colorizeEntry(info.Mode(), name, color)}
	if info.Mode()&os.ModeSymlink != 0 {
		en.target, _ = os.Readlink(path)
	}
	return en
}

// formatLong renders entries in `ls -l` form:
//
//	drwxr-xr-x  3 alice staff 4096 Sep 23 10:12 docs/
//
// A "total" block count heads the listing when a directory was listed.
// It returns the terminal text (colored names) and the plain text returned
// for pipes and command substitution.
func formatLong(entries []lsEntry, human, withTotal bool) (display, plain string) {
	type row struct {
		mode, links, owner, group, size, date string
	}
	rows := make([]row, len(entries))
	var wLinks, wOwner, wGroup, wSize int
	var blocks int64
	for i, en := range entries {
		r := row{
			mode: permString(en.info.Mode()),
			size: sizeString(en.info.Size(), human),
			date: dateString(en.info.ModTime()),
		}
		r.links, r.owner, r.group = "?", "?", "?"
		if st, ok := en.info.Sys().(*syscall.Stat_t); ok {
			r.links = strconv.FormatUint(uint64(st.Nlink), 10)
			r.owner = lookupUser(st.Uid)
			r.group = lookupGroup(st.Gid)
			blocks += int64(st.Blocks)
		}
		wLinks = max(wLinks, len(r.links))
		wOwner = max(wOwner, utf8.RuneCountInString(r.owner))
		wGroup = max(wGroup, utf8.RuneCountInString(r.group))
		wSize = max(wSize, len(r.size))
		rows[i] = r
	}

	var out, raw bytes.Buffer
	// st_blocks is in 512-byte units; ls reports 1K blocks.
	total := fmt.Sprintf("total %s\n", sizeString(blocks*512, human))
	if !human {
		total = fmt.Sprintf("total %d\n", blocks/2)
	}
	if withTotal {
		out.WriteString(total)
		raw.WriteString(total)
	}
	for i, r := range rows {
		prefix := fmt.Sprintf("%s %*s %-*s %-*s %*s %s ",
			r.mode, wLinks, r.links, wOwner, r.owner, wGroup, r.group, wSize, r.size, r.date)
		suffix := ""
		if entries[i].target != "" {
			suffix = " -> " + entries[i].target
		}
		out.WriteString(prefix + entries[i].display + suffix + "\n")
		raw.WriteString(prefix + entries[i].name + suffix + "\n")
	}
	return out.String(), raw.String()
}

// permString renders a mode the way ls does: a type character followed by
// rwx triplets, with setuid/setgid/sticky shown as s/S and t/T.
func permString(m os.FileMode) string {
	var b [10]byte
	switch {
	case m.IsDir():
		b[0] = 'd'
	case m&os.ModeSymlink != 0:
		b[0] = 'l'
	case m&os.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&os.ModeSocket != 0:
		b[0] = 's'
	case m&os.ModeCharDevice != 0:
		b[0] = 'c'
	case m&os.ModeDevice != 0:
		b[0] = 'b'
	default:
		b[0] = '-'
	}
	const rwx = "rwxrwxrwx"
	for i := range 9 {
		if m&(1<<uint(8-i)) != 0 {
			b[i+1] = rwx[i]
		} else {
			b[i+1] = '-'
		}
	}
	special := func(idx int, set bool, lower, upper byte) {
		if !set {
			return
		}
		if b[idx] == '-' {
			b[idx] = upper
		} else {
			b[idx] = lower
		}
	}
	special(3, m&os.ModeSetuid != 0, 's', 'S')
	special(6, m&os.ModeSetgid != 0, 's', 'S')
	special(9, m&os.ModeSticky != 0, 't', 'T')
	return string(b[:])
}

// sizeString formats a byte count, optionally in ls -h style (1.5K, 23M).
func sizeString(n int64, human bool) string {
	if !human || n < 1024 {
		return strconv.FormatInt(n, 10)
	}
	f := float64(n)
	for _, unit := range []string{"K", "M", "G", "T", "P"} {
		f /= 1024
		if f < 1024 {
			if f < 10 {
				return strconv.FormatFloat(f, 'f', 1, 64) + unit
			}
			return strconv.FormatFloat(f, 'f', 0, 64) + unit
		}
	}
	return strconv.FormatFloat(f, 'f', 0, 64) + "E"
}

// dateString shows the time of day for files modified within the last six
// months and the year otherwise, matching ls.
func dateString(t time.Time) string {
	if d := time.Since(t); d < 0 || d > 182*24*time.Hour {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

var (
	idNamesMu  sync.Mutex
	userNames  = map[uint32]string{}
	groupNames = map[uint32]string{}
)

func lookupUser(uid uint32) string {
	return cachedName(userNames, uid, func(id string) (string, error) {
		u, err := user.LookupId(id)
		if err != nil {
			return "", err
		}
		return u.Username, nil
	})
}

func lookupGroup(gid uint32) string {
	return cachedName(groupNames, gid, func(id string) (string, error) {
		g, err := user.LookupGroupId(id)
		if err != nil {
			return "", err
		}
		return g.Name, nil
	})
}

// cachedName resolves a numeric id to a name once, falling back to the number
// itself when there is no passwd/group entry.
func cachedName(cache map[uint32]string, id uint32, lookup func(string) (string, error)) string {
	idNamesMu.Lock()
	defer idNamesMu.Unlock()
	if name, ok := cache[id]; ok {
		return name
	}
	s := strconv.FormatUint(uint64(id), 10)
	name, err := lookup(s)
	if err != nil || strings.TrimSpace(name) == "" {
		name = s
	}
	cache[id] = name
	return name
}
