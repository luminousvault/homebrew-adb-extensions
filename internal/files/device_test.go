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

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
)

const serial = "TESTSERIAL01"

func fakeADB(t *testing.T, r map[string]adbtest.Response) *adbtest.Fake {
	t.Helper()
	f := &adbtest.Fake{Responses: r}
	old := adb.Default
	adb.Default = f
	t.Cleanup(func() { adb.Default = old })
	return f
}

func shellKey(cmd string) string { return "-s " + serial + " shell " + cmd }

func TestParseStat(t *testing.T) {
	out := "directory|3452|1700000000|/sdcard/DCIM\n" +
		"regular file|21131|1700000100|/sdcard/a|b.txt\n" +
		"regular empty file|0|1700000200|/sdcard/empty\n" +
		"regular file|5|1700000300|/sdcard/new\n" + // "new\nline": stat splits the name
		"line\n" +
		"directory|3452|1700000000|/sdcard/.\n" +
		"directory|3452|1700000000|/sdcard/..\n" +
		"stat: '/sdcard/.*': No such file or directory\n" +
		"stat: '/sdcard/secret': Permission denied\n"
	list, err := parseStat(out, "/sdcard")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range list {
		got = append(got, e.Name)
	}
	if strings.Join(got, ",") != "DCIM,a|b.txt,empty" {
		t.Fatalf("names %q", got)
	}
	if !list[0].Dir || list[0].Size != 0 || list[1].Size != 21131 || !list[1].ModTime.Equal(time.Unix(1700000100, 0)) {
		t.Errorf("entries %+v", list)
	}

	if list, err := parseStat("stat: '/sdcard/x/*': No such file or directory\nstat: '/sdcard/x/.*': No such file or directory\n", "/sdcard/x"); err != nil || len(list) != 0 {
		t.Errorf("empty folder: %v %v", list, err)
	}
	if _, err := parseStat("ak:denied\n", "/data"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("denied: %v", err)
	}
	if _, err := parseStat("ak:notdir\n", "/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("not a folder: %v", err)
	}
	if _, err := parseStat("stat: '/x/*': Permission denied\n", "/x"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("glob denied: %v", err)
	}
	// The root: paths are "/name".
	if list, _ := parseStat("directory|0|1|/sdcard\n", "/"); len(list) != 1 || list[0].Name != "sdcard" {
		t.Errorf("root: %+v", list)
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("it's $HOME a b"); got != `'it'\''s $HOME a b'` {
		t.Errorf("got %s", got)
	}
	if got := listCmd("/"); !strings.Contains(got, "''/* ''/.*") {
		t.Errorf("root globs: %s", got)
	}
}

func TestDeviceCommands(t *testing.T) {
	d := &deviceFS{serial: serial, name: "Test Phone"}
	f := fakeADB(t, map[string]adbtest.Response{
		shellKey(listCmd("/sdcard/한 글")):                       {Stdout: "regular file|3|1700000000|/sdcard/한 글/it's.txt\n"},
		shellKey("mkdir '/sdcard/new dir' 2>&1 && echo ak-ok"): {Stdout: "ak-ok\n"},
		shellKey("mkdir '/sdcard/x' 2>&1 && echo ak-ok"):       {Stdout: "mkdir: '/sdcard/x': File exists\n"},
		shellKey("[ -d '/sdcard/old' ] && echo ak-dir; rm -rf '/sdcard/old' 2>&1; [ ! -e '/sdcard/old' ] && [ ! -L '/sdcard/old' ] && echo ak-ok"):                        {Stdout: "ak-ok\n"},
		shellKey("if [ -e '/sdcard/b c' ] || [ -L '/sdcard/b c' ]; then echo '/sdcard/b c: already exists'; exit 1; fi; mv '/sdcard/a' '/sdcard/b c' 2>&1 && echo ak-ok"): {Stdout: "ak-ok\n"},
		shellKey("[ -e '/sdcard/d' ] && { find '/sdcard/d' -type f -exec stat -c %s {} + 2>/dev/null; echo ak-ok; }"):                                                     {Stdout: "10\n20\nak-ok\n"},
	})
	list, err := d.List("/sdcard/한 글")
	if err != nil || len(list) != 1 || list[0].Name != "it's.txt" {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := d.Mkdir("/sdcard/new dir"); err != nil {
		t.Error(err)
	}
	if err := d.Mkdir("/sdcard/x"); err == nil || !strings.Contains(err.Error(), "File exists") {
		t.Errorf("mkdir error: %v", err)
	}
	if err := d.Remove("/sdcard/old"); err != nil {
		t.Error(err)
	}
	if err := d.Rename("/sdcard/a", "/sdcard/b c"); err != nil {
		t.Error(err)
	}
	if b, n, err := d.Usage("/sdcard/d"); err != nil || b != 30 || n != 2 {
		t.Errorf("usage %d %d %v", b, n, err)
	}
	if _, _, err := d.Usage("/sdcard/missing"); err == nil {
		t.Error("a missing path has no usage")
	}
	for _, c := range f.Calls {
		if strings.Contains(c, "unexpected") {
			t.Error(c)
		}
	}
}

// Copying between the device and the PC runs adb pull and push.
func TestJobPullPush(t *testing.T) {
	pollInterval = time.Hour // no size polling in this test
	t.Cleanup(func() { pollInterval = 300 * time.Millisecond })
	pc := t.TempDir()
	write(t, pc, "up.txt")
	pulled := filepath.Join(pc, "photo.jpg")
	f := fakeADB(t, map[string]adbtest.Response{
		"-s " + serial + " pull /sdcard/DCIM/photo.jpg " + pulled:                        {Stdout: "/sdcard/DCIM/photo.jpg: 1 file pulled.\n"},
		"-s " + serial + " push " + filepath.Join(pc, "up.txt") + " /sdcard/DCIM/up.txt": {Stdout: "1 file pushed.\n"},
		"-s " + serial + " pull /sdcard/DCIM/gone.jpg " + filepath.Join(pc, "gone.jpg"):  {Stderr: "adb: error: failed to stat remote object '/sdcard/DCIM/gone.jpg': No such file or directory\n", Err: errors.New("exit 1")},
	})
	d := &deviceFS{serial: serial, name: "Test Phone"}
	pcFS := localFS{PC, computerLabel}

	down := &copyJob{from: d, to: pcFS, dstDir: pc, ops: []copyOp{
		{src: Entry{Name: "photo.jpg", Path: "/sdcard/DCIM/photo.jpg", Size: 4}, name: "photo.jpg"},
		{src: Entry{Name: "gone.jpg", Path: "/sdcard/DCIM/gone.jpg", Size: 4}, name: "gone.jpg"},
	}}
	res := down.run(context.Background(), func(progress) {})
	if len(res.done) != 1 || len(res.failed) != 1 {
		t.Fatalf("pull result %+v", res)
	}
	if !strings.Contains(res.failed[0].msg, "No such file or directory") || strings.HasPrefix(res.failed[0].msg, "adb:") {
		t.Errorf("failure message %q", res.failed[0].msg)
	}

	// adb push skips empty folders, so they are made afterwards.
	write(t, pc, "folder/empty/", "folder/f.txt")
	f.Responses["-s "+serial+" push "+filepath.Join(pc, "folder")+" /sdcard/DCIM/folder"] = adbtest.Response{Stdout: "1 file pushed.\n"}
	f.Responses[shellKey("mkdir -p '/sdcard/DCIM/folder' '/sdcard/DCIM/folder/empty' 2>&1 && echo ak-ok")] = adbtest.Response{Stdout: "ak-ok\n"}
	f.Responses[shellKey("[ -e '/sdcard/DCIM/folder' ] && { find '/sdcard/DCIM/folder' -type f -exec stat -c %s {} + 2>/dev/null; echo ak-ok; }")] = adbtest.Response{Stdout: "12\nak-ok\n"}
	up := &copyJob{from: pcFS, to: d, dstDir: "/sdcard/DCIM", ops: []copyOp{
		{src: Entry{Name: "up.txt", Path: filepath.Join(pc, "up.txt"), Size: 6}, name: "up.txt"},
		{src: Entry{Name: "folder", Path: filepath.Join(pc, "folder"), Dir: true}, name: "folder"},
	}}
	if res := up.run(context.Background(), func(progress) {}); len(res.done) != 2 {
		t.Fatalf("push result %+v (calls %q)", res, f.Calls)
	}
}

// An overwrite replaces the existing item only when the copy has the same size.
func TestOverwriteKeepsExistingOnSizeMismatch(t *testing.T) {
	pollInterval = time.Hour
	t.Cleanup(func() { pollInterval = 300 * time.Millisecond })
	pc := t.TempDir()
	dst := filepath.Join(pc, "v.mp4")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeADB(t, map[string]adbtest.Response{
		// adb "succeeds" but writes nothing (the fake), so the size check fails.
		"-s " + serial + " pull /sdcard/v.mp4 " + filepath.Join(pc, ".ak-tmp-v.mp4"): {Stdout: "1 file pulled.\n"},
	})
	d := &deviceFS{serial: serial, name: "Test Phone"}
	job := &copyJob{from: d, to: localFS{PC, computerLabel}, dstDir: pc, ops: []copyOp{
		{src: Entry{Name: "v.mp4", Path: "/sdcard/v.mp4", Size: 100}, name: "v.mp4", overwrite: true},
	}}
	res := job.run(context.Background(), func(progress) {})
	if len(res.failed) != 1 || res.failed[0].msg != errSizeMismatch.Error() {
		t.Fatalf("result %+v", res)
	}
	if read(t, dst) != "old" {
		t.Error("the existing file was replaced")
	}
	assertNoTemp(t, pc)
}

func TestRecentFiles(t *testing.T) {
	raw := "Row: 0 _data=/storage/emulated/0/DCIM/Camera/a, b.jpg, date_modified=1700000000, _size=2048\n" +
		"Row: 1 _data=/storage/emulated/0/Download/x.pdf, date_modified=1699999999, _size=NULL\n" +
		"garbage\n"
	rows := ParseRecentFiles(raw)
	if len(rows) != 2 || rows[0].Path != "/storage/emulated/0/DCIM/Camera/a, b.jpg" || rows[0].Size != 2048 ||
		!rows[0].Modified.Equal(time.Unix(1700000000, 0)) || rows[1].Size != -1 {
		t.Fatalf("rows %+v", rows)
	}
	e := recentEntries(rows)
	if e[0].Path != "/sdcard/DCIM/Camera/a, b.jpg" || e[0].Folder != "/sdcard/DCIM/Camera" || e[0].Name != "a, b.jpg" {
		t.Errorf("entries %+v", e)
	}

	f := fakeADB(t, map[string]adbtest.Response{shellKey(RecentFilesQuery): {Stdout: raw}})
	s := newState(&deviceFS{serial: serial, name: "Test Phone"}, localFS{PC, computerLabel}, "/sdcard", t.TempDir(), nil)
	s.setSize(120, 30)
	press(t, s, "t")
	p := s.cur()
	if !p.recent || len(p.rows) != 2 || p.rows[0].Name != "a, b.jpg" {
		t.Fatalf("recent rows %q", names(p))
	}
	press(t, s, "g") // pull from the recent list: the file's own folder
	pulled := false
	for _, c := range f.Calls {
		pulled = pulled || strings.Contains(c, " pull /sdcard/DCIM/Camera/a, b.jpg ")
	}
	if !pulled || len(s.summary.Failed) != 1 || !strings.HasPrefix(s.summary.Failed[0], "Couldn't pull 'a, b.jpg' from Test Phone → ") {
		t.Errorf("g in recent: calls %q failed %q", f.Calls, s.summary.Failed)
	}
	press(t, s, "t")
	if p.recent {
		t.Error("t should go back to the folder")
	}
}

func TestHelpers(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1536: "2 KB", 3_355_443: "3.2 MB", 450_887_680: "430.0 MB", 2_147_483_648: "2.0 GB", -1: "?"} {
		if got := HumanSize(n); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", n, got, want)
		}
	}
	taken := map[string]bool{"a.txt": true, "a (1).txt": true, "dir.v2": true}
	if got := UniqueName("a.txt", false, func(n string) bool { return taken[n] }); got != "a (2).txt" {
		t.Errorf("uniqueName %q", got)
	}
	if got := UniqueName("dir.v2", true, func(n string) bool { return taken[n] }); got != "dir.v2 (1)" {
		t.Errorf("folder uniqueName %q", got)
	}
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.Local)
	for _, c := range []struct {
		at            time.Time
		full, compact string
	}{
		{now.Add(-30 * time.Second), "now", "now"},
		{now.Add(30 * time.Second), "now", "now"}, // clocks a little apart
		{now.Add(-5 * time.Minute), "5m ago", "5m"},
		{now.Add(-12 * time.Hour), "12h ago", "12h"},
		{now.Add(-6 * 24 * time.Hour), "6d ago", "6d"},
		{now.Add(-8 * 24 * time.Hour), "11-11 12:00", "11-11"},
		{time.Date(2026, 1, 3, 9, 5, 0, 0, time.Local), "01-03 09:05", "01-03"},
		{time.Date(2025, 12, 31, 23, 0, 0, 0, time.Local), "2025-12-31", "2025"},
		{now.Add(48 * time.Hour), "11-21 12:00", "11-21"}, // device clock ahead
		{time.Time{}, "", ""},
	} {
		if got := fileTime(c.at, now, false); got != c.full {
			t.Errorf("fileTime(%v) = %q, want %q", c.at, got, c.full)
		}
		if got := fileTime(c.at, now, true); got != c.compact {
			t.Errorf("fileTime(%v, compact) = %q, want %q", c.at, got, c.compact)
		}
	}
}

// A file MediaStore has not scanned yet has no size or date_modified: it is
// still listed, its size unknown and its time from date_added.
func TestParseRecentFilesNotScannedYet(t *testing.T) {
	raw := "Row: 0 _data=/storage/emulated/0/Download/new.txt, date_modified=NULL, date_added=1795067834, _size=NULL\n" +
		"Row: 1 _data=/storage/emulated/0/ui.xml, date_modified=1794464621, date_added=1794449494, _size=23194\n"
	rows := ParseRecentFiles(raw)
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	if r := rows[0]; r.Path != "/storage/emulated/0/Download/new.txt" || r.Size != -1 || !r.Modified.Equal(time.Unix(1795067834, 0)) {
		t.Errorf("not scanned: %+v", r)
	}
	if r := rows[1]; r.Path != "/storage/emulated/0/ui.xml" || r.Size != 23194 || !r.Modified.Equal(time.Unix(1794464621, 0)) {
		t.Errorf("scanned: %+v", r)
	}
	if HumanSize(rows[0].Size) != "?" {
		t.Errorf("unknown size shows %q", HumanSize(rows[0].Size))
	}
	if !strings.Contains(RecentFilesQuery, "_size IS NULL OR _size > 0") || !strings.Contains(RecentFilesQuery, "COALESCE(date_modified, date_added) DESC") {
		t.Error("the query should keep unscanned files and sort them by date_added")
	}
}

// Rows MediaStore has not scanned (size NULL) get their size and time from
// one stat on the device, so "?" shows only when even that fails.
func TestFillUnknownSizes(t *testing.T) {
	a, b := "/storage/emulated/0/Download/new.txt", "/storage/emulated/0/Download/gone|x.txt"
	fakeADB(t, map[string]adbtest.Response{
		shellKey("stat -c '%s|%Y|%n' " + ShellQuote(a) + " " + ShellQuote(b) + " 2>/dev/null"): {Stdout: "3|1795067900|" + a + "\n"},
	})
	rows := []RecentFile{{Path: a, Size: -1}, {Path: "/storage/emulated/0/ui.xml", Size: 21}, {Path: b, Size: -1}}
	rows = FillUnknownSizes(serial, rows)
	if rows[0].Size != 3 || !rows[0].Modified.Equal(time.Unix(1795067900, 0)) {
		t.Errorf("filled: %+v", rows[0])
	}
	if rows[1].Size != 21 || rows[2].Size != -1 {
		t.Errorf("others: %+v %+v", rows[1], rows[2])
	}
}

// An unscanned file that stat finds empty is dropped, as the query drops
// scanned empty files: ak pull -l never picks a 0 B placeholder.
func TestFillUnknownSizesDropsEmpty(t *testing.T) {
	a, b := "/storage/emulated/0/Download/empty.lock", "/storage/emulated/0/Download/real.txt"
	fakeADB(t, map[string]adbtest.Response{
		shellKey("stat -c '%s|%Y|%n' " + ShellQuote(a) + " " + ShellQuote(b) + " 2>/dev/null"): {Stdout: "0|1|" + a + "\n5|2|" + b + "\n"},
	})
	rows := FillUnknownSizes(serial, []RecentFile{{Path: a, Size: -1}, {Path: b, Size: -1}})
	if len(rows) != 1 || rows[0].Path != b || rows[0].Size != 5 {
		t.Errorf("rows %+v", rows)
	}
}
