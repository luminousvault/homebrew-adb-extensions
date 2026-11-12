package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

const serial = "TESTSERIAL01"

var reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

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

// run executes ak with a fake adb and returns plain (ANSI-stripped) output and exit code.
func run(t *testing.T, fake *adbtest.Fake, input string, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	stdout, stderr = &out, &out
	stdin = bufio.NewReader(strings.NewReader(input))
	adb.Default = fake
	t.Cleanup(func() {
		stdout, stderr = os.Stdout, os.Stderr
		adb.Default = nil
		selectFn = tui.Select
	})
	code := Main(args)
	return reANSI.ReplaceAllString(out.String(), ""), code
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

func TestParseForegroundPackage(t *testing.T) {
	if got := parseForegroundPackage(activitiesOut); got != "com.example.app" {
		t.Errorf("got %q", got)
	}
}

func TestInfo(t *testing.T) {
	out, code := run(t, baseFake(), "", "info")
	want := `
Auto-detected package: com.example.app

APK Detailed Info:
  versionName: 1.2.3
  versionCode: 1203
  minSdk: 24
  targetSdk: 36
  debuggable: true
  installerPackageName: com.android.vending

`
	if code != 0 || out != want {
		t.Errorf("code %d, output:\n%q\nwant:\n%q", code, out, want)
	}
}

func TestInfoNotInstalled(t *testing.T) {
	out, code := run(t, baseFake(), "", "info", "com.missing.app")
	if code != 1 || !strings.Contains(out, "ERROR: Package not installed on the device: com.missing.app") {
		t.Errorf("code %d:\n%s", code, out)
	}
}

func TestPermissions(t *testing.T) {
	out, _ := run(t, baseFake(), "", "permissions", "com.example.app")
	if !strings.Contains(out, "Permissions for package com.example.app:\nandroid.permission.INTERNET\nandroid.permission.CAMERA\n") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "READ_CONTACTS") {
		t.Error("denied permission listed")
	}
}

func TestActivities(t *testing.T) {
	out, code := run(t, baseFake(), "", "activities")
	want := `
==> Fetching activity stack...

==> Activity Stack (Task #12)
  [#2] com.example.app/.DetailActivity
  [#1] com.example.app/.ListActivity
  [#0] com.example.app/.MainActivity
Total: 3 activities in this task

`
	if code != 0 || out != want {
		t.Errorf("output:\n%q\nwant:\n%q", out, want)
	}
	out, _ = run(t, baseFake(), "", "activities", "--all")
	if !strings.Contains(out, "(Task #12)") || !strings.Contains(out, "(Task #2)") || strings.Count(out, "Total:") != 2 {
		t.Errorf("--all output:\n%s", out)
	}
}

func TestDevicesTable(t *testing.T) {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: devicesOut[:len(devicesOut)-1] + "OTHERSERIAL\tunauthorized\n"}
	out, _ := run(t, f, "", "devices")
	for _, want := range []string{
		"Device ID        Model                 Android       CPU                 Status",
		"TESTSERIAL01     Google Pixel 9 Pro    16 (API 36)   arm64-v8a (64only)  Connected  ",
		"OTHERSERIAL      -                     -             -                   Unauthorized",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestNoDevices(t *testing.T) {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: "List of devices attached\n\n"}
	out, code := run(t, f, "", "info")
	if code != 1 || !strings.Contains(out, "Error: No connected devices found.") {
		t.Errorf("code %d: %s", code, out)
	}
}

func TestMultipleDevicesUsePicker(t *testing.T) {
	f := baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: devicesOut + "SECOND\tdevice\n"}
	f.Responses["-s SECOND shell getprop"] = adbtest.Response{Stdout: getpropOut}
	f.Responses["-s SECOND shell dumpsys activity activities"] = adbtest.Response{Stdout: activitiesOut}
	f.Responses["-s SECOND shell pm list packages --user 0"] = adbtest.Response{Stdout: packagesOut}
	f.Responses["-s SECOND shell am force-stop com.example.app"] = adbtest.Response{}
	var got tui.Options
	selectFn = func(o tui.Options) (tui.Result, error) { got = o; return tui.Result{Index: 1}, nil }
	out, _ := run(t, f, "", "kill")
	if got.Prompt != "Select a device" || !got.NoFilter || got.Items[1] != "Google Pixel 9 Pro (SECOND) Android 16, API 36" {
		t.Errorf("picker options %+v", got)
	}
	if !strings.Contains(out, "Successfully killed") || f.Calls[len(f.Calls)-1] != "-s SECOND shell am force-stop com.example.app" {
		t.Errorf("calls %v\n%s", f.Calls, out)
	}
	selectFn = func(tui.Options) (tui.Result, error) { return tui.Result{}, tui.ErrInterrupted }
	if _, code := run(t, f, "", "kill"); code != 130 {
		t.Errorf("Ctrl+C exit code %d", code)
	}
}

func TestKillMultiple(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell am force-stop com.example.app"] = adbtest.Response{}
	out, _ := run(t, f, "", "kill", "com.example.app", "bad", "com.not.installed", "com.example.app")
	for _, want := range []string{
		"Starting to kill packages...",
		"==> Attempting to kill: com.example.app\nSuccessfully killed",
		"ERROR: Invalid package name format: bad",
		"ERROR: Package not installed on the device: com.not.installed",
		"WARNING: Skipping duplicate package: com.example.app",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestLaunchFailure(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell cmd package resolve-activity --brief com.example.app"] = adbtest.Response{Stdout: "priority=0\ncom.example.app/.MainActivity\n"}
	f.Responses["-s "+serial+" shell am start --user 0 -n com.example.app/.MainActivity"] = adbtest.Response{
		Stdout: "Starting: Intent { cmp=com.example.app/.MainActivity }\nException occurred while executing 'start':\njava.lang.SecurityException\n"}
	out, code := run(t, f, "", "launch", "com.example.app")
	if code != 1 || !strings.Contains(out, "ERROR: Failed to launch app. Reason:\n  Starting: Intent") || !strings.Contains(out, "\n  java.lang.SecurityException") {
		t.Errorf("code %d:\n%s", code, out)
	}
}

func TestHelpAndErrors(t *testing.T) {
	cases := []struct {
		args []string
		code int
		want string
	}{
		{nil, 0, "Usage: ak <command> [arguments...]"},
		{[]string{"nope"}, 1, "Error: Unknown command: nope"},
		{[]string{"info", "-h"}, 1, "Usage: ak info [-h|--help] [packageName]"},
		{[]string{"install", "--help"}, 0, "APK Selection Options"},
		{[]string{"info", "-x"}, 1, "Error: Invalid option: -x\nTry 'ak info --help' for more information."},
		{[]string{"devices", "extra"}, 1, "Error: Unexpected argument: extra"},
		{[]string{"install", "-l", "-a"}, 1, "Options -l and -a cannot be used together."},
		{[]string{"install", "-f"}, 1, "Option -f requires a filter argument."},
		{[]string{"install", "--foo"}, 1, "Invalid option: --foo"},
	}
	for _, c := range cases {
		out, code := run(t, baseFake(), "", c.args...)
		if code != c.code || !strings.Contains(out, c.want) {
			t.Errorf("%v: code %d, missing %q:\n%s", c.args, code, c.want, out)
		}
	}
}

func TestGlobalHelpAlignment(t *testing.T) {
	out, _ := run(t, baseFake(), "", "--help")
	for _, want := range []string{
		"  install [options] [apk_files...] Install APK files\n",
		"  pull [directory]                 Pull recent files from device\n",
		"  devices                          List connected devices\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
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

func TestMacCommand(t *testing.T) {
	f := baseFake()
	status := "WifiInfo: SSID: \"TestNet\", BSSID: 00:11:22:33:44:55, MAC: ae:12:34:56:78:9a, Supplicant state: COMPLETED\n"
	f.Responses["-s "+serial+" shell getprop ro.build.version.sdk"] = adbtest.Response{Stdout: "36\n"}
	f.Responses["-s "+serial+" shell cmd wifi status 2>/dev/null"] = adbtest.Response{Stdout: status}
	f.Responses["-s "+serial+" shell dumpsys wifi 2>/dev/null | grep -m1 wifi_sta_factory_mac_address"] = adbtest.Response{Stdout: ""}
	f.Responses["-s "+serial+" shell settings get secure bluetooth_address"] = adbtest.Response{Stdout: "A0:B1:C2:D3:E4:F5\n"}
	out, _ := run(t, f, "", "mac")
	for _, want := range []string{
		"Google Pixel 9 Pro (TESTSERIAL01) Android 16, API 36",
		`  Wi-Fi (current)  ae:12:34:56:78:9a   (randomized · SSID "TestNet")`,
		"  Wi-Fi (factory)  n/a",
		"  Bluetooth        a0:b1:c2:d3:e4:f5",
		"↳ Factory MAC: set Wi-Fi privacy to 'Use device MAC' and run again",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
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

func TestInstallSelection(t *testing.T) {
	dir := t.TempDir()
	old := writeAPK(t, dir, "app-release.apk", time.Hour)
	newest := writeAPK(t, dir, "app-debug.apk", 0)
	t.Chdir(dir)

	o := parseInstallOptions([]string{"-l"})
	if got := selectInstallAPKs(o); len(got) != 1 || filepath.Base(got[0]) != filepath.Base(newest) {
		t.Errorf("-l picked %v", got)
	}
	o = parseInstallOptions([]string{"-a"})
	if got := selectInstallAPKs(o); len(got) != 2 {
		t.Errorf("-a picked %v", got)
	}
	// -f with a directory: the filter applies even when it leaves one APK.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAPK(t, sub, "only-release.apk", 0)
	var out bytes.Buffer
	stdout = &out
	defer func() { stdout = os.Stdout }()
	code := func() (code int) {
		defer func() { code = int(recover().(exitCode)) }()
		selectInstallAPKs(parseInstallOptions([]string{"-f", "debug", sub}))
		return 0
	}()
	if code != 1 || !strings.Contains(out.String(), "No APK files found matching all filters: debug") {
		t.Errorf("filter not applied: code %d %s", code, out.String())
	}
	_ = old
}

func TestInstallRecovery(t *testing.T) {
	dir := t.TempDir()
	apkFile := writeAPK(t, dir, "app.apk", 0)
	f := baseFake()
	key := func(opts string) string { return "-s " + serial + " install " + opts + " " + apkFile }
	f.Responses[key("-r")] = adbtest.Response{Stdout: "Performing Streamed Install\nadb: failed to install " + apkFile + ": Failure [INSTALL_FAILED_TEST_ONLY: installPackageLI]\n"}
	f.Responses[key("-r -t")] = adbtest.Response{Stdout: "Performing Streamed Install\nSuccess\n"}
	out, _ := run(t, f, "", "install", apkFile)
	for _, want := range []string{
		"Only one APK file found: app.apk",
		"==> Install command: adb install -r app.apk",
		"Installation failed due to 'INSTALL_FAILED_TEST_ONLY'. Retrying with '-t' option.",
		"==> Install command: adb install -r -t app.apk\nPerforming Streamed Install\nSuccess",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}

	// Signature conflict: confirm with Enter, uninstall, reinstall.
	f = baseFake()
	f.Responses[key("-r")] = adbtest.Response{Stdout: "adb: failed to install " + apkFile + ": Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package com.example.app signatures do not match newer version; ignoring!]\n"}
	f.Responses["-s "+serial+" uninstall com.example.app"] = adbtest.Response{Stdout: "Success\n"}
	out, _ = run(t, f, "\n", "install", apkFile)
	if !strings.Contains(out, "Uninstalling package: com.example.app\n==> Uninstallation successful.") {
		t.Errorf("conflict flow:\n%s", out)
	}
	// Declined.
	out, _ = run(t, f, "n\n", "install", apkFile)
	if !strings.Contains(out, "Installation aborted by user.") {
		t.Errorf("decline flow:\n%s", out)
	}
}
