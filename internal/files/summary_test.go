package files

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
	"golang.org/x/text/unicode/norm"
)

// 1: renames, new folders and canceled pastes are in the summary.
func TestSummaryRenameMkdirCancel(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	press(t, s, down+"r"+back+back+back+back+back+"b.txt"+enter)
	press(t, s, "nnew"+enter)
	press(t, s, tab+"nx/y"+enter) // refused input: a message, not a summary line
	want := []string{"Renamed 'a.txt' to 'b.txt' on Test Phone", "Made folder 'new' in Test Phone:" + dev}
	if strings.Join(s.summary.Done, "|") != strings.Join(want, "|") {
		t.Errorf("done %q", s.summary.Done)
	}
	if len(s.summary.Failed) != 0 {
		t.Errorf("failed %q", s.summary.Failed)
	}

	// A mkdir the storage refuses is a failure.
	if err := os.Chmod(pc, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pc, 0o755) })
	press(t, s, esc+"nblocked"+enter)
	if os.Getuid() != 0 && (len(s.summary.Failed) != 1 || !strings.HasPrefix(s.summary.Failed[0], "Couldn't make folder 'blocked' in ")) {
		t.Errorf("failed %q", s.summary.Failed)
	}
	_ = os.Chmod(pc, 0o755)

	// Canceled paste: what finished, and the cancel.
	job := &copyJob{from: s.panes[Device].fs, to: s.panes[PC].fs, dstDir: pc}
	s.finishSend(job, false, copyResult{done: []Entry{{Name: "b.txt"}}, doneAs: []string{"b.txt"}, canceled: true, todo: 3})
	to := displayPath(s.panes[PC].fs, pc)
	n := len(s.summary.Done)
	if got := s.summary.Done[n-1]; got != "Pulled 'b.txt' from Test Phone → "+to {
		t.Errorf("done %q", s.summary.Done)
	}
	if want := "Canceled pulling from Test Phone → " + to + ": 1 of 3 pulled"; len(s.summary.Canceled) != 1 || s.summary.Canceled[0] != want {
		t.Errorf("canceled %q, want %q", s.summary.Canceled, want)
	}
	job = &copyJob{from: s.panes[PC].fs, to: s.panes[Device].fs, dstDir: dev}
	s.finishSend(job, true, copyResult{canceled: true, todo: 2})
	if want := "Canceled pushing to Test Phone:" + dev + " ← Computer: 0 of 2 pushed"; s.summary.Canceled[1] != want {
		t.Errorf("canceled %q, want %q", s.summary.Canceled[1], want)
	}
}

// 2: a pasted item saved under a new name shows it.
func TestSummaryShowsNewNames(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "KEEP.TXT", "x.txt", "y.txt")
	write(t, pc, "KEEP.TXT")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, "\x1b[F"+"g")        // y.txt: no conflict
	press(t, s, home+down+"g"+enter) // KEEP.TXT -> rename
	last := s.summary.Done[len(s.summary.Done)-1]
	if !strings.HasPrefix(last, "Pulled 'KEEP.TXT' as 'KEEP (1).TXT' from Test Phone → ") {
		t.Errorf("summary %q", last)
	}
	if !strings.Contains(s.msg.text, "'KEEP.TXT' as 'KEEP (1).TXT'") {
		t.Errorf("message %q", s.msg.text)
	}
	press(t, s, "ag"+"R") // two conflicts, all renamed
	last = s.summary.Done[len(s.summary.Done)-1]
	if !strings.HasPrefix(last, "Pulled 3 files (2 renamed) from ") {
		t.Errorf("summary %q", last)
	}
}

// 3: q answers no in the conflict and delete questions; a second q quits.
func TestQCancelsQuestions(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt")
	write(t, pc, "a.txt")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, down+"g")
	if s.mode != modeConflict || !strings.Contains(ui.Plain(s.modeKeys()), "q") {
		t.Fatalf("mode %d help %q", s.mode, ui.Plain(s.modeKeys()))
	}
	press(t, s, "q")
	if s.mode != modeNormal || s.exit != exitNone || s.msg.text != "Pull canceled" {
		t.Fatalf("q in conflict: mode %d exit %d %+v", s.mode, s.exit, s.msg)
	}
	press(t, s, down+"d")
	if s.mode != modeDelete || !strings.Contains(ui.Plain(s.modeKeys()), "q") {
		t.Fatalf("mode %d help %q", s.mode, ui.Plain(s.modeKeys()))
	}
	press(t, s, "q")
	if s.mode != modeNormal || s.exit != exitNone {
		t.Fatal("q in delete should only cancel")
	}
	if _, err := os.Stat(filepath.Join(dev, "a.txt")); err != nil {
		t.Fatal("deleted")
	}
	press(t, s, "q")
	if s.exit != exitQuit {
		t.Fatal("a second q quits")
	}
}

// 4: deleting device folders also drops their MediaStore rows, in one call.
func TestDeviceFolderDeleteForgetsMedia(t *testing.T) {
	d := &deviceFS{serial: serial, name: "Test Phone"}
	rm := func(p string) string {
		q := ShellQuote(p)
		return shellKey("[ -d " + q + " ] && echo ak-dir; rm -rf " + q + " 2>&1; [ ! -e " + q + " ] && [ ! -L " + q + " ] && echo ak-ok")
	}
	where := `_data='/storage/emulated/0/Download/it''s_50%' OR _data LIKE '/storage/emulated/0/Download/it''s\_50\%/%' ESCAPE '\'` +
		` OR _data='/storage/emulated/0/DCIM/x' OR _data LIKE '/storage/emulated/0/DCIM/x/%' ESCAPE '\'`
	f := fakeADB(t, map[string]adbtest.Response{
		rm("/sdcard/Download/it's_50%"):  {Stdout: "ak-dir\nak-ok\n"},
		rm("/storage/emulated/0/DCIM/x"): {Stdout: "ak-dir\nak-ok\n"},
		rm("/sdcard/a.txt"):              {Stdout: "ak-ok\n"},
		rm("/data/local/tmp/d"):          {Stdout: "ak-dir\nak-ok\n"}, // not shared storage: no MediaStore
		shellKey("content delete --uri content://media/external/file --where " + ShellQuote(where)): {},
	})
	for _, p := range []string{"/sdcard/Download/it's_50%", "/storage/emulated/0/DCIM/x", "/sdcard/a.txt", "/data/local/tmp/d"} {
		if err := d.Remove(p); err != nil {
			t.Fatal(p, err)
		}
	}
	flushMedia(d)
	flushMedia(d) // nothing left: no second call
	n := 0
	for _, c := range f.Calls {
		if strings.Contains(c, "content delete") {
			n++
			if !strings.Contains(c, ShellQuote(where)) {
				t.Errorf("call %s", c)
			}
		}
	}
	if n != 1 {
		t.Errorf("content delete calls: %d (%q)", n, f.Calls)
	}
	// A failed cleanup does not fail the delete.
	f.Responses[rm("/sdcard/y")] = adbtest.Response{Stdout: "ak-dir\nak-ok\n"}
	if err := d.Remove("/sdcard/y"); err != nil {
		t.Fatal(err)
	}
	flushMedia(d) // no canned response: the fake fails; only logged
	_ = context.Background()
}

// A name that only changed from NFD to NFC on the way is not "renamed".
func TestNormalizedNameIsNotRenamed(t *testing.T) {
	nfd := norm.NFD.String("한글 문서.txt")
	got := pastedItems([]Entry{{Name: nfd}}, []string{norm.NFC.String(nfd)})
	if got != "'"+norm.NFC.String(nfd)+"'" {
		t.Errorf("got %q", got)
	}
	got = pastedItems([]Entry{{Name: nfd}}, []string{"한글 문서 (1).txt"})
	if !strings.Contains(got, "' as '한글 문서 (1).txt'") {
		t.Errorf("a real rename: %q", got)
	}
}

// A summary line that covers several items lists them under it, dim, side
// by side over at most two rows joined with ├─ and └─, then a count.
func TestSummaryListsItems(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt", "sub/c.txt")
	s.open(s.panes[Device], dev, "")
	press(t, s, "a8y")
	if len(s.summary.Done) != 1 || len(s.summary.Items) != 1 {
		t.Fatalf("summary %+v", s.summary)
	}
	if got := strings.Join(s.summary.Items[0], ","); got != "sub/,a.txt,b.txt" { // screen order: folders first
		t.Errorf("items %q", got)
	}
	got := captureStderr(t, s.summary.Print)
	if !strings.Contains(got, "Deleted 3 items") || !strings.Contains(got, "  └─ sub/  a.txt  b.txt\n") {
		t.Errorf("printed %q", got)
	}
	if itemNames([]Entry{{Name: "a.txt"}}, []string{"a (1).txt"})[0] != "a.txt → a (1).txt" {
		t.Error("renamed item")
	}
}

// packItems fills at most two rows of the given width and counts the rest;
// names with two spaces in them stay whole.
func TestPackItems(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("file%02d.png", i)
	}
	rows := packItems(names, 40, 2)
	if len(rows) != 2 {
		t.Fatalf("rows %q", rows)
	}
	for _, r := range rows {
		if ui.TextWidth(r) > 40 {
			t.Errorf("row too wide: %q", r)
		}
	}
	if !strings.HasPrefix(rows[0], "file00.png  file01.png") || !strings.Contains(rows[1], "… and ") {
		t.Errorf("rows %q", rows)
	}
	shown := strings.Count(rows[0]+rows[1], "file")
	if want := fmt.Sprintf("… and %d more", 30-shown); !strings.HasSuffix(rows[1], want) {
		t.Errorf("count: %q, want suffix %q", rows[1], want)
	}
	if got := packItems([]string{"a b.txt", "  lead.txt"}, 40, 2); len(got) != 1 || got[0] != "a b.txt    lead.txt" {
		t.Errorf("two spaces in a name: %q", got)
	}
	if got := packItems([]string{"x"}, 40, 2); len(got) != 1 || got[0] != "x" {
		t.Errorf("one: %q", got)
	}
}

// captureStderr runs f with ui.Err going to a buffer (no color).
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	var b bytes.Buffer
	old := ui.Err
	ui.Err = &ui.Stream{W: &b}
	defer func() { ui.Err = old }()
	f()
	return b.String()
}
