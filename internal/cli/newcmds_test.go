package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
)

func TestParseLogLineAndTracker(t *testing.T) {
	l, ok := parseLogLine("11-12 10:31:22.123  4321  4330 W OkHttp: timeout: 30s")
	if !ok || l.pid != 4321 || l.level != "W" || l.tag != "OkHttp" || l.msg != "timeout: 30s" || l.time != "10:31:22.123" {
		t.Fatalf("%+v %v", l, ok)
	}
	if _, ok := parseLogLine("--------- beginning of main"); ok {
		t.Error("header line parsed")
	}
	tr := &tracker{pkg: "com.example.app", pids: map[int]bool{}}
	start, _ := parseLogLine("11-12 10:31:20.000  1000  1100 I ActivityManager: Start proc 4321:com.example.app/u0a123 for top-activity {com.example.app/.Main}")
	if ev := tr.observe(start); ev != "com.example.app started (pid 4321)" || !tr.has(4321) {
		t.Errorf("start: %q %v", ev, tr.pids)
	}
	remote, _ := parseLogLine("11-12 10:31:21.000  1000  1100 I ActivityManager: Start proc 4400:com.example.app:remote/u0a123 for service")
	if tr.observe(remote); !tr.has(4400) {
		t.Error(":remote process not followed")
	}
	other, _ := parseLogLine("11-12 10:31:21.000  1000  1100 I ActivityManager: Start proc 5000:com.example.application/u0a9 for service")
	if tr.observe(other); tr.has(5000) {
		t.Error("prefix-only package name must not match")
	}
	killed, _ := parseLogLine("11-12 12:06:30.109  1501  2627 I ActivityManager: Killing 31290:com.example.app/u0a344 (adj 0): stop com.example.app due to from pid 31325")
	if ev := tr.observe(killed); ev != "com.example.app killed (pid 31290): stop com.example.app due to from pid 31325" {
		t.Errorf("killed: %q", ev)
	}
	died, _ := parseLogLine("11-12 10:32:00.000  1000  1100 I ActivityManager: Process com.example.app (pid 4321) has died: fg TOP")
	if ev := tr.observe(died); ev != "com.example.app died (pid 4321)" {
		t.Errorf("died: %q", ev)
	}
}

func TestParseLaunch(t *testing.T) {
	out := "Starting: Intent { cmp=com.example.app/.Main }\nStatus: ok\nLaunchState: COLD\nActivity: com.example.app/.Main\nTotalTime: 412\nWaitTime: 420\nComplete\n"
	r, ok := parseLaunch(out)
	if !ok || r.TotalTime != 412 || r.WaitTime != 420 || r.LaunchState != "cold" {
		t.Errorf("%+v %v", r, ok)
	}
}

func TestOutputPath(t *testing.T) {
	resetDeviceCache()
	propsCache[serial] = map[string]string{"ro.product.brand": "google", "ro.product.model": "Pixel 9 Pro"}
	now := time.Date(2026, 11, 12, 10, 30, 0, 0, time.Local)
	if p, _ := outputPath("", serial, "screenshot", ".png", now, false); p != "screenshot-Pixel-9-Pro-20261112-103000.png" {
		t.Errorf("default %q", p)
	}
	dir := t.TempDir()
	if p, _ := outputPath(dir, serial, "record", ".mp4", now, true); p != filepath.Join(dir, "record-Pixel-9-Pro-20261112-103000.mp4") {
		t.Errorf("dir %q", p)
	}
	if p, _ := outputPath("bug", serial, "record", ".mp4", now, false); p != "bug.mp4" {
		t.Errorf("ext %q", p)
	}
	if _, err := outputPath("x.png", serial, "screenshot", ".png", now, true); err == nil {
		t.Error("a file name for several devices must fail")
	}
}

func TestScreenshotSavesPNGAndPrintsPath(t *testing.T) {
	t.Chdir(t.TempDir())
	f := baseFake()
	png := string(pngMagic) + "rest-of-image"
	f.Responses["-s "+serial+" exec-out screencap -p"] = adbtest.Response{Stdout: png}
	r := ak(t, f, "screenshot", "shot.png")
	if r.code != 0 || r.out != "shot.png\n" || !strings.HasPrefix(r.err, "==> Capturing Google Pixel 9 Pro\n✓ Saved 'shot.png'") {
		t.Fatalf("code %d out %q err %q", r.code, r.out, r.err)
	}
	if b, _ := os.ReadFile("shot.png"); string(b) != png {
		t.Error("file content")
	}
	f.Responses["-s "+serial+" exec-out screencap -p"] = adbtest.Response{Stdout: "screencap: permission denied"}
	if r := ak(t, f, "screenshot", "bad.png"); r.code != 1 || !strings.Contains(r.err, "Error: Couldn't capture the screen of Google Pixel 9 Pro\n  screencap: permission denied") {
		t.Errorf("bad capture: %d %q", r.code, r.err)
	}
}

func TestRestartRepeat(t *testing.T) {
	f := baseFake()
	f.Responses["-s "+serial+" shell cmd package resolve-activity --brief com.example.app"] = adbtest.Response{Stdout: "priority=0\ncom.example.app/.Main\n"}
	f.Responses["-s "+serial+" shell am force-stop com.example.app"] = adbtest.Response{}
	f.Responses["-s "+serial+" shell am start -W --user 0 -n com.example.app/.Main"] = adbtest.Response{Stdout: "Status: ok\nLaunchState: COLD\nTotalTime: 400\nWaitTime: 410\n"}
	r := ak(t, f, "restart", "com.example.app", "--json")
	if r.code != 0 || !strings.Contains(r.out, `"avgMs": 400`) || !strings.Contains(r.out, `"launchState": "cold"`) {
		t.Errorf("code %d\n%s\n%s", r.code, r.out, r.err)
	}
}

func TestWifiArgs(t *testing.T) {
	if r := ak(t, baseFake(), "wifi", "on"); r.code != 2 || !strings.Contains(r.err, "Unknown argument: on") {
		t.Errorf("%d %q", r.code, r.err)
	}
	if r := ak(t, baseFake(), "wifi", "off"); r.code != 0 || !strings.Contains(r.err, "No devices connected over Wi-Fi") {
		t.Errorf("%d %q", r.code, r.err)
	}
	if m := reInet.FindStringSubmatch("3: wlan0: <BROADCAST,MULTICAST,UP>\n    inet 192.168.0.21/24 brd 192.168.0.255 scope global wlan0"); m == nil || m[1] != "192.168.0.21" {
		t.Errorf("inet %v", m)
	}
}

func TestScreenshotOpen(t *testing.T) {
	t.Chdir(t.TempDir())
	var opened []string
	orig := openFn
	openFn = func(p string) error { opened = append(opened, p); return nil }
	t.Cleanup(func() { openFn = orig })
	f := baseFake()
	f.Responses["-s "+serial+" exec-out screencap -p"] = adbtest.Response{Stdout: string(pngMagic)}
	if r := ak(t, f, "screenshot", "a.png"); r.code != 0 || len(opened) != 0 {
		t.Fatalf("opened without -o: %v", opened)
	}
	if r := ak(t, f, "screenshot", "b.png", "-o"); r.code != 0 || len(opened) != 1 || opened[0] != "b.png" {
		t.Errorf("-o: code %d opened %v", r.code, opened)
	}
	openFn = func(string) error { return errors.New("no viewer") }
	if r := ak(t, f, "screenshot", "c.png", "--open"); r.code != 0 || !strings.Contains(r.err, "Warning: Couldn't open 'c.png': no viewer") {
		t.Errorf("open failure: %d %q", r.code, r.err)
	}
}

func TestVerifierHidden(t *testing.T) {
	if r := ak(t, baseFake(), "--help"); strings.Contains(r.out, "verifier") {
		t.Error("verifier listed in the global help")
	}
	f := baseFake()
	f.Responses["-s "+serial+" shell settings put global verifier_verify_adb_installs 0"] = adbtest.Response{}
	f.Responses["-s "+serial+" shell settings get global verifier_verify_adb_installs"] = adbtest.Response{Stdout: "0\n"}
	r := ak(t, f, "verifier", "off")
	if r.code != 0 || !strings.Contains(r.out, "Google Pixel 9 Pro  off") {
		t.Errorf("off: %d %q %q", r.code, r.out, r.err)
	}
	if r := ak(t, f, "verifier", "maybe"); r.code != 2 {
		t.Errorf("bad argument: %d", r.code)
	}
}
