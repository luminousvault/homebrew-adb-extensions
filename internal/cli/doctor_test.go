package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var okCompletion = completionReport{Shell: "zsh", Status: "ok", Path: "/home/u/.zshrc", Files: []completionFile{}}

func TestDoctorReport(t *testing.T) {
	ui.Out = &ui.Stream{}
	t.Setenv("HOME", "/home/u")
	t.Setenv("USERPROFILE", "/home/u") // os.UserHomeDir on Windows
	found := []adbFound{
		{Path: "/home/u/sdk/platform-tools/adb", Source: "ANDROID_HOME", Version: "37.0.1", Protocol: "1.0.41", InUse: true},
		{Path: "/usr/local/bin/adb", Source: "PATH", Version: "35.0.0", Protocol: "1.0.41"},
	}
	devices := []deviceFound{
		{Serial: "emulator-5554", State: "device", Name: "Pixel 7"},
		{Serial: "emulator-5556", State: "unauthorized"},
	}
	cats := doctorChecks(found, devices, "emulator-5558", "", okCompletion)
	adbHint := "      Hint: Running them side by side (e.g. ak and Android Studio) restarts the adb server. " +
		"Point ak at the same adb: " + setEnv("AK_ADB", "<path to adb>")
	got := strings.Join(doctorLines(cats, false), "\n")
	want := strings.Join([]string{
		"[✗] adb (37.0.1, ~/sdk/platform-tools/adb)",
		"    ✗ Other adb with a different version",
		"      /usr/local/bin/adb  35.0.0  (PATH)",
		adbHint,
		"[✗] Connected devices (1 available)",
		"    ✗ emulator-5556 is unauthorized",
		"      Hint: Unlock the device and allow USB debugging in the prompt",
		"    ✗ ANDROID_SERIAL=emulator-5558 is not connected",
		"      Hint: Connect it, or unset ANDROID_SERIAL",
		"[✓] Shell completion (zsh, ~/.zshrc)",
		"[✓] apksigner (optional, not installed)", // optional: not an issue, details only with -v
	}, "\n")
	if got != want {
		t.Errorf("summary:\n%s\nwant:\n%s", got, want)
	}

	// -v adds the • details and blank lines between categories.
	got = strings.Join(doctorLines(cats, true), "\n")
	for _, w := range []string{
		"[✗] adb (37.0.1, ~/sdk/platform-tools/adb)\n    • Protocol 1.0.41, found via ANDROID_HOME\n    ✗ Other",
		"\n\n[✗] Connected devices (1 available)\n    • Pixel 7  emulator-5554\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("verbose: missing %q in:\n%s", w, got)
		}
	}

	// Same version elsewhere is fine; no devices is only a warning.
	found[1].Version = "37.0.1"
	cats = doctorChecks(found, nil, "", "/sdk/build-tools/37.0.0/apksigner", okCompletion)
	got = strings.Join(doctorLines(cats, true), "\n")
	for _, w := range []string{"[✓] adb", "    • Same version at /usr/local/bin/adb (PATH)",
		"[!] Connected devices (none)\n    ! No devices connected", "[✓] apksigner (/sdk/build-tools/37.0.0/apksigner)"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if strings.Contains(got, "✗") {
		t.Errorf("unexpected problem:\n%s", got)
	}
}

// Without adb the report still runs: adb is a problem, devices are skipped.
func TestDoctorNoADB(t *testing.T) {
	ui.Out = &ui.Stream{}
	got := strings.Join(doctorLines(doctorChecks(nil, nil, "", "/x/apksigner", okCompletion), false), "\n")
	if !strings.HasPrefix(got, "[✗] adb (not found)\n    ✗ adb not found\n      Hint: Install it") ||
		strings.Contains(got, "Connected devices") {
		t.Errorf("report:\n%s", got)
	}
}

// A device named by ANDROID_SERIAL that is listed but unauthorized is one
// problem, not two.
func TestDoctorSerialNotDoubleCounted(t *testing.T) {
	ui.Out = &ui.Stream{}
	found := []adbFound{{Path: "/sdk/adb", Source: "PATH", Version: "37", Protocol: "1.0.41"}}
	devices := []deviceFound{{Serial: "emulator-5556", State: "unauthorized"}}
	got := strings.Join(doctorLines(doctorChecks(found, devices, "emulator-5556", "/x/apksigner", okCompletion), false), "\n")
	if strings.Count(got, "✗") != 2 || strings.Contains(got, "not connected") { // [✗] and one ✗ item
		t.Errorf("report:\n%s", got)
	}
}

func TestDoctorHeader(t *testing.T) {
	ui.Out = &ui.Stream{}
	got := strings.Join(doctorHeader("2.1.0", "macOS 26.6.2 (darwin/arm64)", false), "\n")
	want := strings.Join([]string{
		"┌───────────────────────────────┐",
		"│ ak 2.1.0 · ADB Extensions Kit │",
		"│ macOS 26.6.2 (darwin/arm64)   │",
		"└───────────────────────────────┘",
		"Doctor summary (to see all details, run ak doctor -v):",
	}, "\n")
	if got != want {
		t.Errorf("header:\n%s\nwant:\n%s", got, want)
	}
	// Colors do not count toward the box width.
	ui.Out = &ui.Stream{Color: true}
	defer func() { ui.Out = &ui.Stream{} }()
	for _, l := range doctorHeader("2.1.0", "한글 OS", true) {
		if w := ui.TextWidth(ui.Plain(l)); w != 33 {
			t.Errorf("width %d: %q", w, l)
		}
	}
}

// fakeCompletionEnv reads files only under root, so nothing on the real
// system leaks into the test.
func fakeCompletionEnv(t *testing.T, goos, shell string) (completionEnv, string) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SHELL", shell)
	brew := filepath.Join(root, "brew")
	return completionEnv{
		goos: goos, shell: shell, home: home, brew: []string{brew},
		zshFpath: func() []string {
			return []string{filepath.Join(root, "fpath1"), filepath.Join(brew, "share/zsh/site-functions")}
		},
		readFile: func(p string) ([]byte, error) {
			if !strings.HasPrefix(p, root) {
				return nil, fs.ErrNotExist
			}
			return os.ReadFile(p)
		},
		isBrew: func(p string) bool { return strings.HasPrefix(p, brew) },
	}, root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func completionLines(r completionReport, verbose bool) string {
	return strings.Join(doctorLines([]checkCategory{completionCategory(r)}, verbose), "\n")
}

func TestDoctorCompletion(t *testing.T) {
	ui.Out = &ui.Stream{}
	zsh, _ := completionScript("zsh")
	fish, _ := completionScript("fish")

	t.Run("zsh missing", func(t *testing.T) {
		e, _ := fakeCompletionEnv(t, "linux", "/bin/zsh")
		r := classifyCompletion(gatherCompletion(e))
		want := "[!] Shell completion (zsh, not installed)\n    ! No ak completion found for zsh\n" +
			"      Hint: Run this, then open a new terminal (or run 'exec zsh'): echo 'source <(ak completion zsh)' >> ~/.zshrc"
		if got := completionLines(r, false); r.Status != "missing" || got != want {
			t.Errorf("%s:\n%s", r.Status, got)
		}
	})

	t.Run("zsh first fpath file wins", func(t *testing.T) {
		e, root := fakeCompletionEnv(t, "linux", "/usr/bin/zsh")
		writeFile(t, filepath.Join(root, "fpath1", "_ak"), zsh+"\n")
		writeFile(t, filepath.Join(root, "brew/share/zsh/site-functions/_ak"), "#compdef ak\nold")
		r := classifyCompletion(gatherCompletion(e))
		if r.Status != "ok" || len(r.Files) != 2 || !r.Files[0].Current || r.Files[1].Current || !r.Files[1].Homebrew {
			t.Errorf("%+v", r)
		}
		got := completionLines(r, true)
		if !strings.HasPrefix(got, "[✓] Shell completion (zsh, ") || !strings.Contains(got, "_ak (from another ak version)") {
			t.Errorf("\n%s", got)
		}
	})

	t.Run("zsh outdated from Homebrew", func(t *testing.T) {
		e, root := fakeCompletionEnv(t, "darwin", "/bin/zsh")
		writeFile(t, filepath.Join(root, "brew/share/zsh/site-functions/_ak"), "#compdef ak\nold")
		r := classifyCompletion(gatherCompletion(e))
		got := completionLines(r, false)
		if r.Status != "outdated" || !strings.Contains(got, "[!] Shell completion (zsh, outdated)") ||
			!strings.Contains(got, "Hint: Run 'brew reinstall ak', then open a new terminal (or run 'exec zsh')") {
			t.Errorf("%s:\n%s", r.Status, got)
		}
	})

	t.Run("zshrc overrides an old file", func(t *testing.T) {
		e, root := fakeCompletionEnv(t, "darwin", "/bin/zsh")
		writeFile(t, filepath.Join(root, "fpath1", "_ak"), "old")
		writeFile(t, filepath.Join(e.home, ".zshrc"), "autoload -U compinit; compinit\nsource <(ak completion zsh)\n")
		r := classifyCompletion(gatherCompletion(e))
		if got := completionLines(r, false); r.Status != "ok" || got != "[✓] Shell completion (zsh, ~/.zshrc)" {
			t.Errorf("%s:\n%s", r.Status, got)
		}
	})

	t.Run("bash outdated outside Homebrew", func(t *testing.T) {
		e, _ := fakeCompletionEnv(t, "linux", "/bin/bash")
		p := filepath.Join(e.home, ".local/share/bash-completion/completions/ak")
		writeFile(t, p, "old")
		r := classifyCompletion(gatherCompletion(e))
		got := completionLines(r, false)
		if r.Status != "outdated" || !strings.Contains(got, "ak completion bash > ~") {
			t.Errorf("%s:\n%s", r.Status, got)
		}
	})

	t.Run("system file needs sudo", func(t *testing.T) {
		r := completionReport{Shell: "zsh", Status: "outdated",
			Files: []completionFile{{Path: "/usr/local/share/zsh/site-functions/_ak"}}}
		want := "ak completion zsh | sudo tee /usr/local/share/zsh/site-functions/_ak > /dev/null"
		if got := completionLines(r, false); !strings.HasSuffix(got, want) {
			t.Errorf("\n%s", got)
		}
	})

	t.Run("bash on macOS uses bash_profile", func(t *testing.T) {
		e, _ := fakeCompletionEnv(t, "darwin", "/bin/bash")
		got := completionLines(classifyCompletion(gatherCompletion(e)), false)
		if !strings.Contains(got, `echo 'eval "$(ak completion bash)"' >> ~/.bash_profile`) {
			t.Errorf("\n%s", got)
		}
		writeFile(t, filepath.Join(e.home, ".bash_profile"), `eval "$(ak completion bash)"`)
		if r := classifyCompletion(gatherCompletion(e)); r.Status != "ok" {
			t.Errorf("%+v", r)
		}
	})

	t.Run("fish current", func(t *testing.T) {
		e, _ := fakeCompletionEnv(t, "linux", "/usr/bin/fish")
		writeFile(t, filepath.Join(e.home, ".config/fish/completions/ak.fish"), fish)
		if r := classifyCompletion(gatherCompletion(e)); r.Status != "ok" || r.Shell != "fish" {
			t.Errorf("%+v", r)
		}
	})

	t.Run("powershell on Windows", func(t *testing.T) {
		e, _ := fakeCompletionEnv(t, "windows", "")
		r := classifyCompletion(gatherCompletion(e))
		if got := completionLines(r, false); r.Shell != "powershell" || r.Status != "missing" ||
			!strings.Contains(got, "Add-Content $PROFILE 'ak completion powershell | Out-String | Invoke-Expression'") {
			t.Errorf("%s:\n%s", r.Status, got)
		}
		writeFile(t, filepath.Join(e.home, "Documents/PowerShell/Microsoft.PowerShell_profile.ps1"),
			"ak completion powershell | Out-String | Invoke-Expression\n")
		if r := classifyCompletion(gatherCompletion(e)); r.Status != "ok" {
			t.Errorf("%+v", r)
		}
	})

	t.Run("unknown shell", func(t *testing.T) {
		for shell, summary := range map[string]string{"/bin/tcsh": "tcsh not supported", "": "unknown shell"} {
			e, _ := fakeCompletionEnv(t, "linux", shell)
			r := classifyCompletion(gatherCompletion(e))
			got := completionLines(r, false)
			if r.Status != "unknown" || !strings.HasPrefix(got, "[!] Shell completion ("+summary+")") ||
				!strings.Contains(got, "Hint: See 'ak completion --help'") {
				t.Errorf("%q: %s\n%s", shell, r.Status, got)
			}
		}
	})
}

// Completion warnings never change the exit code.
func TestDoctorCompletionIsNeverAProblem(t *testing.T) {
	for _, r := range []completionReport{{Status: "unknown"}, {Shell: "zsh", Status: "missing"},
		{Shell: "zsh", Status: "outdated", Files: []completionFile{{Path: "/x/_ak"}}}} {
		if l := completionCategory(r).level(); l != levelWarn {
			t.Errorf("%+v: level %d", r, l)
		}
	}
}

func TestDoctorVerdict(t *testing.T) {
	s := &ui.Stream{}
	if got := doctorVerdict(s, 3, 4); got != "! 1/4 checks passed. Doctor found issues in 3 categories." {
		t.Error(got)
	}
	if got := doctorVerdict(s, 1, 4); got != "! 3/4 checks passed. Doctor found issues in 1 category." {
		t.Error(got)
	}
}

// The -v environment block: no serials, devices with their Android version.
func TestDoctorEnvironmentLines(t *testing.T) {
	ui.Out = &ui.Stream{}
	got := strings.Join(environmentLines(environment{ak: "2.1.0 (2026-11-20)", osLine: "macOS 26.6.2 (darwin/arm64)",
		shell: "zsh", adb: "37.0.1 (protocol 1.0.41)",
		devices: []string{"Pixel 7 — Android 16 (API 36)", "Galaxy — Android 15 (API 35)"}}), "\n")
	want := strings.Join([]string{"", "Environment (paste into a bug report):",
		"  ak       2.1.0 (2026-11-20)",
		"  OS       macOS 26.6.2 (darwin/arm64)",
		"  Shell    zsh",
		"  adb      37.0.1 (protocol 1.0.41)",
		"  Devices  Pixel 7 — Android 16 (API 36)",
		"           Galaxy — Android 15 (API 35)"}, "\n")
	if got != want {
		t.Errorf("environment:\n%s\nwant:\n%s", got, want)
	}
	if got := environmentLines(environment{ak: "dev", adb: "not found"}); got[len(got)-1] != "  Devices  none" {
		t.Errorf("no devices: %q", got[len(got)-1])
	}
}
