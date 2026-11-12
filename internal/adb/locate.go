package adb

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Candidate is a discovered adb binary and where it was found.
type Candidate struct {
	Path   string
	Source string
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// studioSDKDir returns the default SDK directory Android Studio installs to.
func studioSDKDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Android", "sdk")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk")
	default:
		return filepath.Join(home, "Android", "Sdk")
	}
}

// CacheDir is where ak keeps its own platform-tools (~/.ak/platform-tools).
func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ak", "platform-tools")
}

// Candidates lists adb binaries in lookup order:
// AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH, Android Studio default, ak cache.
// The same file found twice is listed once.
func Candidates() []Candidate {
	adb := exeName("adb")
	var list []Candidate
	seen := map[string]bool{}
	add := func(p, src string) {
		if p == "" || !isFile(p) {
			return
		}
		key := p
		if r, err := filepath.EvalSymlinks(p); err == nil {
			key = r
		}
		if seen[key] {
			return
		}
		seen[key] = true
		list = append(list, Candidate{Path: p, Source: src})
	}
	add(os.Getenv("AK_ADB"), "AK_ADB")
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			add(filepath.Join(v, "platform-tools", adb), env)
		}
	}
	if p, err := exec.LookPath(adb); err == nil {
		add(p, "PATH")
	}
	add(filepath.Join(studioSDKDir(), "platform-tools", adb), "Android Studio default")
	add(filepath.Join(CacheDir(), adb), "ak cache")
	return list
}

// ErrNotFound means no adb binary was found.
var ErrNotFound = errors.New("adb not found")

// Locate returns the first adb binary in lookup order.
func Locate() (string, error) {
	if c := Candidates(); len(c) > 0 {
		return c[0].Path, nil
	}
	return "", ErrNotFound
}

// VersionInfo runs `<path> version` and returns the protocol version
// (e.g. "1.0.41") and the build version (e.g. "37.0.1-15733141").
func VersionInfo(path string) (protocol, build string) {
	out, err := exec.Command(path, "version").Output()
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Android Debug Bridge version ") {
			protocol = strings.TrimPrefix(line, "Android Debug Bridge version ")
		} else if strings.HasPrefix(line, "Version ") {
			build = strings.TrimPrefix(line, "Version ")
		}
	}
	return protocol, build
}
