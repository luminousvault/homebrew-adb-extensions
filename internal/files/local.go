package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

var isWindows = runtime.GOOS == "windows"

// computerLabel names this computer on screen: "Computer", the word adb's
// own help uses for it ("PC" would read as Windows only).
const computerLabel = "Computer"

// localFS is this computer's file system. side and label are PC and "Computer"; tests
// also use a localFS in place of the device.
type localFS struct {
	side  Side
	label string
}

func (l localFS) Side() Side                 { return l.side }
func (l localFS) Label() string              { return l.label }
func (localFS) Join(dir, name string) string { return filepath.Join(dir, name) }
func (localFS) Dir(p string) string          { return filepath.Dir(p) }
func (localFS) Base(p string) string         { return filepath.Base(p) }

func (l localFS) List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return nil, pe.Err
		}
		return nil, err
	}
	list := make([]Entry, 0, len(des))
	for _, de := range des {
		// The listing already tells a link apart: no Lstat per entry.
		e, err := statEntry(filepath.Join(dir, de.Name()), de.Type()&fs.ModeSymlink != 0)
		if err != nil {
			continue // a broken symlink, or removed meanwhile
		}
		list = append(list, e)
	}
	return list, nil
}

// Stat follows symlinks, so a linked folder opens like a folder.
func (localFS) Stat(p string) (Entry, error) {
	li, err := os.Lstat(p)
	if err != nil {
		return Entry{}, err
	}
	return statEntry(p, li.Mode()&os.ModeSymlink != 0)
}

// statEntry reads p (following a link) and what this user may do with it.
func statEntry(p string, link bool) (Entry, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Name: filepath.Base(p), Path: p, Dir: fi.IsDir(), Size: fi.Size(), ModTime: fi.ModTime(), Link: link}
	r, w, x := access(p, fi)
	e.NoRead = !r || (e.Dir && !x)
	e.ReadOnly = !e.NoRead && !w
	e.Exec = !e.Dir && x
	if e.Dir {
		e.Size = 0
	}
	return e, nil
}

func (localFS) Mkdir(p string) error { return os.Mkdir(p, 0o755) }

// Remove deletes p permanently. ak files asks "This cannot be undone" first.
// It is the one place a move to the Trash would go (files-design.md §10).
func (localFS) Remove(p string) error { return deleteLocal(p) }

// deleteLocal deletes p and everything under it, permanently.
func deleteLocal(p string) error { return os.RemoveAll(p) }

// Rename renames p within its folder.
// Rename renames p within its folder. It fails when to exists: os.Rename
// writes over a file on Unix (and, on a case-insensitive disk, a name that
// differs only in case is the same file; renameEntry goes through a free
// temporary name for that).
func (localFS) Rename(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s: %w", filepath.Base(to), errExists)
	}
	return os.Rename(from, to)
}

// copyLocal copies a file or folder between two local folders (the computer;
// in tests a local folder stands in for the device). The item itself is
// followed if it is a link, as the list shows a linked folder as a folder;
// links inside are copied as links, never followed: a link back up would
// loop, and Usage, which the copy is checked against, does not follow them
// either (on Windows, where making a link needs extra rights and junctions
// count as links, they are skipped). Other special files (sockets, pipes) are
// skipped. On failure it
// leaves what it wrote; the caller removes its own copy.
func copyLocal(ctx context.Context, from, to string) error {
	fi, err := os.Stat(from)
	if err != nil {
		return err
	}
	return copyTree(ctx, from, to, fi)
}

func copyTree(ctx context.Context, from, to string, fi fs.FileInfo) error {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return copyLink(from, to)
	case fi.Mode().IsRegular():
		return copyFile(ctx, from, to, fi.Mode().Perm())
	case !fi.IsDir():
		return nil // a socket, a pipe, a device: nothing to copy
	}
	if err := os.Mkdir(to, fi.Mode().Perm()|0o700); err != nil {
		return err
	}
	des, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, de := range des {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, err := de.Info() // Lstat: a link stays a link
		if err != nil {
			return err
		}
		if err := copyTree(ctx, filepath.Join(from, de.Name()), filepath.Join(to, de.Name()), info); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(ctx context.Context, from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	err = copyChunks(ctx, out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// copyChunks copies in 1 MB chunks, stopping when ctx is canceled.
func copyChunks(ctx context.Context, out io.Writer, in io.Reader) error {
	buf := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// Usage sums the regular files at or under p.
func (localFS) Usage(p string) (bytes, files int64, err error) {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real // WalkDir does not follow a linked root
	}
	err = filepath.WalkDir(p, func(_ string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.Type().IsRegular() {
			fi, err := de.Info()
			if err != nil {
				return err
			}
			bytes += fi.Size()
			files++
		}
		return nil
	})
	if err != nil {
		return -1, -1, err
	}
	return bytes, files, nil
}
