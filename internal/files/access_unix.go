//go:build !windows

package files

import (
	"io/fs"
	"syscall"
)

// access tells whether this user may read, write and run (or, for a folder,
// list) p, asking the system as the shell's tests do.
func access(p string, _ fs.FileInfo) (r, w, x bool) {
	const rOK, wOK, xOK = 4, 2, 1
	return syscall.Access(p, rOK) == nil, syscall.Access(p, wOK) == nil, syscall.Access(p, xOK) == nil
}
