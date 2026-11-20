package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// cd moves pane p into dir (a test shortcut for Enter on a folder).
func cd(s *state, side Side, dir string) { s.open(s.panes[side], dir, "") }

// find puts the cursor of the active pane on name.
func find(t *testing.T, s *state, name string) {
	t.Helper()
	p := s.cur()
	for i, e := range p.rows {
		if e.Name == name {
			p.cursor = i
			return
		}
	}
	t.Fatalf("%s not in %q", name, names(p))
}

func TestClipboardTakeKeepReplaceClear(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "one/a.txt", "one/b.txt", "two/c.txt")
	cd(s, Device, filepath.Join(dev, "one"))
	press(t, s, "a"+"3") // select all, copy
	if s.clip == nil || s.clip.cut || len(s.clip.items) != 2 || len(s.cur().selected) != 0 {
		t.Fatalf("copy: %+v, selected %d", s.clip, len(s.cur().selected))
	}
	s.setSize(400, 30)
	if got := ui.Plain(s.clipLine()); !strings.Contains(got, "Copy 2 files (") || !strings.Contains(got, "from Test Phone:"+filepath.Join(dev, "one")) || !strings.Contains(got, "6 Paste") {
		t.Errorf("clipboard line %q", got)
	}
	s.setSize(80, 30) // narrow: the folder gives way, the keys stay
	if got := ui.Plain(s.clipLine()); ui.TextWidth(got) > 80 || !strings.HasSuffix(got, "6 Paste  Esc Clear") {
		t.Errorf("narrow clipboard line %q", got)
	}
	cd(s, Device, filepath.Join(dev, "two")) // another folder: kept
	if s.clip == nil {
		t.Fatal("the clipboard should stay across folders")
	}
	find(t, s, "c.txt")
	press(t, s, "x") // replaced by a cut
	if !s.clip.cut || len(s.clip.items) != 1 || s.clip.items[0].Name != "c.txt" {
		t.Fatalf("cut: %+v", s.clip)
	}
	if got := ui.Plain(s.clipLine()); !strings.HasPrefix(strings.TrimSpace(got), "Cut 'c.txt' (") {
		t.Errorf("clipboard line %q", got)
	}
	press(t, s, "/c"+enter+esc) // Esc: the filter first
	if s.cur().filter != "" || s.clip == nil {
		t.Fatalf("first Esc: filter %q, clip %v", s.cur().filter, s.clip)
	}
	s.task = &task{label: "Copying", cancelable: true}
	press(t, s, esc) // a running task before anything
	if !s.canceled || s.clip == nil {
		t.Fatal("Esc should cancel the task and keep the clipboard")
	}
	s.task, s.canceled = nil, false
	press(t, s, esc)
	if s.clip != nil {
		t.Error("then Esc empties the clipboard")
	}
}

// Copy and cut, pasted across (device ↔ computer) and within each side.
func TestPasteFourWays(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "d/photo.jpg", "d/movie.mp4", "x/")
	write(t, pc, "p/doc.txt", "p/notes.txt", "q/")
	cd(s, Device, filepath.Join(dev, "d"))
	cd(s, PC, filepath.Join(pc, "p"))

	// Device → computer, copy: the clipboard stays.
	find(t, s, "photo.jpg")
	press(t, s, "c"+tab+"v")
	if read(t, filepath.Join(pc, "p", "photo.jpg")) != "d/photo.jpg" || s.clip == nil {
		t.Fatalf("pull copy: %+v", s.msg)
	}
	// Computer → device, cut: the original goes, the clipboard empties.
	find(t, s, "doc.txt")
	press(t, s, "x"+tab+"v")
	if read(t, filepath.Join(dev, "d", "doc.txt")) != "p/doc.txt" || s.clip != nil {
		t.Fatalf("push move: %+v", s.msg)
	}
	if _, err := os.Stat(filepath.Join(pc, "p", "doc.txt")); !os.IsNotExist(err) {
		t.Error("a move across keeps no original")
	}
	// Within the device: copy into another folder, then move.
	find(t, s, "movie.mp4")
	press(t, s, "c")
	cd(s, Device, filepath.Join(dev, "x"))
	press(t, s, "v")
	if read(t, filepath.Join(dev, "x", "movie.mp4")) != "d/movie.mp4" || read(t, filepath.Join(dev, "d", "movie.mp4")) != "d/movie.mp4" {
		t.Fatalf("copy within: %+v", s.msg)
	}
	cd(s, Device, filepath.Join(dev, "d"))
	find(t, s, "photo.jpg")
	press(t, s, "x")
	cd(s, Device, filepath.Join(dev, "x"))
	press(t, s, "v")
	if read(t, filepath.Join(dev, "x", "photo.jpg")) != "d/photo.jpg" {
		t.Fatalf("move within: %+v", s.msg)
	}
	if _, err := os.Stat(filepath.Join(dev, "d", "photo.jpg")); !os.IsNotExist(err) {
		t.Error("a move within keeps no original")
	}
	// Within the computer.
	press(t, s, tab)
	find(t, s, "notes.txt")
	press(t, s, "x")
	cd(s, PC, filepath.Join(pc, "q"))
	press(t, s, "v")
	if read(t, filepath.Join(pc, "q", "notes.txt")) != "p/notes.txt" {
		t.Fatalf("move on the computer: %+v", s.msg)
	}
	var lines []string
	lines = append(lines, s.summary.Done...)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Copied 'photo.jpg' from Test Phone → ", "Moved 'doc.txt' to ", "Copied 'movie.mp4' to Test Phone:", "Moved 'notes.txt' to "} {
		if !strings.Contains(joined, want) {
			t.Errorf("summary lacks %q:\n%s", want, joined)
		}
	}
}

func TestPasteSameFolderAndIntoItself(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "box/inner/")
	if err := os.Symlink(filepath.Join(dev, "box"), filepath.Join(dev, "alias")); err != nil {
		t.Fatal(err)
	}
	cd(s, Device, dev)
	find(t, s, "a.txt")
	press(t, s, "c"+"v") // a copy next to the original
	if read(t, filepath.Join(dev, "a (1).txt")) != "a.txt" {
		t.Fatalf("same folder copy: %+v", s.msg)
	}
	press(t, s, "x"+"v") // a cut to where it is: nothing
	if s.msg == nil || !strings.Contains(s.msg.text, "nothing to move") || read(t, filepath.Join(dev, "a.txt")) != "a.txt" {
		t.Errorf("same folder cut: %+v", s.msg)
	}
	find(t, s, "box")
	press(t, s, "c")
	cd(s, Device, filepath.Join(dev, "box", "inner"))
	press(t, s, "v")
	if s.msg == nil || !strings.Contains(s.msg.text, "into itself") {
		t.Errorf("into itself: %+v", s.msg)
	}
	cd(s, Device, filepath.Join(dev, "alias", "inner")) // the same folder through a link
	press(t, s, "v")
	if s.msg == nil || !strings.Contains(s.msg.text, "into itself") {
		t.Errorf("into itself through a link: %+v", s.msg)
	}
}

// A clash asks; overwrite and cancel keep the original and the existing item
// as they should.
func TestPasteClashKeepsData(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt", "b.txt")
	write(t, pc, "a.txt", "b.txt")
	if err := os.WriteFile(filepath.Join(pc, "b.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	cd(s, Device, dev)
	cd(s, PC, pc)
	find(t, s, "a.txt")
	press(t, s, "x"+tab+"v")
	if s.mode != modeConflict {
		t.Fatal("a clash should ask")
	}
	press(t, s, esc)
	if read(t, filepath.Join(dev, "a.txt")) != "a.txt" || s.clip == nil {
		t.Error("a canceled paste keeps the original and the clipboard")
	}
	press(t, s, tab)
	find(t, s, "b.txt")
	press(t, s, "x"+tab+"v"+"o") // move over an existing file: replaced, original gone
	if read(t, filepath.Join(pc, "b.txt")) != "b.txt" {
		t.Errorf("overwrite: %q", read(t, filepath.Join(pc, "b.txt")))
	}
	if _, err := os.Stat(filepath.Join(dev, "b.txt")); !os.IsNotExist(err) {
		t.Error("the moved original should be gone")
	}
	assertNoTemp(t, pc)
}

// A folder whose size can't be measured is copied but not deleted.
func TestCutWithUnknownSizeKeepsOriginal(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, src, "d/x.txt")
	job := &copyJob{from: noUsageFS{localFS{Device, "Test Phone"}}, to: localFS{PC, "PC"}, dstDir: dst, cut: true,
		ops: []copyOp{{src: Entry{Name: "d", Path: filepath.Join(src, "d"), Dir: true}, name: "d"}}}
	res := job.run(context.Background(), func(progress) {})
	if len(res.failed) != 1 || res.failed[0].msg != errNoSize.Error() {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(src, "d", "x.txt")); err != nil {
		t.Error("the original must stay")
	}
	if _, err := os.Stat(filepath.Join(dst, "d")); !os.IsNotExist(err) {
		t.Error("the unchecked copy should go")
	}
}

type noUsageFS struct{ localFS }

func (noUsageFS) Usage(string) (int64, int64, error) { return -1, -1, errors.New("no sizes") }

// Overwriting a folder that holds the item pasted is refused.
func TestPasteOverAncestorRefused(t *testing.T) {
	root := t.TempDir()
	write(t, root, "A/A/keep.txt", "A/sibling.txt")
	f := localFS{Device, "Test Phone"}
	src, _ := f.Stat(filepath.Join(root, "A", "A"))
	for _, cut := range []bool{false, true} {
		job := &copyJob{from: f, to: f, dstDir: root, cut: cut,
			ops: []copyOp{{src: src, name: "A", overwrite: true, dstDir: true, existing: filepath.Join(root, "A")}}}
		res := job.run(context.Background(), func(progress) {})
		if len(res.failed) != 1 || read(t, filepath.Join(root, "A", "sibling.txt")) != "A/sibling.txt" ||
			read(t, filepath.Join(root, "A", "A", "keep.txt")) != "A/A/keep.txt" {
			t.Fatalf("cut=%v: %+v", cut, res)
		}
	}
}

func TestPasteBlockedInRecentView(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	cd(s, Device, dev)
	find(t, s, "a.txt")
	press(t, s, "c")
	p := s.cur()
	p.recent, p.recentList = true, []Entry{}
	s.refreshRows(p, "")
	press(t, s, "v")
	if s.msg == nil || !strings.Contains(s.msg.text, "recent files") || s.pending != nil {
		t.Errorf("paste in the recent view: %+v", s.msg)
	}
}

// The clipboard row is there, blank or not: the list does not move.
func TestClipboardRowKeepsRows(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt")
	cd(s, Device, dev)
	for _, w := range []int{60, 120, 220} {
		s.setSize(w, 30)
		s.clip = nil
		before := s.frame()
		s.clip = &clipboard{side: Device, items: s.cur().entries[:1]}
		after := s.frame()
		if len(before) != len(after) {
			t.Errorf("width %d: %d lines, then %d", w, len(before), len(after))
		}
		for i := range before {
			if strings.Contains(ui.Plain(before[i]), "a.txt") != strings.Contains(ui.Plain(after[i]), "a.txt") && i < len(before)-6 {
				t.Errorf("width %d: line %d moved", w, i)
			}
		}
	}
}

// Keys that do nothing keep their labels, dim, and do nothing.
func TestDisabledKeysDim(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	cd(s, Device, dev)
	old := ui.Err.Color
	ui.Err.Color = true
	defer func() { ui.Err.Color = old }()
	raw := s.bottomBar(200, 1)[0]
	if !strings.Contains(raw, style.Dim+"6"+style.NC+" "+style.Dim+"Paste") || strings.Contains(ui.Plain(raw), "Paste (v)") {
		t.Errorf("paste with an empty clipboard: %q", raw)
	}
	if strings.Contains(ui.Plain(raw), "×") {
		t.Error("with color, dim says it; no ×")
	}
	press(t, s, "v")
	if s.pending != nil || s.plan != nil {
		t.Error("a dim key does nothing")
	}
}

// recentFS is a device stand-in with a recent list.
type recentFS struct {
	localFS
	list *[]Entry
}

func (r recentFS) Recent() ([]Entry, error) { return *r.list, nil }

// Rename works in the recent view: the file, the list and the folder view
// follow.
func TestRenameInRecentView(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "pics/a.txt", "pics/b.txt")
	list := []Entry{
		{Name: "a.txt", Path: filepath.Join(dev, "pics", "a.txt"), Size: 10, Folder: filepath.Join(dev, "pics")},
		{Name: "b.txt", Path: filepath.Join(dev, "pics", "b.txt"), Size: 10, Folder: filepath.Join(dev, "pics")},
	}
	s.panes[Device].fs = recentFS{localFS{Device, "Test Phone"}, &list}
	cd(s, Device, filepath.Join(dev, "pics"))
	press(t, s, "2") // recent view
	p := s.cur()
	if !p.recent || len(p.rows) != 2 {
		t.Fatalf("recent: %q", names(p))
	}
	find(t, s, "a.txt")
	press(t, s, "0"+strings.Repeat(back, 10)+"b.txt"+enter) // b.txt is in the folder
	if s.msg == nil || !strings.Contains(s.msg.text, "already exists") {
		t.Fatalf("clash: %+v", s.msg)
	}
	press(t, s, "0"+strings.Repeat(back, 10)+"new.txt"+enter)
	if read(t, filepath.Join(dev, "pics", "new.txt")) != "pics/a.txt" {
		t.Fatalf("renamed: %+v", s.msg)
	}
	if p.rows[p.cursor].Name != "new.txt" || !strings.HasSuffix(p.rows[p.cursor].Path, "new.txt") {
		t.Errorf("cursor and list: %q", names(p))
	}
	press(t, s, "0"+strings.Repeat(back, 10)+"NEW.TXT"+enter) // case only
	if _, err := os.Stat(filepath.Join(dev, "pics", "NEW.TXT")); err != nil || p.rows[p.cursor].Name != "NEW.TXT" {
		t.Errorf("case only: %v %+v", err, s.msg)
	}
	press(t, s, "2") // back to the folder: the new name
	if !strings.Contains(names(s.cur()), "NEW.TXT") || strings.Contains(names(s.cur()), "a.txt") {
		t.Errorf("folder view: %q", names(s.cur()))
	}
}

// The recent view's sort lasts one visit: it opens newest first again.
func TestRecentViewOpensNewestFirst(t *testing.T) {
	s, dev, _ := testState(t)
	list := []Entry{{Name: "a.txt", Path: filepath.Join(dev, "a.txt"), Size: 10, Folder: dev}}
	s.panes[Device].fs = recentFS{localFS{Device, "Test Phone"}, &list}
	press(t, s, "2")
	p := s.cur()
	press(t, s, "1") // newest -> size
	if p.recentSort != sortSize {
		t.Fatalf("sort %v", p.recentSort)
	}
	press(t, s, "2") // the folder
	press(t, s, "2") // recent again
	if p.recentSort != sortNewest {
		t.Errorf("reopened with %v", p.recentSort)
	}
}

// On the device a copy within is cp -r, and paths resolve in one call.
func TestDeviceCopyWithinAndRealPaths(t *testing.T) {
	d := &deviceFS{serial: serial, name: "Test Phone"}
	old := cpPidFile
	cpPidFile = func() string { return "/data/local/tmp/.ak-cp-1" }
	t.Cleanup(func() { cpPidFile = old })
	pf := "'/data/local/tmp/.ak-cp-1'"
	f := fakeADB(t, map[string]adbtest.Response{
		shellKey("cp -r '/sdcard/a b' '/sdcard/x/a b' 2>&1 & p=$!; echo $p > " + pf + "; wait $p; r=$?; rm -f " + pf + "; [ $r -eq 0 ] && echo ak-ok"): {Stdout: "ak-ok\n"},
	})
	if err := d.copyWithin(context.Background(), "/sdcard/a b", "/sdcard/x/a b"); err != nil {
		t.Errorf("cp: %v (%q)", err, f.Calls)
	}
	// Canceled: the cp on the device is killed and waited for before the
	// caller removes the half copy.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.copyWithin(ctx, "/sdcard/a b", "/sdcard/x/a b"); err == nil {
		t.Error("a canceled copy should fail")
	}
	killed := false
	for _, c := range f.Calls {
		if strings.Contains(c, "kill $p") && strings.Contains(c, pf) {
			killed = true
		}
	}
	if !killed {
		t.Errorf("cp not stopped on cancel: %q", f.Calls)
	}
	cmd := "if [ -d '/sdcard/x' ]; then (cd '/sdcard/x' 2>/dev/null && pwd -P) || echo; else d=$(cd '/sdcard' 2>/dev/null && pwd -P) && echo \"$d/\"'x' || echo; fi; " +
		"if [ -d '/sdcard/x/y' ]; then (cd '/sdcard/x/y' 2>/dev/null && pwd -P) || echo; else d=$(cd '/sdcard/x' 2>/dev/null && pwd -P) && echo \"$d/\"'y' || echo; fi; "
	f.Responses[shellKey(cmd)] = adbtest.Response{Stdout: "/storage/emulated/0/x\n/storage/emulated/0/x/y\n"}
	if !holds(d, "/sdcard/x", "/sdcard/x/y") {
		t.Errorf("holds: calls %q", f.Calls)
	}
}

// Deleting items that are on the clipboard takes them off it (and clears it
// when none is left).
func TestDeleteTakesItemsOffClipboard(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt", "c.txt")
	s.open(s.panes[Device], dev, "")
	press(t, s, "a3") // copy all three
	if s.clip == nil || len(s.clip.items) != 3 {
		t.Fatalf("clipboard %+v", s.clip)
	}
	find(t, s, "b.txt")
	press(t, s, "8y")
	if s.clip == nil || len(s.clip.items) != 2 {
		t.Fatalf("after deleting one: %+v", s.clip)
	}
	press(t, s, "a8y")
	if s.clip != nil {
		t.Errorf("all deleted: clipboard should be empty, got %+v", s.clip)
	}
}

// A rename moves clipboard items with it: the item itself, or items inside a
// renamed folder.
func TestRenameFollowsClipboard(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "box/in.txt")
	s.open(s.panes[Device], dev, "")
	find(t, s, "a.txt")
	press(t, s, "3")
	press(t, s, "0")
	s.inputText, s.inputCursor = "b.txt", len("b.txt")
	press(t, s, enter)
	if s.clip == nil || s.clip.items[0].Path != filepath.Join(dev, "b.txt") || s.clip.items[0].Name != "b.txt" {
		t.Fatalf("clipboard after renaming the item: %+v", s.clip)
	}
	cd(s, Device, filepath.Join(dev, "box"))
	find(t, s, "in.txt")
	press(t, s, "3")
	cd(s, Device, dev)
	find(t, s, "box")
	press(t, s, "0")
	s.inputText, s.inputCursor = "crate", len("crate")
	press(t, s, enter)
	if s.clip == nil || s.clip.items[0].Path != filepath.Join(dev, "crate", "in.txt") || s.clip.items[0].Name != "in.txt" {
		t.Errorf("clipboard after renaming its folder: %+v", s.clip)
	}
}

// The clipboard row dims 6 Paste where it does nothing (the recent view) and
// Esc Clear while a filter would take Esc first.
func TestClipLineDimsUnusableKeys(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.panes[Device], dev, "")
	find(t, s, "a.txt")
	press(t, s, "3")
	if l := s.clipLine(); !strings.Contains(l, style.Cyan+"6"+style.NC) || !strings.Contains(l, style.Cyan+"Esc"+style.NC) {
		t.Errorf("both keys usable: %q", l)
	}
	p := s.cur()
	p.recent, p.recentList = true, []Entry{}
	s.refreshRows(p, "")
	if l := s.clipLine(); !strings.Contains(l, style.Dim+"6 ") {
		t.Errorf("recent view: 6 Paste should be dim: %q", l)
	}
	p.recent = false
	s.refreshRows(p, "")
	p.filter = "a"
	if l := s.clipLine(); !strings.Contains(l, style.Dim+"Esc ") {
		t.Errorf("filter on: Esc Clear should be dim: %q", l)
	}
}
