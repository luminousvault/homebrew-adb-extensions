package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func plain(lines []string) string {
	return ansi.ReplaceAllString(strings.Join(lines, "\n"), "")
}

func press(t *testing.T, s *state, input string) action {
	t.Helper()
	keys, used := parseKeys([]byte(input))
	if used != len(input) {
		t.Fatalf("parse consumed %d of %d bytes", used, len(input))
	}
	act := actNone
	for _, k := range keys {
		if act = s.handle(k); act != actNone {
			break
		}
	}
	return act
}

func render(s *state, height int) string {
	fixed := 0
	l := s.computeLayout(height, &fixed, false)
	return plain(s.frame(l, fixed, 40, 100))
}

func TestParseKeys(t *testing.T) {
	keys, used := parseKeys([]byte("a\x1b[A\x1b[B\r\x7f\x1b[3~\x03\x1b[200~x y\x1b[201~"))
	want := []keyKind{keyRune, keyUp, keyDown, keyEnter, keyBackspace, keyDelete, keyCtrlC, keyPaste}
	if used != len("a\x1b[A\x1b[B\r\x7f\x1b[3~\x03\x1b[200~x y\x1b[201~") || len(keys) != len(want) {
		t.Fatalf("keys=%v used=%d", keys, used)
	}
	for i, k := range want {
		if keys[i].kind != k {
			t.Errorf("[%d] kind %v want %v", i, keys[i].kind, k)
		}
	}
	if keys[7].text != "x y" {
		t.Errorf("paste text %q", keys[7].text)
	}
	// Incomplete sequences wait for more input.
	if _, used := parseKeys([]byte("\x1b[")); used != 0 {
		t.Errorf("incomplete CSI consumed %d", used)
	}
	if _, used := parseKeys([]byte("\x1b[200~abc")); used != 0 {
		t.Errorf("incomplete paste consumed %d", used)
	}
}

func TestSingleSelectArrowsWrap(t *testing.T) {
	s := newState(Options{Prompt: "Pick", Items: []string{"a", "b", "c"}})
	if act := press(t, s, "\x1b[A\r"); act != actConfirm {
		t.Fatal("not confirmed")
	}
	if r := s.result(); r.Index != 2 {
		t.Errorf("up from first should wrap to last, got %d", r.Index)
	}
}

func TestSingleNumberKey(t *testing.T) {
	s := newState(Options{Items: []string{"a", "b", "c"}})
	if press(t, s, "2") != actConfirm || s.result().Index != 1 {
		t.Errorf("number key: %+v", s.result())
	}
	s = newState(Options{Items: make([]string, 10)})
	if press(t, s, "2") != actNone {
		t.Error("number keys are disabled with more than 9 items")
	}
}

func TestMultiEnterSelectsFocusedWhenNothingSelected(t *testing.T) {
	s := newState(Options{Multi: true, Items: []string{"a", "b", "c"}})
	press(t, s, "\x1b[B\r")
	if r := s.result(); len(r.Indices) != 1 || r.Indices[0] != 1 {
		t.Errorf("got %v", r.Indices)
	}
}

func TestMultiToggleOrderAndAll(t *testing.T) {
	s := newState(Options{Multi: true, Items: []string{"a", "b", "c"}})
	press(t, s, "\x1b[B\x1b[B \x1b[A\x1b[A ")
	if r := s.result(); fmtInts(r.Indices) != "2 0" {
		t.Errorf("selection order %v, want [2 0]", r.Indices)
	}
	press(t, s, "a") // not all selected -> select all
	if s.selectedCount() != 3 {
		t.Errorf("A should select all, got %d", s.selectedCount())
	}
	press(t, s, "A") // all selected -> clear
	if s.selectedCount() != 0 {
		t.Errorf("A should clear, got %d", s.selectedCount())
	}
}

func fmtInts(v []int) string {
	var b strings.Builder
	for i, n := range v {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(string(rune('0' + n)))
	}
	return b.String()
}

func TestFilterHighlightAndConfirm(t *testing.T) {
	s := newState(Options{Prompt: "Select APK files", Multi: true,
		Items: []string{"app-release.apk", "App-Debug.apk", "other.apk"}})
	press(t, s, "/DEB")
	if len(s.filtered) != 1 || s.filtered[0] != 1 {
		t.Fatalf("filtered %v", s.filtered)
	}
	if want := "App-" + hlOn + "Deb" + hlOff + "ug.apk"; s.highlighted[0] != want {
		t.Errorf("highlight %q want %q", s.highlighted[0], want)
	}
	out := render(s, 30)
	for _, want := range []string{"1 item(s) (filtered from 3) | 0 selected", "> DEB", "Exit filter"} {
		if !strings.Contains(out, want) {
			t.Errorf("frame missing %q:\n%s", want, out)
		}
	}
	press(t, s, "\r")
	if r := s.result(); len(r.Indices) != 1 || r.Indices[0] != 1 {
		t.Errorf("result %v", r.Indices)
	}
}

func TestFilterKeepsSelectedUnmatchedOnTop(t *testing.T) {
	s := newState(Options{Multi: true, Items: []string{"alpha", "beta", "gamma"}})
	press(t, s, "\x1b[B\x1b[B ") // select gamma
	press(t, s, "/be")
	if len(s.filtered) != 2 || s.filtered[0] != 2 || s.filtered[1] != 1 {
		t.Errorf("filtered %v, want selected gamma pinned then beta", s.filtered)
	}
}

func TestFilterEditing(t *testing.T) {
	s := newState(Options{Items: []string{"abc", "xyz"}})
	press(t, s, "/xz\x1b[D\x1b[Dy") // two lefts move the cursor to the start, y is inserted there
	if s.filterText != "yxz" {
		t.Errorf("text %q", s.filterText)
	}
	press(t, s, "\x05\x7f\x01\x1b[3~") // end, backspace, home, delete
	if s.filterText != "x" {
		t.Errorf("text %q", s.filterText)
	}
	press(t, s, "/")
	if s.filterMode || s.filterText != "" || len(s.filtered) != 2 {
		t.Errorf("'/' should leave filter mode and reset: %+v", s)
	}
}

func TestNoFilterIgnoresSlashAndSort(t *testing.T) {
	s := newState(Options{NoFilter: true, Items: []string{"b", "a"}})
	press(t, s, "/s")
	if s.filterMode || s.sortMode != sortTime {
		t.Error("filter/sort must be disabled")
	}
	if out := render(s, 30); strings.Contains(out, "Filter") || strings.Contains(out, "S:") {
		t.Errorf("help shows disabled keys:\n%s", out)
	}
}

func TestSortByName(t *testing.T) {
	s := newState(Options{Multi: true, Items: []string{"b.apk", "c.apk", "a.apk"}})
	press(t, s, "s")
	press(t, s, " \r") // toggle first row (a.apk, original index 2)
	if r := s.result(); len(r.Indices) != 1 || r.Indices[0] != 2 {
		t.Errorf("got %v", r.Indices)
	}
	if out := render(s, 30); !strings.Contains(out, "S: Name↑") {
		t.Errorf("help should show name sort:\n%s", out)
	}
}

func TestRenderLayout(t *testing.T) {
	s := newState(Options{Prompt: "Select files", Multi: true, Location: []string{"~/a", "~/b"},
		Items: []string{"one.apk\t(a)", "two.apk\t(b)"}})
	got := render(s, 30)
	want := strings.Join([]string{
		"==> Select files",
		"    Location: ~/a • ~/b",
		"",
		"2 item(s) | 0 selected",
		"➤ [ ] 1. one.apk  (a)",
		"  [ ] 2. two.apk  (b)",
		"",
		"↑↓: Move │ Enter: Confirm │ Space: Toggle │ A: All │ S: Time↓ │ /: Filter │ ^C: Exit",
	}, "\n")
	if got != want {
		t.Errorf("frame:\n%s\nwant:\n%s", got, want)
	}
}

func TestScrollWindow(t *testing.T) {
	items := make([]string, 50)
	for i := range items {
		items[i] = "item"
	}
	s := newState(Options{Items: items})
	for i := 0; i < 30; i++ {
		press(t, s, "\x1b[B")
	}
	// height 20, no location: reserved = 2+1+1+1+3 = 8, visible = 20-8-1 = 11
	out := render(s, 20)
	if !strings.Contains(out, "Showing 26-36 / 50") {
		t.Errorf("window:\n%s", out)
	}
}

func TestCtrlC(t *testing.T) {
	s := newState(Options{Items: []string{"a"}})
	if press(t, s, "\x03") != actInterrupt {
		t.Error("Ctrl+C must interrupt")
	}
}

// Small terminals: the frame never has more lines than the terminal, so the
// screen never scrolls; items and the filter input are the last to go.
func TestFrameFitsHeight(t *testing.T) {
	items := make([]string, 40)
	for i := range items {
		items[i] = "app-" + strings.Repeat("x", i%7) + ".apk"
	}
	for _, h := range []int{2, 3, 4, 6, 8, 24} {
		s := newState(Options{Multi: true, Prompt: "Select APK files", Location: []string{"/tmp"}, Items: items})
		press(t, s, "\x1b[B\x1b[B/app") // move, then filter
		fixed := 0
		l := s.computeLayout(h, &fixed, false)
		lines := s.frame(l, fixed, 40, h)
		got := plain(lines)
		if len(lines) > h {
			t.Errorf("height %d: %d lines\n%s", h, len(lines), got)
		}
		if h >= 2 && (!strings.Contains(got, "➤") || !strings.Contains(got, "> app")) {
			t.Errorf("height %d: focused item or filter input missing\n%s", h, got)
		}
		if h >= 24 && !strings.Contains(got, "==> Select APK files") {
			t.Errorf("height %d: prompt dropped although it fits\n%s", h, got)
		}
	}
}

// On a small terminal the rows of dropped lines go to items.
func TestSmallTerminalShowsMoreItems(t *testing.T) {
	items := make([]string, 40)
	for i := range items {
		items[i] = "item"
	}
	s := newState(Options{Multi: true, Prompt: "Select", Location: []string{"/tmp"}, Items: items})
	fixed := 0
	l := s.computeLayout(6, &fixed, false)
	lines := s.frame(l, fixed, 40, 6)
	if len(lines) > 6 || strings.Count(plain(lines), "item") < 4 {
		t.Errorf("height 6:\n%s", plain(lines))
	}
}
