package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Multi-device install: devices in parallel, APKs of one device in order.
// On a terminal a status board is redrawn in place and confirmations are
// collected and asked after the other devices finish. Without a terminal
// every message is printed as it happens with a "[device]" prefix.
// Design: docs/spec/parallel-install.md.

type jobState int

const (
	jobRunning jobState = iota
	jobWaiting          // needs a [Y/n/a] answer
	jobDone             // every APK installed
	jobFailed           // at least one APK failed
)

type jobMsg struct {
	kind ui.Kind
	text string
}

type installJob struct {
	serial, name string
	files        []string
	answer       chan string

	mu          sync.Mutex
	idx         int
	activity    string
	state       jobState
	question    string
	reason      string // last warning, shown with the question
	start       time.Time
	took        time.Duration
	msgs        []jobMsg
	results     []int8 // per APK: 0 not run, 1 installed, -1 failed
	interrupted bool
	skipped     bool // the user declined a confirmation
}

func (j *installJob) with(f func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	f()
}

// printMu keeps prefixed lines from different devices whole.
var printMu sync.Mutex

// jobReporter buffers messages for the board, or prints them prefixed (live).
type jobReporter struct {
	j    *installJob
	live bool
}

func (r *jobReporter) emit(k ui.Kind, text string) {
	r.j.with(func() {
		r.j.msgs = append(r.j.msgs, jobMsg{k, text})
		if k == ui.KindWarn {
			r.j.reason = text
		}
	})
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(k, text))
		printMu.Unlock()
	}
}

func (r *jobReporter) progress(f string, a ...any) {
	text := fmt.Sprintf(f, a...)
	word, _, _ := strings.Cut(text, " ")
	r.j.with(func() { r.j.activity = word + "..." })
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(ui.KindStep, text))
		printMu.Unlock()
	}
}

func (r *jobReporter) done() {}

func (r *jobReporter) success(f string, a ...any) {
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(ui.KindSuccess, fmt.Sprintf(f, a...)))
		printMu.Unlock()
	}
}

func (r *jobReporter) warn(f string, a ...any) { r.emit(ui.KindWarn, fmt.Sprintf(f, a...)) }
func (r *jobReporter) fail(f string, a ...any) {
	text := fmt.Sprintf(f, a...)
	if strings.HasPrefix(text, "Skipped") {
		r.j.with(func() { r.j.skipped = true })
	}
	r.emit(ui.KindError, text)
}
func (r *jobReporter) detail(text string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		r.emit(ui.KindDetail, l)
	}
}

// confirm waits for the main loop to ask the question (board mode).
func (r *jobReporter) confirm(q string) (bool, error) {
	if ui.AssumeYes {
		return true, nil
	}
	if r.live {
		return false, ui.ErrNoInput
	}
	r.j.with(func() { r.j.state, r.j.question = jobWaiting, q })
	ans := <-r.j.answer
	r.j.with(func() { r.j.state, r.j.question = jobRunning, "" })
	switch ans {
	case "y", "a":
		return true, nil
	case "interrupt":
		return false, ui.ErrInterrupted
	}
	return false, nil
}

func runJob(j *installJob, opts []string, live bool) {
	r := &jobReporter{j: j, live: live}
	failed := false
	for i, f := range j.files {
		j.with(func() { j.idx, j.activity = i, "Installing..." })
		ok := installOne(r, j.serial, opts, f)
		j.with(func() {
			if ok {
				j.results[i] = 1
			} else {
				j.results[i] = -1
			}
		})
		failed = failed || !ok
	}
	j.with(func() {
		j.took = time.Since(j.start)
		j.state = jobDone
		if failed {
			j.state = jobFailed
		}
	})
}

var spinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// boardLines renders one status line per device.
func boardLines(jobs []*installJob, frame int) []string {
	nameW, apkW := 0, 0
	for _, j := range jobs {
		nameW = max(nameW, len([]rune(j.name)))
		for _, f := range j.files {
			apkW = max(apkW, len(filepath.Base(f)))
		}
	}
	e := ui.Err
	var lines []string
	for _, j := range jobs {
		j.mu.Lock()
		apk := fmt.Sprintf("[%d/%d] %-*s", j.idx+1, len(j.files), apkW, filepath.Base(j.files[j.idx]))
		var icon, status string
		switch {
		case j.interrupted:
			icon, status = e.Red("✗"), e.Red("Interrupted")
		case j.state == jobRunning:
			icon, status = e.Cyan(string(spinner[frame%len(spinner)])), j.activity
		case j.state == jobWaiting:
			icon, status = e.Yellow("⏸"), e.Yellow("Needs confirmation")
		case j.state == jobFailed && j.skipped:
			icon, status = e.Yellow("–"), e.Yellow("Skipped")
		case j.state == jobDone:
			icon, status = e.Green("✓"), "Done "+e.Dim(fmt.Sprintf("(%.1fs)", j.took.Seconds()))
		default:
			icon, status = e.Red("✗"), e.Red("Failed")
		}
		lines = append(lines, fmt.Sprintf("  %s %-*s  %s  %s", icon, nameW, j.name, apk, status))
		j.mu.Unlock()
	}
	return lines
}

func installParallel(devices, files []string, opts []string) error {
	board := ui.Err.TTY
	jobs := make([]*installJob, len(devices))
	for i, d := range devices {
		jobs[i] = &installJob{serial: d, name: deviceName(d), files: files, answer: make(chan string),
			results: make([]int8, len(files)), start: time.Now()}
	}
	ui.Step("Installing %d APK(s) on %d devices", len(files), len(devices))

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j *installJob) { defer wg.Done(); runJob(j, opts, !board) }(j)
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	var b ui.Board
	if board {
		ui.HideCursor()
		defer ui.ShowCursor()
	}
	draw := func(frame int) {
		if board {
			b.Draw(boardLines(jobs, frame))
		}
	}
	interrupted, applyAll := false, false
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

loop:
	for frame := 0; ; frame++ {
		select {
		case <-finished:
			draw(frame)
			break loop
		case <-sig:
			// The terminal sends SIGINT to adb too, so running installs stop on their own.
			interrupted = true
			for _, j := range jobs {
				j.with(func() {
					if j.state == jobRunning || j.state == jobWaiting {
						j.interrupted = true
					}
				})
				select {
				case j.answer <- "interrupt":
				default:
				}
			}
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
			}
			draw(frame)
			break loop
		case <-tick.C:
			draw(frame)
			if waiting := waitingJobs(jobs); len(waiting) > 0 && !anyRunning(jobs) {
				b.Release()
				if !askWaiting(waiting, &applyAll) {
					interrupted = true
				}
			}
		}
	}
	return finishParallel(jobs, board, interrupted)
}

func waitingJobs(jobs []*installJob) []*installJob {
	var w []*installJob
	for _, j := range jobs {
		j.mu.Lock()
		if j.state == jobWaiting {
			w = append(w, j)
		}
		j.mu.Unlock()
	}
	return w
}

func anyRunning(jobs []*installJob) bool {
	for _, j := range jobs {
		j.mu.Lock()
		running := j.state == jobRunning
		j.mu.Unlock()
		if running {
			return true
		}
	}
	return false
}

// askWaiting asks each paused device in turn. It returns false on Ctrl+C.
func askWaiting(waiting []*installJob, applyAll *bool) bool {
	ui.ShowCursor()
	defer ui.HideCursor()
	for i, j := range waiting {
		if *applyAll {
			j.answer <- "y"
			continue
		}
		j.mu.Lock()
		apk, reason, q := filepath.Base(j.files[j.idx]), j.reason, j.question
		j.mu.Unlock()
		ui.Err.Println("")
		ui.Err.Println(ui.Err.Bold(j.name) + " — " + apk)
		if reason != "" {
			ui.Err.Println("  " + ui.Line(ui.KindWarn, reason))
		}
		k, err := ui.Choose("  "+q, "y", "n", "a")
		if err != nil {
			for _, rest := range waiting[i:] {
				rest.answer <- "interrupt"
			}
			return false
		}
		if k == "a" {
			*applyAll = true
		}
		j.answer <- k
	}
	ui.Err.Println("")
	return true
}

func finishParallel(jobs []*installJob, board, interrupted bool) error {
	// Snapshot under the locks: after Ctrl+C some jobs may still be finishing.
	type snap struct {
		state   jobState
		msgs    []jobMsg
		results []int8
	}
	snaps := make([]snap, len(jobs))
	failed := 0
	for i, j := range jobs {
		j.with(func() {
			snaps[i] = snap{j.state, append([]jobMsg(nil), j.msgs...), append([]int8(nil), j.results...)}
		})
		if snaps[i].state != jobDone {
			failed++
		}
	}
	// Details (warnings, errors, adb output) per device; live mode printed them already.
	if board {
		for i, j := range jobs {
			if len(snaps[i].msgs) == 0 {
				continue
			}
			ui.Err.Println("")
			ui.Err.Println(ui.Err.Bold(j.name) + " " + ui.Err.Dim("("+j.serial+")"))
			for _, m := range snaps[i].msgs {
				ui.Err.Println("  " + ui.Line(m.kind, m.text))
			}
		}
	}
	ui.Err.Println("")
	ui.Err.Println(ui.Err.Bold("Summary"))
	nameW := 0
	for _, j := range jobs {
		nameW = max(nameW, len([]rune(j.name)))
	}
	for n, j := range jobs {
		var cells []string
		for i, f := range j.files {
			mark := ui.Err.Dim("-")
			switch snaps[n].results[i] {
			case 1:
				mark = ui.Err.Green("✓")
			case -1:
				mark = ui.Err.Red("✗")
			}
			cells = append(cells, filepath.Base(f)+" "+mark)
		}
		ui.Err.Printf("  %-*s  %s\n", nameW, j.name, strings.Join(cells, "  "))
	}
	switch {
	case interrupted:
		ui.Errorf("Interrupted; installs that were running may be incomplete")
		return ui.ErrInterrupted
	case failed > 0:
		ui.Errorf("%d of %d devices failed", failed, len(jobs))
		return errReported
	}
	ui.Success("Installed on %d devices", len(jobs))
	return nil
}
