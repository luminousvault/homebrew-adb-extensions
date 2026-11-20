//go:build windows

package files

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A rename to another drive fails with ERROR_NOT_SAME_DEVICE: the move must
// fall back to copy, check and delete.
func TestCrossDeviceWindows(t *testing.T) {
	err := &os.LinkError{Op: "rename", Old: `C:\a`, New: `D:\a`, Err: windows.ERROR_NOT_SAME_DEVICE}
	if !crossDevice(err) {
		t.Error("ERROR_NOT_SAME_DEVICE should count as another volume")
	}
}

// On Windows a link inside a copied folder is skipped, not recreated.
func TestCopyLinkSkippedWindows(t *testing.T) {
	to := t.TempDir() + `\link`
	if err := copyLink(`C:\nowhere`, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(to); !os.IsNotExist(err) {
		t.Errorf("a link was made: %v", err)
	}
}
