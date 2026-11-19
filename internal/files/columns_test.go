package files

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

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
	s.setSize(100, 19) // the bar takes two rows under 200 columns plus a gap over it, the letter keys one and a rule
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
	s.setSize(79, 19)
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
