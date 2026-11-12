package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdDoctor = &Command{
	Name: "doctor", Summary: "Diagnose adb setup",
	Long: "Check the adb ak uses, other adb binaries, connected devices, shell completion and optional tools.\n" +
		"Lookup order: AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH, Android Studio default SDK, ~/.ak/platform-tools.\n" +
		"Running adb binaries of different versions side by side restarts the adb server and drops device connections.\n" +
		"[✓] fine, [!] worth fixing, [✗] a problem. Exits 1 when a problem is found.",
	Flags:    []Flag{{"-v", "--verbose", "", "Show all details"}},
	Examples: []string{"ak doctor", "ak doctor -v", "ak doctor --json"},
	NoADB:    true, JSON: true,
}

func init() { cmdDoctor.Run = runDoctor } // runDoctor reads Commands (via completionScript): set after init

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

// level is how bad a check item is. A category takes its worst item's level.
type level int

const (
	levelOK      level = iota // • detail, shown with -v
	levelWarn                 // ! worth fixing, does not change the exit code
	levelProblem              // ✗ exits 1
)

// checkItem is one line under a category.
type checkItem struct {
	level   level
	text    string
	details []string // dim lines under the text
	hint    string   // how to fix it
}

// checkCategory is one [✓]/[!]/[✗] line of the report.
type checkCategory struct {
	name    string
	summary string // in parentheses after the name
	items   []checkItem
}

func (c checkCategory) level() level {
	l := levelOK
	for _, it := range c.items {
		l = max(l, it.level)
	}
	return l
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
	comp := classifyCompletion(gatherCompletion(systemCompletionEnv()))
	cats := doctorChecks(found, devices, os.Getenv("ANDROID_SERIAL"), apksigner, comp)
	problems, issues := 0, 0
	for _, cat := range cats {
		if cat.level() > levelOK {
			issues++
		}
		for _, it := range cat.items {
			if it.level == levelProblem {
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
			"devices": devices, "apksigner": apksigner, "completion": comp, "problems": problems}); err != nil {
			return err
		}
		if problems > 0 {
			return errReported // same exit code as the text report
		}
		return nil
	}
	verbose := c.Bool("--verbose")
	for _, l := range doctorHeader(Version, osDescription(), verbose) {
		ui.Out.Println(l)
	}
	for _, l := range doctorLines(cats, verbose) {
		ui.Out.Println(l)
	}
	ui.Err.Println("")
	if issues > 0 {
		ui.Err.Println(doctorVerdict(ui.Err, issues))
	} else {
		ui.Success("No issues found!")
	}
	if problems > 0 {
		return errReported
	}
	return nil
}

// doctorVerdict is the closing line when something needs attention.
func doctorVerdict(s *ui.Stream, issues int) string {
	return s.Yellow("!") + fmt.Sprintf(" Doctor found issues in %d %s.", issues, plural(issues, "category", "categories"))
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

// doctorChecks turns what was found into report categories (pure, for tests).
func doctorChecks(found []adbFound, devices []deviceFound, envSerial, apksigner string, comp completionReport) []checkCategory {
	if len(found) == 0 {
		return []checkCategory{
			{name: "adb", summary: "not found", items: []checkItem{{level: levelProblem,
				text: "adb not found", hint: adbInstallHint()}}},
			completionCategory(comp),
			apksignerCategory(apksigner),
		}
	}
	return []checkCategory{adbCategory(found), devicesCategory(devices, envSerial),
		completionCategory(comp), apksignerCategory(apksigner)}
}

func adbCategory(found []adbFound) checkCategory {
	use := found[0]
	cat := checkCategory{name: "adb", summary: use.Version + ", " + tildeHome(use.Path), items: []checkItem{
		{text: "Protocol " + orDefault(use.Protocol, "unknown") + ", found via " + use.Source},
	}}
	others := found[1:]
	switch {
	case len(others) == 0:
		cat.items = append(cat.items, checkItem{text: "No other adb found"})
	case !versionConflict(found):
		for _, f := range others {
			cat.items = append(cat.items, checkItem{text: "Same version at " + tildeHome(f.Path) + " (" + f.Source + ")"})
		}
	default:
		it := checkItem{level: levelProblem, text: "Other adb with a different version",
			hint: "Running them side by side (e.g. ak and Android Studio) restarts the adb server. " +
				"Point ak at the same adb: " + setEnv("AK_ADB", "<path to adb>")}
		for _, f := range others {
			it.details = append(it.details, tildeHome(f.Path)+"  "+f.Version+"  ("+f.Source+")")
		}
		cat.items = append(cat.items, it)
	}
	return cat
}

func devicesCategory(devices []deviceFound, envSerial string) checkCategory {
	var ready []deviceFound
	for _, d := range devices {
		if d.State == "device" {
			ready = append(ready, d)
		}
	}
	cat := checkCategory{name: "Connected devices", summary: fmt.Sprintf("%d available", len(ready))}
	nameW := 0
	for _, d := range ready {
		nameW = max(nameW, ui.TextWidth(d.Name))
	}
	for _, d := range ready {
		cat.items = append(cat.items, checkItem{text: ui.Pad(d.Name, nameW) + "  " + d.Serial})
	}
	for _, d := range devices {
		switch d.State {
		case "device":
		case "unauthorized":
			cat.items = append(cat.items, checkItem{level: levelProblem, text: d.Serial + " is unauthorized",
				hint: "Unlock the device and allow USB debugging in the prompt"})
		case "offline":
			cat.items = append(cat.items, checkItem{level: levelProblem, text: d.Serial + " is offline",
				hint: "Reconnect the cable, or run 'adb kill-server'"})
		default:
			cat.items = append(cat.items, checkItem{level: levelProblem, text: d.Serial + " is " + d.State})
		}
	}
	if len(devices) == 0 {
		cat.summary = "none"
		cat.items = append(cat.items, checkItem{level: levelWarn, text: "No devices connected",
			hint: "Connect a device with USB debugging on"})
	}
	if envSerial != "" {
		// A listed device (even unauthorized or offline) already has its own line.
		listed := false
		for _, d := range devices {
			listed = listed || d.Serial == envSerial
		}
		if !listed {
			cat.items = append(cat.items, checkItem{level: levelProblem,
				text: "ANDROID_SERIAL=" + envSerial + " is not connected",
				hint: "Connect it, or unset ANDROID_SERIAL"})
		}
	}
	return cat
}

func apksignerCategory(apksigner string) checkCategory {
	if apksigner != "" {
		return checkCategory{name: "apksigner", summary: tildeHome(apksigner), items: []checkItem{
			{text: "Fallback for 'ak signature' when it cannot parse an APK"}}}
	}
	return checkCategory{name: "apksigner", summary: "optional, not found", items: []checkItem{{level: levelWarn,
		text:    "apksigner not found",
		details: []string{"Only needed when 'ak signature' cannot parse an APK; comes with SDK build-tools"}}}}
}

func setEnv(name, value string) string {
	if runtime.GOOS == "windows" {
		return "set " + name + "=" + value
	}
	return "export " + name + "=" + value
}

// osDescription is "macOS 26.6.2 (darwin/arm64)"; the name part is best effort.
func osDescription() string {
	arch := runtime.GOOS + "/" + runtime.GOARCH
	name := ""
	switch runtime.GOOS {
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output(); err == nil {
			name = "macOS " + strings.TrimSpace(string(out))
		}
	case "linux":
		if b, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
					name = strings.Trim(v, `"'`)
				}
			}
		}
	case "windows":
		name = "Windows"
	}
	if name == "" {
		return arch
	}
	return name + " (" + arch + ")"
}

// doctorHeader draws the box at the top and the summary heading:
//
//	┌──────────────────────────────┐
//	│ ak 2.1.0 · ADB Extensions Kit │
//	│ macOS 26.6.2 (darwin/arm64)  │
//	└──────────────────────────────┘
//	Doctor summary (to see all details, run ak doctor -v):
func doctorHeader(version, osLine string, verbose bool) []string {
	o := ui.Out
	rows := []string{o.Bold("ak") + " " + o.Green(version) + " · ADB Extensions Kit", osLine}
	w := 0
	for _, r := range rows {
		w = max(w, ui.TextWidth(ui.Plain(r)))
	}
	lines := []string{"┌" + strings.Repeat("─", w+2) + "┐"}
	for _, r := range rows {
		lines = append(lines, "│ "+r+strings.Repeat(" ", w-ui.TextWidth(ui.Plain(r)))+" │")
	}
	lines = append(lines, "└"+strings.Repeat("─", w+2)+"┘")
	if !verbose {
		lines = append(lines, "Doctor summary (to see all details, run ak doctor -v):")
	}
	return lines
}

// doctorLines renders the categories, flutter doctor style:
//
//	[✗] adb (37.0.1, ~/Library/Android/sdk/platform-tools/adb)
//	    • Protocol 1.0.41, found via ANDROID_HOME          (only with -v)
//	    ✗ Other adb with a different version
//	      /usr/local/bin/adb  35.0.0  (PATH)
//	      Hint: Point ak at the same adb: export AK_ADB=<path to adb>
//
// Without verbose, • lines are left out; ! and ✗ lines are always shown.
// With verbose, a blank line separates categories.
func doctorLines(cats []checkCategory, verbose bool) []string {
	o := ui.Out
	icon := map[level]string{levelOK: o.Green("✓"), levelWarn: o.Yellow("!"), levelProblem: o.Red("✗")}
	var lines []string
	for i, cat := range cats {
		if verbose && i > 0 {
			lines = append(lines, "")
		}
		head := "[" + icon[cat.level()] + "] " + o.Bold(cat.name)
		if cat.summary != "" {
			head += " (" + cat.summary + ")"
		}
		lines = append(lines, head)
		for _, it := range cat.items {
			if it.level == levelOK && !verbose {
				continue
			}
			mark := icon[it.level]
			if it.level == levelOK {
				mark = "•"
			}
			lines = append(lines, "    "+mark+" "+it.text)
			for _, d := range it.details {
				lines = append(lines, "      "+o.Dim(d))
			}
			if it.hint != "" {
				lines = append(lines, "      "+o.Dim("Hint: ")+it.hint)
			}
		}
	}
	return lines
}
