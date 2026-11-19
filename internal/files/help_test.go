package files

import (
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func helpAll(w int) string { return ui.Plain(strings.Join(helpText(w), "\n")) }

// The help says that 2 sorts one pane, the recent view on its own (the
// description wraps, so spaces are compared loosely; one help column, as
// two columns interleave line by line).
func TestHelpExplainsSortPerPane(t *testing.T) {
	for _, w := range []int{60, 100} {
		flat := strings.Join(strings.Fields(helpAll(w)), " ")
		if !strings.Contains(flat, "for the pane with the cursor only") || !strings.Contains(flat, "recent view keeps its own order") {
			t.Errorf("width %d: help does not explain per-pane sorting", w)
		}
	}
}

func TestHelpHasEveryKey(t *testing.T) {
	for _, w := range []int{60, 140} {
		all := helpAll(w)
		for _, want := range []string{
			"1 Sort (s)", "2 Recent (t)", "3 Copy (c)", "4 Cut (x)", "5 To computer (g)", "5 To device (p)",
			"6 Paste (v)", "7 Mkdir (n)", "8 Delete (d)", "9 Show computer (w)", "0 Rename (r)",
			"Tab", "Space", "a", "b", "/", ":", ".", "Esc", "Enter", "Backspace", "← →", "↑ ↓", "PgUp PgDn", "Home End", "Ctrl+C", "?", "q",
			"o s r", "rm -rf", "MediaStore", "(user 0)", "adb pull", "adb push",
		} {
			if !strings.Contains(all, want) {
				t.Errorf("width %d: help has no %q", w, want)
			}
		}
		if strings.Contains(all, "Letter keys") {
			t.Errorf("width %d: the old letter-key line is back", w)
		}
		if !strings.Contains(all, displayPath(localFS{PC, "PC"}, tempDir())) {
			t.Errorf("width %d: Open should name the temp folder", w)
		}
	}
	// Wide: two columns side by side.
	if n1, n2 := len(helpText(80)), len(helpText(160)); n2 >= n1 {
		t.Errorf("two columns should be shorter: %d vs %d lines", n2, n1)
	}
}

func TestHelpFitsAndScrolls(t *testing.T) {
	s, _, _ := testState(t)
	press(t, s, "?")
	for _, w := range []int{30, 60, 100, 160} {
		for _, h := range []int{8, 15, 60} {
			s.setSize(w, h)
			lines := s.frame()
			if len(lines) > h {
				t.Fatalf("%dx%d: %d lines", w, h, len(lines))
			}
			for i, l := range lines {
				if i < len(lines)-3 && visWidth(l) > w { // the help itself; the bar and status are cut when drawn
					t.Fatalf("%dx%d: too wide: %q", w, h, ui.Plain(l))
				}
			}
		}
	}
	s.setSize(80, 15)
	first := ui.Plain(strings.Join(s.frame(), "\n"))
	if !strings.Contains(first, "↑↓ PgUp PgDn scroll · any other key closes") {
		t.Fatalf("no scroll hint:\n%s", first)
	}
	press(t, s, down+down+"\x1b[6~")
	if s.mode != modeHelp || ui.Plain(strings.Join(s.frame(), "\n")) == first {
		t.Fatal("↓ and PgDn should scroll the help")
	}
	press(t, s, "\x1b[F") // End: the last lines, clamped
	last := ui.Plain(strings.Join(s.frame(), "\n"))
	if !strings.Contains(last, "Status line") && !strings.Contains(last, "the sort") {
		t.Errorf("End should show the end:\n%s", last)
	}
	press(t, s, "x")
	if s.mode != modeNormal {
		t.Error("another key closes the help")
	}
	s.setSize(160, 80) // all of it fits: no scrolling
	press(t, s, "?")
	if all := ui.Plain(strings.Join(s.frame(), "\n")); !strings.Contains(all, "Any key closes") {
		t.Errorf("fits: %s", all)
	}
}
