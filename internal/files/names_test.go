package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// 9: an NFD name (macOS Finder) shows as its syllables and keeps the columns.
func TestNFDRowWidth(t *testing.T) {
	if w := ui.TextWidth(norm.NFD.String("자모")); w != 4 {
		t.Errorf("TextWidth of NFD 자모 = %d", w)
	}
	s, dev, _ := testState(t)
	nfd := norm.NFD.String("자모 분리.txt")
	write(t, dev, nfd, "plain.txt")
	s.open(s.cur(), dev, "")
	p := s.cur()
	for i := range p.rows {
		for _, brief := range []bool{false, true} {
			row := s.row(p, i, 50, true, false, brief)
			if got := visWidth(row); got != 50 {
				t.Errorf("row %q brief=%v: width %d", ui.Plain(row), brief, got)
			}
			if strings.Contains(ui.Plain(row), "자모") != (p.rows[i].Name == nfd) && p.rows[i].Name == nfd {
				t.Errorf("NFD name not shown as NFC: %q", ui.Plain(row))
			}
		}
	}
}

// 9, 11: names that differ only in normalization clash; a push gets the
// NFC name, a pull keeps the name; an overwrite replaces the item as it is.
func TestNormalizedNamesClash(t *testing.T) {
	nfc := "자모.txt"
	nfd := norm.NFD.String(nfc)
	s, dev, pc := testState(t)
	write(t, pc, nfd)
	write(t, dev, "other.txt")
	s.panes[Device].fs = caseFS{localFS{Device, "Test Phone"}, true} // keeps NFD and NFC apart
	s.open(s.panes[Device], dev, "")
	s.open(s.panes[PC], pc, "")
	press(t, s, tab+home+down)
	s.send(s.panes[PC], true)
	if s.mode == modeConflict || s.plan == nil && s.pending == nil {
		t.Fatalf("no clash expected yet: mode %d", s.mode)
	}
	runTasks(s)
	if _, err := os.Stat(filepath.Join(dev, nfc)); err != nil {
		t.Fatalf("pushed as NFC: %v", err)
	}

	// The device now has the NFC name; the computer's NFD one clashes with it.
	press(t, s, "5")
	if s.mode != modeConflict {
		t.Fatalf("NFD vs NFC should ask (mode %d)", s.mode)
	}
	s.handle(tui.Key{Kind: tui.KeyRune, Rune: 'o'})
	runTasks(s)
	if len(s.summary.Failed) != 0 {
		t.Errorf("overwrite: %q", s.summary.Failed)
	}
	if fold(nfd) != fold(nfc) {
		t.Error("fold should match NFD and NFC")
	}
}
