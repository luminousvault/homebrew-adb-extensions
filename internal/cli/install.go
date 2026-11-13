package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/apk"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdInstall = &Command{
	Name: "install", Summary: "Install APKs on one or more devices", Args: "[apk|directory...]",
	Long: "Install APK files on one or more devices.\n" +
		"APK files given by name are installed; with a directory or no argument, pick APKs (newest first).\n" +
		"Common failures are handled automatically: test-only APKs are retried with -t, downgrades\n" +
		"with -d, and when that is not allowed or the signature differs, ak offers to uninstall first.\n" +
		"Several devices install in parallel with a status board; a device that needs that answer\n" +
		"is asked right away while the others keep going, and declining stops that device.\n" +
		"If '<file>.apk.idsig' exists, --no-incremental is added.",
	ArgHelp: [][2]string{{"apk", "APK files to install"}, {"directory", "Pick APKs from these directories"}},
	Flags: []Flag{
		{"-l", "--latest", "", "Install the newest APK (in the current or given directory)"},
		{"-e", "--every-apk", "", "Install every APK (in the current or given directory)"},
		{"-f", "--filter", "WORDS", "Pick among APKs whose names contain all the words"},
		{"-r", "", "", "Replace the existing app, keeping its data (always on)"},
		{"-t", "--test-only", "", "Allow test-only APKs (otherwise retried with -t when needed)"},
		{"-d", "--downgrade", "", "Allow version downgrade (otherwise retried with -d when needed)"},
		{"-y", "--yes", "", "Uninstall and reinstall without asking when a device refuses the update (removes the app's data)"},
	},
	Examples: []string{
		"ak install", "ak install app-debug.apk", "ak install -l build/outputs/apk/debug",
		"ak install -f \"myapp release\"", "ak install app.apk --all", "ak install -e build/outputs/apk --all", "ak install app.apk -s emulator-5554 -y",
	},
	Device: manyDevices, Run: runInstall,
}

type installOptions struct {
	latest, all, filtered bool
	filter                string
	adbOpts               []string // starts with -r
	args                  []string
}

func installOptionsFrom(c *Ctx) installOptions {
	o := installOptions{latest: c.Bool("--latest"), all: c.Bool("--every-apk"), filter: c.String("--filter"), args: c.Args}
	o.filtered = c.Bool("--filter")
	o.adbOpts = []string{"-r"}
	if c.Bool("--test-only") {
		o.adbOpts = append(o.adbOpts, "-t")
	}
	if c.Bool("--downgrade") {
		o.adbOpts = append(o.adbOpts, "-d")
	}
	return o
}

func validateInstallOptions(cmd *Command, o installOptions) error {
	n := 0
	for _, b := range []bool{o.latest, o.all, o.filtered} {
		if b {
			n++
		}
	}
	if n > 1 {
		return usageErr(cmd, "Use only one of -l, -e and -f")
	}
	for _, a := range o.args {
		st, err := os.Stat(a)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		if filepath.Ext(a) != ".apk" {
			return usageErr(cmd, "Not an APK file: %s", a)
		}
		if n > 0 {
			return usageErr(cmd, "-l, -e and -f choose APKs from a directory; they cannot be used with APK files (%s)", a)
		}
	}
	return nil
}

// matchesFilter requires every space-separated word to match (case-insensitive, like grep -i).
func matchesFilter(s, filter string) bool {
	for _, w := range strings.Fields(filter) {
		re, err := regexp.Compile("(?i)" + w)
		if err != nil {
			if !strings.Contains(strings.ToLower(s), strings.ToLower(w)) {
				return false
			}
			continue
		}
		if !re.MatchString(s) {
			return false
		}
	}
	return true
}

func filterAPKs(list []string, filter string) (out []string, keep []int) {
	for i, p := range list {
		if matchesFilter(filepath.Base(p), filter) {
			out = append(out, p)
			keep = append(keep, i)
		}
	}
	return out, keep
}

// displayDir returns an absolute, ~-abbreviated directory for display.
func displayDir(dir string) string {
	if a, err := filepath.Abs(dir); err == nil {
		dir = a
	}
	return tildeHome(dir)
}

const hintChooseAPK = "Pass APK files, or use -l (newest) or -e (every APK)"

// pickAPKs asks which APKs to install (or uses the only one).
// scanned says the list came from a directory, so a single result is announced.
func pickAPKs(list, sources, location []string, prompt string, scanned bool) ([]string, error) {
	if len(list) == 1 {
		if scanned {
			ui.Step("Using %s %s", q(filepath.Base(list[0])), ui.Err.Dim("(the only APK found)"))
		}
		return list, nil
	}
	distinct := map[string]bool{}
	for _, s := range sources {
		distinct[s] = true
	}
	items := make([]string, len(list))
	for i, p := range list {
		items[i] = filepath.Base(p)
		if len(distinct) > 1 {
			items[i] += "\t(" + path.Base(filepath.ToSlash(sources[i])) + ")"
		}
	}
	r, err := pick(tui.Options{Multi: true, Prompt: prompt, Location: location, Items: items}, hintChooseAPK)
	if err != nil {
		return nil, err
	}
	var picked []string
	for _, i := range r.Indices {
		picked = append(picked, list[i])
	}
	return picked, nil
}

// apksInCurrentDir is the default: pick from ./*.apk (newest first).
func apksInCurrentDir(filter string) ([]string, error) {
	list := listAPKs(".", sortNewest)
	if len(list) == 0 {
		return nil, failErr("Pass APK files or a directory that contains them", "No APK files in %s", displayDir("."))
	}
	if filter != "" {
		if list, _ = filterAPKs(list, filter); len(list) == 0 {
			return nil, failErr("", "No APK files match all of: %s", filter)
		}
	}
	return pickAPKs(list, nil, []string{displayDir(".")}, "Select APK files to install", true)
}

// apksFromArgs resolves APK file and directory arguments.
func apksFromArgs(args []string, filter string) ([]string, error) {
	var list, sources, dirs, missing []string
	for _, a := range args {
		st, err := os.Stat(a)
		switch {
		case err != nil:
			missing = append(missing, a)
		case st.Mode().IsRegular():
			list = append(list, a)
			sources = append(sources, displayDir(filepath.Dir(a)))
		case st.IsDir():
			dirs = append(dirs, a)
			for _, p := range listAPKs(a, sortNewest) {
				list = append(list, p)
				sources = append(sources, displayDir(a))
			}
		}
	}
	for _, m := range missing {
		ui.Warn("Not found: %s", m)
	}
	if len(list) == 0 {
		if len(dirs) > 0 {
			return nil, failErr("", "No APK files in %s", strings.Join(dirs, ", "))
		}
		return nil, failErr("", "No APK files to install")
	}
	if filter != "" {
		var keep []int
		list, keep = filterAPKs(list, filter)
		var s []string
		for _, i := range keep {
			s = append(s, sources[i])
		}
		sources = s
		if len(list) == 0 {
			return nil, failErr("", "No APK files match all of: %s", filter)
		}
	}
	// APK files given by name are all installed; directories (or -f) mean "let me pick".
	if len(dirs) == 0 && filter == "" {
		return list, nil
	}
	var location []string
	for _, d := range dirs {
		location = append(location, displayDir(d))
	}
	return pickAPKs(list, sources, location, "Select APK files to install", true)
}

func selectInstallAPKs(o installOptions) ([]string, error) {
	if o.latest || o.all {
		dir := "."
		for _, a := range o.args {
			if st, err := os.Stat(a); err == nil && st.IsDir() {
				dir = a
				break
			}
		}
		list := listAPKs(dir, sortNewest)
		if len(list) == 0 {
			return nil, failErr("", "No APK files in %s", displayDir(dir))
		}
		if o.latest {
			return list[:1], nil
		}
		return list, nil
	}
	if len(o.args) == 0 {
		return apksInCurrentDir(o.filter)
	}
	return apksFromArgs(o.args, o.filter)
}

func runInstall(c *Ctx) error {
	installStop.Store(false)
	o := installOptionsFrom(c)
	if err := validateInstallOptions(c.Cmd, o); err != nil {
		return err
	}
	release := holdPickers() // APK picker, then device picker, on one screen
	defer release()
	files, err := selectInstallAPKs(o)
	if err != nil {
		return err
	}
	devices, err := c.Devices()
	if err != nil {
		return err
	}
	release()
	// After the device picker, which clears the screen, so the list stays visible.
	if o.all {
		ui.Step("Found %d APK(s) in %s", len(files), q(displayDir(filepath.Dir(files[0]))))
		for _, l := range apkListLines(files) {
			ui.Err.Println(l)
		}
		ui.Err.Println("")
	}
	if len(devices) > 1 {
		return installParallel(devices, files, o.adbOpts)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sig:
			installStop.Store(true)
		case <-done:
		}
	}()
	r := &directReporter{}
	ok := true
	for _, f := range files {
		if installStop.Load() || r.stopped {
			break
		}
		if !installOne(r, devices[0], o.adbOpts, f) {
			ok = false
		}
	}
	switch {
	case installStop.Load():
		return ui.ErrInterrupted
	case !ok:
		return errReported
	}
	return nil
}

// ── Installing one APK ───────────────────────────────

// reporter receives the messages of one device's installs. The single-device
// reporter prints immediately; the parallel one buffers and drives the status board.
type reporter interface {
	progress(m say) // current activity (spinner text)
	done()          // activity finished
	success(m say)
	warn(m say)
	fail(m say)
	detail(text string)
	confirm(question say) (bool, error)
}

// say is one message in two forms. full stands on a line of its own (one
// device, or "[device]" lines without a terminal), so it names the APK and
// the device. short goes inside a device's block under the APK name, where
// both are already on screen:
//
//	Samsung SM-S928N (SERIAL)
//	  app.apk
//	    Warning: Older than the installed version, retrying with -d
type say struct{ full, short string }

func sayf(short, format string, args ...any) say { return say{fmt.Sprintf(format, args...), short} }

// A step that takes longer than this is usually waiting for the user on the
// device: Play Protect asking to scan the app, or an "install via USB" prompt.
var slowStep = 10 * time.Second

const slowHint = "(check the device screen)"

type directReporter struct {
	sp      *ui.Spinner
	slow    *time.Timer
	stopped bool // the user declined: later APKs are not installed (as with several devices)
}

func (r *directReporter) progress(m say) {
	r.done()
	text := m.full
	sp := ui.Spin("%s", text)
	r.sp = sp
	r.slow = time.AfterFunc(slowStep, func() { sp.SetText("%s %s", text, ui.Err.Dim(slowHint)) })
}
func (r *directReporter) done() {
	if r.slow != nil {
		r.slow.Stop()
		r.slow = nil
	}
	if r.sp != nil {
		r.sp.Stop()
		r.sp = nil
	}
}
func (r *directReporter) success(m say) { r.done(); ui.Success("%s", m.full) }
func (r *directReporter) warn(m say)    { r.done(); ui.Warn("%s", m.full) }
func (r *directReporter) fail(m say) {
	r.done()
	ui.Errorf("%s", m.full)
	// A declined confirmation stops here: later APKs may depend on this one.
	// Without a terminal nobody declined ("pass -y"), so the others still install.
	if strings.HasPrefix(m.short, "Skipped") && !strings.Contains(m.short, "(pass -y)") {
		r.stopped = true
	}
}
func (r *directReporter) detail(t string)             { ui.Detail("%s", t) }
func (r *directReporter) confirm(m say) (bool, error) { r.done(); return ui.Confirm(m.full, true) }

var reFailure = regexp.MustCompile(`\[?(INSTALL_[A-Z_]+|DELETE_[A-Z_]+)`)

// failureCode extracts e.g. INSTALL_FAILED_INSUFFICIENT_STORAGE from adb output.
func failureCode(out string) string {
	if m := reFailure.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return "unknown error"
}

// installStop is set on Ctrl+C: no further adb install, retry or uninstall starts.
var installStop atomic.Bool

// resultStopped stands in for adb output when Ctrl+C stopped an install.
const resultStopped = "\x00stopped"

func adbInstall(serial string, opts []string, apkFile string) string {
	if installStop.Load() {
		return resultStopped
	}
	out, err := adb.Combined(append(append([]string{"-s", serial, "install"}, opts...), apkFile)...)
	if installStop.Load() || killedBySignal(err) {
		return resultStopped // the terminal sent SIGINT to adb as well
	}
	return strings.TrimRight(out, "\n")
}

func killedBySignal(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return true
		}
	}
	return false
}

func hasOpt(opts []string, opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

// installOne installs one APK on one device and handles common failures.
// It reports through r and returns whether the APK ended up installed.
func installOne(r reporter, serial string, opts []string, apkFile string) bool {
	opts = append([]string(nil), opts...)
	base := filepath.Base(apkFile)
	if st, err := os.Stat(apkFile + ".idsig"); err == nil && !st.IsDir() {
		opts = append(opts, "--no-incremental")
	}
	start := time.Now()
	dev := deviceName(serial)
	attempt := func(o []string) string {
		r.progress(sayf("Installing", "Installing %s on %s", q(base), dev))
		return adbInstall(serial, o, apkFile)
	}
	// finish reports the last attempt; reinstalled names the package that
	// was uninstalled first, if any.
	finish := func(result, reinstalled string) bool {
		if result == resultStopped {
			r.fail(sayf("Stopped: interrupted", "Stopped installing %s on %s: interrupted", q(base), dev))
			return false
		}
		if strings.Contains(result, "Success") {
			took := ui.Err.Dim("(" + elapsed(start) + ")")
			short := "Installed " + took
			if reinstalled != "" {
				short = "Installed " + took + ", after uninstalling " + q(reinstalled) + " (its data was removed)"
			}
			r.success(sayf(short, "Installed %s on %s %s", q(base), dev, took))
			return true
		}
		code := failureCode(result)
		r.fail(sayf("Couldn't install: "+code, "Couldn't install %s on %s: %s", q(base), dev, code))
		r.detail(result)
		return false
	}

	result := attempt(opts)
	switch {
	case strings.Contains(result, "INSTALL_FAILED_TEST_ONLY"):
		r.warn(sayf("Test-only APK, retrying with -t", "%s is a test-only APK, retrying with -t", q(base)))
		opts = append(opts, "-t")
		result = attempt(opts)
		if strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE") {
			return reinstallAfterDowngrade(r, serial, opts, apkFile, finish)
		}
		return finish(result, "")
	case strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE"):
		if !hasOpt(opts, "-d") {
			r.warn(sayf("Older than the installed version, retrying with -d",
				"%s is older than the installed version, retrying with -d", q(base)))
			result = attempt(append(opts, "-d"))
			if !strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE") {
				return finish(result, "")
			}
		}
		return reinstallAfterDowngrade(r, serial, opts, apkFile, finish)
	case strings.Contains(result, "INSTALL_FAILED_UPDATE_INCOMPATIBLE"):
		pkg := conflictPackage(result)
		if pkg == "" {
			pkg, _ = apk.PackageName(apkFile)
		}
		r.warn(sayf("Signature differs from the installed app",
			"The signature of %s differs from %s installed on %s", q(base), q(pkg), dev))
		return uninstallThenInstall(r, serial, pkg, opts, apkFile, finish)
	}
	return finish(result, "")
}

func reinstallAfterDowngrade(r reporter, serial string, opts []string, apkFile string, finish func(string, string) bool) bool {
	pkg, err := apk.PackageName(apkFile)
	if err != nil {
		r.fail(sayf(fmt.Sprintf("Couldn't read the package name: %v", err),
			"Couldn't read the package name of %s: %v", q(filepath.Base(apkFile)), err))
		return false
	}
	r.warn(sayf("Downgrade not allowed without uninstalling",
		"%s does not allow downgrading %s without uninstalling it", deviceName(serial), q(pkg)))
	return uninstallThenInstall(r, serial, pkg, opts, apkFile, finish)
}

var reConflictPackage = regexp.MustCompile(`^.*package ([^ ]*)`)

func conflictPackage(result string) string {
	for _, l := range strings.Split(result, "\n") {
		if m := reConflictPackage.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

func uninstallThenInstall(r reporter, serial, pkg string, opts []string, apkFile string, finish func(string, string) bool) bool {
	base, dev := q(filepath.Base(apkFile)), deviceName(serial)
	ok, err := r.confirm(say{
		"Uninstall " + q(pkg) + " from " + dev + " (removes its data) and reinstall?",
		"Uninstall " + q(pkg) + " (removes its data) and reinstall?"})
	switch {
	case errors.Is(err, ui.ErrNoInput):
		r.fail(sayf("Skipped: uninstalling needs confirmation (pass -y)",
			"Skipped %s on %s: uninstalling needs confirmation (pass -y)", base, dev))
		return false
	case errors.Is(err, ui.ErrInterrupted):
		r.fail(sayf("Skipped: interrupted", "Skipped %s on %s: interrupted", base, dev))
		return false
	case err != nil || !ok:
		r.fail(sayf("Skipped", "Skipped %s on %s", base, dev))
		return false
	}
	if installStop.Load() {
		return finish(resultStopped, "")
	}
	r.progress(sayf("Uninstalling "+q(pkg), "Uninstalling %s from %s", q(pkg), dev))
	if out, err := adb.Combined("-s", serial, "uninstall", pkg); err != nil || !strings.Contains(out, "Success") {
		r.fail(sayf("Couldn't uninstall "+q(pkg), "Couldn't uninstall %s from %s", q(pkg), dev))
		r.detail(out)
		return false
	}
	r.progress(sayf("Reinstalling", "Reinstalling %s on %s", base, dev))
	return finish(adbInstall(serial, opts, apkFile), pkg)
}

// apkListLines numbers APKs in install order with their size and time:
//
//  1. app-debug.apk  12.3 MB · 2026-11-10 14:02
func apkListLines(list []string) []string {
	nameW := 0
	for _, f := range list {
		nameW = max(nameW, ui.TextWidth(filepath.Base(f)))
	}
	numW := len(strconv.Itoa(len(list)))
	var lines []string
	for i, f := range list {
		meta := ""
		if st, err := os.Stat(f); err == nil {
			meta = humanSize(strconv.FormatInt(st.Size(), 10)) + " · " + st.ModTime().Format("2006-01-02 15:04")
		}
		lines = append(lines, fmt.Sprintf("  %*d. %s  %s", numW, i+1, ui.Pad(filepath.Base(f), nameW), ui.Err.Dim(meta)))
	}
	return lines
}
