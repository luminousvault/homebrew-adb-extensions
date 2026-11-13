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
// On a terminal a status board is redrawn in place; a device that needs
// confirmation is asked right away while the others keep installing, and
// further questions queue up. Without a terminal every message is printed as
// it happens with a "[device]" prefix.
// Design: docs/dev/cli-design.md (parallel install).

type jobState int

const (
	jobRunning jobState = iota
	jobWaiting          // needs a [Y/n/a] answer
	jobDone             // every APK installed
	jobFailed           // at least one APK failed
)

type jobMsg struct {
	kind  ui.Kind
	text  string // full form, for "[device]" lines
	short string // inside the device's block, under the APK name
	apk   int    // index into files
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
	reason      string // the current APK's warnings (short), shown with the question
	start       time.Time
	took        time.Duration
	msgs        []jobMsg
	results     []int8   // per APK: 0 not run, 1 installed, -1 failed
	notes       []string // per APK: why it failed, for the summary
	stepStart   time.Time
	interrupted bool
	skipped     bool // a confirmation was declined, could not be asked, or was interrupted
	stopped     bool // the user declined: later APKs of this device are not installed
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

func (r *jobReporter) emit(k ui.Kind, m say) {
	r.j.with(func() {
		r.j.msgs = append(r.j.msgs, jobMsg{k, m.full, m.short, r.j.idx})
		if k == ui.KindWarn {
			r.j.reason = joinWarnings(r.j.reason, m.short)
		}
	})
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(k, m.full))
		printMu.Unlock()
	}
}

// joinWarnings puts warnings about one APK on one line:
// "Older than the installed version, retrying with -d; downgrade not allowed ...".
func joinWarnings(a, b string) string {
	if a == "" {
		return b
	}
	if b != "" {
		b = strings.ToLower(b[:1]) + b[1:]
	}
	return a + "; " + b
}

func (r *jobReporter) progress(m say) {
	word, _, _ := strings.Cut(m.full, " ")
	r.j.with(func() {
		r.j.activity = word + "..."
		r.j.stepStart = time.Now()
	})
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(ui.KindStep, m.full))
		printMu.Unlock()
	}
}

func (r *jobReporter) done() {}

func (r *jobReporter) success(m say) {
	r.j.with(func() { r.j.msgs = append(r.j.msgs, jobMsg{ui.KindSuccess, m.full, m.short, r.j.idx}) })
	if r.live {
		printMu.Lock()
		ui.Err.Println(ui.Err.Dim("["+r.j.name+"]") + " " + ui.Line(ui.KindSuccess, m.full))
		printMu.Unlock()
	}
}

func (r *jobReporter) warn(m say) { r.emit(ui.KindWarn, m) }
func (r *jobReporter) fail(m say) {
	r.j.with(func() {
		// "Couldn't install: INSTALL_FAILED_..." -> "INSTALL_FAILED_..."
		note := ""
		if i := strings.LastIndex(m.short, ": "); i >= 0 {
			note = m.short[i+2:]
		}
		if strings.HasPrefix(m.short, "Skipped") {
			r.j.skipped = true
			note = strings.TrimSuffix("skipped: "+note, ": ")
			// Declined by the user (or interrupted): stop this device, as later
			// APKs may depend on this one. Without a terminal nobody declined,
			// so the other APKs still install.
			r.j.stopped = !strings.Contains(m.short, "(pass -y)")
		}
		r.j.notes[r.j.idx] = note
	})
	r.emit(ui.KindError, m)
}
func (r *jobReporter) detail(text string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		r.emit(ui.KindDetail, say{l, l})
	}
}

// confirm waits for the main loop to ask the question (board mode).
func (r *jobReporter) confirm(q say) (bool, error) {
	if ui.AssumeYes {
		return true, nil
	}
	if r.live {
		return false, ui.ErrNoInput
	}
	r.j.with(func() { r.j.state, r.j.question = jobWaiting, q.short })
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
		if installStop.Load() {
			break // Ctrl+C: do not start the next APK
		}
		j.with(func() { j.idx, j.activity, j.reason = i, "Installing...", "" })
		ok := installOne(r, j.serial, opts, f)
		j.with(func() {
			if ok {
				j.results[i] = 1
			} else {
				j.results[i] = -1
			}
		})
		failed = failed || !ok
		// A declined confirmation stops this device: later APKs may depend on this one.
		stop := false
		j.with(func() { stop = j.stopped })
		if stop {
			break
		}
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
		nameW = max(nameW, ui.TextWidth(j.name))
		for _, f := range j.files {
			apkW = max(apkW, ui.TextWidth(filepath.Base(f)))
		}
	}
	e := ui.Err
	var lines []string
	for _, j := range jobs {
		j.mu.Lock()
		apk := fmt.Sprintf("[%d/%d] %s", j.idx+1, len(j.files), ui.Pad(filepath.Base(j.files[j.idx]), apkW))
		var icon, status string
		switch {
		case j.interrupted:
			icon, status = e.Red("✗"), e.Red("Interrupted")
		case j.state == jobRunning:
			icon, status = e.Cyan(string(spinner[frame%len(spinner)])), j.activity
			if !j.stepStart.IsZero() && time.Since(j.stepStart) > slowStep {
				status += " " + e.Dim(slowHint)
			}
		case j.state == jobWaiting:
			icon, status = e.Yellow("⏸"), e.Yellow("Needs confirmation")
		case j.state == jobFailed && j.skipped:
			icon, status = e.Yellow("–"), e.Yellow("Skipped")
		case j.state == jobDone:
			icon, status = e.Green("✓"), "Done "+e.Dim(fmt.Sprintf("(%.1fs)", j.took.Seconds()))
		default:
			icon, status = e.Red("✗"), e.Red("Failed")
		}
		lines = append(lines, fmt.Sprintf("  %s %s  %s  %s", icon, ui.Pad(j.name, nameW), apk, status))
		j.mu.Unlock()
	}
	return lines
}

func installParallel(devices, files []string, opts []string) error {
	board := ui.Err.TTY
	jobs := make([]*installJob, len(devices))
	for i, d := range devices {
		jobs[i] = &installJob{serial: d, name: deviceName(d), files: files, answer: make(chan string),
			results: make([]int8, len(files)), notes: make([]string, len(files)), start: time.Now()}
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
	// After questions the board starts again below them, showing only the
	// devices that were still unfinished; finished ones are already on screen.
	active := jobs
	draw := func(frame int) {
		if board {
			b.Draw(boardLines(active, frame))
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
			// The terminal sends SIGINT to adb too, so running installs stop on their
			// own; installStop keeps jobs from starting the next APK or a retry.
			installStop.Store(true)
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
			// Ask as soon as a device needs confirmation; the other devices keep
			// installing meanwhile, and the board resumes below the questions.
			if len(waitingJobs(jobs)) > 0 {
				b.Release()
				active = unfinished(jobs)
				if !askWaiting(jobs, &applyAll) {
					// Ctrl+C at a question reached ak as a key, not as SIGINT, so
					// the installs still running would go on: stop them like a
					// Ctrl+C during the board (the sig case below finishes up).
					interrupted = true
					installStop.Store(true)
					interruptGroup()
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

func unfinished(jobs []*installJob) []*installJob {
	var u []*installJob
	for _, j := range jobs {
		j.mu.Lock()
		if j.state == jobRunning || j.state == jobWaiting {
			u = append(u, j)
		}
		j.mu.Unlock()
	}
	return u
}

// askWaiting asks paused devices one at a time, in board order, until none is
// waiting: a device that pauses while a question is on screen is asked next
// (a queue), so nobody waits for the other devices to finish. It returns false
// on Ctrl+C.
func askWaiting(jobs []*installJob, applyAll *bool) bool {
	ui.ShowCursor()
	defer ui.HideCursor()
	// answer resumes the job; its state changes here, before the send, so the
	// next scan cannot ask the same device twice.
	answer := func(j *installJob, a string) {
		j.with(func() { j.state, j.question = jobRunning, "" })
		j.answer <- a
	}
	for asked := 0; ; asked++ {
		waiting := waitingJobs(jobs)
		if len(waiting) == 0 {
			if asked > 0 {
				ui.Err.Println("") // keep the questions apart from the board below
			}
			return true
		}
		j := waiting[0]
		if *applyAll {
			answer(j, "y")
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
		k, err := ui.Choose("  "+q, "(a: yes for every device)", "y", "n", "a")
		if err != nil {
			for _, rest := range waitingJobs(jobs) {
				answer(rest, "interrupt")
			}
			ui.Err.Println("")
			return false
		}
		if k == "a" {
			*applyAll = true
		}
		answer(j, k)
	}
}

func finishParallel(jobs []*installJob, board, interrupted bool) error {
	// Snapshot under the locks: after Ctrl+C some jobs may still be finishing.
	type snap struct {
		state   jobState
		msgs    []jobMsg
		results []int8
		notes   []string
		skipped bool
	}
	snaps := make([]snap, len(jobs))
	failed := 0
	for i, j := range jobs {
		j.with(func() {
			snaps[i] = snap{j.state, append([]jobMsg(nil), j.msgs...), append([]int8(nil), j.results...),
				append([]string(nil), j.notes...), j.skipped}
		})
		if snaps[i].state != jobDone {
			failed++
		}
	}
	// Details per device; live mode printed them already.
	if board {
		for i, j := range jobs {
			for _, l := range detailLines(j, snaps[i].msgs, snaps[i].results) {
				ui.Err.Println(l)
			}
		}
	}
	ui.Err.Println("")
	ui.Err.Println(ui.Err.Bold("Summary"))
	for _, l := range summaryLines(jobs, func(i int) ([]int8, []string, bool) {
		return snaps[i].results, snaps[i].notes, snaps[i].skipped
	}) {
		ui.Err.Println(l)
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

// summaryLines prints one line per device ("✓ Pixel 7  5/5 installed") and,
// under a device that did not finish, the APKs that failed or did not run.
func summaryLines(jobs []*installJob, get func(int) ([]int8, []string, bool)) []string {
	e := ui.Err
	nameW := 0
	for _, j := range jobs {
		nameW = max(nameW, ui.TextWidth(j.name))
	}
	var lines []string
	for n, j := range jobs {
		results, notes, skipped := get(n)
		ok := 0
		for _, r := range results {
			if r == 1 {
				ok++
			}
		}
		failed := false
		for i, r := range results {
			failed = failed || (r == -1 && !strings.HasPrefix(notes[i], "skipped"))
		}
		icon := e.Green("✓")
		switch {
		case ok == len(results):
		case failed || !skipped:
			icon = e.Red("✗") // a real failure outranks a skip
		default:
			icon = e.Yellow("–")
		}
		lines = append(lines, fmt.Sprintf("  %s %s  %d/%d installed", icon, ui.Pad(j.name, nameW), ok, len(results)))
		if ok == len(results) {
			continue
		}
		for i, f := range j.files {
			switch results[i] {
			case -1:
				if strings.HasPrefix(notes[i], "skipped") {
					lines = append(lines, "      "+e.Yellow("–")+" "+filepath.Base(f)+"  "+e.Dim(notes[i]))
					continue
				}
				line := "      " + e.Red("✗") + " " + filepath.Base(f)
				if notes[i] != "" {
					line += "  " + e.Dim(notes[i])
				}
				lines = append(lines, line)
			case 0:
				lines = append(lines, "      "+e.Dim("– "+filepath.Base(f)+"  not installed"))
			}
		}
	}
	return lines
}

// detailLines tells, for each APK that needed attention (a warning or an
// error), what happened, in the short form since the device and the APK are
// the headings. Warnings about one APK share a line; the outcome follows:
//
//	Samsung SM-S928N (SERIAL)
//	  app.apk
//	    Warning: Older than the installed version, retrying with -d; downgrade not allowed without uninstalling
//	    ✓ Installed (24.9s), after uninstalling 'com.example.app' (its data was removed)
//	  ✓ 4 other APK(s) installed
//
// APKs without trouble are only counted on the last line, so the block does
// not read as if the device stopped after the APK shown.
func detailLines(j *installJob, msgs []jobMsg, results []int8) []string {
	eventful := map[int]bool{}
	for _, m := range msgs {
		if m.kind == ui.KindWarn || m.kind == ui.KindError {
			eventful[m.apk] = true
		}
	}
	if len(eventful) == 0 {
		return nil
	}
	lines := []string{"", ui.Err.Bold(j.name) + " " + ui.Err.Dim("("+j.serial+")")}
	for i, f := range j.files {
		if !eventful[i] {
			continue
		}
		lines = append(lines, "  "+filepath.Base(f))
		warning := ""
		flush := func() {
			if warning != "" {
				lines = append(lines, "    "+ui.Line(ui.KindWarn, warning))
				warning = ""
			}
		}
		for _, m := range msgs {
			if m.apk != i {
				continue
			}
			if m.kind == ui.KindWarn {
				warning = joinWarnings(warning, m.short)
				continue
			}
			flush()
			lines = append(lines, "    "+ui.Line(m.kind, m.short))
		}
		flush()
	}
	installed, notRun := 0, 0
	for i, r := range results {
		switch {
		case eventful[i]:
		case r == 1:
			installed++
		case r == 0:
			notRun++
		}
	}
	if installed > 0 {
		lines = append(lines, "  "+ui.Line(ui.KindSuccess, fmt.Sprintf("%d other APK(s) installed", installed)))
	}
	if notRun > 0 {
		lines = append(lines, "  "+ui.Err.Dim(fmt.Sprintf("– %d other APK(s) not installed", notRun)))
	}
	return lines
}
