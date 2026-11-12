package adb

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func hasBuildTools(sdk string) bool {
	entries, err := os.ReadDir(filepath.Join(sdk, "build-tools"))
	return err == nil && len(entries) > 0
}

// SDKRoot finds an Android SDK that contains build-tools:
// ANDROID_HOME, then the parent of the located adb's platform-tools, then common paths.
func SDKRoot() string {
	if v := os.Getenv("ANDROID_HOME"); v != "" && hasBuildTools(v) {
		return v
	}
	if p, err := Locate(); err == nil {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		dir := filepath.Dir(p)
		if filepath.Base(dir) == "platform-tools" && hasBuildTools(filepath.Dir(dir)) {
			return filepath.Dir(dir)
		}
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{studioSDKDir(), filepath.Join(home, "Android", "Sdk"), "/usr/local/share/android-sdk"} {
		if hasBuildTools(p) {
			return p
		}
	}
	return ""
}

// compareVersions compares dotted versions numerically ("34.0.0" < "35.0.1").
func compareVersions(a, b string) int {
	pa, pb := strings.FieldsFunc(a, isSep), strings.FieldsFunc(b, isSep)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		switch {
		case ex == nil && ey == nil && nx != ny:
			if nx < ny {
				return -1
			}
			return 1
		case (ex != nil || ey != nil) && x != y:
			return strings.Compare(x, y)
		}
	}
	return 0
}

func isSep(r rune) bool { return r == '.' || r == '-' }

// FindBuildTool finds an SDK build-tools binary (e.g. apksigner): PATH first,
// then the newest build-tools version in the SDK.
func FindBuildTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	sdk := SDKRoot()
	if sdk == "" {
		return ""
	}
	dir := filepath.Join(sdk, "build-tools")
	entries, _ := os.ReadDir(dir)
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) < 0 })
	for _, n := range []string{name, name + ".bat", name + ".exe"} {
		p := filepath.Join(dir, versions[len(versions)-1], n)
		if isFile(p) {
			return p
		}
	}
	return ""
}
