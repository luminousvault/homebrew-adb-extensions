package files

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

// pressOnly feeds keys like press but leaves a task that starts pending.
func pressOnly(t *testing.T, s *state, input string) {
	t.Helper()
	keys, used := tui.ParseKeys([]byte(input))
	if used != len(input) {
		t.Fatalf("parse consumed %d of %d bytes", used, len(input))
	}
	for _, k := range keys {
		s.handle(k)
	}
}

// runWatching runs the pending task as the key loop does, calling its tick
// when item `at` starts copying, and returns what see saw then.
func runWatching(t *testing.T, s *state, at int, see func() string) string {
	t.Helper()
	task := s.pending
	if task == nil || task.tick == nil {
		t.Fatal("no copy task with a tick")
	}
	s.pending, s.task = nil, task
	seen := ""
	apply := task.run(context.Background(), func(p progress) {
		if p.index == at && !p.measuring && p.done == 0 && seen == "" {
			task.tick(s)
			seen = see()
		}
	})
	s.task = nil
	apply(s)
	return seen
}

// A copy of several items shows each one in the folder as it finishes, and
// a move takes each original out: a finished item stays whatever happens
// to the next.
func TestCopyShowsItemsAsTheyFinish(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "d/a.txt", "d/b.txt", "d/c.txt", "e/")
	write(t, pc, "p/old.txt")
	cd(s, Device, filepath.Join(dev, "d"))
	cd(s, PC, filepath.Join(pc, "p"))

	pressOnly(t, s, "a"+"5") // all three to the computer
	mid := runWatching(t, s, 3, func() string { return names(s.panes[PC]) })
	if mid != ">.. a.txt b.txt old.txt" {
		t.Errorf("while c.txt copies, the computer shows %q", mid)
	}
	if got := names(s.panes[PC]); got != ">.. a.txt b.txt c.txt old.txt" {
		t.Errorf("after: %q", got)
	}

	// A move from the computer: the originals leave as they go.
	press(t, s, tab+"a"+"4") // the computer pane: select all, cut
	cd(s, Device, filepath.Join(dev, "e"))
	pressOnly(t, s, tab+"6") // paste into an empty device folder
	both := func() string { return names(s.panes[PC]) + " | " + names(s.panes[Device]) }
	mid = runWatching(t, s, 3, both)
	if mid != ">.. c.txt old.txt | >.. a.txt b.txt" {
		t.Errorf("while moving the third: %q", mid)
	}
}

// An overwrite replaces the row of the item it replaces, even when that
// one's name differs in case.
func TestShownFinishedOverwrite(t *testing.T) {
	s, dev, pc := testState(t)
	write(t, dev, "d/b.txt")
	write(t, pc, "p/B.txt", "p/z.txt")
	cd(s, Device, filepath.Join(dev, "d"))
	cd(s, PC, filepath.Join(pc, "p"))
	from, to := s.panes[Device].fs, s.panes[PC].fs
	src := s.panes[Device].rows[1]
	job := &copyJob{from: from, to: to, dstDir: filepath.Join(pc, "p")}
	job.finished = []copyOp{{src: src, name: "b.txt", overwrite: true, existing: filepath.Join(pc, "p", "B.txt")}}
	s.showFinished(job)
	if got := names(s.panes[PC]); got != ">.. b.txt z.txt" {
		t.Errorf("rows %q", got)
	}
	if len(job.takeFinished()) != 0 {
		t.Error("shown items are taken")
	}
}
