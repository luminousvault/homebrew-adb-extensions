// Package files implements ak files, an interactive file manager that shows a
// device folder and a PC folder and copies, moves, deletes and renames files
// between and within them (docs/dev/files-design.md).
//
// state.go holds the screen state and its key handling (no terminal I/O),
// render.go draws it, run.go drives the terminal, and the storage side is the
// FS interface with deviceFS (adb) and localFS (os).
package files

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Side is where a pane's files live.
type Side int

const (
	Device Side = iota
	PC
)

// Entry is one file or folder.
type Entry struct {
	Name    string
	Path    string
	Dir     bool
	Size    int64 // bytes; 0 for folders, -1 if unknown
	ModTime time.Time
	// What this user (adb's shell on the device) may do with it, for the
	// name's color. The zero value is an ordinary readable, writable entry.
	NoRead   bool   // cannot be read (a folder: cannot be listed)
	ReadOnly bool   // can be read but not changed
	Exec     bool   // a file that can be run
	Link     bool   // a symbolic link (shown as what it points to)
	Folder   string // recent view: the folder shown beside the name
}

// FS is one storage: the device (over adb) or this PC. Paths use the storage's
// own separator; Join, Dir and Base handle them.
type FS interface {
	Side() Side
	// Label names the storage on screen: the device name or "Computer".
	Label() string
	// List returns the entries of dir. A folder the shell may not read fails
	// with fs.ErrPermission, a missing one with fs.ErrNotExist.
	List(dir string) ([]Entry, error)
	// Stat returns one entry; a missing path fails with fs.ErrNotExist.
	Stat(p string) (Entry, error)
	Mkdir(p string) error
	// Rename renames p within its folder (r, and swapping in an overwrite).
	Rename(from, to string) error
	// Remove deletes p and everything under it, permanently.
	Remove(p string) error
	// Usage is the total size and number of the regular files at or under
	// p. A missing p is an error.
	Usage(p string) (bytes, files int64, err error)
	Join(dir, name string) string
	Dir(p string) string
	Base(p string) string
}

// HumanSize formats bytes like ak pull: whole B and KB, one decimal MB and GB.
func HumanSize(n int64) string {
	switch {
	case n < 0:
		return "?"
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1048576:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	case n < 1073741824:
		t := (n*10 + 524288) / 1048576
		return fmt.Sprintf("%d.%d MB", t/10, t%10)
	}
	t := (n*10 + 536870912) / 1073741824
	return fmt.Sprintf("%d.%d GB", t/10, t%10)
}

// shortSize is a size in at most 5 columns, like ls -h: "12B", "12K",
// "3.2M", "430M", "1.5G".
func shortSize(n int64) string {
	if n < 0 {
		return "?"
	}
	units := "BKMGT"
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	switch {
	case i == 0:
		return fmt.Sprintf("%d%c", n, units[i])
	case v < 9.95:
		return fmt.Sprintf("%.1f%c", v, units[i])
	}
	return fmt.Sprintf("%.0f%c", v, units[i])
}

// fileTime formats a modification time for the list: how long ago for the
// last week ("now", "5m ago", "12h ago", "3d ago"), then the date: "11-03 14:05"
// this year, "2025-11-03" before. compact (two panes) drops " ago", the time
// and the month-day of older years: "5m", "11-03", "2025". A time in the
// future (a device clock ahead of this PC) shows the date as well.
func fileTime(t, now time.Time, compact bool) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(now.Location())
	d := now.Sub(t)
	ago := func(n int, unit string) string {
		if compact {
			return fmt.Sprintf("%d%s", n, unit)
		}
		return fmt.Sprintf("%d%s ago", n, unit)
	}
	switch {
	case d < -time.Minute: // the future: fall through to the date
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return ago(int(d/time.Minute), "m")
	case d < 24*time.Hour:
		return ago(int(d/time.Hour), "h")
	case d < 7*24*time.Hour:
		return ago(int(d/(24*time.Hour)), "d")
	}
	switch {
	case t.Year() == now.Year() && compact:
		return t.Format("01-02")
	case t.Year() == now.Year():
		return t.Format("01-02 15:04")
	case compact:
		return t.Format("2006")
	}
	return t.Format("2006-01-02")
}

// UniqueName adds " (N)" before the extension until taken reports false.
// Folders keep dots in their names.
func UniqueName(name string, dir bool, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); !dir && i > 0 {
		base, ext = name[:i], name[i:]
	}
	for n := 1; ; n++ {
		c := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if !taken(c) {
			return c
		}
	}
}

// validName rejects names that cannot be a single file name.
func validName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("the name is empty")
	case strings.ContainsAny(name, "/\n") || (strings.Contains(name, `\`) && isWindows):
		return fmt.Errorf("%s contains '/'", ui.Quote(name))
	case name == "." || name == "..":
		return fmt.Errorf("%s is not a valid name", ui.Quote(name))
	}
	return nil
}
