package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdRecord = &Command{
	Name: "record", Summary: "Record the screen", Args: "[file|directory]",
	Long: "Record the screen to MP4 until you press Ctrl+C or the time limit is reached, then save the\n" +
		"video on this computer and delete it from the device. There is no time limit unless -t gives\n" +
		"one; devices whose screenrecord cannot drop the limit (older Android) stop at 180 seconds.\n" +
		"Audio is not recorded. The saved path is printed to stdout.\n" +
		"Records one device at a time (pick it with -s).",
	ArgHelp: [][2]string{{"file", "Output .mp4 file (default: record-<model>-<time>.mp4)"}, {"directory", "Save into this directory"}},
	Flags: []Flag{
		{"-t", "--time-limit", "SECONDS", "Stop after this many seconds (0 = no limit, the default; 1-180 on older Android)"},
		{"", "--size", "WxH", "Video size, e.g. 1280x720 (default: screen size)"},
		flagOpen,
	},
	Examples: []string{"ak record", "ak record -o", "ak record bug.mp4", "ak record -t 30 --size 720x1600"},
	Device:   oneDevice, Run: runRecord,
}

var reSize = regexp.MustCompile(`^\d+x\d+$`)

// remoteSize returns the size of a device file, or -1.
func remoteSize(serial, path string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(adb.Shell(serial, "stat -c %s "+path+" 2>/dev/null")), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// waitStable waits until the device file stops growing (screenrecord finishing the MP4).
func waitStable(serial, path string, timeout time.Duration) int64 {
	deadline := time.Now().Add(timeout)
	last := remoteSize(serial, path)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		n := remoteSize(serial, path)
		if n == last && n > 0 {
			return n
		}
		last = n
	}
	return last
}

// noTimeLimit reports whether screenrecord accepts --time-limit 0 (newer
// Android; its --help says "Set to 0 to remove the time limit").
func noTimeLimit(serial string) bool {
	return strings.Contains(adb.Shell(serial, "screenrecord --help 2>&1"), "remove the time limit")
}

// recordClock shows elapsed time, with the limit when there is one: "1:05" or "1:05 / 3:00".
func recordClock(el, limit int) string {
	s := fmt.Sprintf("%d:%02d", el/60, el%60)
	if limit > 0 {
		s += fmt.Sprintf(" / %d:%02d", limit/60, limit%60)
	}
	return s
}

func runRecord(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	limit := -1 // not given: no limit where the device allows it
	if v := c.String("--time-limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return usageErr(c.Cmd, "--time-limit must be 0 (no limit) or a number of seconds: %s", q(v))
		}
		limit = n
	}
	size := c.String("--size")
	if size != "" && !reSize.MatchString(size) {
		return usageErr(c.Cmd, "--size must look like 1280x720: %s", q(size))
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	// screenrecord fails on a screen that is off (UNASSIGNED_LAYER_STACK),
	// and finding that out after it exits takes several seconds.
	if screenOff(serial) {
		return failErr(wakeHint(serial),
			"Couldn't record the screen on %s: the screen is off", deviceName(serial))
	}
	if !noTimeLimit(serial) {
		switch {
		case limit < 0:
			limit = 180
		case limit == 0 || limit > 180:
			return failErr("Pass -t with 1-180 seconds", "%s records at most 180 seconds", deviceName(serial))
		}
	} else if limit < 0 {
		limit = 0
	}
	arg := ""
	if len(c.Args) == 1 {
		arg = c.Args[0]
	}
	now := time.Now()
	local, err := outputPath(arg, serial, "record", ".mp4", now, false)
	if err != nil {
		return err
	}
	remote := "/sdcard/ak-record-" + now.Format("20060102-150405") + ".mp4"
	args := []string{"-s", serial, "shell", "screenrecord", "--time-limit", strconv.Itoa(limit)}
	if size != "" {
		args = append(args, "--size", size)
	}
	cmd, out, err := adb.Start(append(args, remote)...)
	if err != nil {
		return err
	}
	var log bytes.Buffer
	go func() { _, _ = io.Copy(&log, out) }()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, adb.StopSignals...)
	defer signal.Stop(sig)

	start := time.Now()
	// The clock changes every second, so it goes in with Update: off a
	// terminal only the step itself is printed.
	recording := "Recording the screen on " + deviceName(serial)
	sp := ui.Spin("%s (Ctrl+C to stop)", recording)
	sp.Update("%s (%s, Ctrl+C to stop)", recording, recordClock(0, limit))
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	stopped := false
wait:
	for {
		select {
		case werr := <-exited:
			debuglog.Logf("screenrecord exited: %v", werr)
			break wait
		case <-sig:
			if stopped {
				continue
			}
			stopped = true
			sp.SetText("Stopping the recording on %s", deviceName(serial))
			// SIGINT lets screenrecord finish the MP4 properly.
			if _, err := adb.Combined("-s", serial, "shell", "pkill", "-INT", "screenrecord"); err != nil {
				_, _ = adb.Combined("-s", serial, "shell", "kill -2 $(pidof screenrecord)")
			}
			go func() {
				time.Sleep(15 * time.Second)
				_ = cmd.Process.Kill()
			}()
		case <-tick.C:
			if !stopped {
				el := int(time.Since(start).Seconds())
				sp.Update("%s (%s, Ctrl+C to stop)", recording, recordClock(el, limit))
			}
		}
	}
	duration := time.Since(start)
	sp.SetText("Saving the recording from %s", deviceName(serial))
	n := waitStable(serial, remote, 10*time.Second)
	if n <= 0 {
		sp.Stop()
		ui.Errorf("Couldn't save the recording on %s", deviceName(serial))
		if s := strings.TrimSpace(log.String()); s != "" {
			ui.Detail("%s", s)
			if strings.Contains(s, "UNASSIGNED_LAYER_STACK") {
				ui.Hint("%s", wakeHint(serial))
			}
		}
		_, _ = adb.Combined("-s", serial, "shell", "rm", "-f", remote)
		return errReported
	}
	pullOut, err := adb.Combined("-s", serial, "pull", remote, local)
	_, _ = adb.Combined("-s", serial, "shell", "rm", "-f", remote)
	sp.Stop()
	if err != nil {
		ui.Errorf("Couldn't pull the recording from %s", deviceName(serial))
		ui.Detail("%s", pullOut)
		return errReported
	}
	abs, shown := savedPath(local)
	ui.Success("Saved %s %s", q(shown), ui.Err.Dim(fmt.Sprintf("(%s, %s, %ds)", deviceName(serial), files.HumanSize(n), int(duration.Seconds()))))
	ui.Out.Println(abs)
	openSaved(c, local)
	return nil
}
