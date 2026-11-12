package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdDoctor = &Command{
	Name: "doctor", Summary: "Diagnose adb setup",
	Long: "Check the adb ak uses, other adb binaries, connected devices and optional tools.\n" +
		"Lookup order: AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH, Android Studio default SDK, ~/.ak/platform-tools.\n" +
		"Running adb binaries of different versions side by side restarts the adb server and drops device connections.\n" +
		"Exits 1 when a problem is found.",
	Examples: []string{"ak doctor", "ak doctor --json"},
	NoADB:    true, JSON: true, Run: runDoctor,
}

type adbFound struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
	InUse    bool   `json:"inUse"`
}

type deviceFound struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Name   string `json:"name,omitempty"`
}

// checkResult is one line of the report: ✓ fine, ✗ a problem, – nothing to do.
type checkResult struct {
	status  checkStatus
	title   string
	details []string // dim lines under the title
	hint    string   // how to fix a problem
}

type checkStatus int

const (
	checkOK checkStatus = iota
	checkProblem
	checkNone
)

type checkSection struct {
	name   string
	checks []checkResult
}

func runDoctor(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	var found []adbFound
	for i, cand := range adb.Candidates() {
		proto, build := adb.VersionInfo(cand.Path)
		found = append(found, adbFound{cand.Path, cand.Source, orDefault(build, "unknown"), proto, i == 0})
	}
	var devices []deviceFound
	if len(found) > 0 {
		if adb.Default == nil {
			adb.Default = adb.Exec{Path: found[0].Path}
		}
		for _, d := range adb.ListDevices() {
			f := deviceFound{Serial: d.Serial, State: d.State}
			if d.State == "device" {
				f.Name = deviceName(d.Serial)
			}
			devices = append(devices, f)
		}
	}
	apksigner := adb.FindBuildTool("apksigner")
	sections := doctorChecks(found, devices, os.Getenv("ANDROID_SERIAL"), apksigner)
	problems := 0
	for _, s := range sections {
		for _, ch := range s.checks {
			if ch.status == checkProblem {
				problems++
			}
		}
	}

	if c.JSON() {
		if found == nil {
			found = []adbFound{}
		}
		if devices == nil {
			devices = []deviceFound{}
		}
		if err := ui.JSON(map[string]any{"adb": found, "versionConflict": versionConflict(found),
			"devices": devices, "apksigner": apksigner, "problems": problems}); err != nil {
			return err
		}
		if problems > 0 || len(found) == 0 {
			return errReported // same exit code as the text report
		}
		return nil
	}
	if len(found) == 0 {
		return failErr(adbInstallHint(), "adb not found")
	}
	for _, l := range doctorLines(sections) {
		ui.Out.Println(l)
	}
	ui.Err.Println("")
	if problems > 0 {
		ui.Errorf("%d %s found", problems, plural(problems, "problem", "problems"))
		return errReported
	}
	ui.Success("No problems found")
	return nil
}

func versionConflict(found []adbFound) bool {
	for _, f := range found {
		if f.Version != found[0].Version {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// doctorChecks turns what was found into report sections (pure, for tests).
func doctorChecks(found []adbFound, devices []deviceFound, envSerial, apksigner string) []checkSection {
	if len(found) == 0 {
		return nil
	}
	use := found[0]
	adbSec := checkSection{name: "adb", checks: []checkResult{{
		status:  checkOK,
		title:   use.Version + " " + "(protocol " + orDefault(use.Protocol, "unknown") + ")",
		details: []string{tildeHome(use.Path) + "  (" + use.Source + ")"},
	}}}
	others := found[1:]
	switch {
	case len(others) == 0:
		adbSec.checks = append(adbSec.checks, checkResult{status: checkOK, title: "No other adb found"})
	case !versionConflict(found):
		ch := checkResult{status: checkOK, title: fmt.Sprintf("%d other adb, same version", len(others))}
		for _, f := range others {
			ch.details = append(ch.details, tildeHome(f.Path)+"  ("+f.Source+")")
		}
		adbSec.checks = append(adbSec.checks, ch)
	default:
		ch := checkResult{status: checkProblem, title: "Other adb with a different version",
			hint: "Running them side by side (e.g. ak and Android Studio) restarts the adb server. " +
				"Point ak at the same adb: " + setEnv("AK_ADB", "<path to adb>")}
		for _, f := range others {
			ch.details = append(ch.details, tildeHome(f.Path)+"  "+f.Version+"  ("+f.Source+")")
		}
		adbSec.checks = append(adbSec.checks, ch)
	}

	devSec := checkSection{name: "Devices"}
	var ready []deviceFound
	for _, d := range devices {
		if d.State == "device" {
			ready = append(ready, d)
		}
	}
	nameW := 0
	for _, d := range ready {
		nameW = max(nameW, ui.TextWidth(d.Name))
	}
	if len(ready) > 0 {
		ch := checkResult{status: checkOK, title: fmt.Sprintf("%d connected", len(ready))}
		for _, d := range ready {
			ch.details = append(ch.details, ui.Pad(d.Name, nameW)+"  "+d.Serial)
		}
		devSec.checks = append(devSec.checks, ch)
	}
	for _, d := range devices {
		switch d.State {
		case "device":
		case "unauthorized":
			devSec.checks = append(devSec.checks, checkResult{status: checkProblem, title: d.Serial + " is unauthorized",
				hint: "Unlock the device and allow USB debugging in the prompt"})
		case "offline":
			devSec.checks = append(devSec.checks, checkResult{status: checkProblem, title: d.Serial + " is offline",
				hint: "Reconnect the cable, or run 'adb kill-server'"})
		default:
			devSec.checks = append(devSec.checks, checkResult{status: checkProblem, title: d.Serial + " is " + d.State})
		}
	}
	if len(devices) == 0 {
		devSec.checks = append(devSec.checks, checkResult{status: checkNone, title: "No devices connected",
			hint: "Connect a device with USB debugging on"})
	}
	if envSerial != "" {
		// A listed device (even unauthorized or offline) already has its own line.
		listed := false
		for _, d := range devices {
			listed = listed || d.Serial == envSerial
		}
		if !listed {
			devSec.checks = append(devSec.checks, checkResult{status: checkProblem,
				title: "ANDROID_SERIAL=" + envSerial + " is not connected",
				hint:  "Connect it, or unset ANDROID_SERIAL"})
		}
	}

	toolSec := checkSection{name: "Optional tools"}
	if apksigner != "" {
		toolSec.checks = append(toolSec.checks, checkResult{status: checkOK, title: "apksigner",
			details: []string{tildeHome(apksigner) + "  (fallback for 'ak signature')"}})
	} else {
		toolSec.checks = append(toolSec.checks, checkResult{status: checkNone, title: "apksigner not found",
			details: []string{"Only needed when 'ak signature' cannot parse an APK; comes with SDK build-tools"}})
	}
	return []checkSection{adbSec, devSec, toolSec}
}

func setEnv(name, value string) string {
	if runtime.GOOS == "windows" {
		return "set " + name + "=" + value
	}
	return "export " + name + "=" + value
}

// doctorLines renders the report:
//
//	adb
//	  ✓ 37.0.1 (protocol 1.0.41)
//	      ~/Library/Android/sdk/platform-tools/adb  (ANDROID_HOME)
func doctorLines(sections []checkSection) []string {
	o := ui.Out
	var lines []string
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, o.Bold(s.name))
		for _, ch := range s.checks {
			icon := o.Green("✓")
			switch ch.status {
			case checkProblem:
				icon = o.Red("✗")
			case checkNone:
				icon = o.Dim("–")
			}
			lines = append(lines, "  "+icon+" "+ch.title)
			for _, d := range ch.details {
				lines = append(lines, "      "+o.Dim(d))
			}
			if ch.hint != "" {
				lines = append(lines, "      "+o.Dim("Hint: ")+ch.hint)
			}
		}
	}
	return lines
}
