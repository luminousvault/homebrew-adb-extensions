package cli

import (
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
		"[Google Pixel 9 Pro] ✓ Installed app.apk on Google Pixel 9 Pro",
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
	if r.code != 1 || !strings.Contains(r.err, "Error: Couldn't install app.apk on Google Pixel 9 Pro: INSTALL_FAILED_INSUFFICIENT_STORAGE") ||
		!strings.Contains(r.err, "app.apk ✗") || !strings.HasSuffix(r.err, "Error: 1 of 2 devices failed\n") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

func TestParallelConflictWithoutTerminalNeedsYes(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	conflict := adbtest.Response{Stdout: "Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package com.example.app signatures do not match newer version; ignoring!]\n"}
	f := parallelFake(apkFile, map[string]adbtest.Response{serial: conflict, "SECOND": {Stdout: "Success\n"}})
	r := ak(t, f, "install", apkFile, "--all")
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
}
