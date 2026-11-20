package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
)

// 8: a local file named like an option reaches adb as a path.
func TestPushDashFile(t *testing.T) {
	dir := pushFiles(t, "-dash.txt")
	t.Chdir(dir)
	abs := filepath.Join(dir, "-dash.txt")
	f := pushFake([]string{}, map[string]adbtest.Response{abs + " /sdcard/Download/-dash.txt": {}})
	r := ak(t, f, "push", "--", "-dash.txt")
	if r.code != 0 {
		t.Errorf("code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
}

func TestPullDashFileIntoCurrentDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := baseFake()
	f.Responses["-s "+serial+" shell "+files.RecentFilesQuery] = adbtest.Response{Stdout: "Row: 0 _data=/storage/emulated/0/Download/-x.txt, date_modified=2, _size=10\n"}
	local := "." + string(filepath.Separator) + "-x.txt" // never "-x.txt": adb would read an option
	f.Responses["-s "+serial+" pull /storage/emulated/0/Download/-x.txt "+local] = adbtest.Response{}
	r := ak(t, f, "pull", "-l")
	if r.code != 0 {
		t.Errorf("code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
}

// 9, 11: a macOS (NFD) name goes to the device as NFC, and one already there
// in either form is a clash.
func TestPushNormalizesNames(t *testing.T) {
	nfd := norm.NFD.String("자모 분리.txt")
	nfc := norm.NFC.String(nfd)
	dir := pushFiles(t, nfd)
	local := filepath.Join(dir, nfd)
	f := pushFake([]string{}, map[string]adbtest.Response{local + " /sdcard/Download/" + nfc: {}})
	if r := ak(t, f, "push", local); r.code != 0 {
		t.Errorf("NFC name: code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
	f = pushFake([]string{nfd}, map[string]adbtest.Response{local + " /sdcard/Download/" + norm.NFC.String("자모 분리 (1).txt"): {}})
	if r := ak(t, f, "push", local); r.code != 0 || !strings.Contains(r.err, "already exists") {
		t.Errorf("NFD there: code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
}

// 10: the hint can be pasted into a shell.
func TestPushDirectoryHintIsQuoted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my folder")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := ak(t, baseFake(), "push", dir)
	if r.code != 1 || !strings.Contains(r.err, "e.g. ak push '"+dir+"'/*") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}
