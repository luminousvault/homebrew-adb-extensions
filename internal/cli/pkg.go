package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
)

var (
	reForeground  = regexp.MustCompile(`^.* u0 ([^/ ]*)/`)
	rePackageName = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)+$`)
)

// foregroundPackage returns the package of the top activity
// (first "Hist" line of `dumpsys activity activities`).
func foregroundPackage(serial string) string {
	return parseForegroundPackage(adb.Shell(serial, "dumpsys activity activities"))
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

const hintPackage = "Run 'ak info' on the foreground app to check the name"

// requirePackage fails unless pkg is well-formed and installed on serial.
func requirePackage(serial, pkg string) error {
	if pkg == "" {
		return failErr("Open the app on the device or pass a package name", "No foreground app found on %s", deviceName(serial))
	}
	if !validPackageName(pkg) {
		return failErr("", "Invalid package name: %s", pkg)
	}
	if !packageInstalled(serial, pkg) {
		return failErr(hintPackage, "Package not installed on %s: %s", deviceName(serial), pkg)
	}
	return nil
}

// apkPathForPackage returns the device path of the package's base APK.
func apkPathForPackage(serial, pkg string) (string, error) {
	out := adb.Shell(serial, "pm path "+pkg)
	if strings.TrimSpace(out) == "" {
		return "", failErr("", "Package not found on %s: %s", deviceName(serial), pkg)
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
	return strings.TrimSpace(path), nil
}

// target is the package a command acts on.
type target struct {
	pkg        string
	foreground bool
}

// describe returns "pkg (foreground app on Device)" or "pkg on Device".
func (t target) describe(serial string) string {
	if t.foreground {
		return t.pkg + " (foreground app on " + deviceName(serial) + ")"
	}
	return t.pkg + " on " + deviceName(serial)
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
