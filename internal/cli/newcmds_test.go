package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func TestParseLogLineAndTracker(t *testing.T) {
	l, ok := parseLogLine("11-12 10:31:22.123  4321  4330 W OkHttp: timeout: 30s")
	if !ok || l.pid != 4321 || l.level != "W" || l.tag != "OkHttp" || l.msg != "timeout: 30s" || l.time != "10:31:22.123" {
		t.Fatalf("%+v %v", l, ok)
	}
	if _, ok := parseLogLine("--------- beginning of main"); ok {
		t.Error("header line parsed")
	}
	tr := &tracker{pkgs: []string{"com.example.app", "com.example.store"}, pids: map[int]string{}}
	start, _ := parseLogLine("11-12 10:31:20.000  1000  1100 I ActivityManager: Start proc 4321:com.example.app/u0a123 for top-activity {com.example.app/.Main}")
	if ev := tr.observe(start); ev != "com.example.app started (pid 4321)" || tr.app(4321) != "com.example.app" {
		t.Errorf("start: %q %v", ev, tr.pids)
	}
	remote, _ := parseLogLine("11-12 10:31:21.000  1000  1100 I ActivityManager: Start proc 4400:com.example.app:remote/u0a123 for service")
	if tr.observe(remote); tr.app(4400) != "com.example.app" {
		t.Error(":remote process not followed")
	}
	other, _ := parseLogLine("11-12 10:31:21.000  1000  1100 I ActivityManager: Start proc 5000:com.example.application/u0a9 for service")
	if tr.observe(other); tr.app(5000) != "" {
		t.Error("prefix-only package name must not match")
	}
	store, _ := parseLogLine("11-12 10:31:21.000  1000  1100 I ActivityManager: Start proc 4500:com.example.store/u0a124 for service")
	if ev := tr.observe(store); ev != "com.example.store started (pid 4500)" || tr.app(4500) != "com.example.store" {
		t.Errorf("second package: %q %v", ev, tr.pids)
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

// -c alone copies without saving; a path or -o saves too; one device only.
func TestScreenshotCopy(t *testing.T) {
	t.Chdir(t.TempDir())
	var copied []string
	var copiedData []byte
	origCopy, origOpen := copyImageFn, openFn
	copyImageFn = func(p string) error {
		copied = append(copied, p)
		copiedData, _ = os.ReadFile(p)
		return nil
	}
	var opened []string
	openFn = func(p string) error { opened = append(opened, p); return nil }
	t.Cleanup(func() { copyImageFn, openFn = origCopy, origOpen })
	png := string(pngMagic) + "image"
	f := baseFake()
	f.Responses["-s "+serial+" exec-out screencap -p"] = adbtest.Response{Stdout: png}

	r := ak(t, f, "screenshot", "-c")
	files, _ := filepath.Glob("*.png")
	if r.code != 0 || r.out != "" || len(files) != 0 || len(copied) != 1 || string(copiedData) != png ||
		!strings.Contains(r.err, "✓ Copied the screenshot to the clipboard") {
		t.Fatalf("-c: code %d out %q files %v copied %v\n%s", r.code, r.out, files, copied, r.err)
	}
	if _, err := os.Stat(copied[0]); !os.IsNotExist(err) {
		t.Errorf("temporary file left: %s", copied[0])
	}

	copied = nil
	r = ak(t, f, "screenshot", "-c", "shot.png")
	if r.code != 0 || r.out != "shot.png\n" || len(copied) != 1 || copied[0] != "shot.png" || !strings.Contains(r.err, "✓ Copied to the clipboard") {
		t.Errorf("-c with a file: code %d out %q copied %v\n%s", r.code, r.out, copied, r.err)
	}

	copied, opened = nil, nil
	r = ak(t, f, "screenshot", "-co")
	if r.code != 0 || len(copied) != 1 || len(opened) != 1 || copied[0] != opened[0] || strings.TrimSpace(r.out) != opened[0] {
		t.Errorf("-c -o: code %d out %q copied %v opened %v", r.code, r.out, copied, opened)
	}

	copyImageFn = func(string) error { return errors.New("no clipboard tool found") }
	if r := ak(t, f, "screenshot", "-c"); r.code != 1 || !strings.Contains(r.err, "Error: Couldn't copy the screenshot to the clipboard: no clipboard tool found") {
		t.Errorf("copy failure: %d %q", r.code, r.err)
	}

	if r := ak(t, twoDevices(), "screenshot", "-c", "-a"); r.code != 2 || !strings.Contains(r.err, "choose one device") {
		t.Errorf("-c with several devices: %d %q", r.code, r.err)
	}
}

func TestTagColorMatchesAndroidStudio(t *testing.T) {
	// Java String.hashCode() % 16 picks the palette entry, as in Android Studio.
	for tag, want := range map[string]int{
		"":                   246, // hash 0
		"a":                  146, // 97 % 16 = 1
		"hello":              173, // 99162322 % 16 = 2
		"polygenelubricants": 246, // hash is Integer.MIN_VALUE
		"한":                  68,  // UTF-16 0xD55C % 16 = 12
	} {
		if got := tagColor(tag); got != fmt.Sprintf("\033[38;5;%dm", want) {
			t.Errorf("tagColor(%q) = %q, want color %d", tag, got, want)
		}
	}
}

func TestFormatLogLine(t *testing.T) {
	l, _ := parseLogLine("11-12 10:31:22.123  4321  4330 W OkHttp: timeout: 30s")
	if got := formatLogLine(&ui.Stream{}, l, logLayout{cols: 0, tagW: 0}); got != "10:31:22.123 W OkHttp: timeout: 30s" {
		t.Errorf("plain: %q", got)
	}
	got := formatLogLine(&ui.Stream{Color: true}, l, logLayout{cols: 0, tagW: 0})
	for _, part := range []string{levelStyles["W"].badge + " W ", tagColor("OkHttp") + "\033[1mOkHttp:", levelStyles["W"].msg + "timeout: 30s"} {
		if !strings.Contains(got, part) {
			t.Errorf("colored line %q lacks %q", got, part)
		}
	}
}

func TestTagMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, tag string
		want         bool
	}{
		{"OkHttp", "OkHttp", true},
		{"View", "MyView", false}, // exact, not a substring
		{"VRI[*", "VRI[MainActivity]", true},
		{"ViewRootImpl*", "ViewRootImpl@3a1b[Main]", true},
		{"*View", "MyView", true},
		{"My*Tag", "MyFancyTag", true},
		{"My*Tag", "MyFancyTags", false},
		{"*", "", true},
	} {
		if got := tagMatch(c.pattern, c.tag); got != c.want {
			t.Errorf("tagMatch(%q, %q) = %v", c.pattern, c.tag, got)
		}
	}
}

func TestTagFilter(t *testing.T) {
	line := func(level, tag string) logLine { return logLine{level: level, tag: tag} }
	def := tagFilter{runtimeTags: []string{"dk.demo.android"}}
	for _, c := range []struct {
		f    tagFilter
		l    logLine
		want bool
	}{
		{def, line("D", "OneSDK"), true},                  // the app's own tag
		{def, line("D", "VRI[MainActivity]"), false},      // framework noise
		{def, line("I", "ImeTracker"), false},             //
		{def, line("I", "dk.demo.android"), false},        // ART under the process name
		{def, line("W", "WindowOnBackDispatcher"), false}, // framework warnings too
		{def, line("E", "ViewRootImpl"), true},            // framework errors stay
		{def, line("F", "libc"), true},                    // native crash
		{def, line("W", "libc"), false},
		{def, line("E", "Adreno-AppProfiles"), false}, // GPU driver
		{def, line("F", "qdgralloc"), true},
		{def, line("I", "View"), false},                   // Samsung
		{def, line("I", "MyView"), true},                  // exact names only
		{def, line("E", "AndroidRuntime"), true},          // crashes
		{def, line("I", "Choreographer"), true},           // dropped frames
		{tagFilter{every: true}, line("D", "HWUI"), true}, // -e
		{tagFilter{only: []string{"OkHttp"}}, line("D", "OkHttp"), true},
		{tagFilter{only: []string{"OkHttp"}}, line("E", "OneSDK"), false},
		{tagFilter{only: []string{"HWUI"}}, line("D", "HWUI"), true}, // -t names a framework tag
		{tagFilter{ignore: []string{"Glide"}}, line("E", "Glide"), false},
	} {
		if got := c.f.show(c.l); got != c.want {
			t.Errorf("%+v show(%s %s) = %v", c.f, c.l.level, c.l.tag, got)
		}
	}
}

func TestFormatLogLineWraps(t *testing.T) {
	l, _ := parseLogLine("11-12 10:31:22.123  4321  4330 D OneSDK: " + strings.Repeat("abcdefghij", 6))
	// "10:31:22.123 D OneSDK: " is 23 columns; 23 + 30 + 1 = 54
	got := formatLogLine(&ui.Stream{}, l, logLayout{cols: 54, tagW: 6})
	want := "10:31:22.123 D OneSDK: abcdefghijabcdefghijabcdefghij\n" +
		strings.Repeat(" ", 23) + "abcdefghijabcdefghijabcdefghij"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := formatLogLine(&ui.Stream{}, l, logLayout{cols: 30, tagW: 6}); strings.Contains(got, "\n") {
		t.Errorf("too narrow to indent, should not wrap: %q", got)
	}
	if got := wrapText("가나다라", 5); strings.Join(got, "|") != "가나|다라" { // Hangul is 2 columns
		t.Errorf("wrapText: %q", got)
	}
}

func TestFormatLogLineAlignsTags(t *testing.T) {
	short, _ := parseLogLine("11-12 10:31:22.123  4321  4330 D OneSDK: hello")
	long, _ := parseLogLine("11-12 10:31:22.123  4321  4330 V PurchaseClientImpl: hello")
	// in a terminal the tag column is padded to the given width
	if got := formatLogLine(&ui.Stream{}, short, logLayout{cols: 200, tagW: 18}); got != "10:31:22.123 D OneSDK:             hello" {
		t.Errorf("padded: %q", got)
	}
	if got := formatLogLine(&ui.Stream{}, long, logLayout{cols: 200, tagW: 18}); got != "10:31:22.123 V PurchaseClientImpl: hello" {
		t.Errorf("widest: %q", got)
	}
	// to a pipe or file it is not
	if got := formatLogLine(&ui.Stream{}, short, logLayout{cols: 0, tagW: 18}); got != "10:31:22.123 D OneSDK: hello" {
		t.Errorf("pipe: %q", got)
	}
	huge, _ := parseLogLine("11-12 10:31:22.123  4321  4330 D BackgroundInstallControlService: hello")
	if got := formatLogLine(&ui.Stream{}, huge, logLayout{cols: 200, tagW: maxTagWidth}); got != "10:31:22.123 D BackgroundI…trolService: hello" {
		t.Errorf("shortened: %q", got)
	}
	for in, want := range map[string]string{"OneSDK": "OneSDK", "abcdefghij": "abcd…hij", "가나다라마바": "가나…바"} {
		if got := shortenMiddle(in, 8); got != want {
			t.Errorf("shortenMiddle(%q, 8) = %q, want %q", in, got, want)
		}
	}
}

func TestAppLabels(t *testing.T) {
	got := appLabels([]string{"com.example.app", "com.example.store", "org.example.app", "single"})
	want := map[string]string{"com.example.app": "com.example.app", "com.example.store": "example.store",
		"org.example.app": "org.example.app", "single": "single"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFormatLogLineWithApp(t *testing.T) {
	l, _ := parseLogLine("11-12 10:31:22.123  4321  4330 D OneSDK: hello")
	l.app = "app"
	if got := formatLogLine(&ui.Stream{}, l, logLayout{}); got != "10:31:22.123 D app OneSDK: hello" {
		t.Errorf("pipe: %q", got)
	}
	if got := formatLogLine(&ui.Stream{}, l, logLayout{cols: 200, tagW: 8, appW: 5}); got != "10:31:22.123 D app   OneSDK:   hello" {
		t.Errorf("terminal: %q", got)
	}
}

func TestFitAppLabels(t *testing.T) {
	pkgs := []string{"com.onestorecorp.onesdk.demo.android", "com.skt.skaf.OA00018282"}
	if got := fitAppLabels(pkgs, 0); got[pkgs[0]] != pkgs[0] || got[pkgs[1]] != pkgs[1] {
		t.Errorf("pipe: whole names, got %v", got)
	}
	if got := fitAppLabels(pkgs, 200); got[pkgs[0]] != "…demo.android" || got[pkgs[1]] != "…skaf.OA00018282" { // capped at 16
		t.Errorf("wide: %v", got)
	}
	got := fitAppLabels([]string{"com.example.app", "com.example.store"}, 120) // 16 columns: a name that fits stays whole
	if got["com.example.app"] != "com.example.app" || got["com.example.store"] != "…example.store" {
		t.Errorf("narrower: %v", got)
	}
	if got := fitAppLabels(pkgs, 40); got[pkgs[0]] != "…demo.android" { // 12 columns: never below the telling part
		t.Errorf("narrow: %v", got)
	}
}

func TestFormatLogLineAppBar(t *testing.T) {
	l, _ := parseLogLine("11-12 10:31:22.123  4321  4330 D OneSDK: hello")
	l.app, l.appColor = "demo.android", appColors[0]
	got := formatLogLine(&ui.Stream{Color: true}, l, logLayout{})
	if !strings.Contains(got, appColors[0]+"▌\033[0mdemo.android ") {
		t.Errorf("bar in the app color, name plain: %q", got)
	}
}
