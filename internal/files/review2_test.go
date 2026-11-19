package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
)

// 1: "error:" in a file name is not an adb failure; adb's own error line is.
func TestAdbTransferErrorLines(t *testing.T) {
	src, dst := "/sdcard/build error: log.txt", "/tmp/x/build error: log.txt"
	key := "-s " + serial + " pull " + src + " " + dst
	fakeADB(t, map[string]adbtest.Response{key: {Stdout: src + ": 1 file pulled, 0 skipped. 1.2 MB/s\n"}})
	if err := adbTransfer(context.Background(), serial, "pull", src, dst); err != nil {
		t.Errorf("a name with 'error:': %v", err)
	}
	// Old adb ends with 0 even on errors: its error line still fails.
	fakeADB(t, map[string]adbtest.Response{key: {Stdout: "adb: error: failed to stat remote object '" + src + "': No such file or directory\n"}})
	if err := adbTransfer(context.Background(), serial, "pull", src, dst); err == nil || !strings.HasPrefix(err.Error(), "failed to stat remote object") {
		t.Errorf("adb error line: %v", err)
	}
	fakeADB(t, map[string]adbtest.Response{key: {Stderr: "something broke\n", Err: errors.New("exit status 1")}})
	if err := adbTransfer(context.Background(), serial, "pull", src, dst); err == nil {
		t.Error("a non-zero exit fails")
	}
}

// 3: renaming onto an existing name fails instead of moving into a folder
// or writing over a file.
func TestRenameRefusesAnExistingTarget(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", "b.txt", "folder/")
	l := localFS{PC, "PC"}
	if err := l.Rename(filepath.Join(dir, "a.txt"), filepath.Join(dir, "folder")); err == nil {
		t.Error("onto a folder")
	}
	if err := l.Rename(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")); err == nil || read(t, filepath.Join(dir, "b.txt")) != "b.txt" {
		t.Errorf("onto a file: %v", err)
	}
	if err := l.Rename(filepath.Join(dir, "a.txt"), filepath.Join(dir, "c.txt")); err != nil {
		t.Errorf("to a free name: %v", err)
	}
	// A case-only rename still works (through a temporary name).
	e, _ := l.Stat(filepath.Join(dir, "c.txt"))
	if err := renameEntry(l, e, filepath.Join(dir, "C.TXT")); err != nil {
		t.Errorf("case only: %v", err)
	}

	d := &deviceFS{serial: serial, name: "Test Phone"}
	cmd := "if [ -e '/sdcard/b' ] || [ -L '/sdcard/b' ]; then echo '/sdcard/b: already exists'; exit 1; fi; mv '/sdcard/a' '/sdcard/b' 2>&1 && echo ak-ok"
	fakeADB(t, map[string]adbtest.Response{shellKey(cmd): {Stdout: "/sdcard/b: already exists\n"}})
	if err := d.Rename("/sdcard/a", "/sdcard/b"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("device: %v", err)
	}
}

// 6: going to a path lists it once; a failure keeps the folder.
func TestGotoListsOnce(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "sub/x.txt")
	s.open(s.panes[Device], dev, "")
	counter := &countFS{localFS: localFS{Device, "Test Phone"}}
	s.panes[Device].fs = counter
	press(t, s, ":"+strings.Repeat(back, 300)+filepath.Join(dev, "sub")+enter)
	if counter.lists != 1 || s.panes[Device].dir != filepath.Join(dev, "sub") {
		t.Errorf("lists %d, dir %s", counter.lists, s.panes[Device].dir)
	}
	press(t, s, ":"+strings.Repeat(back, 300)+"/no/such/place"+enter)
	if s.panes[Device].dir != filepath.Join(dev, "sub") || s.msg == nil || !strings.HasPrefix(s.msg.text, "Can't open") {
		t.Errorf("failed go to: dir %s msg %+v", s.panes[Device].dir, s.msg)
	}
	if len(s.panes[Device].rows) == 0 || s.panes[Device].err != nil {
		t.Error("the folder should still be listed")
	}
}

type countFS struct {
	localFS
	lists int
}

func (c *countFS) List(dir string) ([]Entry, error) { c.lists++; return c.localFS.List(dir) }

// 4: List gives the same entries as Stat, with fewer system calls.
func TestLocalListMatchesStat(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", "sub/", "run.sh", "ro.txt")
	_ = os.Chmod(filepath.Join(dir, "run.sh"), 0o755)
	_ = os.Chmod(filepath.Join(dir, "ro.txt"), 0o444)
	_ = os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link"))
	l := localFS{PC, "PC"}
	list, err := l.List(dir)
	if err != nil || len(list) != 5 {
		t.Fatalf("%v %v", list, err)
	}
	for _, e := range list {
		want, _ := l.Stat(e.Path)
		if e.NoRead != want.NoRead || e.ReadOnly != want.ReadOnly || e.Exec != want.Exec || e.Link != want.Link || e.Dir != want.Dir {
			t.Errorf("%s: list %+v, stat %+v", e.Name, e, want)
		}
	}
}

// BenchmarkLocalList lists a folder of 50,000 files.
func BenchmarkLocalList(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 50000; i++ {
		f, _ := os.Create(filepath.Join(dir, "f"+strings.Repeat("0", 6-len(itoa(i)))+itoa(i)))
		_ = f.Close()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := (localFS{PC, "PC"}).List(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func itoa(i int) string {
	s := ""
	for {
		s = string(rune('0'+i%10)) + s
		if i /= 10; i == 0 {
			return s
		}
	}
}
