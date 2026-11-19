package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func TestNumberKeys(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "a.txt")
	write(t, pc, "b.txt")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, down+"5") // device side: pull
	if read(t, filepath.Join(pc, "a.txt")) != "a.txt" {
		t.Fatalf("5 on the device should pull: %+v", s.msg)
	}
	press(t, s, tab+home+down+down+"5") // PC side: push b.txt
	if read(t, filepath.Join(dev, "b.txt")) != "b.txt" {
		t.Fatalf("5 on the PC should push: %+v", s.msg)
	}
	press(t, s, "?")
	if s.mode != modeHelp {
		t.Fatal("? is help")
	}
	press(t, s, "x"+"1") // close help, then sort
	if s.cur().sortBy != sortNewest {
		t.Errorf("1 is sort: %v", s.cur().sortBy)
	}
	press(t, s, "9")
	if !s.split {
		t.Error("9 splits")
	}
	press(t, s, "9w") // off, then the letter alias on again
	if !s.split {
		t.Error("w still splits")
	}
	press(t, s, "/12"+enter) // digits are text while typing
	if s.cur().filter != "12" || s.exit != exitNone {
		t.Errorf("filter %q", s.cur().filter)
	}
	press(t, s, esc+"7new1"+enter)
	if _, err := os.Stat(filepath.Join(pc, "new1")); err != nil {
		t.Errorf("7 then a name with a digit: %v %+v", err, s.msg)
	}
	press(t, s, "8")
	if s.mode != modeDelete {
		t.Error("8 deletes")
	}
	press(t, s, esc+"0")
	if s.mode != modeInput || s.inputKind != inputRename {
		t.Error("0 renames")
	}
	press(t, s, esc+"q")
	if s.exit != exitQuit {
		t.Error("q quits")
	}
}

// leftOf drops the sort order at the right end of the status line.
func leftOf(line string) string {
	if i := strings.Index(line, "  sort: "); i >= 0 {
		return strings.TrimRight(line[:i], " ")
	}
	return line
}

func TestInfoLine(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "d1/", "d2/", "a.txt", "bb.txt", "ccc.txt", ".hidden")
	s.open(s.cur(), dev, "")
	press(t, s, down+down)
	if got := leftOf(ui.Plain(s.infoLine(100))); got != "2/5  2 folders  3 files  18 B" {
		t.Errorf("info %q", got)
	}
	press(t, s, end+" ")
	if got := leftOf(ui.Plain(s.infoLine(100))); got != "5/5  2 folders  3 files  18 B  1 selected 7 B" {
		t.Errorf("selected %q", got)
	}
	press(t, s, ".")
	if got := ui.Plain(s.infoLine(100)); !strings.HasPrefix(got, "6/6  2 folders  4 files  25 B") {
		t.Errorf("hidden shown %q", got)
	}
	press(t, s, "./cc"+enter)
	if got := leftOf(ui.Plain(s.infoLine(100))); got != "1/1  2 folders  3 files  18 B  1 selected 7 B  filter: cc (Esc clears)" {
		t.Errorf("filter %q", got)
	}
	// The device's free space follows the folder's size in parentheses.
	s.cur().free = 54 << 30
	got := ui.Plain(s.infoLine(100))
	if got != "1/1  2 folders  3 files  18 B (54.0 GB free)  1 selected 7 B  filter: cc (Esc clears)" {
		t.Errorf("free %q", got)
	}
	if raw := s.infoLine(100); !strings.Contains(raw, style.Dim+"(54.0 GB free)") {
		t.Errorf("free space not dim: %q", raw)
	}
}

func TestParseDF(t *testing.T) {
	n, err := parseDF("Filesystem     1K-blocks      Used Available Use% Mounted on\n/dev/fuse      236000000 180000000  56623104  77% /storage/emulated\n")
	if err != nil || n != 56623104*1024 {
		t.Errorf("%d %v", n, err)
	}
	if _, err := parseDF(""); err == nil {
		t.Error("empty df")
	}
}

func TestNumberBar(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	got := ui.Plain(s.bottomBar(200, 1)[0])
	if ui.TextWidth(got) != 200 {
		t.Fatalf("width %d: %q", ui.TextWidth(got), got)
	}
	// Each key that works shows its letter alias after it, in parentheses; one
	// that does nothing here keeps its label, dim, with an × when color is off.
	for i, want := range []string{"1 Sort (s)", "2 Recent (t)", "3 ×Copy", "4 ×Cut", "5 ×To computer", "6 ×Paste", "7 Mkdir (n)", "8 ×Delete", "9 Show computer (w)", "0 ×Rename"} {
		cell := strings.TrimRight(string([]rune(got)[i*20:i*20+20]), " ")
		if cell != want {
			t.Errorf("cell %d = %q, want %q", i, cell, want)
		}
	}
	press(t, s, down) // on a file: copy, cut, send, delete and rename work
	got = ui.Plain(s.bottomBar(200, 1)[0])
	for _, want := range []string{"3 Copy (c)", "4 Cut (x)", "5 To computer (g)", "8 Delete (d)", "0 Rename (r)"} {
		if !strings.Contains(got, want) {
			t.Errorf("on a file: %q lacks %q", got, want)
		}
	}
	press(t, s, "c") // something in the clipboard: paste works
	if got = ui.Plain(s.bottomBar(200, 1)[0]); !strings.Contains(got, "6 Paste (v)") {
		t.Errorf("clipboard: %q", got)
	}
	press(t, s, tab) // PC: push
	got = ui.Plain(s.bottomBar(200, 1)[0])
	if !strings.Contains(got, "5 ×To device") {
		t.Errorf("PC bar %q", got)
	}
	// Narrow: labels are cut to the cell, never wrapped.
	got = ui.Plain(s.bottomBar(40, 1)[0])
	if !strings.HasPrefix(got, "1So 2Re 3×C 4×C 5×T") || ui.TextWidth(got) != 40 {
		t.Errorf("narrow %q", got)
	}
	// A question puts its keys in the bar's place.
	press(t, s, tab+down+"d")
	if got := ui.Plain(s.bottomBar(100, 1)[0]); !strings.Contains(got, "y Delete") {
		t.Errorf("delete keys %q", got)
	}
}

// When the window is short the letter keys go first (second row, then all),
// then the bar's second row, the bar, the rules and the header; the status
// line stays.
func TestShortWindowDropOrder(t *testing.T) {
	s, dev, _ := testState(t)
	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, string(rune('a'+i))+".txt")
	}
	write(t, dev, many...)
	s.open(s.cur(), dev, "")
	type shape struct {
		header, topRule, bottomRule bool
		keys, bar, list             int
	}
	for h, want := range map[int]shape{
		12: {true, true, true, 2, 2, 3}, // header, rule, 2 key rows, rule, list, rule, status, bar
		11: {true, true, true, 1, 2, 3},
		10: {true, true, true, 0, 2, 4}, // no keys: their rule goes too
		9:  {true, true, true, 0, 2, 3},
		8:  {true, true, true, 0, 1, 3},
		7:  {true, true, true, 0, 0, 3},
		6:  {true, true, false, 0, 0, 3},
		5:  {true, false, false, 0, 0, 3},
		4:  {false, false, false, 0, 0, 3},
		2:  {false, false, false, 0, 0, 1},
	} {
		s.setSize(80, h)
		l := s.layout()
		if got := (shape{l.header, l.topRule, l.bottomRule, len(l.keys), l.bar, l.list}); got != want {
			t.Errorf("height %d: %+v, want %+v", h, got, want)
		}
		lines := s.frame()
		if len(lines) > h || !strings.Contains(ui.Plain(strings.Join(lines, "\n")), "/20") && h >= 2 {
			t.Errorf("height %d: %q", h, lines)
		}
	}
}

// Every key is on screen: the letter keys on a row under the header, over
// the list (the number bar is at the bottom), on two rows when narrow.
func TestLetterKeysShown(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	s.setSize(160, 30)
	frame := s.frame()
	if !strings.Contains(ui.Plain(frame[0]), "Test Phone") || !strings.HasPrefix(ui.Plain(frame[1]), "───") ||
		!strings.Contains(ui.Plain(frame[2]), "Tab Computer") {
		t.Errorf("want header, rule, letter keys: %q / %q / %q", ui.Plain(frame[0]), ui.Plain(frame[1]), ui.Plain(frame[2]))
	}
	all := ui.Plain(strings.Join(frame, "\n"))
	for _, want := range []string{"Tab Computer", "←→ Column", "Space Select", "a All", "/ Filter", ": Go to", ". Show hidden", "b 1 column", "? Help", "q Quit"} {
		if !strings.Contains(all, want) {
			t.Errorf("wide: %q not shown", want)
		}
	}
	if strings.Contains(all, "Enter Open") || strings.Contains(all, "Bksp Up") {
		t.Error("Enter and Backspace are in the help, not on the key row")
	}
	if l := s.layout(); len(l.keys) != 1 {
		t.Errorf("wide: %d rows of letter keys", len(l.keys))
	}
	s.setSize(80, 30)
	l := s.layout()
	if len(l.keys) != 2 {
		t.Fatalf("narrow: %d rows of letter keys", len(l.keys))
	}
	for _, row := range l.keys {
		if visWidth(row) > 79 || strings.Contains(ui.Plain(row), "…") {
			t.Errorf("narrow row cut: %q", ui.Plain(row))
		}
	}
	press(t, s, ".") // hidden files shown: the key now hides them
	if l := s.layout(); !strings.Contains(ui.Plain(strings.Join(l.keys, " ")), ". Hide hidden") {
		t.Errorf("hidden shown: %q", l.keys)
	}
	// Esc is told where it acts (status line, progress line), not in the key
	// rows, so a filter does not change how many rows they take.
	rows := len(s.layout().keys)
	press(t, s, "/a\r")
	if l := s.layout(); len(l.keys) != rows || strings.Contains(ui.Plain(strings.Join(l.keys, " ")), "Esc") {
		t.Errorf("filter: key rows %q", l.keys)
	}
	if got := ui.Plain(s.infoLine(160)); !strings.Contains(got, "filter: a (Esc clears)") {
		t.Errorf("filter: no Esc hint on the status line: %q", got)
	}
}

// countingFree is a device stand-in whose free space drops by one each read.
type countingFree struct {
	localFS
	free *int64
}

func (c countingFree) Free(string) (int64, error) { *c.free--; return *c.free, nil }

// Free space is read again after a push or a delete, not only when a folder opens.
func TestFreeSpaceRefreshes(t *testing.T) {
	devDir, pcDir := t.TempDir(), t.TempDir()
	free := int64(1000)
	s := newState(countingFree{localFS{Device, "Test Phone"}, &free}, localFS{PC, computerLabel}, devDir, pcDir, nil)
	s.tempDir = t.TempDir()
	s.setSize(120, 30)
	if err := os.WriteFile(filepath.Join(pcDir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.reload(s.panes[PC])
	before := s.panes[Device].free
	press(t, s, "\t/a.txt\r5") // to the PC, find a.txt, push it
	if _, err := os.Stat(filepath.Join(devDir, "a.txt")); err != nil {
		t.Fatalf("not pushed: %v", err)
	}
	afterPush := s.panes[Device].free
	if afterPush == before {
		t.Errorf("free space not read again after a push: %d", afterPush)
	}
	press(t, s, "\t/a.txt\r8y") // back to the device, delete it
	if s.panes[Device].free == afterPush {
		t.Errorf("free space not read again after a delete")
	}
}

// A narrow window puts the bar on two rows (1-5 over 6-0) instead of
// cutting the labels; a wide one keeps one row.
func TestNumberBarTwoRows(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	s.setSize(100, 20)
	lines := s.frame()
	top, bottom := ui.Plain(lines[len(lines)-2]), ui.Plain(lines[len(lines)-1])
	for _, want := range []string{"1 Sort (s)", "2 Recent (t)", "3 ×Copy", "4 ×Cut", "5 ×To computer"} {
		if !strings.Contains(top, want) {
			t.Errorf("first row %q lacks %q", top, want)
		}
	}
	for _, want := range []string{"6 ×Paste", "7 Mkdir (n)", "8 ×Delete", "9 Show computer (w)", "0 ×Rename"} {
		if !strings.Contains(bottom, want) {
			t.Errorf("second row %q lacks %q", bottom, want)
		}
	}
	// At 80 columns the cells are too small for the longest labels' aliases:
	// the alias goes, the label stays whole.
	s.setSize(minColumnsWidth, 20)
	lines = s.frame()
	top, bottom = ui.Plain(lines[len(lines)-2]), ui.Plain(lines[len(lines)-1])
	if !strings.Contains(top, "5 ×To computer") || !strings.Contains(bottom, "9 Show computer ") ||
		!strings.Contains(top, "1 Sort (s)") || strings.Contains(top+bottom, "…") {
		t.Errorf("80 columns: %q / %q", top, bottom)
	}
	s.setSize(barOneRowWidth, 20)
	lines = s.frame()
	if last := ui.Plain(lines[len(lines)-1]); !strings.Contains(last, "1 Sort (s)") || !strings.Contains(last, "0 ×Rename") {
		t.Errorf("wide: one row expected, got %q", last)
	}
}

// A blank row parts the status line from the number bar; a short window
// gives it up first.
func TestGapOverNumberBar(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	s.setSize(100, 30)
	lines := s.frame()
	bar := len(lines) - s.layout().bar
	if lines[bar-1] != "" || !strings.Contains(ui.Plain(lines[bar-2]), "1 folder") && !strings.Contains(ui.Plain(lines[bar-2]), "/1") {
		t.Errorf("want status, blank, bar: %q / %q / %q", ui.Plain(lines[bar-2]), lines[bar-1], ui.Plain(lines[bar]))
	}
	s.setSize(100, 9)
	if s.layout().barGap {
		t.Error("short window: the gap should go first")
	}
}

// The key rows are as many as the keys need at this width: no blank row is
// kept for a key that is not there.
func TestLetterKeyRowsNoSpare(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt")
	s.open(s.cur(), dev, "")
	for _, w := range []int{80, 100, 120, 160, 220} {
		s.setSize(w, 40)
		for _, row := range s.layout().keys {
			if strings.TrimSpace(ui.Plain(row)) == "" {
				t.Errorf("width %d: a blank key row", w)
			}
		}
	}
}

// Each pane sorts on its own and shows its order in its header; the recent
// view sorts apart from the folders and starts newest first. The other pane
// is left alone.
func TestSortPerPane(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "b.txt", "a.txt")
	write(t, pc, "y.txt", "x.txt")
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, "1") // the device pane: newest
	if s.panes[Device].sortBy != sortNewest || s.panes[PC].sortBy != sortName {
		t.Errorf("sort device %v, computer %v", s.panes[Device].sortBy, s.panes[PC].sortBy)
	}
	if h := ui.Plain(s.header(s.panes[Device], 100, true)); !strings.HasSuffix(strings.TrimSpace(h), "newest ↓") {
		t.Errorf("device header %q", h)
	}
	if h := ui.Plain(s.header(s.panes[PC], 100, false)); !strings.HasSuffix(strings.TrimSpace(h), "name ↑") {
		t.Errorf("computer header %q", h)
	}
	if strings.Contains(ui.Plain(s.infoLine(100)), "sort") {
		t.Error("the sort order is no longer on the status line")
	}
	p := s.panes[Device]
	p.recent = true
	p.recentList = []Entry{{Name: "small", Path: "/r/small", Size: 1}, {Name: "big", Path: "/r/big", Size: 9}}
	s.refreshRows(p, "")
	press(t, s, "1") // recent: newest -> size
	if p.recentSort != sortSize || p.sortBy != sortNewest || p.rows[0].Name != "big" {
		t.Errorf("recent sort %v, folder sort %v, first %s", p.recentSort, p.sortBy, p.rows[0].Name)
	}
	if h := ui.Plain(s.header(p, 100, true)); !strings.HasSuffix(strings.TrimSpace(h), "size ↓") {
		t.Errorf("recent header %q", h)
	}
}
