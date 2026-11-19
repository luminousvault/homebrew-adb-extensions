package files

import (
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func progressState(t *testing.T, done, total int64) *state {
	t.Helper()
	s, _, _ := testState(t)
	job := &copyJob{from: s.panes[Device].fs, to: s.panes[PC].fs, dstDir: "/home/me/Downloads"}
	s.task = &task{label: "Pulling", route: route(job, false), cancelable: true}
	s.prog = progress{index: 2, count: 3, name: "huge.bin", done: done, total: total}
	return s
}

func TestProgressBar(t *testing.T) {
	s := progressState(t, 215*1048576, 430*1048576)
	got := ui.Plain(s.progressText(200))
	want := "Pulling 2/3  huge.bin  Test Phone → /home/me/Downloads  " +
		strings.Repeat("█", 20) + strings.Repeat("░", 20) + " 50% (215.0 MB / 430.0 MB)  Esc Cancel"
	if got != want {
		t.Errorf("wide:\n got %q\nwant %q", got, want)
	}

	// Narrower: the bar takes what is left, between 10 and 40 columns.
	got = ui.Plain(s.progressText(100))
	if n := strings.Count(got, "█") + strings.Count(got, "░"); n < 10 || n > 40 || ui.TextWidth(got) != 100 {
		t.Errorf("100 columns: bar %d, width %d: %q", n, ui.TextWidth(got), got)
	}

	// Narrow: name and route are cut, then the sizes go; the bar keeps 10
	// columns and the percentage stays.
	for _, w := range []int{70, 50, 30} {
		got = ui.Plain(s.progressText(w))
		if n := strings.Count(got, "█") + strings.Count(got, "░"); n != 10 {
			t.Errorf("%d columns: bar %d: %q", w, n, got)
		}
		if !strings.Contains(got, " 50%") {
			t.Errorf("%d columns: no percentage: %q", w, got)
		}
		if w <= 50 && strings.Contains(got, "MB") {
			t.Errorf("%d columns: sizes should go before the bar shrinks: %q", w, got)
		}
	}
	if got = ui.Plain(s.progressText(85)); !strings.Contains(got, "(215.0 MB / 430.0 MB)") || !strings.Contains(got, "…") {
		t.Errorf("85 columns: cut names, keep sizes: %q", got)
	}

	// Unknown size: a spinner, no bar.
	s = progressState(t, 0, -1)
	got = ui.Plain(s.progressText(200))
	if strings.ContainsAny(got, "█░%") || !strings.Contains(got, "Pulling 2/3  huge.bin") {
		t.Errorf("unknown size: %q", got)
	}
}

func TestPushRoute(t *testing.T) {
	s, dev, _ := testState(t)
	job := &copyJob{from: s.panes[PC].fs, to: s.panes[Device].fs, dstDir: dev}
	s.task = &task{label: "Pushing", route: route(job, true), cancelable: true}
	s.prog = progress{index: 1, count: 1, name: "video.mp4", done: 42, total: 100}
	got := ui.Plain(s.progressText(300))
	if !strings.HasPrefix(got, "Pushing 1/1  video.mp4  Test Phone:"+dev+" ← Computer  ") || !strings.HasSuffix(got, " 42% (42 B / 100 B)  Esc Cancel") {
		t.Errorf("%q", got)
	}
}

// A long path is cut, not the device name or the arrow.
func TestTruncateRouteKeepsArrow(t *testing.T) {
	for _, c := range []struct{ rt, want string }{
		{"Pixel → /very/long/pc/path/Downloads", "Pixel → …/Downloads"},
		{"Pixel:/sdcard/very/long/Download ← Computer", "Pixel:…/Download ← Computer"},
		{"Pixel → ~/x", "Pixel → ~/x"},
	} {
		if got := truncateRoute(c.rt, ui.TextWidth(c.want)); got != c.want {
			t.Errorf("truncateRoute(%q) = %q, want %q", c.rt, got, c.want)
		}
	}
}
