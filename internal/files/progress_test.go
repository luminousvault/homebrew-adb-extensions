package files

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
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

// Deleting many items of unknown size shows a bar by items; one long item a
// spinner with how long it has run.
func TestProgressByItems(t *testing.T) {
	s, _, _ := testState(t)
	s.setSize(120, 30)
	s.task = &task{label: "Deleting 100 files", cancelable: true}
	s.prog = progress{index: 37, count: 100, name: "f36.txt", total: -1}
	got := ui.Plain(s.progressText(120))
	if !strings.Contains(got, "█") || !strings.Contains(got, "36% (36 / 100)") || !strings.Contains(got, "Esc Cancel") || strings.Contains(got, "37/100") {
		t.Errorf("many items: %q", got)
	}
	now := time.Now()
	s.now = func() time.Time { return now }
	s.task = &task{label: "Deleting 'big'", started: now.Add(-12 * time.Second)}
	s.prog = progress{index: 1, count: 1, name: "big", total: -1}
	if got := ui.Plain(s.progressText(120)); !strings.Contains(got, "Deleting 'big'") || !strings.Contains(got, "12s") || strings.Contains(got, "█") {
		t.Errorf("one item: %q", got)
	}
}

// Esc stops a delete between items: what was deleted is reported, the rest
// is kept.
func TestDeleteStopsBetweenItems(t *testing.T) {
	s, dev, _ := testState(t)
	write(t, dev, "a.txt", "b.txt", "c.txt")
	s.open(s.panes[Device], dev, "")
	press(t, s, "a8")
	if s.mode != modeDelete {
		t.Fatal("no delete question")
	}
	s.handle(tui.Key{Kind: tui.KeyRune, Rune: 'y'})
	job := s.pending
	s.pending, s.task = nil, job
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	apply := job.run(ctx, func(progress) {
		if n++; n == 2 { // Esc while the second item is going: it finishes, the third is kept
			cancel()
		}
	})
	s.task = nil
	apply(s)
	left, _ := os.ReadDir(dev)
	if len(left) != 1 || s.msg == nil || !strings.Contains(s.msg.text, "Canceled: 2 of 3 deleted") {
		t.Errorf("left %d, msg %+v", len(left), s.msg)
	}
	if len(s.summary.Canceled) != 1 {
		t.Errorf("summary %+v", s.summary)
	}
}
