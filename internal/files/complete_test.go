package files

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// gotoTyping opens Go to on the computer pane, clears the prefilled path
// and types text.
func gotoTyping(t *testing.T, s *state, text string) {
	t.Helper()
	press(t, s, ":"+strings.Repeat(back, 300)+text)
}

func TestGotoTabCompletes(t *testing.T) {
	s, _, pc := testState(t)
	write(t, pc, "Download/Screenshots/", "Documents/", "DCIM/", "Music/", "Dfile.txt", ".Dhidden/", "dl/")
	s.active = PC
	s.open(s.panes[PC], pc, "")
	sep := string(filepath.Separator)

	for _, c := range []struct{ typed, want, matches, hint string }{
		{"Mu", "Mu" + "sic" + sep, "", ""},                                       // one folder: filled, with a separator
		{"D", "D", "DCIM/ Documents/ Download/", ""},                             // several: listed (no file, no hidden)
		{"Do", "Do", "Documents/ Download/", ""},                                 // nothing more they share
		{"Dow", "Download" + sep, "", ""},                                        // one again
		{"dow", "Download" + sep, "", ""},                                        // case does not matter when nothing has it
		{"d", "dl" + sep, "", ""},                                                // the same case wins
		{"Download" + sep + "S", "Download" + sep + "Screenshots" + sep, "", ""}, // inside a folder
		{".", ".Dhidden" + sep, "", ""},                                          // hidden only for a dot
		{"xyz", "xyz", "", "No folder starts with 'xyz'"},                        // none: the text stays
		{"nope" + sep + "a", "nope" + sep + "a", "", "Can't read nope" + sep},    // a folder that is not there
	} {
		gotoTyping(t, s, c.typed)
		press(t, s, tab)
		matches := strings.ReplaceAll(strings.Join(s.inputMatches, " "), sep, "/")
		if s.inputText != c.want || matches != c.matches || !strings.HasPrefix(s.inputHint, c.hint) || (c.hint == "") != (s.inputHint == "") {
			t.Errorf("%q + Tab = %q, matches %q, hint %q", c.typed, s.inputText, matches, s.inputHint)
		}
		press(t, s, esc)
	}

	// The next key puts the keys back; Enter goes to the completed folder.
	gotoTyping(t, s, "xyz")
	press(t, s, tab)
	if !strings.Contains(ui.Plain(s.modeKeys()), "No folder") {
		t.Errorf("key line: %q", ui.Plain(s.modeKeys()))
	}
	press(t, s, back)
	if got := ui.Plain(s.modeKeys()); !strings.Contains(got, "Tab Complete") {
		t.Errorf("key line after a key: %q", got)
	}
	press(t, s, strings.Repeat(back, 10)+"Dow"+tab+"S"+tab+enter)
	if s.panes[PC].dir != filepath.Join(pc, "Download", "Screenshots") {
		t.Errorf("dir %s", s.panes[PC].dir)
	}
}

func TestGotoTabDevice(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "Pictures/", "Podcasts/")
	s.open(s.cur(), dev, "")
	press(t, s, ":"+strings.Repeat(back, 300)+"Pi"+tab)
	if s.inputText != "Pictures/" {
		t.Errorf("device: %q", s.inputText)
	}
	// Tab elsewhere than Go to does nothing to the text.
	press(t, s, esc+"n"+"Pi"+tab)
	if s.inputText != "Pi" {
		t.Errorf("new folder: %q", s.inputText)
	}
}

// Tab lists only what it does not know: the folder shown is the pane's own
// listing, and a folder read once is kept until Go to closes.
func TestGotoTabListsOnce(t *testing.T) {
	s, _, pc := testState(t)
	write(t, pc, "Download/Screenshots/", "Documents/")
	s.active = PC
	s.open(s.panes[PC], pc, "")
	counter := &countFS{localFS: localFS{PC, computerLabel}}
	s.panes[PC].fs = counter
	sep := string(filepath.Separator)

	gotoTyping(t, s, "D"+tab+tab)                         // the folder shown: "Do", twice
	gotoTyping(t, s, pc+sep+"Do"+tab)                     // the same folder typed in full, with a separator
	if counter.lists != 0 || s.inputText != pc+sep+"Do" { // nothing more to fill
		t.Errorf("the folder shown: %d listings, %q", counter.lists, s.inputText)
	}
	gotoTyping(t, s, "Download"+sep+"S"+tab+back+tab) // Download: once for both
	if counter.lists != 1 || s.inputText != "Download"+sep+"Screenshots"+sep {
		t.Errorf("Download: %d listings, %q", counter.lists, s.inputText)
	}
	press(t, s, esc)
	write(t, pc, "Download/Saved/") // made while Go to was closed
	gotoTyping(t, s, "Download"+sep+"S"+tab)
	if counter.lists != 2 || strings.Join(s.inputMatches, " ") != "Saved"+sep+" Screenshots"+sep {
		t.Errorf("after reopening: %d listings, %q", counter.lists, s.inputMatches)
	}
}

// A second Tab opens a menu over the folders, as zsh's: Tab and Shift+Tab
// pick into the text, the arrows move column by column, Esc puts the text
// back and keeps Go to open, a typed key keeps the pick, Enter goes there.
func TestGotoTabMenu(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "Alarms/", "Android/", "Audio/", "Books/", "Bin/")
	s.open(s.cur(), dev, "")
	s.setSize(30, 30) // 13-column cells: two columns, three rows (Alarms Android Audio | Bin Books)
	const shiftTab = "\x1b[Z"
	right, left := "\x1b[C", "\x1b[D"

	gotoTyping(t, s, tab)
	if s.inputText != "" || s.menuSel != -1 || len(s.inputMatches) != 5 {
		t.Fatalf("first Tab: %q sel %d %q", s.inputText, s.menuSel, s.inputMatches)
	}
	for _, c := range []struct{ keys, want string }{
		{tab, "Alarms/"},
		{tab, "Android/"},
		{shiftTab, "Alarms/"},
		{shiftTab, "Books/"}, // wraps to the last
		{tab, "Alarms/"},     // and back to the first
		{down + down, "Audio/"},
		{down, "Audio/"},  // the end stays
		{right, "Audio/"}, // nothing to its right
		{up + right, "Books/"},
		{left, "Android/"},
	} {
		press(t, s, c.keys)
		if s.inputText != c.want {
			t.Errorf("%q: %q, want %q", c.keys, s.inputText, c.want)
		}
	}
	press(t, s, esc) // the text before the menu, Go to still open
	if s.mode != modeInput || s.inputText != "" || len(s.inputMatches) != 0 {
		t.Errorf("Esc: mode %v %q %q", s.mode, s.inputText, s.inputMatches)
	}
	press(t, s, "A"+tab+tab+tab+"x") // a typed key closes it and goes on after the pick
	if s.inputText != "Android/x" || len(s.inputMatches) != 0 {
		t.Errorf("typed: %q %q", s.inputText, s.inputMatches)
	}
	press(t, s, strings.Repeat(back, 20)+tab+tab+enter) // the first folder, and Enter goes there
	if s.mode != modeNormal || s.cur().dir != filepath.Join(dev, "Alarms") {
		t.Errorf("Enter: mode %v dir %s", s.mode, s.cur().dir)
	}
}

func TestGotoTabMenuPanel(t *testing.T) {
	s, dev, _ := testState(t)
	for i := range 40 {
		write(t, dev, fmt.Sprintf("dir%02d/", i))
	}
	s.open(s.cur(), dev, "")
	s.setSize(40, 24) // 8-column cells: four columns, ten rows; a third of 24 shows six
	gotoTyping(t, s, tab)
	screen := func() string {
		var b strings.Builder
		for _, l := range s.frame() {
			b.WriteString(ui.Plain(l) + "\n")
		}
		return b.String()
	}
	got := screen()
	if !strings.Contains(got, "── 40 folders") || !strings.Contains(got, "dir00/  dir10/") || !strings.Contains(got, "rows 1-6 of 10") || strings.Contains(got, "dir06/  dir16/") {
		t.Errorf("panel:\n%s", got)
	}
	if !strings.Contains(got, "Tab Next") {
		t.Errorf("menu keys:\n%s", got)
	}
	press(t, s, tab+strings.Repeat(down, 7)) // dir07: the rows scroll to it
	if got = screen(); !strings.Contains(got, "rows 3-8 of 10") || !strings.Contains(got, "dir07/") || strings.Contains(got, "dir00/  dir10/") {
		t.Errorf("scrolled:\n%s", got)
	}

}
