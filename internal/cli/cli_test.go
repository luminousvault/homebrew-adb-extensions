package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

const serial = "TESTSERIAL01"

// Sanitized fixtures shaped like real Pixel/Galaxy output.
const (
	devicesOut = "List of devices attached\n" + serial + "\tdevice\n\n"
	getpropOut = "[ro.build.version.release]: [16]\n[ro.build.version.sdk]: [36]\n[ro.product.brand]: [google]\n" +
		"[ro.product.cpu.abi]: [arm64-v8a]\n[ro.product.cpu.abilist32]: []\n[ro.product.cpu.abilist64]: [arm64-v8a]\n[ro.product.model]: [Pixel 9 Pro]\n"
	activitiesOut = `ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)
Display #0 (activities from top to bottom):
  * Task{a1 #12 type=standard A=10234:com.example.app}
      * Hist  #2: ActivityRecord{1a2 u0 com.example.app/.DetailActivity t12}
        keysPaused=false inHistory=true idle=true
      * Hist  #1: ActivityRecord{1a3 u0 com.example.app/.ListActivity t12}
      * Hist  #0: ActivityRecord{1a4 u0 com.example.app/.MainActivity t12}
  * Task{b1 #2 type=home}
      * Hist  #0: ActivityRecord{2b2 u0 com.google.android.apps.nexuslauncher/.NexusLauncherActivity t2}
`
	packagesOut = "package:com.android.chrome\npackage:com.example.app\n"
	dumpsysPkg  = `Packages:
  Package [com.example.app] (4f2):
    versionCode=1203 minSdk=24 targetSdk=36
    versionName=1.2.3
    flags=[ DEBUGGABLE HAS_CODE ALLOW_CLEAR_USER_DATA ]
    installerPackageName=com.android.vending
    install permissions:
      android.permission.INTERNET: granted=true
    runtime permissions:
      android.permission.CAMERA: granted=true, flags=[ USER_SET ]
      android.permission.READ_CONTACTS: granted=false, flags=[ USER_SET ]
`
)

type result struct {
	out, err string
	code     int
}

// ak runs a command with a fake adb, no colors and no terminal.
func ak(t *testing.T, fake *adbtest.Fake, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	ui.Out = &ui.Stream{W: &out}
	ui.Err = &ui.Stream{W: &errb}
	ui.InTTY, ui.AssumeYes = false, false
	adb.Default = fake
	resetDeviceCache()
	t.Setenv("ANDROID_SERIAL", "")
	t.Cleanup(func() {
		ui.Out, ui.Err = &ui.Stream{W: os.Stdout}, &ui.Stream{W: os.Stderr}
		adb.Default = nil
		selectFn = tui.Select
	})
	code := run(args)
	return result{out.String(), errb.String(), code}
}

func baseFake() *adbtest.Fake {
	return &adbtest.Fake{Responses: map[string]adbtest.Response{
		"devices":                         {Stdout: devicesOut},
		"-s " + serial + " shell getprop": {Stdout: getpropOut},
		"-s " + serial + " shell dumpsys activity activities":     {Stdout: activitiesOut},
		"-s " + serial + " shell pm list packages --user 0":       {Stdout: packagesOut},
		"-s " + serial + " shell dumpsys package com.example.app": {Stdout: dumpsysPkg},
	}}
}

func twoDevices() *adbtest.Fake {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: devicesOut + "SECOND\tdevice\n"}
	for k, v := range f.Responses {
		if strings.HasPrefix(k, "-s "+serial+" ") {
			f.Responses["-s SECOND "+strings.TrimPrefix(k, "-s "+serial+" ")] = v
		}
	}
	return f
}

func TestInfoSplitsStdoutAndStderr(t *testing.T) {
	r := ak(t, baseFake(), "info")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.err)
	}
	if r.err != "==> 'com.example.app' (foreground app on Google Pixel 9 Pro)\n" {
		t.Errorf("stderr %q", r.err)
	}
	want := "  versionName           1.2.3\n  versionCode           1203\n  minSdk                24\n  targetSdk             36\n" +
		"  debuggable            true\n  installerPackageName  com.android.vending\n"
	if r.out != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.out, want)
	}
}

func TestInfoJSON(t *testing.T) {
	r := ak(t, baseFake(), "info", "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(r.out), &got); err != nil || r.err != "" {
		t.Fatalf("json %v, stderr %q, out %s", err, r.err, r.out)
	}
	if got["package"] != "com.example.app" || got["versionCode"] != float64(1203) || got["debuggable"] != true {
		t.Errorf("%v", got)
	}
}

func TestOptionsAnywhereAndSerial(t *testing.T) {
	r := ak(t, twoDevices(), "info", "com.example.app", "-s", "SECOND")
	if r.code != 0 || !strings.Contains(r.err, "'com.example.app' on Google Pixel 9 Pro") {
		t.Errorf("code %d: %s", r.code, r.err)
	}
	r = ak(t, twoDevices(), "info", "--serial=SECOND")
	if r.code != 0 {
		t.Errorf("--serial=: code %d %s", r.code, r.err)
	}
}

func TestAndroidSerialEnv(t *testing.T) {
	f := twoDevices()
	var out bytes.Buffer
	ui.Out = &ui.Stream{W: &out}
	t.Setenv("ANDROID_SERIAL", "SECOND")
	adb.Default = f
	resetDeviceCache()
	defer func() { adb.Default = nil; ui.Out = &ui.Stream{W: os.Stdout} }()
	ui.Err = &ui.Stream{W: &bytes.Buffer{}}
	if code := run([]string{"info"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(strings.Join(f.Calls, "\n"), "-s SECOND shell dumpsys package") {
		t.Errorf("ANDROID_SERIAL not used: %v", f.Calls)
	}
}

func TestMultipleDevicesWithoutTerminal(t *testing.T) {
	r := ak(t, twoDevices(), "info")
	want := "Error: Multiple devices connected: TESTSERIAL01, SECOND\n" +
		"Hint: Use -s <serial> or set ANDROID_SERIAL\n"
	if r.code != 1 || r.err != want || r.out != "" {
		t.Errorf("code %d stderr %q", r.code, r.err)
	}
}

func TestMultipleDevicesPicker(t *testing.T) {
	f := twoDevices()
	f.Responses["-s SECOND shell am force-stop com.example.app"] = adbtest.Response{}
	var got tui.Options
	selectFn = func(o tui.Options) (tui.Result, error) { got = o; return tui.Result{Index: 1}, nil }
	ui.InTTY = true
	r := func() result {
		var out, errb bytes.Buffer
		ui.Out, ui.Err = &ui.Stream{W: &out}, &ui.Stream{W: &errb, TTY: true}
		adb.Default = f
		resetDeviceCache()
		defer func() { adb.Default = nil; selectFn = tui.Select; ui.InTTY = false }()
		code := run([]string{"kill"})
		return result{out.String(), errb.String(), code}
	}()
	if got.Prompt != "Select a device" || got.Items[1] != "Google Pixel 9 Pro (SECOND) Android 16, API 36" {
		t.Errorf("picker %+v", got)
	}
	if r.code != 0 || f.Calls[len(f.Calls)-1] != "-s SECOND shell am force-stop com.example.app" {
		t.Errorf("code %d calls %v", r.code, f.Calls)
	}
	// The picker closes its alternate screen, so the choice is printed.
	if !strings.HasPrefix(r.err, "==> Selected Google Pixel 9 Pro (SECOND) Android 16, API 36\n") {
		t.Errorf("choice not shown: %q", r.err)
	}
}

func TestPickerInterrupted(t *testing.T) {
	selectFn = func(tui.Options) (tui.Result, error) { return tui.Result{}, tui.ErrInterrupted }
	f := twoDevices()
	var errb bytes.Buffer
	ui.Out, ui.Err = &ui.Stream{W: &bytes.Buffer{}}, &ui.Stream{W: &errb, TTY: true}
	ui.InTTY = true
	adb.Default = f
	resetDeviceCache()
	defer func() { adb.Default = nil; selectFn = tui.Select; ui.InTTY = false }()
	if code := run([]string{"kill"}); code != 130 || errb.Len() != 0 {
		t.Errorf("code %d stderr %q", code, errb.String())
	}
}

func TestDeviceErrors(t *testing.T) {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: "List of devices attached\n\n"}
	r := ak(t, f, "info")
	if r.code != 1 || !strings.Contains(r.err, "Error: No devices connected\nHint: Connect a device") {
		t.Errorf("no devices: %d %q", r.code, r.err)
	}
	r = ak(t, baseFake(), "info", "-s", "NOPE")
	if r.code != 1 || !strings.Contains(r.err, "Error: Device not found: NOPE") {
		t.Errorf("unknown serial: %d %q", r.code, r.err)
	}
	f = baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: "List of devices attached\nLOCKED\tunauthorized\n"}
	r = ak(t, f, "info", "-s", "LOCKED")
	if r.code != 1 || !strings.Contains(r.err, "Device LOCKED is unauthorized\nHint: Accept the USB debugging prompt") {
		t.Errorf("unauthorized: %d %q", r.code, r.err)
	}
}

func TestKillMultipleExitCode(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell am force-stop com.example.app"] = adbtest.Response{}
	r := ak(t, f, "kill", "com.example.app", "bad", "com.not.installed", "com.example.app")
	want := "✓ Stopped 'com.example.app'\nError: Invalid package name: 'bad'\n" +
		"Error: Package not installed on Google Pixel 9 Pro: 'com.not.installed'\nWarning: Skipping duplicate package 'com.example.app'\n"
	if r.code != 1 || r.err != want {
		t.Errorf("code %d stderr:\n%s", r.code, r.err)
	}
}

func TestLaunchFailure(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell cmd package resolve-activity --brief com.example.app"] = adbtest.Response{Stdout: "priority=0\ncom.example.app/.MainActivity\n"}
	f.Responses["-s "+serial+" shell am start --user 0 -n com.example.app/.MainActivity"] = adbtest.Response{
		Stdout: "Starting: Intent { cmp=com.example.app/.MainActivity }\nException occurred while executing 'start':\njava.lang.SecurityException\n"}
	r := ak(t, f, "launch", "com.example.app")
	if r.code != 1 || !strings.HasPrefix(r.err, "Error: Couldn't launch 'com.example.app'\n  Starting: Intent") {
		t.Errorf("code %d:\n%s", r.code, r.err)
	}
	if r := ak(t, f, "launch"); r.code != 2 || !strings.Contains(r.err, "Missing package name\nRun 'ak launch --help' for usage.") {
		t.Errorf("missing arg: %d %q", r.code, r.err)
	}
}

func TestHelpAndUsageErrors(t *testing.T) {
	cases := []struct {
		args     []string
		code     int
		out, err string
	}{
		{nil, 0, "ak - ADB extensions kit", ""},
		{[]string{"--help"}, 0, "  install      Install APKs on one or more devices\n", ""},
		{[]string{"info", "-h"}, 0, "Usage:\n  ak info [options] [package]", ""},
		{[]string{"install", "--help"}, 0, "  -f, --filter WORDS", ""},
		{[]string{"nope"}, 2, "", "Error: Unknown command: nope\nHint: Run 'ak --help' to see all commands\n"},
		{[]string{"info", "-x"}, 2, "", "Error: Unknown option: -x\nRun 'ak info --help' for usage.\n"},
		{[]string{"info", "--json=1"}, 2, "", "Error: Option --json does not take a value"},
		{[]string{"info", "-s"}, 2, "", "Error: Option -s requires a value (ID)"},
		{[]string{"devices", "extra"}, 2, "", "Error: Too many arguments: extra"},
		{[]string{"install", "-l", "-e"}, 2, "", "Error: Use only one of -l, -e and -f"},
	}
	for _, c := range cases {
		r := ak(t, baseFake(), c.args...)
		if r.code != c.code || !strings.Contains(r.out, c.out) || !strings.Contains(r.err, c.err) {
			t.Errorf("%v: code %d\nstdout %q\nstderr %q", c.args, r.code, r.out, r.err)
		}
	}
}

func TestDevicesTableAndJSON(t *testing.T) {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: devicesOut[:len(devicesOut)-1] + "OTHERSERIAL\tunauthorized\n"}
	r := ak(t, f, "devices")
	want := "SERIAL        MODEL               ANDROID      ABI                 STATE\n" +
		"TESTSERIAL01  Google Pixel 9 Pro  16 (API 36)  arm64-v8a (64only)  connected\n" +
		"OTHERSERIAL   -                   -            -                   unauthorized\n"
	if r.out != want || r.err != "" {
		t.Errorf("stdout:\n%s\nwant:\n%s\nstderr %q", r.out, want, r.err)
	}
	r = ak(t, f, "devices", "--json")
	var got []map[string]any
	if err := json.Unmarshal([]byte(r.out), &got); err != nil || len(got) != 2 || got[0]["bits"] != "64only" || got[1]["state"] != "unauthorized" {
		t.Errorf("json %v %s", err, r.out)
	}
}

func TestActivities(t *testing.T) {
	r := ak(t, baseFake(), "activities")
	want := "Task #12\n  #2   com.example.app/.DetailActivity  (top)\n  #1   com.example.app/.ListActivity\n  #0   com.example.app/.MainActivity  (root)\n"
	if r.out != want {
		t.Errorf("stdout:\n%q\nwant:\n%q", r.out, want)
	}
	r = ak(t, baseFake(), "activities", "-e", "--json")
	var got []taskJSON
	if err := json.Unmarshal([]byte(r.out), &got); err != nil || len(got) != 2 || got[1].Task != "2" {
		t.Errorf("json %v %s", err, r.out)
	}
}

func TestPermissionsJSON(t *testing.T) {
	r := ak(t, baseFake(), "permissions", "--json")
	if r.out != "{\n  \"granted\": [\n    \"android.permission.INTERNET\",\n    \"android.permission.CAMERA\"\n  ],\n  \"package\": \"com.example.app\"\n}\n" {
		t.Errorf("%s", r.out)
	}
}

func TestMacCommand(t *testing.T) {
	f := baseFake()
	status := "WifiInfo: SSID: \"TestNet\", BSSID: 00:11:22:33:44:55, MAC: ae:12:34:56:78:9a, Supplicant state: COMPLETED\n"
	f.Responses["-s "+serial+" shell getprop ro.build.version.sdk"] = adbtest.Response{Stdout: "36\n"}
	f.Responses["-s "+serial+" shell cmd wifi status 2>/dev/null"] = adbtest.Response{Stdout: status}
	f.Responses["-s "+serial+" shell dumpsys wifi 2>/dev/null | grep -m1 wifi_sta_factory_mac_address"] = adbtest.Response{}
	f.Responses["-s "+serial+" shell settings get secure bluetooth_address"] = adbtest.Response{Stdout: "A0:B1:C2:D3:E4:F5\n"}
	r := ak(t, f, "mac")
	want := "Google Pixel 9 Pro (TESTSERIAL01)\n" +
		"  Wi-Fi (current)  ae:12:34:56:78:9a (randomized, SSID \"TestNet\")\n" +
		"  Wi-Fi (factory)  n/a\n  Bluetooth        a0:b1:c2:d3:e4:f5\n"
	if r.out != want || !strings.HasPrefix(r.err, "Hint: To see the factory MAC") {
		t.Errorf("stdout:\n%s\nstderr %q", r.out, r.err)
	}
}

func TestInstallSelection(t *testing.T) {
	dir := t.TempDir()
	writeAPK(t, dir, "app-release.apk", time.Hour)
	newest := writeAPK(t, dir, "app-debug.apk", 0)
	t.Chdir(dir)
	got, err := selectInstallAPKs(installOptions{latest: true})
	if err != nil || len(got) != 1 || filepath.Base(got[0]) != filepath.Base(newest) {
		t.Errorf("-l picked %v %v", got, err)
	}
	if got, _ := selectInstallAPKs(installOptions{all: true}); len(got) != 2 {
		t.Errorf("-e picked %v", got)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAPK(t, sub, "only-release.apk", 0)
	if _, err := selectInstallAPKs(installOptions{filtered: true, filter: "debug", args: []string{sub}}); err == nil ||
		!strings.Contains(err.Error(), "No APK files match all of: debug") {
		t.Errorf("filter not applied: %v", err)
	}
	// Several APKs without a terminal: ask for explicit choice.
	r := ak(t, baseFake(), "install")
	if r.code != 1 || !strings.Contains(r.err, "Hint: Pass APK files, or use -l (newest) or -e (every APK)") {
		t.Errorf("non-interactive: %d %q", r.code, r.err)
	}
}

func installFake(apkFile string, responses map[string]string) *adbtest.Fake {
	f := baseFake()
	for opts, out := range responses {
		key := "-s " + serial + " install " + opts + " " + apkFile
		if opts == "uninstall" {
			key = "-s " + serial + " uninstall com.example.app"
		}
		f.Responses[key] = adbtest.Response{Stdout: out}
	}
	return f
}

func TestInstallRecovery(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	f := installFake(apkFile, map[string]string{
		"-r":    "Performing Streamed Install\nadb: failed to install app.apk: Failure [INSTALL_FAILED_TEST_ONLY: installPackageLI]\n",
		"-r -t": "Performing Streamed Install\nSuccess\n",
	})
	r := ak(t, f, "install", apkFile)
	// Without a terminal each attempt prints its start line instead of a spinner.
	if r.code != 0 || !strings.Contains(r.err, "==> Installing 'app.apk' on Google Pixel 9 Pro\n"+
		"Warning: 'app.apk' is a test-only APK, retrying with -t\n==> Installing 'app.apk' on Google Pixel 9 Pro\n✓ Installed 'app.apk' on Google Pixel 9 Pro (") {
		t.Errorf("test-only: %d\n%s", r.code, r.err)
	}

	conflict := "adb: failed to install app.apk: Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package com.example.app signatures do not match newer version; ignoring!]\n"
	f = installFake(apkFile, map[string]string{"-r": conflict, "uninstall": "Success\n"})
	r = ak(t, f, "install", apkFile)
	if r.code != 1 || !strings.Contains(r.err, "Warning: The signature of 'app.apk' differs from 'com.example.app' installed on Google Pixel 9 Pro\n"+
		"Error: Skipped 'app.apk' on Google Pixel 9 Pro: uninstalling needs confirmation (pass -y)") {
		t.Errorf("conflict without input: %d\n%s", r.code, r.err)
	}

	// With -y: uninstall once, then reinstall (the fake keeps answering "conflict").
	f = installFake(apkFile, map[string]string{"-r": conflict, "uninstall": "Success\n"})
	calls := 0
	r = ak(t, f, "install", apkFile, "-y")
	for _, c := range f.Calls {
		if c == "-s "+serial+" uninstall com.example.app" {
			calls++
		}
	}
	if calls != 1 {
		t.Errorf("-y should uninstall once, calls %v\n%s", f.Calls, r.err)
	}

	f = installFake(apkFile, map[string]string{"-r": "Failure [INSTALL_FAILED_INSUFFICIENT_STORAGE]\n"})
	r = ak(t, f, "install", apkFile)
	if r.code != 1 || !strings.Contains(r.err, "Error: Couldn't install 'app.apk' on Google Pixel 9 Pro: INSTALL_FAILED_INSUFFICIENT_STORAGE\n  Failure [") {
		t.Errorf("failure: %d\n%s", r.code, r.err)
	}
}

func TestPullNeedsYesForNewDirWithoutTerminal(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "new")
	r := ak(t, baseFake(), "pull", dest)
	if r.code != 1 || !strings.Contains(r.err, "Error: Directory does not exist: '"+dest+"'\nHint: Pass -y to create it") {
		t.Errorf("%d %q", r.code, r.err)
	}
}
func TestPullChoosesWithoutTerminal(t *testing.T) {
	dest := t.TempDir()
	recent := "Row: 0 _data=/storage/emulated/0/Pictures/Screenshot_2.png, date_modified=3, _size=2048\n" +
		"Row: 1 _data=/storage/emulated/0/Download/report.pdf, date_modified=2, _size=10\n" +
		"Row: 2 _data=/storage/emulated/0/Pictures/Screenshot_1.png, date_modified=1, _size=10\n"
	fake := func() *adbtest.Fake {
		f := baseFake()
		f.Responses["-s "+serial+" shell "+recentFilesQuery] = adbtest.Response{Stdout: recent}
		for _, p := range []string{"Pictures/Screenshot_2.png", "Download/report.pdf", "Pictures/Screenshot_1.png"} {
			f.Responses["-s "+serial+" pull /storage/emulated/0/"+p+" "+filepath.Join(dest, filepath.Base(p))] = adbtest.Response{}
		}
		return f
	}
	pulled := func(f *adbtest.Fake) []string {
		var got []string
		for _, c := range f.Calls {
			if strings.HasPrefix(c, "-s "+serial+" pull ") {
				got = append(got, filepath.Base(c))
			}
		}
		return got
	}

	f := fake()
	r := ak(t, f, "pull", dest)
	if r.code != 1 || !strings.Contains(r.err, "interactive selection needs a terminal\nHint: Use -l for the newest file, or -f to narrow it to one") || len(pulled(f)) != 0 {
		t.Errorf("no choice: %d %q", r.code, r.err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-l"}, "Screenshot_2.png"},
		{[]string{"-f", "report"}, "report.pdf"},
		{[]string{"-l", "-f", "screenshot png"}, "Screenshot_2.png"},
	} {
		f = fake()
		r = ak(t, f, append([]string{"pull", dest}, c.args...)...)
		if got := pulled(f); r.code != 0 || len(got) != 1 || got[0] != c.want {
			t.Errorf("%v: %d pulled %v\n%s", c.args, r.code, got, r.err)
		}
	}
	f = fake()
	r = ak(t, f, "pull", dest, "-f", "screenshot")
	if r.code != 1 || len(pulled(f)) != 0 {
		t.Errorf("two matches without terminal: %d %q", r.code, r.err)
	}
	r = ak(t, fake(), "pull", dest, "-f", "nothing")
	if r.code != 1 || !strings.Contains(r.err, "Error: No recent files on Google Pixel 9 Pro match 'nothing'") {
		t.Errorf("no match: %d %q", r.code, r.err)
	}
}

func TestParseForegroundPackage(t *testing.T) {
	if got := parseForegroundPackage(activitiesOut); got != "com.example.app" {
		t.Errorf("got %q", got)
	}
}

func TestHumanSizeAndMeta(t *testing.T) {
	cases := map[string]string{"": "?", "x": "?", "512": "512 B", "1536": "2 KB", "5242880": "5.0 MB", "1610612736": "1.5 GB"}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%q)=%q want %q", in, got, want)
		}
	}
	if got := fileMeta("/storage/emulated/0/Download/a.pdf", "2048"); got != "Download, 2 KB" {
		t.Errorf("meta %q", got)
	}
	if got := fileMeta("/storage/emulated/0/a.pdf", "1"); got != "sdcard, 1 B" {
		t.Errorf("meta %q", got)
	}
}

func TestParseRecentFilesAndUniqueName(t *testing.T) {
	raw := "Row: 0 _data=/storage/emulated/0/DCIM/Camera/a, b.jpg, date_modified=1700000000, _size=2048\nRow: 1 _data=/storage/emulated/0/Download/x.zip, date_modified=1, _size=10\n"
	files := parseRecentFiles(raw)
	if len(files) != 2 || files[0].path != "/storage/emulated/0/DCIM/Camera/a, b.jpg" || files[0].size != "2048" {
		t.Errorf("files %+v", files)
	}
	used := map[string]bool{"a.jpg": true, "a (1).jpg": true, "README": true}
	if got := uniqueName("a.jpg", used); got != "a (2).jpg" {
		t.Errorf("got %q", got)
	}
	if got := uniqueName("README", used); got != "README (1)" {
		t.Errorf("got %q", got)
	}
}

func TestMacHelpers(t *testing.T) {
	if m, ok := normalizeMAC("AA:BB:CC:DD:EE:0F\r\nnext"); !ok || m != "aa:bb:cc:dd:ee:0f" {
		t.Errorf("normalize %q %v", m, ok)
	}
	for _, bad := range []string{"02:00:00:00:00:00", "00:00:00:00:00:00", "null", ""} {
		if _, ok := normalizeMAC(bad); ok {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !randomizedMAC("ae:00:00:00:00:01") || randomizedMAC("ac:00:00:00:00:01") {
		t.Error("randomized bit")
	}
	status := `Wifi is connected to "TestNet"
WifiInfo: SSID: "TestNet", BSSID: 00:11:22:33:44:55, MAC: ae:12:34:56:78:9a, IP: /192.168.0.2, Supplicant state: COMPLETED, Wi-Fi standard: 11ax`
	raw := grepF(status, "Supplicant state: COMPLETED")
	mac, _ := normalizeMAC(macAfter(raw, "MAC: "))
	if mac != "ae:12:34:56:78:9a" || wifiSSID(raw, mac) != "TestNet" {
		t.Errorf("mac %q ssid %q", mac, wifiSSID(raw, mac))
	}
}

func TestSplitExtractArgs(t *testing.T) {
	cases := [][]string{
		{"com.a.b", "com.a.b", ""},
		{"out.apk", "", "out.apk"},
		{"dir/out", "", "dir/out"},
	}
	for _, c := range cases {
		if p, f := splitExtractArgs(c[:1]); p != c[1] || f != c[2] {
			t.Errorf("%v -> %q %q", c[0], p, f)
		}
	}
	if p, f := splitExtractArgs([]string{"out.apk", "com.a.b"}); p != "com.a.b" || f != "out.apk" {
		t.Errorf("reversed -> %q %q", p, f)
	}
}

func TestAppInfoCutSemantics(t *testing.T) {
	a := parseAppInfo("    versionName=1.0 extra=x\n    versionCode=7 minSdk=21 targetSdk=33\n")
	if a.VersionName != "1.0 extra" || a.VersionCode != "7" || a.MinSdk != "21" || a.TargetSdk != "33" || a.Debuggable {
		t.Errorf("%+v", a)
	}
}

func TestActivitiesTaskPrefixNotConfused(t *testing.T) {
	lines := []string{"* Hist #0: ActivityRecord{1 u0 a.b/.A t2}", "* Hist #0: ActivityRecord{2 u0 c.d/.B t12}"}
	if got := linesForTask(lines, "2"); len(got) != 1 {
		t.Errorf("task 2 picked %v", got)
	}
}

func writeAPK(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Add(-age)
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListAPKsOrder(t *testing.T) {
	dir := t.TempDir()
	writeAPK(t, dir, "b.apk", time.Hour)
	writeAPK(t, dir, "a.apk", 2*time.Hour)
	writeAPK(t, dir, "c.apk", 0)
	writeAPK(t, dir, "notes.txt", 0)
	names := func(ps []string) string {
		var n []string
		for _, p := range ps {
			n = append(n, filepath.Base(p))
		}
		return strings.Join(n, ",")
	}
	if got := names(listAPKs(dir, sortNewest)); got != "c.apk,b.apk,a.apk" {
		t.Errorf("newest: %s", got)
	}
	if got := names(listAPKs(dir, sortByName)); got != "a.apk,b.apk,c.apk" {
		t.Errorf("name: %s", got)
	}
}

func TestInstallExplicitFilesSkipPicker(t *testing.T) {
	dir := t.TempDir()
	a := writeAPK(t, dir, "a.apk", 0)
	b := writeAPK(t, dir, "b.apk", 0)
	got, err := apksFromArgs([]string{a, b}, "")
	if err != nil || len(got) != 2 {
		t.Errorf("explicit files should all be installed without asking: %v %v", got, err)
	}
}

func TestInstallLongFlags(t *testing.T) {
	apkFile := writeAPK(t, t.TempDir(), "app.apk", 0)
	f := installFake(apkFile, map[string]string{"-r -t -d": "Performing Streamed Install\nSuccess\n"})
	if r := ak(t, f, "install", apkFile, "--test-only", "--downgrade"); r.code != 0 {
		t.Errorf("--test-only --downgrade: %d\n%s", r.code, r.err)
	}
}

func TestInstallEveryAPKListsThemFirst(t *testing.T) {
	dir := t.TempDir()
	older := writeAPK(t, dir, "app-release.apk", time.Hour)
	newer := writeAPK(t, dir, "app-debug.apk", 0)
	f := baseFake()
	for _, p := range []string{newer, older} {
		f.Responses["-s "+serial+" install -r "+p] = adbtest.Response{Stdout: "Success\n"}
	}
	r := ak(t, f, "install", "-e", dir)
	want := "==> Found 2 APK(s) in '" + dir + "'\n  1. app-debug.apk    1 B · "
	if r.code != 0 || !strings.HasPrefix(r.err, want) || !strings.Contains(r.err, "\n  2. app-release.apk  1 B · ") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

// The device picker clears the screen, so the -e list must come after it.
func TestInstallEveryAPKListAfterDevicePicker(t *testing.T) {
	dir := t.TempDir()
	apk := writeAPK(t, dir, "app.apk", 0)
	f := twoDevices()
	f.Responses["-s SECOND install -r "+apk] = adbtest.Response{Stdout: "Success\n"}
	var errb bytes.Buffer
	before := -1
	selectFn = func(o tui.Options) (tui.Result, error) {
		before = errb.Len()
		return tui.Result{Index: 1, Indices: []int{1}}, nil
	}
	ui.InTTY = true
	ui.Out, ui.Err = &ui.Stream{W: &bytes.Buffer{}}, &ui.Stream{W: &errb, TTY: true}
	adb.Default = f
	resetDeviceCache()
	t.Setenv("ANDROID_SERIAL", "")
	defer func() {
		adb.Default = nil
		selectFn = tui.Select
		ui.InTTY = false
		ui.Out, ui.Err = &ui.Stream{W: os.Stdout}, &ui.Stream{W: os.Stderr}
	}()
	code := run([]string{"install", "-e", dir})
	if code != 0 || before != 0 || !strings.Contains(errb.String(), "1. app.apk") ||
		!strings.HasPrefix(errb.String(), "==> Selected devices (1)\n  Google Pixel 9 Pro (SECOND)") {
		t.Errorf("code %d, %d bytes before the picker\n%s", code, before, errb.String())
	}
}

// APK picker then device picker share one screen: nothing is printed between
// them, and both choices are printed afterwards, in order.
func TestChainedPickersPrintChoicesAfterBoth(t *testing.T) {
	dir := t.TempDir()
	writeAPK(t, dir, "app-release.apk", time.Hour)
	apk := writeAPK(t, dir, "app-debug.apk", 0)
	f := twoDevices()
	f.Responses["-s SECOND install -r "+apk] = adbtest.Response{Stdout: "Success\n"}
	var errb bytes.Buffer
	var at []int
	selectFn = func(o tui.Options) (tui.Result, error) {
		at = append(at, errb.Len())
		if strings.HasPrefix(o.Prompt, "Select APK") {
			return tui.Result{Indices: []int{0}}, nil
		}
		return tui.Result{Indices: []int{1}}, nil
	}
	ui.InTTY = true
	ui.Out, ui.Err = &ui.Stream{W: &bytes.Buffer{}}, &ui.Stream{W: &errb, TTY: true}
	adb.Default = f
	resetDeviceCache()
	t.Setenv("ANDROID_SERIAL", "")
	defer func() {
		adb.Default = nil
		selectFn = tui.Select
		ui.InTTY = false
		ui.Out, ui.Err = &ui.Stream{W: os.Stdout}, &ui.Stream{W: os.Stderr}
	}()
	code := run([]string{"install", dir})
	want := "==> Selected APK files to install (1)\n  app-debug.apk\n\n==> Selected devices (1)\n  Google Pixel 9 Pro (SECOND)"
	if code != 0 || len(at) != 2 || at[0] != 0 || at[1] != 0 || !strings.HasPrefix(errb.String(), want) {
		t.Errorf("code %d, output before pickers %v\n%s", code, at, errb.String())
	}
}

func TestBundledShortOptions(t *testing.T) {
	cases := []struct {
		cmd   *Command
		args  []string
		want  map[string]string // option -> value ("" for a switch)
		args2 []string
	}{
		{cmdInstall, []string{"-ea", "dir"}, map[string]string{"--every-apk": "", "--all": ""}, []string{"dir"}},
		{cmdInstall, []string{"app.apk", "-tdy"}, map[string]string{"--test-only": "", "--downgrade": "", "--yes": ""}, []string{"app.apk"}},
		{cmdLogcat, []string{"-cl", "W"}, map[string]string{"--clear": "", "--level": "W"}, nil},
		{cmdLogcat, []string{"-clW", "com.example.app"}, map[string]string{"--clear": "", "--level": "W"}, []string{"com.example.app"}},
		{cmdRestart, []string{"-r5"}, map[string]string{"--repeat": "5"}, nil},
		{cmdLogcat, []string{"-cl=W"}, map[string]string{"--clear": "", "--level": "W"}, nil},
	}
	for _, tc := range cases {
		c, help, err := parse(tc.cmd, tc.args)
		if err != nil || help {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		for opt, v := range tc.want {
			if !c.Bool(opt) || c.String(opt) != v {
				t.Errorf("%v: %s = %q (set %v)", tc.args, opt, c.String(opt), c.Bool(opt))
			}
		}
		if strings.Join(c.Args, " ") != strings.Join(tc.args2, " ") {
			t.Errorf("%v: args %v", tc.args, c.Args)
		}
	}
	if _, _, err := parse(cmdInstall, []string{"-ez"}); err == nil || !strings.Contains(err.Error(), "Unknown option: -z (in -ez)") {
		t.Errorf("unknown in bundle: %v", err)
	}
	if _, help, _ := parse(cmdInstall, []string{"-eh"}); !help {
		t.Error("-h in a bundle shows help")
	}
}

// One device: declining the uninstall stops there, like with several devices
// (later APKs may depend on the declined one).
func TestInstallDeclineStopsOneDevice(t *testing.T) {
	dir := t.TempDir()
	first := writeAPK(t, dir, "a.apk", time.Hour)
	second := writeAPK(t, dir, "b.apk", 0)
	conflict := "Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package com.example.app signatures do not match newer version; ignoring!]\n"
	f := baseFake()
	f.Responses["-s "+serial+" install -r "+first] = adbtest.Response{Stdout: conflict}
	f.Responses["-s "+serial+" install -r "+second] = adbtest.Response{Stdout: "Success\n"}
	var errb bytes.Buffer
	ui.Out, ui.Err = &ui.Stream{W: &bytes.Buffer{}}, &ui.Stream{W: &errb}
	ui.In, ui.InTTY, ui.AssumeYes = bufio.NewReader(strings.NewReader("n\n")), true, false
	adb.Default = f
	resetDeviceCache()
	t.Setenv("ANDROID_SERIAL", "")
	defer func() {
		adb.Default = nil
		ui.InTTY = false
		ui.In = bufio.NewReader(os.Stdin)
		ui.Out, ui.Err = &ui.Stream{W: os.Stdout}, &ui.Stream{W: os.Stderr}
	}()
	code := run([]string{"install", first, second})
	for _, c := range f.Calls {
		if strings.Contains(c, second) {
			t.Errorf("b.apk installed after declining a.apk:\n%s", errb.String())
		}
	}
	if code != 1 || !strings.Contains(errb.String(), "Error: Skipped 'a.apk'") {
		t.Errorf("code %d\n%s", code, errb.String())
	}
}

const (
	powerDozing = `POWER MANAGER (dumpsys power)

Power Manager State:
  mDirty=0x0
  mWakefulness=Dozing
  mWakefulnessChanging=false
  mIsPowered=true

Power Manager Constants:
mWakefulness=3
`
	powerAwake = "Power Manager State:\n  mDirty=0x0\n  mWakefulness=Awake\n  mWakefulnessChanging=false\n"
)

func TestParseWakefulness(t *testing.T) {
	for in, want := range map[string]string{
		powerDozing:               "dozing",
		powerAwake:                "awake",
		"  mWakefulness=Asleep\n": "asleep",
		"mWakefulness=3\n":        "",
		"":                        "",
	} {
		if got := parseWakefulness(in); got != want {
			t.Errorf("parseWakefulness(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScreenOffWarnings(t *testing.T) {
	t.Chdir(t.TempDir())
	png := string(pngMagic) + "rest-of-image"
	for _, c := range []struct {
		power string
		warn  bool
	}{{powerDozing, true}, {powerAwake, false}, {"", false}} {
		f := baseFake()
		f.Responses["-s "+serial+" shell dumpsys power"] = adbtest.Response{Stdout: c.power}
		f.Responses["-s "+serial+" exec-out screencap -p"] = adbtest.Response{Stdout: png}

		r := ak(t, f, "screenshot", "shot.png")
		want := "Warning: The screen of Google Pixel 9 Pro is off; the image may be black\n" +
			"Hint: Turn the screen on: 'adb -s " + serial + " shell input keyevent WAKEUP'\n"
		if r.code != 0 || r.out != "shot.png\n" || strings.Contains(r.err, want) != c.warn {
			t.Errorf("screenshot (%q): code %d out %q err %q", c.power, r.code, r.out, r.err)
		}

		r = ak(t, f, "info")
		want = "Warning: The screen of Google Pixel 9 Pro is off; 'com.example.app' is the app that was on screen before it turned off\n" +
			"Hint: Pass a package name, or turn the screen on: 'adb -s " + serial + " shell input keyevent WAKEUP'\n"
		if r.code != 0 || strings.Contains(r.err, want) != c.warn {
			t.Errorf("info (%q): code %d err %q", c.power, r.code, r.err)
		}
		if r := ak(t, f, "info", "com.example.app"); strings.Contains(r.err, "Warning") {
			t.Errorf("info with a package must not warn: %q", r.err)
		}
	}
}

func TestRecordFailsFastWhenScreenOff(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell dumpsys power"] = adbtest.Response{Stdout: powerDozing}
	r := ak(t, f, "record", "-t", "3")
	want := "Error: The screen of Google Pixel 9 Pro is off; it can't be recorded\n" +
		"Hint: Turn the screen on: 'adb -s " + serial + " shell input keyevent WAKEUP'\n"
	if r.code != 1 || r.err != want {
		t.Errorf("code %d err %q", r.code, r.err)
	}
	for _, call := range f.Calls {
		if strings.Contains(call, "screenrecord") {
			t.Errorf("screenrecord started: %s", call)
		}
	}
}
