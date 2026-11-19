package files

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Options configures Run.
type Options struct {
	Serial     string
	DeviceName string // shown in the header, e.g. "Galaxy S24 Ultra"
	DevicePath string // first device folder (default /sdcard)
	LocalPath  string // first PC folder (default: the current folder)
	// OpenLocal opens a PC file with its default app (Enter on a file).
	// Device files are copied to a temp folder first.
	OpenLocal func(path string) error
}

// Summary is what ak files did, for stderr after the screen closes
// (files-design.md §8).
type Summary struct {
	Done     []string   // "Pulled 3 files from Pixel → ~/Downloads"
	Items    [][]string // per Done line: the names it covers when there are several, else nil
	Canceled []string   // "Canceled pulling from Pixel → ~/Downloads: 1 of 3 pulled"
	Failed   []string   // "Couldn't pull 'a.txt' from Pixel → ~/Downloads: <reason>"
}

// Print writes the summary to stderr: ✓ lines, Warning: lines for
// canceled pushes and pulls, then Error: lines.
func (s Summary) Print() {
	for i, d := range s.Done {
		ui.Success("%s", d)
		if i < len(s.Items) {
			printItems(s.Items[i])
		}
	}
	for _, c := range s.Canceled {
		ui.Warn("%s", c)
	}
	for _, f := range s.Failed {
		ui.Errorf("%s", f)
	}
}

// summaryItemsShown is how many names a summary line lists; the rest are
// counted, so deleting 500 files does not bury the other lines.
const summaryItemsShown = 20

// printItems lists names under a summary line, indented and dim.
func printItems(items []string) {
	shown := items
	if len(items) > summaryItemsShown {
		shown = items[:summaryItemsShown]
	}
	var b strings.Builder
	for _, n := range shown {
		b.WriteString(n + "\n")
	}
	if more := len(items) - len(shown); more > 0 {
		fmt.Fprintf(&b, "… and %d more\n", more)
	}
	if b.Len() > 0 {
		ui.Detail("%s", b.String())
	}
}

// HasFailures reports whether anything failed (exit code 1).
func (s Summary) HasFailures() bool { return len(s.Failed) > 0 }

// Run shows the file manager until the user quits. It fails with
// tui.ErrNoTerminal without a terminal, and returns tui.ErrInterrupted
// (exit 130) when it ended with Ctrl+C or a signal; the summary is valid
// either way.
func Run(o Options) (Summary, error) {
	if o.DevicePath == "" {
		o.DevicePath = "/sdcard"
	}
	if o.LocalPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Summary{}, err
		}
		o.LocalPath = wd
	}
	if o.DeviceName == "" {
		o.DeviceName = o.Serial
	}
	scr, err := tui.OpenScreen()
	if err != nil {
		return Summary{}, err
	}
	defer scr.Close()
	cleanTempDir(tempDir(), time.Now()) // files opened a day ago or more (§5)

	st := newState(&deviceFS{serial: o.Serial, name: o.DeviceName}, localFS{PC, computerLabel},
		o.DevicePath, o.LocalPath, o.OpenLocal)
	err = loop(scr, st)
	return st.summary, err
}

// loop runs keys, redraws and tasks until the state asks to exit.
func loop(scr *tui.Screen, st *state) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, adb.StopSignals...) // a kill or a closed terminal: stop adb, restore the terminal
	defer signal.Stop(sig)

	var (
		latest   progressBox
		finished = make(chan func(*state), 1)
	)
	start := func(t *task) context.CancelFunc {
		ctx, cancel := context.WithCancel(context.Background())
		latest.reset() // a task that reports nothing shows a spinner, not the last task's bar
		go func() { finished <- t.run(ctx, latest.set) }()
		return cancel
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	cancel := context.CancelFunc(func() {})
	lastW, lastH := 0, 0
	draw, clear := true, false
	keys := scr.Keys()
	for {
		if st.canceled {
			cancel()
		}
		if st.task == nil && st.pending != nil {
			st.task, st.pending, st.canceled = st.pending, nil, false
			st.task.started = time.Now()
			st.prog = progress{}
			cancel = start(st.task)
		}
		if st.exit != exitNone && st.task == nil {
			if st.exit == exitInterrupt {
				return tui.ErrInterrupted
			}
			return nil
		}
		// Windows has no resize signal: a changed size is noticed here.
		if w, h := scr.Size(); w != lastW || h != lastH {
			clear = clear || lastW != 0
			draw = true
			lastW, lastH = w, h
			st.setSize(w, h)
		}
		if draw || clear {
			scr.Draw(st.frame(), clear)
			draw, clear = false, false
		}

		select {
		case batch, ok := <-keys:
			if !ok {
				keys = nil // stdin closed: quit once the running task ends
				st.quit(exitQuit)
			}
			for _, k := range batch {
				st.handle(k)
			}
			draw = true
		case <-scr.Resized():
			clear = true
		case apply := <-finished:
			cancel()
			st.task, st.canceled = nil, false
			apply(st)
			draw = true
		case <-tick.C:
			if st.scrolling && !time.Now().Before(st.scrollNext) { // a long cursor name moves now
				draw = true
			}
			if st.task != nil {
				st.spin++
				st.prog = latest.get()
				draw = true
			}
		case <-sig:
			st.quit(exitInterrupt)
		}
	}
}

// progressBox is the latest progress a running task reported, read by the
// key loop's ticks.
type progressBox struct {
	mu sync.Mutex
	p  progress
}

func (b *progressBox) set(p progress) { b.mu.Lock(); b.p = p; b.mu.Unlock() }
func (b *progressBox) reset()         { b.set(progress{}) }
func (b *progressBox) get() progress  { b.mu.Lock(); defer b.mu.Unlock(); return b.p }
