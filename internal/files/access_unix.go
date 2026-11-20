//go:build !windows

package files

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// access tells whether this user may read, write and run (or, for a folder,
// list) p, asking the system as the shell's tests do. It asks for the usual
// answer in one call (a folder: rwx; a file: rw, then x) and only asks bit
// by bit when that fails: a big folder lists with fewer calls, same result.
func access(p string, fi fs.FileInfo) (r, w, x bool) {
	const rOK, wOK, xOK = 4, 2, 1
	ok := func(mode uint32) bool { return syscall.Access(p, mode) == nil }
	if fi.IsDir() && ok(rOK|wOK|xOK) {
		return true, true, true
	}
	if ok(rOK | wOK) {
		r, w = true, true
	} else {
		r, w = ok(rOK), ok(wOK)
	}
	return r, w, ok(xOK)
}

// crossDevice reports a rename that failed because the two paths are on
// different volumes: the move then copies, checks and deletes.
func crossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }

// copyLink copies the link from as a link at to.
func copyLink(from, to string) error {
	target, err := os.Readlink(from)
	if err != nil {
		return err
	}
	return os.Symlink(target, to)
}
