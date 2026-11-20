//go:build windows

package files

import (
	"errors"
	"io/fs"
	"syscall"

	"golang.org/x/sys/windows"
)

// access on Windows goes by the read-only attribute: files are readable,
// and there is no execute bit (folders can be listed).
func access(_ string, fi fs.FileInfo) (r, w, x bool) {
	return true, fi.Mode().Perm()&0o200 != 0, fi.IsDir()
}

// crossDevice reports a rename that failed because the two paths are on
// different drives (ERROR_NOT_SAME_DEVICE): the move then copies, checks and
// deletes.
func crossDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) || errors.Is(err, syscall.EXDEV)
}

// copyLink skips a link on Windows: making one needs Developer Mode or admin
// rights, and junctions, common in user folders, count as links, so a copy
// would fail as a whole. Usage does not count links either, so a move's
// size check still matches.
func copyLink(string, string) error { return nil }
