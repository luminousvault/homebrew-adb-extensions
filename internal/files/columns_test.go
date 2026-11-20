package files

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// columnsState has ".." and f01..f25 in one pane with 10 rows per column.
func columnsState(t *testing.T) (*state, string) {
	t.Helper()
	s, dev, _ := testState(t)
	var names []string
	for i := 1; i <= 25; i++ {
		names = append(names, fmt.Sprintf("f%02d", i))
	}
	write(t, dev, names...)
	s.open(s.cur(), dev, "")
	s.setSize(100, 21) // the bar takes two rows under 200 columns plus a gap over it, the letter keys two and a rule, the clipboard row one
	if h := s.listHeight(); h != 10 {
		t.Fatalf("list height %d", h)
	}
	return s, dev
}

// cellsAt returns the two column cells of list row r on screen.
func cellsAt(t *testing.T, s *state, r int) (string, string) {
	t.Helper()
	line := ui.Plain(s.frame()[paneRuleRow(s)+1+r])
	l, rt, ok := strings.Cut(line, "│")
	if !ok {
		t.Fatalf("row %d has no column divider: %q", r, line)
	}
	return strings.TrimSpace(l), strings.TrimSpace(rt)
}

func TestTwoColumnsLayout(t *testing.T) {
	s, _ := columnsState(t)
	l, r := cellsAt(t, s, 0)
	if !strings.HasPrefix(l, "➤") || !strings.Contains(l, "..") || !strings.HasPrefix(r, "f10") {
		t.Errorf("row 0: %q | %q", l, r)
	}
	if l, r = cellsAt(t, s, 9); !strings.HasPrefix(l, "f09") || !strings.HasPrefix(r, "f19") {
		t.Errorf("row 9: %q | %q", l, r)
	}
	if !strings.Contains(ui.Plain(s.frame()[paneRuleRow(s)]), "┬") {
		t.Error("the rule should mark the column divider")
	}
	// Short sizes and times in a column.
	if l, _ = cellsAt(t, s, 1); !strings.Contains(l, "3B") || !strings.HasSuffix(l, "now") {
		t.Errorf("brief row %q", l)
	}

	// Down past the second column shifts one column: 11-20 | 21-25.
	press(t, s, strings.Repeat(down, 20))
	if l, r = cellsAt(t, s, 0); !strings.HasPrefix(l, "f10") || !strings.Contains(r, "f20") || !strings.HasPrefix(r, "➤") {
		t.Errorf("after 20 down: %q | %q", l, r)
	}
	if l, r = cellsAt(t, s, 5); !strings.HasPrefix(l, "f15") || !strings.HasPrefix(r, "f25") {
		t.Errorf("row 5: %q | %q", l, r)
	}
	if _, r = cellsAt(t, s, 6); r != "" {
		t.Errorf("row 6 right should be empty: %q", r)
	}
	// Up one column worth: back to 1-10 | 11-20.
	press(t, s, strings.Repeat(up, 11))
	if l, _ = cellsAt(t, s, 0); !strings.Contains(l, "..") {
		t.Errorf("after going up: %q", l)
	}
}

func TestTwoColumnsKeys(t *testing.T) {
	s, dev := columnsState(t)
	p := s.cur()
	press(t, s, "\x1b[D") // ← at the first item: nothing (not the parent folder)
	if p.dir != dev || p.cursor != 0 {
		t.Fatalf("← in two columns: %s %d", p.dir, p.cursor)
	}
	press(t, s, "\x1b[C\x1b[C") // → twice: rows 10, 20
	if p.cursor != 20 || p.dir != dev {
		t.Fatalf("→→: %d %s", p.cursor, p.dir)
	}
	press(t, s, "\x1b[C") // no column after: stays
	if p.cursor != 20 {
		t.Errorf("→ at the last column: %d", p.cursor)
	}
	press(t, s, "\x1b[D\x1b[D") // back to 0, scrolling back a column
	if p.cursor != 0 || p.top != 0 {
		t.Errorf("←←: cursor %d top %d", p.cursor, p.top)
	}
	press(t, s, strings.Repeat(down, 18)+"\x1b[C") // row 18 → 28 does not exist: the last item
	if p.cursor != 25 {
		t.Errorf("→ into a shorter column: %d", p.cursor)
	}
	press(t, s, home+pgdn) // a screen is two columns
	if p.cursor != 20 {
		t.Errorf("PgDn: %d", p.cursor)
	}
	press(t, s, enter) // Enter still opens (a file here: no open function)
	press(t, s, home+back)
	if p.dir != filepath.Dir(dev) {
		t.Errorf("Backspace goes up: %s", p.dir)
	}
}

// Switching between two columns and two panes keeps the cursor's item and
// scrolls it into view.
func TestColumnsAndPanesKeepCursor(t *testing.T) {
	s, dev := columnsState(t)
	p := s.cur()
	press(t, s, end+up+up) // f23
	press(t, s, "9")
	if !s.split || p.rows[p.cursor].Name != "f23" || !strings.Contains(ui.Plain(strings.Join(s.frame(), "\n")), "➤    f23") {
		t.Fatalf("split: cursor %q\n%s", p.rows[p.cursor].Name, ui.Plain(strings.Join(s.frame(), "\n")))
	}
	press(t, s, "\x1b[D") // two panes: ← is the parent folder
	if p.dir != filepath.Dir(dev) {
		t.Fatalf("← in two panes: %s", p.dir)
	}
	press(t, s, "\x1b[C"+end+up+up+"9") // → goes back in; one pane again
	if s.split || p.rows[p.cursor].Name != "f23" || !strings.Contains(ui.Plain(strings.Join(s.frame(), "\n")), "➤    f23") {
		t.Errorf("columns: cursor %q", p.rows[p.cursor].Name)
	}
}

func TestNarrowIsOneColumn(t *testing.T) {
	s, dev := columnsState(t)
	s.setSize(79, 21)
	if s.columns() != 1 || strings.Contains(ui.Plain(s.frame()[2]), "│") {
		t.Fatal("under 80 columns: one column")
	}
	press(t, s, "\x1b[D")
	if s.cur().dir != filepath.Dir(dev) {
		t.Error("← in one column is the parent folder")
	}
}

func TestShortSize(t *testing.T) {
	for n, want := range map[int64]string{12: "12B", 12 * 1024: "12K", 3_355_443: "3.2M", 450_887_680: "430M", 1_610_612_736: "1.5G", -1: "?"} {
		if got := shortSize(n); got != want {
			t.Errorf("shortSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// Long names: other rows drop the middle (the end stays), the cursor's row
// shows the whole name over its size and date, which go to the status line,
// and a name longer than the row scrolls after a moment.
func TestLongNames(t *testing.T) {
	if got := truncateMiddle("ONEBillingLab-release-v1.0.1.100010.-20251230-0601-02.apk", 30); ui.TextWidth(got) != 30 ||
		!strings.HasSuffix(got, "-0601-02.apk") || !strings.HasPrefix(got, "ONEBillingLab") || !strings.Contains(got, "…") {
		t.Errorf("truncateMiddle %q", got)
	}
	if got := truncateMiddle("짧은.txt", 30); got != "짧은.txt" {
		t.Errorf("fits: %q", got)
	}
	if got := truncateMiddle("가나다라마바사아자차카타파하.txt", 12); ui.TextWidth(got) > 12 {
		t.Errorf("wide chars %q", got)
	}

	s, dev, _ := testState(t)
	long := "ONEBillingLab-release-v1.0.1.100010.-20251230-0601-02.apk"
	write(t, dev, long, long+".bak")
	s.open(s.cur(), dev, "")
	s.setSize(70, 20) // one column; the name is longer than its column but fits the row
	press(t, s, down)
	frame := strings.Join(s.frame(), "\n")
	plain := ui.Plain(frame)
	if !strings.Contains(plain, "➤    "+long) {
		t.Errorf("cursor row not expanded:\n%s", plain)
	}
	if !s.focusExpanded || !strings.Contains(ui.Plain(s.infoLine(98)), time.Now().Format("2006-01-02")) {
		t.Errorf("status line has no size/date: %q", ui.Plain(s.infoLine(98)))
	}
	if !strings.Contains(plain, "….bak") && !strings.Contains(plain, ".apk.bak") {
		t.Errorf("other row should keep its end:\n%s", plain)
	}

	// Narrower than the name: it scrolls left after marqueeWait.
	now := time.Now()
	s.now = func() time.Time { return now }
	s.setSize(40, 20)
	s.frame()
	first := s.marquee(Device, s.cur().rows[s.cur().cursor].Path, long, 30)
	now = now.Add(marqueeWait + 5*marqueeStep)
	later := s.marquee(Device, s.cur().rows[s.cur().cursor].Path, long, 30)
	if !strings.HasPrefix(first, "ONEBilling") || first == later || !strings.HasPrefix(long[5:], later[:10]) {
		t.Errorf("marquee: %q then %q", first, later)
	}
	if !s.scrolling {
		t.Error("a scrolling name must ask the loop to redraw")
	}
}

// b (brief) switches one pane between two columns and one wide column; the cursor
// stays on its item.
func TestOneColumnToggle(t *testing.T) {
	s, _ := columnsState(t)
	press(t, s, down+down+down)
	at := s.cur().rows[s.cur().cursor].Name
	if s.columns() != 2 {
		t.Fatal("two columns first")
	}
	press(t, s, "b")
	if s.columns() != 1 || s.cur().rows[s.cur().cursor].Name != at {
		t.Errorf("b: columns %d, cursor on %s", s.columns(), s.cur().rows[s.cur().cursor].Name)
	}
	press(t, s, "b")
	if s.columns() != 2 {
		t.Error("b again: two columns")
	}
	press(t, s, "9") // two panes: b only says why
	before := s.oneColumn
	press(t, s, "b")
	if s.oneColumn != before || s.msg == nil {
		t.Error("b in two panes should not change anything")
	}
}

// The loop redraws a scrolling name only when it moves: scrollNext is the
// end of the wait, the next step, or the end of the hold.
func TestMarqueeNextMove(t *testing.T) {
	s, _, _ := testState(t)
	start := time.Now()
	now := start
	s.now = func() time.Time { return now }
	name := strings.Repeat("x", 40) // 10 columns over a row of 30
	s.marquee(Device, "/a", name, 30)
	if !s.scrollNext.Equal(start.Add(marqueeWait)) {
		t.Errorf("waiting: next %v, want %v", s.scrollNext.Sub(start), marqueeWait)
	}
	now = start.Add(marqueeWait + 2*marqueeStep + marqueeStep/3)
	s.marquee(Device, "/a", name, 30)
	if want := start.Add(marqueeWait + 3*marqueeStep); !s.scrollNext.Equal(want) {
		t.Errorf("moving: next %v, want %v", s.scrollNext.Sub(start), want.Sub(start))
	}
	now = start.Add(marqueeWait + 10*marqueeStep + time.Millisecond) // at the end: held
	s.marquee(Device, "/a", name, 30)
	if want := start.Add(marqueeWait + 10*marqueeStep + marqueeHold); !s.scrollNext.Equal(want) {
		t.Errorf("held: next %v, want %v", s.scrollNext.Sub(start), want.Sub(start))
	}
}

// In the recent view the expanded cursor row has no room for the folder:
// the status line shows it with the size and date.
func TestRecentExpandedShowsFolder(t *testing.T) {
	s, _, _ := testState(t)
	p := s.cur()
	long := "screen-20260813-142551-1786598746592-recording.mp4"
	p.recent = true
	p.recentList = []Entry{{Name: long, Path: "/sdcard/Movies/" + long, Size: 10, ModTime: time.Now(), Folder: "/sdcard/Movies"}}
	s.refreshRows(p, "")
	s.setSize(60, 20)
	s.frame()
	if got := ui.Plain(s.infoLine(58)); !s.focusExpanded || !strings.Contains(got, "/sdcard/Movies") {
		t.Errorf("expanded %v, status %q", s.focusExpanded, got)
	}
}

// A size MediaStore does not know yet is left out of the status line rather
// than shown as a bare "?".
func TestUnknownSizeNotInStatus(t *testing.T) {
	s, _, _ := testState(t)
	p := s.cur()
	long := "screen-20260813-142551-1786598746592-recording.mp4"
	p.recent = true
	p.recentList = []Entry{{Name: long, Path: "/sdcard/Download/" + long, Size: -1, ModTime: time.Now(), Folder: "/sdcard/Download"}}
	s.refreshRows(p, "")
	s.setSize(60, 20)
	s.frame()
	if got := ui.Plain(s.infoLine(58)); strings.Contains(got, "?") || !strings.Contains(got, "/sdcard/Download") {
		t.Errorf("status %q", got)
	}
}
