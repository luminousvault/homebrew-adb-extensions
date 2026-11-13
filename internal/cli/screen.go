package cli

import (
	"regexp"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
)

var reWakefulness = regexp.MustCompile(`(?m)^\s*mWakefulness=([A-Za-z]+)\s*$`)

// parseWakefulness returns "awake", "asleep", "dozing" or "dreaming" from
// `dumpsys power`, or "" when the output has no such line.
func parseWakefulness(dumpsys string) string {
	if m := reWakefulness.FindStringSubmatch(dumpsys); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// screenOff reports whether the device's screen is off (asleep or dozing).
// An unknown state counts as on, so nothing is warned about.
func screenOff(serial string) bool {
	switch parseWakefulness(adb.Shell(serial, "dumpsys power")) {
	case "asleep", "dozing":
		return true
	}
	return false
}

// wakeHint tells how to turn the screen on.
func wakeHint(serial string) string { return "Turn the screen on: " + wakeCmd(serial) }

func wakeCmd(serial string) string { return "'adb -s " + serial + " shell input keyevent WAKEUP'" }
