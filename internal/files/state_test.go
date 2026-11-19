package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
	"golang.org/x/text/unicode/norm"
)

// write makes files (content = name) and folders (names ending in /) under dir.
func write(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// testState has a fake "device" (a local folder) and a PC folder.
func testState(t *testing.T) (s *state, devDir, pcDir string) {
	t.Helper()
	devDir, pcDir = t.TempDir(), t.TempDir()
	s = newState(localFS{Device, "Test Phone"}, localFS{PC, computerLabel}, devDir, pcDir, nil)
	s.tempDir = t.TempDir()
	s.wantSplit = false // most tests start on one pane; TestPCPaneByDefault covers the default
	s.setSize(120, 30)
	return s, devDir, pcDir
}

// press feeds keys as a terminal would send them, then runs any task that
// started, as the runner does.
func press(t *testing.T, s *state, input string) {
	t.Helper()
	keys, used := tui.ParseKeys([]byte(input))
	if tui.LoneEsc([]byte(input[used:])) {
		keys = append(keys, tui.Key{Kind: tui.KeyEsc})
		used++
	}
	if used != len(input) {
		t.Fatalf("parse consumed %d of %d bytes", used, len(input))
	}
	for _, k := range keys {
		s.handle(k)
		runTasks(s)
	}
}

func runTasks(s *state) {
	for s.pending != nil {
		t := s.pending
		s.pending, s.task = nil, t
		apply := t.run(context.Background(), func(progress) {})
		s.task = nil
		apply(s)
	}
}

func names(p *pane) string {
	var n []string
	for i, e := range p.rows {
		mark := ""
		if i == p.cursor {
			mark = ">"
		}
		if p.selected[e.Path] {
			mark += "*"
		}
		n = append(n, mark+e.Name)
	}
	return strings.Join(n, " ")
}

const (
	up, down, enter, back, esc, tab = "\x1b[A", "\x1b[B", "\r", "\x7f", "\x1b", "\t"
	home, end, pgdn                 = "\x1b[H", "\x1b[F", "\x1b[6~"
)

func TestNavigate(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "b.txt", "a.txt", "zdir/", "adir/inner.txt", ".hidden")
	s.open(s.cur(), dev, "")
	p := s.cur()
	if got := names(p); got != ">.. adir zdir a.txt b.txt" {
		t.Fatalf("rows = %q", got)
	}
	press(t, s, down+down+up+down)
	if p.cursor != 2 {
		t.Fatalf("cursor %d", p.cursor)
	}
	press(t, s, up+enter) // into adir
	if p.dir != filepath.Join(dev, "adir") || names(p) != ">.. inner.txt" {
		t.Fatalf("in %s: %q", p.dir, names(p))
	}
	press(t, s, back) // parent: cursor back on adir
	if p.dir != dev || p.rows[p.cursor].Name != "adir" {
		t.Fatalf("after back: %s %q", p.dir, names(p))
	}
	press(t, s, end)
	if p.rows[p.cursor].Name != "b.txt" {
		t.Errorf("End: %q", names(p))
	}
	press(t, s, home+pgdn)
	if p.cursor != len(p.rows)-1 {
		t.Errorf("PgDn: %d", p.cursor)
	}
	press(t, s, home+enter) // ".." goes up
	if p.dir != filepath.Dir(dev) {
		t.Errorf("Enter on ..: %s", p.dir)
	}
}

func TestHiddenAndSort(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "small.txt", "a-much-longer-file.txt", ".hidden")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dev, "a-much-longer-file.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	s.open(s.cur(), dev, "")
	p := s.cur()
	press(t, s, ".")
	if !strings.Contains(names(p), ".hidden") {
		t.Fatalf(". should show hidden files: %q", names(p))
	}
	press(t, s, ".")
	press(t, s, "s") // newest first
	if got := names(p); got != ">.. small.txt a-much-longer-file.txt" {
		t.Errorf("newest: %q", got)
	}
	press(t, s, "s") // size
	if got := names(p); got != ">.. a-much-longer-file.txt small.txt" {
		t.Errorf("size: %q", got)
	}
}

func TestSelect(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a", "b", "c")
	s.open(s.cur(), dev, "")
	p := s.cur()
	press(t, s, " ") // on "..": nothing
	if len(p.selected) != 0 {
		t.Fatal(".. was selected")
	}
	press(t, s, down+" "+" ")
	if got := names(p); got != ".. *a *b >c" {
		t.Fatalf("space: %q", got)
	}
	press(t, s, "a")
	if got := names(p); got != ".. *a *b >*c" {
		t.Fatalf("*: %q", got)
	}
	press(t, s, "a")
	if len(p.selected) != 0 {
		t.Fatalf("* again: %q", names(p))
	}
	press(t, s, home+down+down)
	if got := s.targets(p); len(got) != 1 || got[0].Name != "b" {
		t.Errorf("no selection targets the cursor: %v", got)
	}
}

func TestFilter(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "Apple.txt", "banana.txt", "grape.TXT")
	s.open(s.cur(), dev, "")
	p := s.cur()
	press(t, s, "/AP")
	if got := names(p); got != ">Apple.txt grape.TXT" {
		t.Fatalf("filter: %q", got)
	}
	press(t, s, "c"+back) // typed, then erased: still in filter input
	if p.filter != "AP" || s.mode != modeFilter {
		t.Fatalf("filter %q mode %d", p.filter, s.mode)
	}
	press(t, s, enter) // keep the filter, back to keys
	if s.mode != modeNormal || p.filter != "AP" {
		t.Fatal("Enter should keep the filter")
	}
	press(t, s, down+"g") // keys act again: pull grape
	if read(t, filepath.Join(pc, "grape.TXT")) != "grape.TXT" {
		t.Fatalf("g after the filter: %+v", s.msg)
	}
	press(t, s, esc) // Esc: filter off
	if p.filter != "" {
		t.Fatalf("Esc: filter %q", p.filter)
	}
}

func TestTabAndSplit(t *testing.T) {
	s, _, _ := testState(t)
	press(t, s, tab)
	if s.active != PC {
		t.Fatal("Tab should show the PC")
	}
	s.setSize(79, 30)
	press(t, s, "w")
	if s.split || s.msg == nil || s.msg.text != "Window too narrow for two panes" {
		t.Fatalf("narrow split: %v %+v", s.split, s.msg)
	}
	if bar := ui.Plain(strings.Join(s.bottomBar(79, 2), "\n")); strings.Contains(bar, "Show computer") {
		t.Errorf("9 offered where two panes do not fit: %q", bar)
	}
	// Two panes come at the width where two columns do: no width shows two
	// columns but refuses the PC pane.
	s.setSize(minColumnsWidth, 30)
	press(t, s, "w")
	if !s.split {
		t.Fatalf("w should split at %d columns", minColumnsWidth)
	}
	press(t, s, tab)
	if s.active != Device {
		t.Fatal("Tab should move to the other pane")
	}
	s.setSize(79, 30) // shrinks: one pane, the side the cursor was on
	if s.split || s.active != Device {
		t.Fatalf("after shrink: split %v active %v", s.split, s.active)
	}
}

func TestPushAndPull(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt", "b.txt", "folder/x/y.txt")
	s.open(s.panes[Device], dev, "")
	press(t, s, "ap") // p is for the PC side: ignored here
	if s.pending != nil || s.mode != modeNormal || len(s.summary.Done) != 0 {
		t.Fatal("p on the device side should do nothing")
	}
	press(t, s, "g") // one pane: pulled to the folder the PC side remembers
	for _, n := range []string{"a.txt", "b.txt", "folder/x/y.txt"} {
		if _, err := os.Stat(filepath.Join(pc, n)); err != nil {
			t.Errorf("not pulled: %s", n)
		}
	}
	want := "Pulled 3 items from Test Phone → " + displayPath(s.panes[PC].fs, pc)
	if len(s.summary.Done) != 1 || s.summary.Done[0] != want {
		t.Errorf("summary %q, want %q", s.summary.Done, want)
	}
	if ui.Plain(s.statusText()) != "✓ "+want {
		t.Errorf("message %q", ui.Plain(s.statusText()))
	}
	if _, err := os.Stat(filepath.Join(dev, "a.txt")); err != nil {
		t.Error("a pull keeps the original")
	}

	// Push from the PC into the device's current folder.
	press(t, s, home+down+enter) // device: into folder/
	press(t, s, tab+"/a.txt"+enter+"g")
	if s.pending != nil || len(s.summary.Done) != 1 {
		t.Fatal("g on the PC side should do nothing")
	}
	press(t, s, "p")
	if read(t, filepath.Join(dev, "folder", "a.txt")) != "a.txt" {
		t.Fatalf("not pushed: %+v", s.msg)
	}
	if _, err := os.Stat(filepath.Join(pc, "a.txt")); err != nil {
		t.Error("a push keeps the original")
	}
	if got, want := s.summary.Done[1], "Pushed 'a.txt' to Test Phone:"+filepath.Join(dev, "folder")+" ← Computer"; got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
}

func TestPushPullConflicts(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "1.txt", "2.txt", "3.txt", "4.txt")
	for _, n := range []string{"1.txt", "2.txt", "3.txt", "4.txt"} {
		if err := os.WriteFile(filepath.Join(pc, n), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.open(s.panes[Device], dev, "")
	press(t, s, "ag")
	if s.mode != modeConflict || !strings.Contains(ui.Plain(s.statusText()), "'1.txt' already exists.") {
		t.Fatalf("no question: %q", s.statusText())
	}
	press(t, s, "o")   // 1: overwrite
	press(t, s, "s")   // 2: skip
	press(t, s, enter) // 3: rename (the default)
	if s.mode != modeConflict {
		t.Fatal("4.txt should still be asked about")
	}
	press(t, s, "O") // the rest: overwrite
	want := map[string]string{"1.txt": "1.txt", "2.txt": "old", "3.txt": "old", "3 (1).txt": "3.txt", "4.txt": "4.txt"}
	for n, w := range want {
		if b, _ := os.ReadFile(filepath.Join(pc, n)); string(b) != w {
			t.Errorf("%s = %q, want %q", n, b, w)
		}
	}
	// Shift = all; Esc cancels.
	press(t, s, "ag"+"S")
	if s.mode != modeNormal || s.msg == nil || !strings.Contains(s.msg.text, "skipped") {
		t.Errorf("S: %+v", s.msg)
	}
	press(t, s, "g"+esc) // nothing was copied: the selection is still there
	if s.mode != modeNormal || s.msg == nil || s.msg.text != "Pull canceled" {
		t.Errorf("Esc: %+v", s.msg)
	}
}

// The divider shows the way p or g copies from the side with the cursor.
func TestDividerArrow(t *testing.T) {
	s, _, _ := testState(t)
	s.setSize(120, 20)
	press(t, s, "w")
	for _, c := range []struct {
		key, arrow string
	}{{"", "▶"}, {tab, "◀"}} {
		press(t, s, c.key)
		lines := s.frame()
		if got := ui.Plain(lines[paneRuleRow(s)]); !strings.Contains(got, c.arrow) || strings.Contains(got, "┼") {
			t.Errorf("active %v: %q", s.active, got)
		}
	}
}

func TestDelete(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "d/inner.txt")
	s.open(s.cur(), dev, "")
	press(t, s, "ad")
	if s.mode != modeDelete || !strings.Contains(ui.Plain(s.statusText()), "Delete 2 items from Test Phone? This cannot be undone. [y/N]") {
		t.Fatalf("question %q", ui.Plain(s.statusText()))
	}
	press(t, s, enter) // the default is No
	if _, err := os.Stat(filepath.Join(dev, "a.txt")); err != nil {
		t.Fatal("Enter deleted")
	}
	press(t, s, "\x1b[3~y") // the Delete key, then y (the selection was kept)
	if _, err := os.Stat(filepath.Join(dev, "d")); !os.IsNotExist(err) {
		t.Fatal("not deleted")
	}
	if len(s.summary.Done) != 1 || s.summary.Done[0] != "Deleted 2 items on Test Phone" {
		t.Errorf("summary %q", s.summary.Done)
	}
}

func TestRenameAndMkdir(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt")
	s.open(s.cur(), dev, "")
	p := s.cur()
	press(t, s, down+"r"+back+back+back+back+back+"b.txt"+enter)
	if s.msg == nil || s.msg.text != "'b.txt' already exists" {
		t.Fatalf("rename onto b.txt: %+v", s.msg)
	}
	press(t, s, "r"+back+back+back+back+back+"x/y"+enter)
	if s.msg == nil || !strings.Contains(s.msg.text, "'/'") {
		t.Fatalf("rename with /: %+v", s.msg)
	}
	press(t, s, "r"+back+back+back+back+back+"새 이름.txt"+enter)
	if _, err := os.Stat(filepath.Join(dev, "새 이름.txt")); err != nil {
		t.Fatalf("rename: %v (%+v)", err, s.msg)
	}
	if p.rows[p.cursor].Name != "새 이름.txt" {
		t.Errorf("cursor should follow the renamed file: %q", names(p))
	}
	press(t, s, "n"+enter)
	if s.msg == nil || s.msg.text != "name is empty" {
		t.Errorf("empty folder name: %+v", s.msg)
	}
	press(t, s, "nnew"+enter)
	if e, err := os.Stat(filepath.Join(dev, "new")); err != nil || !e.IsDir() {
		t.Fatal("mkdir")
	}
	if p.rows[p.cursor].Name != "new" {
		t.Errorf("cursor should be on the new folder: %q", names(p))
	}
}

func TestOpen(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "photo.jpg")
	write(t, pc, "doc.txt")
	var opened []string
	s.openLocal = func(p string) error { opened = append(opened, p); return nil }
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, down+enter) // a device file: copied to the temp folder, then opened
	if len(opened) != 1 || filepath.Base(opened[0]) != "photo.jpg" || !strings.HasPrefix(opened[0], s.tempDir) {
		t.Fatalf("opened %v", opened)
	}
	press(t, s, tab+down+enter)
	if len(opened) != 2 || opened[1] != filepath.Join(pc, "doc.txt") {
		t.Fatalf("opened %v", opened)
	}
}

func TestBusyBlocksActions(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	s.task = &task{label: "Pulling", cancelable: true}
	press(t, s, down+"d")
	if s.mode != modeNormal || s.msg == nil || !strings.HasPrefix(s.msg.text, "Wait until pulling finishes") {
		t.Fatalf("d while busy: %+v", s.msg)
	}
	press(t, s, up+down) // moving still works
	press(t, s, esc)
	if !s.canceled {
		t.Error("Esc should cancel the paste")
	}
	press(t, s, "q")
	if s.exit != exitQuit {
		t.Error("q should quit")
	}
	press(t, s, "\x03")
	if s.exit != exitInterrupt {
		t.Error("Ctrl+C should interrupt")
	}
}

func TestCanceledCopyRemovesPartialCopy(t *testing.T) {
	_, dev, pc := testState(t)
	write(t, dev, "a.txt", "b.txt")
	from, to := localFS{Device, "Test Phone"}, localFS{PC, computerLabel}
	ctx, cancel := context.WithCancel(context.Background())
	job := &copyJob{from: from, to: to, dstDir: pc, ops: []copyOp{
		{src: Entry{Name: "a.txt", Path: filepath.Join(dev, "a.txt"), Size: 5}, name: "a.txt"},
		{src: Entry{Name: "b.txt", Path: filepath.Join(dev, "b.txt"), Size: 5}, name: "b.txt"},
	}}
	n := 0
	res := job.run(ctx, func(p progress) {
		if p.index == 2 && p.total > 0 && n == 0 {
			n++
			cancel() // during the second file
		}
	})
	if !res.canceled || len(res.done) != 1 {
		t.Fatalf("result %+v", res)
	}
	if _, err := os.Stat(filepath.Join(pc, "b.txt")); !os.IsNotExist(err) {
		t.Error("the canceled copy should be removed")
	}
	if read(t, filepath.Join(pc, "a.txt")) != "a.txt" || read(t, filepath.Join(dev, "a.txt")) != "a.txt" {
		t.Error("the finished copy and the originals stay")
	}
}

func TestFrameFits(t *testing.T) {
	s, dev, _ := testState(t)
	var many []string
	for i := 0; i < 50; i++ {
		many = append(many, strings.Repeat("긴이름", 10)+string(rune('a'+i%26))+".txt")
	}
	write(t, dev, many...)
	s.open(s.cur(), dev, "")
	s.msg = &message{text: strings.Repeat("a long message ", 20)} // with a status line
	for _, split := range []bool{false, true} {
		for h := 1; h <= 30; h++ {
			for _, w := range []int{20, 60, 100, 160} {
				s.setSize(w, h)
				s.split = split && w >= minSplitWidth
				lines := s.frame()
				if len(lines) > h {
					t.Fatalf("%dx%d split=%v: %d lines", w, h, s.split, len(lines))
				}
				for _, l := range lines {
					if visWidth(ui.Fit(l, w)) > w {
						t.Fatalf("%dx%d: line too wide: %q", w, h, ui.Plain(l))
					}
				}
			}
		}
	}
	s.setSize(100, 10)
	press(t, s, end)
	plain := ui.Plain(strings.Join(s.frame(), "\n"))
	if !strings.Contains(plain, "➤") {
		t.Errorf("the cursor row must stay on screen:\n%s", plain)
	}
}

func TestFrameSymbolsWithoutColor(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt")
	s.open(s.cur(), dev, "")
	press(t, s, down+" ")
	plain := ui.Plain(strings.Join(s.frame(), "\n"))
	if !strings.Contains(plain, "[✓] a.txt") || !strings.Contains(plain, "➤    b.txt") {
		t.Errorf("selection and cursor need symbols:\n%s", plain)
	}
}

// With two panes the rules of the pane with the cursor are colored, the
// other pane's stay dim.
func TestActivePaneRules(t *testing.T) {
	s, _, _ := testState(t)
	s.setSize(120, 20)
	press(t, s, "w")
	rules := func() (top, bottom string) {
		lines := s.frame()
		top = lines[paneRuleRow(s)]
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.Contains(lines[i], "┴") {
				return top, lines[i]
			}
		}
		t.Fatal("no bottom rule")
		return
	}
	top, bottom := rules()
	if !strings.HasPrefix(top, style.Cyan) || !strings.HasPrefix(bottom, style.Cyan) {
		t.Errorf("device active: left rules not colored: %q / %q", top, bottom)
	}
	press(t, s, tab)
	top, bottom = rules()
	if strings.HasPrefix(top, style.Cyan) || strings.HasPrefix(bottom, style.Cyan) ||
		!strings.Contains(top, style.Cyan+"─") || !strings.Contains(bottom, style.Cyan+"─") {
		t.Errorf("PC active: right rules should be colored, left dim: %q / %q", top, bottom)
	}
}

func TestWrapKeysBalances(t *testing.T) {
	items := []string{"aaaa", "bbbb", "cccc", "dddd", "e"}
	if got := wrapKeys(items, 40); len(got) != 1 {
		t.Errorf("fits: %q", got)
	}
	got := wrapKeys(items, 20)
	if len(got) != 2 || got[0] != "aaaa  bbbb" || got[1] != "cccc  dddd  e" {
		t.Errorf("two rows: %q", got)
	}
}

// Open takes the file under the cursor only; with files selected it says
// what the selection is for instead of opening them all.
func TestOpenWithSelectionOpensOne(t *testing.T) {
	s, _, pc := testState(t)
	write(t, pc, "a.txt", "b.txt", "c.txt")
	var opened []string
	s.openLocal = func(p string) error { opened = append(opened, filepath.Base(p)); return nil }
	press(t, s, tab)
	s.reload(s.panes[PC])
	press(t, s, down+" "+" "+up+"\r") // select a.txt and b.txt, back on b.txt, open
	if len(opened) != 1 || opened[0] != "b.txt" {
		t.Fatalf("opened %q, want only b.txt", opened)
	}
	if s.msg == nil || !strings.Contains(s.msg.text, "the selection is for 3, 4, 5 and 8") {
		t.Errorf("no note about the selection: %+v", s.msg)
	}
	press(t, s, "aa") // select all, then none: open again with nothing selected
	opened = nil
	press(t, s, "\r")
	if len(opened) != 1 || s.msg == nil || strings.Contains(s.msg.text, "selection") {
		t.Errorf("no selection: opened %q, msg %+v", opened, s.msg)
	}
}

// paneRuleRow is the screen row of the rule over the list: under the header,
// the rule parting it from the letter keys, and the letter key rows.
func paneRuleRow(s *state) int {
	l := s.layout()
	row := len(l.keys)
	if l.header {
		row++
		if len(l.keys) > 0 {
			row++
		}
	}
	return row
}

// The PC pane is on from the start when the window is wide enough; a narrow
// window shows one pane and gets the PC pane back when it widens. 9 turns it
// off for good (until 9 again).
func TestPCPaneByDefault(t *testing.T) {
	s := newState(localFS{Device, "Test Phone"}, localFS{PC, computerLabel}, t.TempDir(), t.TempDir(), nil)
	s.tempDir = t.TempDir()
	s.setSize(120, 30)
	if !s.split || s.active != Device {
		t.Fatalf("start: split %v active %v, want two panes on the device", s.split, s.active)
	}
	s.setSize(70, 30)
	if s.split {
		t.Error("narrow: one pane expected")
	}
	s.setSize(120, 30)
	if !s.split {
		t.Error("wide again: the PC pane should come back")
	}
	press(t, s, "9")
	s.setSize(130, 30)
	if s.split {
		t.Error("after 9: the PC pane should stay off")
	}
	press(t, s, "9")
	if !s.split {
		t.Error("9 again: two panes")
	}
}

// Names are colored by what this user may do with them, not by extension.
func TestNameColor(t *testing.T) {
	for _, c := range []struct {
		e    Entry
		want string
	}{
		{Entry{Name: "data", Dir: true, NoRead: true}, style.Red},
		{Entry{Name: "sdcard", Dir: true, Link: true}, style.Purple},
		{Entry{Name: "app", Dir: true, ReadOnly: true}, style.Yellow},
		{Entry{Name: "DCIM", Dir: true}, style.Blue},
		{Entry{Name: "run.sh", Exec: true}, style.Green},
		{Entry{Name: ".nomedia"}, style.Dim},
		{Entry{Name: "app-release.apk"}, ""},
		{Entry{Name: "clip.mp4"}, ""},
	} {
		if got := nameColor(c.e); got != c.want {
			t.Errorf("%s: %q, want %q", c.e.Name, got, c.want)
		}
	}
	// The focused row stays cyan whatever it is.
	s, dev, _ := testState(t)
	write(t, dev, "a.zip")
	s.open(s.cur(), dev, "")
	press(t, s, down)
	if row := s.row(s.cur(), s.cur().cursor, 60, true, false, false); !strings.Contains(row, style.Cyan+style.Bold+"a.zip") {
		t.Errorf("focused row: %q", row)
	}
}

// The device's shell tests (rwxl per path) and the computer's access checks
// say what may be done with each entry.
func TestAccessFlags(t *testing.T) {
	list := []Entry{
		{Name: "a.txt", Path: "/x/a.txt"},
		{Name: "ro.txt", Path: "/x/ro.txt"},
		{Name: "tool", Path: "/x/tool"},
		{Name: "secret", Path: "/x/secret", Dir: true},
		{Name: "lnk", Path: "/x/lnk", Dir: true},
		{Name: "nodata", Path: "/x/nodata"},
	}
	applyAccess(list, "rw|/x/a.txt\nr|/x/ro.txt\nrwx|/x/tool\nr|/x/secret\nrwxl|/x/lnk\n")
	want := []Entry{
		{Name: "a.txt", Path: "/x/a.txt"},
		{Name: "ro.txt", Path: "/x/ro.txt", ReadOnly: true},
		{Name: "tool", Path: "/x/tool", Exec: true},
		{Name: "secret", Path: "/x/secret", Dir: true, NoRead: true}, // a folder needs x to be listed
		{Name: "lnk", Path: "/x/lnk", Dir: true, Link: true},
		{Name: "nodata", Path: "/x/nodata"}, // no line: ordinary
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("%s: %+v, want %+v", want[i].Name, list[i], want[i])
		}
	}

	// The list command's two parts.
	out := "regular file|3|1700000000|/x/a.txt\nak:access\nr|/x/a.txt\n"
	if l, err := parseStat(out, "/x"); err != nil || len(l) != 1 || !l[0].ReadOnly {
		t.Errorf("parseStat with access: %+v %v", l, err)
	}

	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		return // no execute bit / root reads everything
	}
	dir := t.TempDir()
	files := map[string]os.FileMode{"tool": 0o755, "ro": 0o444, "none": 0o000}
	for name, mode := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "tool"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	fs := localFS{PC, computerLabel}
	for name, check := range map[string]func(Entry) bool{
		"tool": func(e Entry) bool { return e.Exec && !e.ReadOnly && !e.NoRead },
		"ro":   func(e Entry) bool { return e.ReadOnly && !e.Exec },
		"none": func(e Entry) bool { return e.NoRead },
		"link": func(e Entry) bool { return e.Link && e.Exec },
	} {
		if e, err := fs.Stat(filepath.Join(dir, name)); err != nil || !check(e) {
			t.Errorf("computer %s: %+v %v", name, e, err)
		}
	}
}

// A relative go-to path is from the folder shown, as in a shell's cd.
func TestGotoRelative(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "sub/inner/x.txt")
	s.open(s.cur(), dev, "")
	press(t, s, ":")
	s.inputText, s.inputCursor = "sub/inner", len("sub/inner")
	press(t, s, enter)
	if want := filepath.Join(dev, "sub", "inner"); s.cur().dir != want {
		t.Errorf("dir %q, want %q (msg %+v)", s.cur().dir, want, s.msg)
	}
	press(t, s, ":")
	s.inputText, s.inputCursor = "..", 2
	press(t, s, enter)
	if want := filepath.Join(dev, "sub"); s.cur().dir != want {
		t.Errorf("..: dir %q, want %q", s.cur().dir, want)
	}
}

// A macOS NFD name is printed composed and pre-filled composed for rename;
// Enter on it unchanged renames nothing.
func TestNFDNameInMessagesAndRename(t *testing.T) {
	s, dev, _ := testState(t)
	nfd := norm.NFD.String("문서.txt")
	write(t, dev, nfd)
	s.open(s.cur(), dev, "")
	press(t, s, down+"0")
	if s.inputText != norm.NFC.String(nfd) {
		t.Fatalf("pre-filled %q", s.inputText)
	}
	press(t, s, enter) // unchanged
	if len(s.summary.Done) != 0 {
		t.Errorf("an unchanged name was renamed: %+v", s.summary.Done)
	}
	press(t, s, "0")
	s.inputText, s.inputCursor = "보고서.txt", len("보고서.txt")
	press(t, s, enter)
	if len(s.summary.Done) != 1 || !strings.Contains(s.summary.Done[0], "'문서.txt' to '보고서.txt'") ||
		strings.Contains(s.summary.Done[0], nfd) {
		t.Errorf("summary %+v", s.summary.Done)
	}
}

// Leaving the recent view lists the folder again: what was deleted there
// is gone from the folder view too.
type recentLocal struct{ localFS }

func (recentLocal) Recent() ([]Entry, error) { return nil, nil }

func TestLeavingRecentReloadsFolder(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "big.mp4", "keep.txt")
	p := s.panes[Device]
	p.fs = recentLocal{localFS{Device, "Test Phone"}}
	s.open(p, dev, "")
	p.recent, p.folderPath = true, ""
	p.recentList = []Entry{{Name: "big.mp4", Path: filepath.Join(dev, "big.mp4"), Size: 1}}
	s.refreshRows(p, "")
	if err := os.Remove(filepath.Join(dev, "big.mp4")); err != nil { // deleted meanwhile
		t.Fatal(err)
	}
	s.toggleRecent(p)
	if got := names(p); strings.Contains(got, "big.mp4") || !strings.Contains(got, "keep.txt") {
		t.Errorf("folder after recent: %s", got)
	}
}
