//go:build !windows

package files

import (
	"os"
	"syscall"
	"testing"
)

func TestCrossDeviceUnix(t *testing.T) {
	if !crossDevice(&os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EXDEV}) {
		t.Error("EXDEV should count as another volume")
	}
	if crossDevice(&os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EACCES}) {
		t.Error("EACCES is not another volume")
	}
}
