package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
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

// requirePackage exits unless pkg is well-formed and installed.
func requirePackage(serial, pkg string) {
	if !validPackageName(pkg) {
		say("%sERROR: Invalid package name format:%s %s", Red, NC, pkg)
		say("")
		exit(1)
	}
	if !packageInstalled(serial, pkg) {
		say("%sERROR: Package not installed on the device:%s %s", Red, NC, pkg)
		say("")
		exit(1)
	}
}

// apkPathForPackage returns the device path of the package's base APK.
func apkPathForPackage(serial, pkg string) (string, bool) {
	out := adb.Shell(serial, "pm path "+pkg)
	if strings.TrimSpace(out) == "" {
		say("%sERROR: Package not found:%s %s", Red, NC, pkg)
		say("")
		return "", false
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
	return strings.TrimSpace(path), true
}

// packageArgOrForeground prints which package is used and returns it.
func packageArgOrForeground(serial string, args []string) string {
	say("")
	var pkg string
	if len(args) == 0 || args[0] == "" {
		pkg = foregroundPackage(serial)
		say("%sAuto-detected package:%s %s", Yellow, NC, pkg)
	} else {
		pkg = args[0]
		say("%sUsing specified package:%s %s", Blue, NC, pkg)
	}
	say("")
	requirePackage(serial, pkg)
	return pkg
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
