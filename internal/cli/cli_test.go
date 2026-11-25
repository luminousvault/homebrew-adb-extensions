package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
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
    codePath=/data/app/~~abc==/com.example.app-xyz==
    flags=[ DEBUGGABLE HAS_CODE ALLOW_CLEAR_USER_DATA ]
    appId=10234
    pkgFlags=[ DEBUGGABLE HAS_CODE ALLOW_CLEAR_USER_DATA ]
    installerPackageName=com.android.vending
    splits=[base, config.arm64_v8a, config.xxhdpi]
    User 0: ceDataInode=1 installed=true hidden=false suspended=false stopped=false notLaunched=false enabled=0 instant=false virtual=false
    User 0:
      firstInstallTime=2024-03-01 10:00:00
      lastUpdateTime=2024-05-02 11:30:00
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
		"-s " + serial + " shell pidof com.example.app":           {Stdout: "4321\n"},
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
	if r.err != "==> App info on Google Pixel 9 Pro (foreground app)\n" {
		t.Errorf("stderr %q", r.err)
	}
	want := "  package     com.example.app\n  version     1.2.3 (1203)\n  minSdk      24 (Android 7.0)\n" +
		"  targetSdk   36 (Android 16)\n  debuggable  yes\n  system      no\n  enabled     yes\n" +
		"  installer   com.android.vending\n  installed   2024-03-01 10:00:00\n  updated     2024-05-02 11:30:00\n" +
		"  path        /data/app/~~abc==/com.example.app-xyz== (+2 splits)\n  uid         10234\n  pid         4321\n"
	if r.out != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.out, want)
	}
}

func TestInfoNamedPackageTitle(t *testing.T) {
	r := ak(t, baseFake(), "info", "com.example.app")
	if r.code != 0 || r.err != "==> App info on Google Pixel 9 Pro\n" {
		t.Errorf("code %d stderr %q", r.code, r.err)
	}
	if !strings.HasPrefix(r.out, "  package     com.example.app\n") {
		t.Errorf("stdout %q", r.out)
	}
}

func TestSdkWithRelease(t *testing.T) {
	for level, want := range map[string]string{
		"21": "21 (Android 5.0)", "23": "23 (Android 6)", "28": "28 (Android 9)", "29": "29 (Android 10)",
		"30": "30 (Android 11)", "31": "31 (Android 12)", "32": "32 (Android 12L)", "33": "33 (Android 13)",
		"34": "34 (Android 14)", "35": "35 (Android 15)", "36": "36 (Android 16)",
		"37": "37", "": "",
	} {
		if got := sdkWithRelease(level); got != want {
			t.Errorf("sdkWithRelease(%q) = %q, want %q", level, got, want)
		}
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
	splits, _ := json.Marshal(got["splits"])
	if got["system"] != false || got["enabled"] != true || got["firstInstallTime"] != "2024-03-01 10:00:00" ||
		got["lastUpdateTime"] != "2024-05-02 11:30:00" || got["path"] != "/data/app/~~abc==/com.example.app-xyz==" ||
		string(splits) != `["base","config.arm64_v8a","config.xxhdpi"]` || got["uid"] != float64(10234) || got["pid"] != float64(4321) {
		t.Errorf("%v", got)
	}
}

// A field that dumpsys does not print is "-" (pid is left out) in text and null in JSON.
func TestInfoMissingFieldsAndNotRunning(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell dumpsys package com.example.app"] = adbtest.Response{
		Stdout: "  Package [com.example.app] (1):\n    versionCode=1 minSdk=21 targetSdk=34\n    versionName=1.0\n"}
	f.Responses["-s "+serial+" shell pidof com.example.app"] = adbtest.Response{Err: errors.New("exit status 1")}
	r := ak(t, f, "info")
	want := "  package     com.example.app\n  version     1.0 (1)\n  minSdk      21 (Android 5.0)\n" +
		"  targetSdk   34 (Android 14)\n  debuggable  no\n  system      no\n  enabled     -\n" +
		"  installer   -\n  installed   -\n  updated     -\n  path        -\n  uid         -\n"
	if r.code != 0 || r.out != want {
		t.Errorf("code %d stdout:\n%s\nwant:\n%s", r.code, r.out, want)
	}
	r = ak(t, f, "info", "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(r.out), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"enabled", "firstInstallTime", "lastUpdateTime", "path", "splits", "uid", "pid"} {
		if v, ok := got[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, ok)
		}
	}
	if got["system"] != false {
		t.Errorf("system = %v", got["system"])
	}
}

// An updated system app prints the current package block first and the hidden
// system one later; every field comes from the first.
func TestAppInfoTwoBlocksTakesFirst(t *testing.T) {
	dump := `Packages:
  Package [com.example.sys] (1):
    appId=1000
    codePath=/data/app/~~new==/com.example.sys-1==
    versionCode=20 minSdk=34 targetSdk=35
    versionName=2.0
    splits=[base]
    pkgFlags=[ SYSTEM HAS_CODE ALLOW_CLEAR_USER_DATA UPDATED_SYSTEM_APP ]
    User 0: ceDataInode=1 installed=true hidden=false enabled=3 instant=false
    User 0:
      firstInstallTime=2022-01-01 09:00:00
      lastUpdateTime=2025-06-01 12:00:00

Hidden system packages:
  Package [com.example.sys] (2):
    appId=1001
    codePath=/system/priv-app/Sys
    versionCode=10 minSdk=30 targetSdk=33
    versionName=1.0
    splits=[base, config.en]
    pkgFlags=[ HAS_CODE ]
    User 0: ceDataInode=0 installed=true hidden=false enabled=0 instant=false
    User 0:
      firstInstallTime=2009-01-01 00:00:00
      lastUpdateTime=2009-01-01 00:00:00
`
	a := parseAppInfo(dump)
	if a.UID != "1000" || a.Path != "/data/app/~~new==/com.example.sys-1==" || a.VersionCode != "20" ||
		a.VersionName != "2.0" || !a.System || a.Enabled != "3" || a.FirstInstallTime != "2022-01-01 09:00:00" ||
		a.LastUpdateTime != "2025-06-01 12:00:00" || len(a.Splits) != 1 || a.pathText() != a.Path {
		t.Errorf("%+v", a)
	}
}

func TestAppInfoSystemFlag(t *testing.T) {
	for line, want := range map[string]bool{
		"    pkgFlags=[ SYSTEM HAS_CODE ]":  true,
		"    pkgFlags=[ HAS_CODE ]":         false,
		"    pkgFlags=[ SYSTEM_EXT ]":       false,
		"    privateFlags=[ SYSTEM_EXT ]\n": false,
	} {
		if got := parseAppInfo(line).System; got != want {
			t.Errorf("%q: system %v, want %v", line, got, want)
		}
	}
}

func TestAppInfoEnabled(t *testing.T) {
	for digit, want := range map[string][2]any{
		"0": {"yes", true}, "1": {"yes", true}, "2": {"no", false},
		"3": {"no (by user)", false}, "4": {"no (until used)", false},
	} {
		a := parseAppInfo("    User 0:\n    User 0: ceDataInode=1 installed=true enabled=" + digit + " instant=false\n")
		if a.enabledText() != want[0] || a.enabledJSON() != want[1] {
			t.Errorf("enabled=%s: %q %v", digit, a.enabledText(), a.enabledJSON())
		}
	}
	if a := parseAppInfo("    User 0:\n"); a.enabledText() != "-" || a.enabledJSON() != nil {
		t.Errorf("missing: %q %v", a.enabledText(), a.enabledJSON())
	}
}

func TestAppInfoSplits(t *testing.T) {
	for line, want := range map[string]string{
		"    codePath=/p\n    splits=[base]":                      "/p",
		"    codePath=/p\n    splits=[base, config.en]":           "/p (+1 split)",
		"    codePath=/p\n    splits=[base, config.en, config.x]": "/p (+2 splits)",
		"    codePath=/p": "/p",
	} {
		if got := parseAppInfo(line).pathText(); got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
}

func TestAppInfoUID(t *testing.T) {
	for dumpsys, want := range map[string]string{
		"    appId=10234\n":                 "10234",
		"    userId=10123 gids=[3003]\n":    "10123", // Android 9 and older
		"    appId=1000\n    userId=1001\n": "1000",
		"    versionName=1\n":               "",
	} {
		if got := parseAppInfo(dumpsys).UID; got != want {
			t.Errorf("%q: uid %q, want %q", dumpsys, got, want)
		}
	}
}

func TestParsePidof(t *testing.T) {
	for out, want := range map[string]string{"4321\n": "4321", "4321 999\n": "4321", "": "", "\n": "", "error\n": ""} {
		if got := parsePidof(out); got != want {
			t.Errorf("parsePidof(%q) = %q, want %q", out, got, want)
		}
	}
}

func TestOptionsAnywhereAndSerial(t *testing.T) {
	r := ak(t, twoDevices(), "info", "com.example.app", "-s", "SECOND")
	if r.code != 0 || r.err != "==> App info on Google Pixel 9 Pro\n" {
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
		"Hint: Pass -s <serial>, or set ANDROID_SERIAL\n"
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
	if r.code != 1 || !strings.Contains(r.err, "Error: Device 'NOPE' was not found") {
		t.Errorf("unknown serial: %d %q", r.code, r.err)
	}
	f = baseFake()
	f.Responses["devices"] = adbtest.Response{Stdout: "List of devices attached\nLOCKED\tunauthorized\n"}
	r = ak(t, f, "info", "-s", "LOCKED")
	if r.code != 1 || !strings.Contains(r.err, "Device 'LOCKED' is unauthorized\nHint: Accept the USB debugging prompt") {
		t.Errorf("unauthorized: %d %q", r.code, r.err)
	}
}

func TestKillMultipleExitCode(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell am force-stop com.example.app"] = adbtest.Response{}
	r := ak(t, f, "kill", "com.example.app", "bad", "com.not.installed", "com.example.app")
	hint := "Hint: Open the app and run 'ak info' to see its package name\n"
	want := "✓ Stopped 'com.example.app' on Google Pixel 9 Pro\nError: 'bad' is not a valid package name\n" + hint +
		"Error: 'com.not.installed' is not installed on Google Pixel 9 Pro\n" + hint + "Warning: Skipping duplicate package 'com.example.app'\n"
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
	if r.code != 1 || !strings.HasPrefix(r.err, "Error: Couldn't launch 'com.example.app' on Google Pixel 9 Pro\n  Starting: Intent") {
		t.Errorf("code %d:\n%s", r.code, r.err)
	}
	if r := ak(t, f, "launch"); r.code != 2 || !strings.Contains(r.err, "Missing package name\nHint: Run 'ak launch --help' to see the options\n") {
		t.Errorf("missing arg: %d %q", r.code, r.err)
	}
}

func TestLaunchWithoutLauncherActivity(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell cmd package resolve-activity --brief com.example.app"] = adbtest.Response{Stdout: "No activity found\n"}
	r := ak(t, f, "launch", "com.example.app")
	if r.code != 1 || r.err != "Error: 'com.example.app' on Google Pixel 9 Pro has no launcher activity\n" {
		t.Errorf("code %d:\n%s", r.code, r.err)
	}
	for _, c := range f.Calls {
		if strings.Contains(c, "am start") {
			t.Errorf("am start called: %s", c)
		}
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
		{[]string{"nope"}, 2, "", "Error: Unknown command 'nope'\nHint: Run 'ak --help' to see all commands\n"},
		{[]string{"info", "-x"}, 2, "", "Error: Unknown option '-x'\nHint: Run 'ak info --help' to see the options\n"},
		{[]string{"info", "--json=1"}, 2, "", "Error: --json does not take a value"},
		{[]string{"info", "-s"}, 2, "", "Error: -s requires a value (ID)"},
		{[]string{"devices", "extra"}, 2, "", "Error: Unexpected argument 'extra'"},
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
	if r.out != "{\n  \"device\": \"Google Pixel 9 Pro\",\n  \"granted\": [\n    \"android.permission.INTERNET\",\n    \"android.permission.CAMERA\"\n  ],\n"+
		"  \"package\": \"com.example.app\",\n  \"serial\": \"TESTSERIAL01\"\n}\n" {
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
	want := "  Wi-Fi (current)  ae:12:34:56:78:9a (randomized, SSID \"TestNet\")\n" +
		"  Wi-Fi (factory)  -\n  Bluetooth        a0:b1:c2:d3:e4:f5\n"
	if r.out != want || !strings.HasPrefix(r.err, "==> Google Pixel 9 Pro (TESTSERIAL01)\nHint: Set Wi-Fi privacy to 'Use device MAC' on Google Pixel 9 Pro") {
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
		!strings.Contains(err.Error(), "No APK files match all of 'debug'") {
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
		"Warning: 'app.apk' is a test-only APK, retrying with -t on Google Pixel 9 Pro\n==> Installing 'app.apk' on Google Pixel 9 Pro\n✓ Installed 'app.apk' on Google Pixel 9 Pro (") {
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
	if r.code != 1 || !strings.Contains(r.err, "Error: '"+dest+"' does not exist\nHint: Pass -y to create it") {
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
		f.Responses["-s "+serial+" shell "+files.RecentFilesQuery] = adbtest.Response{Stdout: recent}
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
	if r.code != 1 || !strings.Contains(r.err, "Error: No recent files on Google Pixel 9 Pro match all of 'nothing'\nHint: ") {
		t.Errorf("no match: %d %q", r.code, r.err)
	}
}

func TestParseForegroundPackage(t *testing.T) {
	if got := parseForegroundPackage(activitiesOut); got != "com.example.app" {
		t.Errorf("got %q", got)
	}
}

func TestFileMeta(t *testing.T) {
	if got := fileMeta("/storage/emulated/0/Download/a.pdf", 2048); got != "Download, 2 KB" {
		t.Errorf("meta %q", got)
	}
	if got := fileMeta("/storage/emulated/0/a.pdf", 1); got != "sdcard, 1 B" {
		t.Errorf("meta %q", got)
	}
	if got := fileMeta("/storage/emulated/0/a.pdf", -1); got != "sdcard, ?" {
		t.Errorf("unknown size %q", got)
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
	want := "==> Found 2 APKs in '" + dir + "'\n  1. app-debug.apk    1 B · "
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
	if _, _, err := parse(cmdInstall, []string{"-ez"}); err == nil || !strings.Contains(err.Error(), "Unknown option '-z' (in '-ez')") {
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
		want := "Warning: The screen of Google Pixel 9 Pro is off, so the image may be black\n" +
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
	want := "Error: Couldn't record the screen on Google Pixel 9 Pro: the screen is off\n" +
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

// pushFake answers the destination listing of /sdcard/Download (nil: missing,
// so it gets created) and pushes into it.
func pushFake(listing []string, pushes map[string]adbtest.Response) *adbtest.Fake {
	f := baseFake()
	ls := "-s " + serial + " shell " + pushListCmd
	if listing == nil {
		f.Responses[ls] = adbtest.Response{Stdout: "ak:missing\n"}
		f.Responses["-s "+serial+" shell mkdir -p '/sdcard/Download'"] = adbtest.Response{}
	} else {
		f.Responses[ls] = adbtest.Response{Stdout: strings.Join(append(listing, "ak:listed"), "\n") + "\n"}
	}
	for k, v := range pushes {
		f.Responses["-s "+serial+" push "+k] = v
	}
	return f
}

func pushFiles(t *testing.T, names ...string) (dir string) {
	dir = t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPushCreatesDefaultDestination(t *testing.T) {
	dir := pushFiles(t, "notes.txt")
	local := filepath.Join(dir, "notes.txt")
	f := pushFake(nil, map[string]adbtest.Response{local + " /sdcard/Download/notes.txt": {Stdout: local + ": 1 file pushed.\n"}})
	r := ak(t, f, "push", local)
	if r.code != 0 || !strings.Contains(r.err, "==> Created '/sdcard/Download' on Google Pixel 9 Pro\n") ||
		!strings.Contains(r.err, "✓ Pushed '"+local+"' to '/sdcard/Download/notes.txt' (Google Pixel 9 Pro, 6 B, ") ||
		r.out != "/sdcard/Download/notes.txt\n" {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
	for _, c := range f.Calls {
		if strings.Contains(c, "am broadcast") {
			t.Errorf("API 36 must not scan: %s", c)
		}
	}
}

func TestPushRenamesInsteadOfOverwriting(t *testing.T) {
	dir := pushFiles(t, "a.txt")
	local := filepath.Join(dir, "a.txt")
	f := pushFake([]string{"a.txt", "a (1).txt", "other"}, map[string]adbtest.Response{local + " /sdcard/Download/a (2).txt": {}})
	r := ak(t, f, "push", local)
	// The warning follows the step it is about.
	if r.code != 0 || !strings.Contains(r.err, "==> Pushing 'a.txt' to Google Pixel 9 Pro\n"+
		"Warning: '/sdcard/Download/a.txt' already exists on Google Pixel 9 Pro, pushing as 'a (2).txt'\n✓ Pushed") ||
		!strings.Contains(r.err, "to '/sdcard/Download/a (2).txt' (Google Pixel 9 Pro, ") || r.out != "/sdcard/Download/a (2).txt\n" {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

func TestPushQuotesDestination(t *testing.T) {
	dir := pushFiles(t, "a.txt")
	local := filepath.Join(dir, "a.txt")
	f := baseFake()
	f.Responses["-s "+serial+` shell if [ ! -e '/sdcard/it'\''s here' ] && [ ! -L '/sdcard/it'\''s here' ]; then echo ak:missing; elif [ ! -d '/sdcard/it'\''s here' ]; then echo ak:notdir; else cd '/sdcard/it'\''s here' && ls -A && echo ak:listed; fi`] = adbtest.Response{Stdout: "ak:listed\n"}
	f.Responses["-s "+serial+" push "+local+" /sdcard/it's here/a.txt"] = adbtest.Response{}
	r := ak(t, f, "push", local, "-d", "/sdcard/it's here/")
	if r.code != 0 {
		t.Errorf("code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
	if r := ak(t, baseFake(), "push", local, "-d", "Download"); r.code != 2 {
		t.Errorf("relative -d: code %d\n%s", r.code, r.err)
	}
}

func TestPushFailureContinuesAndExitsOne(t *testing.T) {
	dir := pushFiles(t, "a.txt", "b.txt")
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	f := pushFake([]string{}, map[string]adbtest.Response{
		a + " /sdcard/Download/a.txt": {Stderr: "adb: error: failed to copy: Read-only file system\n", Err: errors.New("exit status 1")},
		b + " /sdcard/Download/b.txt": {},
	})
	r := ak(t, f, "push", a, b)
	if r.code != 1 || !strings.Contains(r.err, "Error: Couldn't push '"+a+"' to Google Pixel 9 Pro\n  adb: error: failed to copy") ||
		!strings.Contains(r.err, "✓ Pushed '"+b+"'") || !strings.Contains(r.err, "Error: Couldn't push 1 of 2 files") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}

func TestPushRejectsMissingFilesAndDirectories(t *testing.T) {
	dir := pushFiles(t)
	r := ak(t, baseFake(), "push", filepath.Join(dir, "nope.txt"))
	if r.code != 1 || !strings.Contains(r.err, "' was not found\nHint: ") || !strings.Contains(r.err, "Hint: ") {
		t.Errorf("missing: %d %q", r.code, r.err)
	}
	r = ak(t, baseFake(), "push", dir)
	if r.code != 1 || !strings.Contains(r.err, "is a directory, and ak push copies files only\nHint: Pass the files inside it") {
		t.Errorf("directory: %d %q", r.code, r.err)
	}
}

func TestPushWithoutFilesNeedsTerminal(t *testing.T) {
	t.Chdir(pushFiles(t, "a.txt"))
	f := pushFake([]string{}, nil)
	r := ak(t, f, "push")
	if r.code != 1 || !strings.Contains(r.err, "interactive selection needs a terminal\nHint: Pass the files to push") {
		t.Errorf("%d %q", r.code, r.err)
	}

	// With a terminal: newest first, hidden files left out.
	dir := pushFiles(t, "old.txt", "new.txt", ".hidden")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var got tui.Options
	abs, _ := filepath.Abs("old.txt") // adb gets an absolute path: a name like "-x" is not an option
	f = pushFake([]string{}, map[string]adbtest.Response{abs + " /sdcard/Download/old.txt": {}})
	selectFn = func(o tui.Options) (tui.Result, error) { got = o; return tui.Result{Indices: []int{1}}, nil }
	ui.InTTY = true
	ui.Out, ui.Err = &ui.Stream{W: &bytes.Buffer{}}, &ui.Stream{W: &bytes.Buffer{}, TTY: true}
	adb.Default = f
	resetDeviceCache()
	defer func() {
		adb.Default = nil
		selectFn = tui.Select
		ui.InTTY = false
		ui.Out, ui.Err = &ui.Stream{W: os.Stdout}, &ui.Stream{W: os.Stderr}
	}()
	if code := run([]string{"push"}); code != 0 || len(got.Items) != 2 || !strings.HasPrefix(got.Items[0], "new.txt\t") {
		t.Errorf("code %d items %q calls %q", code, got.Items, f.Calls)
	}
}

func TestPushScansMediaBeforeAndroid11(t *testing.T) {
	dir := pushFiles(t, "a b#1.jpg")
	local := filepath.Join(dir, "a b#1.jpg")
	f := pushFake([]string{}, map[string]adbtest.Response{local + " /sdcard/Download/a b#1.jpg": {}})
	f.Responses["-s "+serial+" shell getprop"] = adbtest.Response{Stdout: strings.Replace(getpropOut, "[36]", "[29]", 1)}
	scan := "-s " + serial + " shell am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d 'file:///sdcard/Download/a%20b%231.jpg'"
	f.Responses[scan] = adbtest.Response{Stdout: "Broadcast completed: result=0\n"}
	r := ak(t, f, "push", local)
	found := false
	for _, c := range f.Calls {
		found = found || c == scan
	}
	if r.code != 0 || !found {
		t.Errorf("code %d calls %q\n%s", r.code, f.Calls, r.err)
	}
}

func TestFilesNeedsTerminal(t *testing.T) {
	r := ak(t, baseFake(), "files")
	if r.code != 1 || !strings.Contains(r.err, "ak files needs a terminal\nHint: Run 'ak pull' or 'ak push' instead") {
		t.Errorf("no terminal: %d\n%s", r.code, r.err)
	}
	if r := ak(t, baseFake(), "files", "sdcard"); r.code != 2 {
		t.Errorf("relative path: %d\n%s", r.code, r.err)
	}
}

func TestRecordTimeLimitOnOlderAndroid(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell dumpsys power"] = adbtest.Response{Stdout: powerAwake}
	f.Responses["-s "+serial+" shell screenrecord --help 2>&1"] = adbtest.Response{Stdout: "--time-limit TIME\n    Set the maximum recording time, in seconds.  Default / maximum is 180.\n"}
	for _, limit := range []string{"0", "181"} {
		r := ak(t, f, "record", "-t", limit)
		want := "Error: Google Pixel 9 Pro records at most 180 seconds\nHint: Pass -t with 1-180 seconds\n"
		if r.code != 1 || r.err != want {
			t.Errorf("-t %s: code %d err %q", limit, r.code, r.err)
		}
	}
	if r := ak(t, f, "record", "-t", "-1"); r.code != 2 {
		t.Errorf("-t -1: code %d err %q", r.code, r.err)
	}
	for _, call := range f.Calls {
		if strings.Contains(call, "screenrecord --time-limit") {
			t.Errorf("screenrecord started: %s", call)
		}
	}
}

func TestRecordClock(t *testing.T) {
	if got := recordClock(65, 0); got != "1:05" {
		t.Errorf("no limit: %q", got)
	}
	if got := recordClock(65, 180); got != "1:05 / 3:00" {
		t.Errorf("limit: %q", got)
	}
}

// Made-up, Luhn-valid IMEIs (not from a real device).
const (
	fakeIMEI  = "490154203237518"
	fakeIMEI2 = "356938035643809"
)

// parcelDump formats s the way `service call` prints a String16 reply.
func parcelDump(s string) string {
	words := []uint32{0, uint32(len(s))}
	for i := 0; i < len(s); i += 2 {
		w := uint32(s[i])
		if i+1 < len(s) {
			w |= uint32(s[i+1]) << 16
		}
		words = append(words, w)
	}
	var b strings.Builder
	b.WriteString("Result: Parcel(\n")
	for i := 0; i < len(words); i += 4 {
		fmt.Fprintf(&b, "0x%08x:", i*4)
		for _, w := range words[i:min(i+4, len(words))] {
			fmt.Fprintf(&b, " %08x", w)
		}
		b.WriteString(" '................'\n")
	}
	return strings.TrimSuffix(b.String(), "\n") + ")\n"
}

func TestParseParcelString(t *testing.T) {
	if got := parseParcelString(parcelDump(fakeIMEI)); got != fakeIMEI {
		t.Errorf("got %q", got)
	}
	exception := "Result: Parcel(\n0x00000000: fffffffe 0000002f 00610050 00630072 '..../...P.a.r.c.'\n" +
		"0x00000010: 006c0065 00640020 00740061 00200061 'e.l. .d.a.t.a. .')\n"
	for _, out := range []string{
		exception,
		"Result: Parcel(\t00000000 ffffffff   '........')\n",                    // null string
		"Result: Parcel(\n0x00000000: 00000000 0000000f 00350033 '....3.5.')\n", // truncated
		"Service iphonesubinfo does not exist\n",
		"",
	} {
		if got := parseParcelString(out); got != "" {
			t.Errorf("%q: got %q", out, got)
		}
	}
}

func TestValidIMEI(t *testing.T) {
	for s, want := range map[string]bool{
		fakeIMEI: true, fakeIMEI2: true, "490154203237519": false, "49015420323751": false,
		"49015420323751a": false, "": false,
	} {
		if validIMEI(s) != want {
			t.Errorf("%q: want %v", s, want)
		}
	}
}

func sysinfoFake(dev string) *adbtest.Fake {
	f := baseFake()
	if dev != serial {
		f = twoDevices()
	}
	for _, d := range []string{serial, "SECOND"} {
		p := "-s " + d + " shell "
		f.Responses[p+"getprop"] = adbtest.Response{Stdout: getpropOut +
			"[ro.product.manufacturer]: [Google]\n[ro.build.version.security_patch]: [2025-01-05]\n" +
			"[ro.build.display.id]: [BP1A.250105.001]\n[ro.serialno]: [HWSERIAL01]\n"}
		f.Responses[p+"settings get global device_name"] = adbtest.Response{Stdout: "Test Phone\n"}
		f.Responses[p+"settings get secure android_id"] = adbtest.Response{Stdout: "0123456789abcdef\n"}
		f.Responses[p+"service call iphonesubinfo 1 s16 com.android.shell"] = adbtest.Response{Stdout: parcelDump(fakeIMEI)}
		f.Responses[p+"service call iphonesubinfo 4 i32 1 s16 com.android.shell"] = adbtest.Response{Stdout: parcelDump(fakeIMEI2)}
		f.Responses[p+"ip -f inet addr show wlan0"] = adbtest.Response{Stdout: "    inet 192.168.0.10/24 brd 192.168.0.255 scope global wlan0\n"}
		f.Responses[p+"getprop ro.build.version.sdk"] = adbtest.Response{Stdout: "36\n"}
		f.Responses[p+"cmd wifi status 2>/dev/null"] = adbtest.Response{Stdout: "WifiInfo: SSID: \"TestNet\", MAC: ae:12:34:56:78:9a, Supplicant state: COMPLETED\n"}
		f.Responses[p+"settings get secure bluetooth_address"] = adbtest.Response{Stdout: "A0:B1:C2:D3:E4:F5\n"}
		f.Responses[p+"wm size; wm density"] = adbtest.Response{Stdout: "Physical size: 1440x3120\nOverride size: 1080x2340\nPhysical density: 560\n"}
	}
	return f
}

// sysinfoText is the text output of sysinfoFake for the device with adb serial dev.
func sysinfoText(dev string) string {
	return "Device\n" + sysRow("serial", dev) + sysRow("model", "Pixel 9 Pro") + sysRow("manufacturer", "Google") +
		sysRow("name", "Test Phone") + "\n" +
		"Software\n" + sysRow("android", "16 (API 36)") + sysRow("securityPatch", "2025-01-05") +
		sysRow("build", "BP1A.250105.001") + "\n" +
		"Identifiers\n" + sysRow("hardwareSerial", "HWSERIAL01") + sysRow("imei", fakeIMEI) + sysRow("imei2", fakeIMEI2) +
		sysRow("androidId", "0123456789abcdef") + "\n" +
		"Network\n" + sysRow("ip", "192.168.0.10") + sysRow("wifiMac", `ae:12:34:56:78:9a (randomized, SSID "TestNet")`) +
		sysRow("wifiFactory", "-") + sysRow("bluetooth", "a0:b1:c2:d3:e4:f5") + "\n" +
		"Hardware\n" + sysRow("abi", "arm64-v8a") + sysRow("screen", "1440x3120, 560 dpi (in use: 1080x2340)")
}

// sysRow is one sysinfo row: every section shares the key column of the
// longest key, hardwareSerial (14).
func sysRow(k, v string) string { return fmt.Sprintf("  %-14s  %s\n", k, v) }

func TestSysinfoText(t *testing.T) {
	r := ak(t, sysinfoFake(serial), "sysinfo")
	if want := sysinfoText(serial); r.code != 0 || r.out != want || r.err != "==> Google Pixel 9 Pro\n" {
		t.Errorf("code %d\nstdout:\n%s\nwant:\n%s\nstderr %q", r.code, r.out, want, r.err)
	}
	if r := ak(t, sysinfoFake(serial), "sysinfo", "extra"); r.code != 2 {
		t.Errorf("extra argument: code %d", r.code)
	}
}

func TestSysinfoMissingValues(t *testing.T) {
	f := baseFake() // getprop only: no telephony, no Wi-Fi, no settings
	r := ak(t, f, "sysinfo")
	for _, want := range []string{
		sysRow("serial", "TESTSERIAL01"), sysRow("name", "-"), sysRow("android", "16 (API 36)"), sysRow("securityPatch", "-"),
		sysRow("hardwareSerial", "-"), sysRow("imei", "-") + sysRow("androidId", "-"),
		sysRow("wifiMac", "- (Wi-Fi is off or not connected)"), sysRow("screen", "-"),
	} {
		if !strings.Contains(r.out, want) {
			t.Errorf("missing %q in:\n%s", want, r.out)
		}
	}
	if strings.Contains(r.out, "imei2") {
		t.Error("imei2 row shown without a second IMEI")
	}
	if r.code != 0 || r.err != "==> Google Pixel 9 Pro\nHint: "+hintIMEI("TESTSERIAL01")+"\n" {
		t.Errorf("code %d stderr %q", r.code, r.err)
	}
}

func TestSysinfoIMEIFallbacks(t *testing.T) {
	f := sysinfoFake(serial)
	p := "-s " + serial + " shell service call iphonesubinfo "
	// Code 1 is not getDeviceId here; slot 0 of code 4 is. Slot 1 repeats slot 0 (single SIM).
	f.Responses[p+"1 s16 com.android.shell"] = adbtest.Response{Stdout: parcelDump("123")}
	f.Responses[p+"4 i32 0 s16 com.android.shell"] = adbtest.Response{Stdout: parcelDump(fakeIMEI)}
	f.Responses[p+"4 i32 1 s16 com.android.shell"] = adbtest.Response{Stdout: parcelDump(fakeIMEI)}
	r := ak(t, f, "sysinfo")
	if !strings.Contains(r.out, sysRow("imei", fakeIMEI)+"  androidId") {
		t.Errorf("stdout:\n%s", r.out)
	}
}

func TestSysinfoJSON(t *testing.T) {
	r := ak(t, sysinfoFake(serial), "sysinfo", "--json")
	var arr []map[string]any
	if err := json.Unmarshal([]byte(r.out), &arr); err != nil || len(arr) != 1 {
		t.Fatalf("want a one-element array: %v: %s", err, r.out)
	}
	got := arr[0]
	want := map[string]any{
		"serial": serial, "device": "Google Pixel 9 Pro", "model": "Pixel 9 Pro", "manufacturer": "Google",
		"name": "Test Phone", "android": "16", "api": 36.0, "securityPatch": "2025-01-05", "build": "BP1A.250105.001",
		"hardwareSerial": "HWSERIAL01", "imei": fakeIMEI, "imei2": fakeIMEI2, "androidId": "0123456789abcdef",
		"ip": "192.168.0.10", "wifiMac": "ae:12:34:56:78:9a", "wifiSsid": "TestNet", "wifiRandomized": true,
		"wifiFactory": nil, "bluetooth": "a0:b1:c2:d3:e4:f5", "abi": "arm64-v8a",
		"screen": map[string]any{"width": 1440.0, "height": 3120.0, "density": 560.0},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	if r := ak(t, baseFake(), "sysinfo", "--json"); !strings.Contains(r.out, `"imei2": null`) || !strings.Contains(r.out, `"screen": null`) {
		t.Errorf("missing values: %s", r.out)
	}
}

func TestSysinfoAllDevices(t *testing.T) {
	r := ak(t, sysinfoFake("SECOND"), "sysinfo", "--all")
	if r.code != 0 || r.out != sysinfoText(serial)+"\n"+sysinfoText("SECOND") ||
		strings.Count(r.err, "==> Google Pixel 9 Pro\n") != 2 { // the serial row tells them apart
		t.Errorf("code %d stdout:\n%s\nstderr:\n%s", r.code, r.out, r.err)
	}
	r = ak(t, sysinfoFake("SECOND"), "sysinfo", "--all", "--json")
	var got []map[string]any
	if err := json.Unmarshal([]byte(r.out), &got); err != nil || len(got) != 2 || got[1]["serial"] != "SECOND" {
		t.Errorf("%v: %s", err, r.out)
	}
}

func TestMacHidden(t *testing.T) {
	if r := ak(t, baseFake(), "--help"); strings.Contains(r.out, " mac ") || !strings.Contains(r.out, "sysinfo") {
		t.Errorf("global help:\n%s", r.out)
	}
	if strings.Contains(zshCompletion(), "'mac:") {
		t.Error("mac in the zsh completion")
	}
}

// An empty result is not a failure: a "No …" line on stderr, exit 0.
func TestEmptyResultsExitZero(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell "+files.RecentFilesQuery] = adbtest.Response{Stdout: "No result found.\n"}
	r := ak(t, f, "pull", t.TempDir())
	if r.code != 0 || r.out != "" || !strings.HasSuffix(r.err, "  No recent files found on Google Pixel 9 Pro\n") {
		t.Errorf("pull: code %d out %q err %q", r.code, r.out, r.err)
	}

	f = baseFake()
	f.Responses["-s "+serial+" shell dumpsys activity activities"] = adbtest.Response{
		Stdout: "  topResumedActivity=ActivityRecord{1a2 u0 com.example.app/.Gone t99}\n"}
	r = ak(t, f, "activities")
	if r.code != 0 || r.out != "" || r.err != "  No activities found on Google Pixel 9 Pro\n" {
		t.Errorf("activities: code %d out %q err %q", r.code, r.out, r.err)
	}
	r = ak(t, f, "activities", "--json")
	if r.code != 0 || r.out != "[]\n" {
		t.Errorf("activities --json: code %d out %q", r.code, r.out)
	}
}
