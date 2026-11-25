package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var (
	reForeground  = regexp.MustCompile(`^.* u0 ([^/ ]*)/`)
	rePackageName = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)+$`)
)

// foregroundPackage returns the package of the top activity
// (first "Hist" line of `dumpsys activity activities`). When the screen is
// off, that is the app shown before it turned off, so it warns.
func foregroundPackage(serial string) string {
	pkg := parseForegroundPackage(adb.Shell(serial, "dumpsys activity activities"))
	if pkg != "" && pkg != "null" && screenOff(serial) {
		ui.Warn("The screen of %s is off; %s is the app that was on screen before it turned off", deviceName(serial), q(pkg))
		ui.Hint("Pass a package name, or turn the screen on: %s", wakeCmd(serial))
	}
	return pkg
}

func parseForegroundPackage(dumpsys string) string {
	for _, line := range strings.Split(dumpsys, "\n") {
		if strings.Contains(strings.ToLower(line), "hist") {
			if m := reForeground.FindStringSubmatch(line); m != nil {
				return m[1]
			}
			return ""
		}
	}
	return ""
}

func validPackageName(p string) bool { return rePackageName.MatchString(p) }

func packageInstalled(serial, pkg string) bool {
	for _, line := range strings.Split(adb.Shell(serial, "pm list packages --user 0"), "\n") {
		if line == "package:"+pkg {
			return true
		}
	}
	return false
}

// hintPackage tells how to find a package name. With several devices
// connected, the command names the device so it runs on the same one.
func hintPackage(serial string) string {
	return "Open the app and run 'ak info" + serialArg(serial) + "' to see its package name"
}

// serialArg is " -s <serial>" when several devices are connected, so a
// command in a hint reaches the same device; "" otherwise.
func serialArg(serial string) string {
	if len(adb.ReadySerials()) > 1 {
		return " -s " + serial
	}
	return ""
}

// requirePackage fails unless pkg is well-formed and installed on serial.
func requirePackage(serial, pkg string) error {
	if pkg == "" {
		return failErr("Open the app on the device or pass a package name", "No foreground app found on %s", deviceName(serial))
	}
	if !validPackageName(pkg) {
		return failErr(hintPackage(serial), "%s is not a valid package name", q(pkg))
	}
	if !packageInstalled(serial, pkg) {
		return failErr(hintPackage(serial), "%s is not installed on %s", q(pkg), deviceName(serial))
	}
	return nil
}

// apkPathForPackage returns the device path of the package's base APK and
// how many split APKs it has besides it.
func apkPathForPackage(serial, pkg string) (string, int, error) {
	out := adb.Shell(serial, "pm path "+pkg)
	if strings.TrimSpace(out) == "" {
		return "", 0, failErr(hintPackage(serial), "%s was not found on %s", q(pkg), deviceName(serial))
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	chosen := lines[0]
	for _, l := range lines {
		if strings.Contains(l, "base.apk") {
			chosen = l
			break
		}
	}
	_, path, _ := strings.Cut(chosen, ":")
	return strings.TrimSpace(path), len(lines) - 1, nil
}

// target is the package a command acts on.
type target struct {
	pkg        string
	foreground bool
}

// describe names an app on a device the same way in every message:
// "'pkg' on Device", or "'pkg' (foreground app) on Device" when it was detected.
func (t target) describe(serial string) string {
	return appOn(t.pkg, t.foreground, deviceName(serial))
}

// onDevice says where, for titles whose result already has a package row:
// "on Device", or "on Device (foreground app)" when it was detected.
func (t target) onDevice(serial string) string {
	if t.foreground {
		return "on " + deviceName(serial) + " (foreground app)"
	}
	return "on " + deviceName(serial)
}

func appOn(pkg string, foreground bool, device string) string {
	if foreground {
		return q(pkg) + " (foreground app) on " + device
	}
	return q(pkg) + " on " + device
}

// resolvePackage uses the given package or the foreground app, and checks it.
func resolvePackage(serial string, args []string) (target, error) {
	t := target{}
	if len(args) > 0 && args[0] != "" {
		t.pkg = args[0]
	} else {
		t.pkg, t.foreground = foregroundPackage(serial), true
	}
	return t, requirePackage(serial, t.pkg)
}

const (
	sortByName = "name"
	sortNewest = "time-newest"
)

// listAPKs returns regular *.apk files directly in dir, sorted by name or newest first.
func listAPKs(dir, order string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type apk struct {
		path string
		mod  int64
	}
	var list []apk
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".apk") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, apk{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if order == sortNewest && list[i].mod != list[j].mod {
			return list[i].mod > list[j].mod
		}
		return list[i].path < list[j].path
	})
	paths := make([]string, len(list))
	for i, a := range list {
		paths[i] = a.path
	}
	return paths
}

// tildeHome abbreviates the home directory prefix as "~".
func tildeHome(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
