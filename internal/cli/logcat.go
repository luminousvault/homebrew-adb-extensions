package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdLogcat = &Command{
	Name: "logcat", Summary: "Show logs of one app", Args: "[package]",
	Long: "Show the logcat lines of one app and keep following it when it restarts (new process IDs\n" +
		"are picked up automatically; processes like 'pkg:remote' are included). The app's lines still\n" +
		"in the device log buffer are shown first (use -c to start empty). Press Ctrl+C to stop.",
	ArgHelp: [][2]string{argPackage},
	Flags: []Flag{
		{"-l", "--level", "LEVEL", "Minimum level: V, D, I, W, E or F (default V)"},
		{"-c", "--clear", "", "Clear the device log buffer first"},
	},
	Examples: []string{"ak logcat", "ak logcat com.example.app -l W", "ak logcat -c > app.log"},
	Device:   oneDevice, Run: runLogcat,
}

const levels = "VDIWEF"

var (
	// threadtime: "11-12 10:31:22.123  1234  1250 I Tag: message"
	reLogLine   = regexp.MustCompile(`^(\d\d-\d\d) (\d\d:\d\d:\d\d\.\d+)\s+(\d+)\s+(\d+) ([VDIWEFS]) (.*?)\s*: (.*)$`)
	reProcStart = regexp.MustCompile(`Start proc (\d+):([^/ ]+)`)
	reProcDied  = regexp.MustCompile(`Process (\S+) \(pid (\d+)\) has died`)
	reProcKill  = regexp.MustCompile(`Killing (\d+):([^/ ]+)/\S+ \(adj -?\d+\): (.*)$`)
)

type logLine struct {
	time, level, tag, msg string
	pid                   int
}

func parseLogLine(s string) (logLine, bool) {
	m := reLogLine.FindStringSubmatch(s)
	if m == nil {
		return logLine{}, false
	}
	pid, _ := strconv.Atoi(m[3])
	return logLine{time: m[2], pid: pid, level: m[5], tag: m[6], msg: m[7]}, true
}

// belongs reports whether a process name is the app or one of its processes ("pkg:remote").
func belongs(name, pkg string) bool { return name == pkg || strings.HasPrefix(name, pkg+":") }

// appPIDs lists the app's process IDs from `ps -A -o PID,NAME`.
func appPIDs(serial, pkg string) map[int]bool {
	pids := map[int]bool{}
	for _, l := range strings.Split(adb.Shell(serial, "ps -A -o PID,NAME"), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && belongs(f[1], pkg) {
			if n, err := strconv.Atoi(f[0]); err == nil {
				pids[n] = true
			}
		}
	}
	return pids
}

func pidList(pids map[int]bool) string {
	var l []string
	for p := range pids {
		l = append(l, strconv.Itoa(p))
	}
	sort.Strings(l)
	return strings.Join(l, ", ")
}

func levelColor(level string) func(string) string {
	o := ui.Out
	switch level {
	case "V":
		return o.Dim
	case "D":
		return o.Cyan
	case "I":
		return o.Green
	case "W":
		return o.Yellow
	}
	return o.Red // E, F
}

// tracker follows the app's PIDs as processes start and die.
type tracker struct {
	mu   sync.Mutex
	pkg  string
	pids map[int]bool
}

func (t *tracker) has(pid int) bool { t.mu.Lock(); defer t.mu.Unlock(); return t.pids[pid] }

// observe updates PIDs from ActivityManager lines and returns an event to show.
func (t *tracker) observe(l logLine) string {
	if m := reProcStart.FindStringSubmatch(l.msg); m != nil && belongs(m[2], t.pkg) {
		pid, _ := strconv.Atoi(m[1])
		t.mu.Lock()
		t.pids[pid] = true
		t.mu.Unlock()
		return fmt.Sprintf("%s started (pid %d)", m[2], pid)
	}
	if m := reProcDied.FindStringSubmatch(l.msg); m != nil && belongs(m[1], t.pkg) {
		return fmt.Sprintf("%s died (pid %s)", m[1], m[2])
	}
	if m := reProcKill.FindStringSubmatch(l.msg); m != nil && belongs(m[2], t.pkg) {
		return fmt.Sprintf("%s killed (pid %s): %s", m[2], m[1], m[3])
	}
	return ""
}

func (t *tracker) merge(pids map[int]bool) {
	t.mu.Lock()
	for p := range pids {
		t.pids[p] = true
	}
	t.mu.Unlock()
}

func runLogcat(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	minLevel := 0
	if v := strings.ToUpper(c.String("--level")); v != "" {
		if len(v) != 1 || !strings.Contains(levels, v) {
			return usageErr(c.Cmd, "--level must be one of V, D, I, W, E, F: %s", v)
		}
		minLevel = strings.Index(levels, v)
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	pkg := ""
	if len(c.Args) == 1 {
		pkg = c.Args[0]
		if !validPackageName(pkg) {
			return failErr("", "Invalid package name: %s", pkg)
		}
	} else {
		pkg = foregroundPackage(serial)
		if err := requirePackage(serial, pkg); err != nil {
			return err
		}
	}
	if c.Bool("--clear") {
		_, _ = adb.Combined("-s", serial, "logcat", "-c")
	}
	t := &tracker{pkg: pkg, pids: appPIDs(serial, pkg)}
	if len(t.pids) == 0 {
		ui.Step("Waiting for %s to start on %s (Ctrl+C to stop)", pkg, deviceName(serial))
	} else {
		ui.Step("Logs of %s (pid %s) on %s (Ctrl+C to stop)", pkg, pidList(t.pids), deviceName(serial))
	}

	cmd, out, err := adb.Start("-s", serial, "logcat", "-v", "threadtime")
	if err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
		case <-done:
		}
		_ = cmd.Process.Kill()
	}()
	// Refresh PIDs in case a start line was missed (e.g. the buffer was cleared).
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				t.merge(appPIDs(serial, pkg))
			}
		}
	}()

	o := ui.Out
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l, ok := parseLogLine(sc.Text())
		if !ok {
			continue
		}
		if ev := t.observe(l); ev != "" {
			o.Println(o.Dim("--- " + ev))
			continue
		}
		if !t.has(l.pid) || strings.Index(levels, l.level) < minLevel {
			continue
		}
		color := levelColor(l.level)
		msg := l.msg
		if l.level == "W" || l.level == "E" || l.level == "F" {
			msg = color(msg)
		}
		o.Printf("%s %s %s %s\n", o.Dim(l.time), color(l.level), o.Bold(l.tag+":"), msg)
	}
	close(done)
	_ = cmd.Wait()
	return nil
}
