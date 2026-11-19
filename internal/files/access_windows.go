//go:build windows

package files

import "io/fs"

// access on Windows goes by the read-only attribute: files are readable,
// and there is no execute bit (folders can be listed).
func access(_ string, fi fs.FileInfo) (r, w, x bool) {
	return true, fi.Mode().Perm()&0o200 != 0, fi.IsDir()
}
