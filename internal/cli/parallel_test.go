package cli

import (
	"bytes"
	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func parallelFake(apkFile string, perDevice map[string]adbtest.Response) *adbtest.Fake {
	f := twoDevices()
	for dev, resp := range perDevice {
		f.Responses["-s "+dev+" install -r "+apkFile] = resp
	}
	return f
}

func TestParallelInstallRunsConcurrently(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	ok := adbtest.Response{Stdout: "Performing Streamed Install\nSuccess\n", Delay: 400 * time.Millisecond}
	f := parallelFake(apkFile, map[string]adbtest.Response{serial: ok, "SECOND": ok})
	start := time.Now()
	r := ak(t, f, "install", apkFile, "--all")
	took := time.Since(start)
	if r.code != 0 {
		t.Fatalf("code %d\n%s", r.code, r.err)
	}
	if took > 700*time.Millisecond {
		t.Errorf("two 400ms installs took %v; not parallel", took)
	}
	// Without a terminal each line is prefixed with the device.
	for _, want := range []string{
		"==> Installing 1 APK(s) on 2 devices",
		"[Google Pixel 9 Pro] ✓ Installed 'app.apk' on Google Pixel 9 Pro",
		"Summary\n",
		"✓ Installed on 2 devices",
	} {
		if !strings.Contains(r.err, want) {
			t.Errorf("missing %q in:\n%s", want, r.err)
		}
	}
}

func TestParallelInstallFailureSummary(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	f := parallelFake(apkFile, map[string]adbtest.Response{
		serial:   {Stdout: "Success\n"},
		"SECOND": {Stdout: "adb: failed to install app.apk: Failure [INSTALL_FAILED_INSUFFICIENT_STORAGE]\n"},
	})
	r := ak(t, f, "install", apkFile, "-s", serial, "-s", "SECOND")
	if r.code != 1 || !strings.Contains(r.err, "Error: Couldn't install 'app.apk' on Google Pixel 9 Pro: INSTALL_FAILED_INSUFFICIENT_STORAGE") ||
		!strings.Contains(r.err, "✗ app.apk  INSTALL_FAILED_INSUFFICIENT_STORAGE") || !strings.HasSuffix(r.err, "Error: 1 of 2 devices failed\n") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

func TestParallelConflictWithoutTerminalNeedsYes(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	conflict := adbtest.Response{Stdout: "Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package com.example.app signatures do not match newer version; ignoring!]\n"}
	f := parallelFake(apkFile, map[string]adbtest.Response{serial: conflict, "SECOND": {Stdout: "Success\n"}})
	r := ak(t, f, "install", apkFile, "-a")
	if r.code != 1 || !strings.Contains(r.err, "uninstalling needs confirmation (pass -y)") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

func TestBoardLines(t *testing.T) {
	ui.Err = &ui.Stream{}
	files := []string{"/x/app-debug.apk", "/x/feature.apk"}
	jobs := []*installJob{
		{name: "Google Pixel 9 Pro", files: files, idx: 0, activity: "Installing...", state: jobRunning},
		{name: "Samsung SM-S928N", files: files, idx: 1, state: jobWaiting},
		{name: "Pixel 7", files: files, idx: 1, state: jobDone, took: 12400 * time.Millisecond},
		{name: "Pixel 6", files: files, idx: 0, state: jobFailed},
		{name: "Pixel 5", files: files, idx: 0, state: jobFailed, skipped: true},
	}
	got := strings.Join(boardLines(jobs, 0), "\n")
	want := strings.Join([]string{
		"  ⠋ Google Pixel 9 Pro  [1/2] app-debug.apk  Installing...",
		"  ⏸ Samsung SM-S928N    [2/2] feature.apk    Needs confirmation",
		"  ✓ Pixel 7             [2/2] feature.apk    Done (12.4s)",
		"  ✗ Pixel 6             [1/2] app-debug.apk  Failed",
		"  – Pixel 5             [1/2] app-debug.apk  Skipped",
	}, "\n")
	if got != want {
		t.Errorf("board:\n%s\nwant:\n%s", got, want)
	}
	// A step that runs long is probably waiting for the user on the device.
	jobs[0].stepStart = time.Now().Add(-slowStep - time.Second)
	if got := boardLines(jobs[:1], 0)[0]; !strings.HasSuffix(got, "Installing... (check the device screen)") {
		t.Errorf("slow step: %q", got)
	}
}

func TestParallelDetailAndSummaryLines(t *testing.T) {
	j := &installJob{serial: "emulator-5554", name: "Pixel 7", files: []string{"a.apk", "b.apk", "c.apk"}}
	msgs := []jobMsg{
		{ui.KindStep, "Installing a.apk on Pixel 7", 0},
		{ui.KindWarn, "a.apk is older than the installed version, retrying with -d", 0},
		{ui.KindStep, "Installing a.apk on Pixel 7", 0},
		{ui.KindStep, "Uninstalling com.example.a from Pixel 7", 0},
		{ui.KindSuccess, "Installed a.apk on Pixel 7 (3.0s)", 0},
		{ui.KindStep, "Installing b.apk on Pixel 7", 1},
		{ui.KindSuccess, "Installed b.apk on Pixel 7 (1.0s)", 1},
	}
	want := []string{"", "Pixel 7 (emulator-5554)", "  a.apk",
		"    Warning: a.apk is older than the installed version, retrying with -d",
		"    ==> Uninstalling com.example.a from Pixel 7",
		"    ✓ Installed a.apk on Pixel 7 (3.0s)"}
	if got := detailLines(j, msgs); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("details:\n%s", strings.Join(got, "\n"))
	}
	if got := detailLines(j, msgs[5:]); got != nil {
		t.Errorf("trouble-free APKs need no details: %q", got)
	}

	other := &installJob{name: "Galaxy S24", files: j.files}
	got := summaryLines([]*installJob{j, other}, func(i int) ([]int8, []string, bool) {
		if i == 0 {
			return []int8{1, 1, 1}, make([]string, 3), false
		}
		return []int8{1, -1, 0}, []string{"", "INSTALL_FAILED_INSUFFICIENT_STORAGE", ""}, false
	})
	want = []string{
		"  ✓ Pixel 7     3/3 installed",
		"  ✗ Galaxy S24  1/3 installed",
		"      ✗ b.apk  INSTALL_FAILED_INSUFFICIENT_STORAGE",
		"      – c.apk  not installed",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("summary:\n%s", strings.Join(got, "\n"))
	}
	got = summaryLines([]*installJob{other}, func(int) ([]int8, []string, bool) {
		return []int8{-1, 0, 0}, []string{"skipped", "", ""}, true
	})
	want = []string{
		"  – Galaxy S24  0/3 installed",
		"      – a.apk  skipped",
		"      – b.apk  not installed",
		"      – c.apk  not installed",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("summary:\n%s", strings.Join(got, "\n"))
	}
}

// After Ctrl+C a device stops: the running APK is reported as interrupted and
// the next APK is not started (it used to start and keep ak waiting).
func TestInstallStopsAfterInterrupt(t *testing.T) {
	dir := t.TempDir()
	first := writeAPK(t, dir, "a.apk", 0)
	second := writeAPK(t, dir, "b.apk", 0)
	f := baseFake()
	f.Responses["-s "+serial+" install -r "+first] = adbtest.Response{Stdout: "Performing Streamed Install\n", Delay: 300 * time.Millisecond}
	f.Responses["-s "+serial+" install -r "+second] = adbtest.Response{Stdout: "Success\n"}
	adb.Default = f
	ui.Err = &ui.Stream{W: &bytes.Buffer{}}
	t.Cleanup(func() { adb.Default = nil; installStop.Store(false); ui.Err = &ui.Stream{W: os.Stderr} })
	installStop.Store(false)
	j := &installJob{serial: serial, name: "Pixel", files: []string{first, second}, answer: make(chan string),
		results: make([]int8, 2), notes: make([]string, 2)}
	time.AfterFunc(100*time.Millisecond, func() { installStop.Store(true) })
	runJob(j, []string{"-r"}, true)
	if j.results[0] != -1 || j.notes[0] != "interrupted" || j.results[1] != 0 {
		t.Errorf("results %v notes %q", j.results, j.notes)
	}
	for _, c := range f.Calls {
		if strings.Contains(c, second) {
			t.Errorf("second APK started after Ctrl+C: %v", f.Calls)
		}
	}
}
