package files

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"

	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// caseFS compares names as a case-sensitive (Linux, ext4) or a
// case-insensitive (macOS, /sdcard) file system does, whatever the test
// machine's own file system is.
type caseFS struct {
	localFS
	sensitive bool
}

func (c caseFS) Stat(p string) (Entry, error) {
	des, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		return Entry{}, err
	}
	for _, de := range des {
		if de.Name() == filepath.Base(p) || (!c.sensitive && strings.EqualFold(de.Name(), filepath.Base(p))) {
			return c.localFS.Stat(filepath.Join(filepath.Dir(p), de.Name()))
		}
	}
	return Entry{}, fs.ErrNotExist
}

// 4: a name that differs only in case collides only where the file system
// ignores case.
func TestConflictFollowsTheFileSystemsCase(t *testing.T) {
	for _, sensitive := range []bool{true, false} {
		s, dev, pc := testState(t)
		write(t, dev, "report.pdf")
		write(t, pc, "REPORT.pdf")
		s.panes[PC].fs = caseFS{localFS{PC, "PC"}, sensitive}
		s.open(s.panes[Device], dev, "")
		s.open(s.panes[PC], pc, "")
		s.handle(tui.Key{Kind: tui.KeyDown})
		s.send(s.panes[Device], false)
		if got := s.mode == modeConflict; got == sensitive {
			t.Errorf("sensitive=%v: conflict=%v", sensitive, got)
		}
		if !sensitive {
			s.resolve(s.ci, 'r')
			if got := s.plan.ops[0].name; got != "report (1).pdf" {
				t.Errorf("rename: %q", got)
			}
		}
		// New names follow the same rule.
		p := s.panes[PC]
		err := s.checkNewName(p, "report.PDF", "")
		if (err == nil) != sensitive {
			t.Errorf("sensitive=%v: new name report.PDF: %v", sensitive, err)
		}
		if err := s.checkNewName(p, "Report.pdf", "REPORT.pdf"); err != nil {
			t.Errorf("a case-only rename of itself: %v", err)
		}
	}
}

// 5: only a marker line starts the access part, not a name that ends in it.
func TestParseStatAccessMarkerOnItsOwnLine(t *testing.T) {
	out := "regular file|3|1700000000|/sdcard/x" + accessMark + "\n" +
		"regular file|3|1700000000|/sdcard/y.txt\n" +
		accessMark + "\n" +
		"r|/sdcard/x" + accessMark + "\n" +
		"rw|/sdcard/y.txt\n"
	list, err := parseStat(out, "/sdcard")
	if err != nil || len(list) != 2 || list[0].Name != "x"+accessMark || !list[0].ReadOnly || list[1].ReadOnly {
		t.Errorf("%+v %v", list, err)
	}
	if list, _ := parseStat(accessMark+"\n", "/sdcard"); len(list) != 0 {
		t.Errorf("marker first: %+v", list)
	}
}

// 7: canceling the name question keeps the selection.
func TestCanceledQuestionKeepsSelection(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt", "b.txt")
	write(t, pc, "a.txt")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, "*g")
	if s.mode != modeConflict {
		t.Fatal("no question")
	}
	press(t, s, esc)
	if n := len(s.targetsSelected(s.panes[Device])); n != 2 {
		t.Errorf("selection after Esc: %d", n)
	}
	press(t, s, "g"+"S") // copying starts: now the selection goes
	if n := len(s.panes[Device].selected); n != 0 {
		t.Errorf("selection after copying: %d", n)
	}
}

// 9: Usage follows a linked folder.
func TestLocalUsageFollowsALinkedFolder(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "real/a.txt", "real/sub/bb.txt")
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	b, n, err := localFS{PC, "PC"}.Usage(filepath.Join(dir, "link"))
	if err != nil || b != int64(len("real/a.txt")+len("real/sub/bb.txt")) || n != 2 {
		t.Errorf("%d %d %v", b, n, err)
	}
}

// 11: the frame keeps its rows when a copy runs, a question or an input
// shows, so the list does not jump.
func TestFrameRowsStayPut(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt", "b.txt")
	write(t, pc, "a.txt")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	for _, w := range []int{60, 120, 220} {
		s.setSize(w, 30)
		firstRow := func() (int, int) {
			lines := s.frame()
			for i, l := range lines {
				if strings.Contains(ui.Plain(l), "a.txt") {
					return len(lines), i
				}
			}
			return len(lines), -1
		}
		n0, r0 := firstRow()
		check := func(what string) {
			t.Helper()
			if n, r := firstRow(); n != n0 || r != r0 {
				t.Errorf("width %d, %s: %d lines, list at %d; want %d, %d", w, what, n, r, n0, r0)
			}
		}
		s.task = &task{label: "Pulling", route: "x", cancelable: true}
		check("copying")
		s.task = nil
		press(t, s, down+"g")
		check("question")
		press(t, s, esc+"/")
		check("filter input")
		press(t, s, esc+"6")
		check("rename input")
		press(t, s, esc)
		check("normal")
	}
}

// brokenStatFS cannot stat (a device that went away).
type brokenStatFS struct{ localFS }

func (brokenStatFS) Stat(string) (Entry, error) { return Entry{}, errors.New("device offline") }

// 2: freeName gives up on errors other than "not there".
func TestFreeNameFailsOnStatErrors(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, src, "a.txt")
	job := &copyJob{from: localFS{Device, "Test Phone"}, to: brokenStatFS{localFS{PC, "PC"}}, dstDir: dst, ops: []copyOp{
		{src: Entry{Name: "a.txt", Path: filepath.Join(src, "a.txt"), Size: 5}, name: "a.txt", overwrite: true},
	}}
	done := make(chan copyResult, 1)
	go func() { done <- job.run(context.Background(), func(progress) {}) }()
	select {
	case res := <-done:
		if len(res.failed) != 1 || !strings.Contains(res.failed[0].msg, "device offline") {
			t.Errorf("%+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("freeName loops forever on a stat error")
	}
	// A canceled context stops it too.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&copyJob{to: localFS{PC, "PC"}, dstDir: dst}).freeName(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}

// 6: files opened from the device live in the user's cache, and only
// folders older than a day are cleaned up at start.
func TestTempDirCleanup(t *testing.T) {
	if cache, err := os.UserCacheDir(); err == nil && !strings.HasPrefix(tempDir(), cache) {
		t.Errorf("tempDir %s is not under %s", tempDir(), cache)
	}
	dir := t.TempDir()
	write(t, dir, "old/a.jpg", "new/b.jpg")
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old"), old, old); err != nil {
		t.Fatal(err)
	}
	cleanTempDir(dir, time.Now())
	if _, err := os.Stat(filepath.Join(dir, "old")); !os.IsNotExist(err) {
		t.Error("a day-old folder should go")
	}
	if _, err := os.Stat(filepath.Join(dir, "new", "b.jpg")); err != nil {
		t.Error("a recent folder (another ak files may show it) must stay")
	}
}

// 8: progress of a folder on the device reads one du, not find + stat per
// file, and not as often as for a file.
func TestFolderProgressIsCheap(t *testing.T) {
	d := &deviceFS{serial: serial, name: "Test Phone"}
	f := fakeADB(t, map[string]adbtest.Response{
		shellKey("du -s -b '/sdcard/big dir' 2>/dev/null"): {Stdout: "123456\t/sdcard/big dir\n"},
	})
	if n, err := d.progressSize("/sdcard/big dir"); err != nil || n != 123456 {
		t.Errorf("%d %v", n, err)
	}
	if len(f.Calls) != 1 {
		t.Errorf("calls %q", f.Calls)
	}
	if got := pollEvery(usage{1 << 20, 1}); got != pollInterval {
		t.Errorf("a file: %v", got)
	}
	if a, b := pollEvery(usage{1 << 30, 50}), pollEvery(usage{1 << 30, 50000}); a < time.Second || b <= a || b > 3*time.Second {
		t.Errorf("folders: %v, %v", a, b)
	}
}
