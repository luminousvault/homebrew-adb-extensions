package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

func entry(t *testing.T, f FS, p string) Entry {
	t.Helper()
	e, err := f.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// R2/R3/R5: a failed or canceled overwrite keeps the existing item and
// leaves no temporary copy.
func TestOverwriteKeepsExistingOnFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	src, dst := t.TempDir(), t.TempDir()
	write(t, src, "f.txt", "d/ok.txt", "d/secret.txt")
	write(t, dst, "f.txt", "d/old.txt")
	for _, p := range []string{"f.txt", "d/secret.txt"} {
		if err := os.Chmod(filepath.Join(src, p), 0); err != nil {
			t.Fatal(err)
		}
	}
	from, to := localFS{Device, "Test Phone"}, localFS{PC, computerLabel}
	job := &copyJob{from: from, to: to, dstDir: dst, ops: []copyOp{
		{src: entry(t, from, filepath.Join(src, "f.txt")), name: "f.txt", overwrite: true},
		{src: entry(t, from, filepath.Join(src, "d")), name: "d", overwrite: true, dstDir: true},
	}}
	res := job.run(context.Background(), func(progress) {})
	if len(res.failed) != 2 {
		t.Fatalf("%+v", res)
	}
	if read(t, filepath.Join(dst, "f.txt")) != "f.txt" || read(t, filepath.Join(dst, "d", "old.txt")) != "d/old.txt" {
		t.Fatal("the existing items were lost")
	}
	assertNoTemp(t, dst)

	// Canceled: the same.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job = &copyJob{from: from, to: to, dstDir: dst, ops: []copyOp{
		{src: entry(t, from, filepath.Join(src, "d")), name: "d", overwrite: true, dstDir: true},
	}}
	job.run(ctx, func(progress) {})
	if read(t, filepath.Join(dst, "d", "old.txt")) != "d/old.txt" {
		t.Fatal("cancel lost the existing folder")
	}
	assertNoTemp(t, dst)
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	des, _ := os.ReadDir(dir)
	for _, de := range des {
		if strings.HasPrefix(de.Name(), ".ak-") {
			t.Errorf("left behind: %s", de.Name())
		}
	}
}

// A working overwrite replaces the item and keeps the source.
func TestOverwriteReplaces(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, src, "f.txt", "d/new.txt")
	write(t, dst, "f.txt", "d/old.txt")
	if err := os.WriteFile(filepath.Join(dst, "f.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	from, to := localFS{Device, "Test Phone"}, localFS{PC, computerLabel}
	job := &copyJob{from: from, to: to, dstDir: dst, ops: []copyOp{
		{src: entry(t, from, filepath.Join(src, "f.txt")), name: "f.txt", overwrite: true},
		{src: entry(t, from, filepath.Join(src, "d")), name: "d", overwrite: true, dstDir: true},
	}}
	res := job.run(context.Background(), func(progress) {})
	if len(res.done) != 2 {
		t.Fatalf("%+v", res)
	}
	if read(t, filepath.Join(dst, "f.txt")) != "f.txt" || read(t, filepath.Join(dst, "d", "new.txt")) != "d/new.txt" {
		t.Error("not replaced")
	}
	if _, err := os.Stat(filepath.Join(dst, "d", "old.txt")); !os.IsNotExist(err) {
		t.Error("the old folder's files remain")
	}
	if _, err := os.Stat(filepath.Join(src, "f.txt")); err != nil {
		t.Error("the source must stay")
	}
	assertNoTemp(t, dst)
}

// R4: names collide regardless of case.
func TestCaseInsensitiveNames(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "photo.jpg", "b.txt")
	write(t, pc, "PHOTO.JPG")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, end+"g")
	if s.mode != modeConflict {
		t.Fatalf("PHOTO.JPG should conflict with photo.jpg (mode %d, msg %+v)", s.mode, s.msg)
	}
	press(t, s, enter)
	if _, err := os.Stat(filepath.Join(pc, "photo (1).jpg")); err != nil {
		t.Errorf("rename: %v", err)
	}
	// Same-folder copies skip names that differ only in case.
	if got := UniqueName("a.txt", false, takenFold([]string{"A.TXT", "a (1).TXT"})); got != "a (2).txt" {
		t.Errorf("uniqueName %q", got)
	}

	press(t, s, home+down+"r"+back+back+back+back+back+"PHOTO.jpg"+enter) // b.txt -> PHOTO.jpg: taken
	if s.msg == nil || !strings.Contains(s.msg.text, "already exists") {
		t.Fatalf("rename onto a case variant: %+v", s.msg)
	}
	p := s.cur()
	press(t, s, home+"/b.txt"+enter+"r"+back+back+back+back+back+"B.TXT"+enter) // only the case changes
	if _, err := os.Stat(filepath.Join(dev, "B.TXT")); err != nil {
		t.Fatalf("case-only rename: %v (%+v)", err, s.msg)
	}
	des, _ := os.ReadDir(dev)
	for _, de := range des {
		if de.Name() == "b.txt" {
			t.Error("b.txt still listed under its old case")
		}
	}
	_ = p
	press(t, s, esc+"nPhoto.JPG"+enter)
	if s.msg == nil || !strings.Contains(s.msg.text, "already exists") {
		t.Errorf("mkdir onto a case variant: %+v", s.msg)
	}
}

// R6: d, c and x act only on selected rows that are shown.
func TestHiddenSelectionIsNotTargeted(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "apple", "banana")
	s.open(s.cur(), dev, "")
	press(t, s, "a/app"+enter)
	got := s.targets(s.cur())
	if len(got) != 1 || got[0].Name != "apple" {
		t.Fatalf("filtered targets %+v", got)
	}
	press(t, s, esc+".") // hidden files shown, filter off: both again
	if got := s.targets(s.cur()); len(got) != 2 {
		t.Fatalf("targets after the filter %+v", got)
	}
}

// cancelFS cancels the copy while an overwrite swaps the new item in, the
// last step of an item.
type cancelFS struct {
	localFS
	cancel func()
}

func (c cancelFS) Rename(from, to string) error {
	err := c.localFS.Rename(from, to)
	c.cancel()
	return err
}

// R7: an item that finished counts as done even if Esc came during it.
func TestCancelAfterSuccessCountsDone(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", "out/a.txt")
	ctx, cancel := context.WithCancel(context.Background())
	to := &cancelFS{localFS{PC, computerLabel}, cancel} // a pointer: a struct with a func is not comparable
	job := &copyJob{from: localFS{Device, "Test Phone"}, to: to, dstDir: filepath.Join(dir, "out"), ops: []copyOp{
		{src: Entry{Name: "a.txt", Path: filepath.Join(dir, "a.txt"), Size: 5}, name: "a.txt", overwrite: true},
	}}
	res := job.run(ctx, func(progress) {})
	if !res.canceled || len(res.done) != 1 {
		t.Fatalf("result %+v", res)
	}
}

// A local folder copy keeps links as links: a link back up does not loop,
// and the copy's size matches the source's (Usage does not follow links
// either), so a move's check passes.
func TestLocalCopyKeepsLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	src, dst := t.TempDir(), t.TempDir()
	proj := filepath.Join(src, "proj")
	write(t, src, "proj/a.txt", "outside.txt")
	if err := os.Symlink("..", filepath.Join(proj, "up")); err != nil { // a loop if followed
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "outside.txt"), filepath.Join(proj, "out.txt")); err != nil {
		t.Fatal(err)
	}
	f := localFS{PC, computerLabel}
	b, n, _ := f.Usage(proj)
	if err := copyLocal(context.Background(), proj, filepath.Join(dst, "proj")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(dst, "proj", "up")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("up should stay a link: %v %v", fi, err)
	}
	if b2, n2, _ := f.Usage(filepath.Join(dst, "proj")); b2 != b || n2 != n {
		t.Errorf("sizes differ: source %d/%d, copy %d/%d", b, n, b2, n2)
	}
}
