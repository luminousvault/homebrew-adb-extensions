package cli

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/apk"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdInstall = &Command{
	Name: "install", Summary: "Install APK files", Args: "[apk|directory...]",
	Long: "Install APK files on one or more devices.\n" +
		"APK files given by name are installed; with a directory or no argument, pick APKs (newest first).\n" +
		"Common failures are handled automatically: test-only APKs are retried with -t, downgrades\n" +
		"with -d, and when that is not allowed or the signature differs, ak offers to uninstall first.\n" +
		"If '<file>.apk.idsig' exists, --no-incremental is added.",
	ArgHelp: [][2]string{{"apk", "APK files to install"}, {"directory", "Pick APKs from these directories"}},
	Flags: []Flag{
		{"-l", "--latest", "", "Install the newest APK (in the current or given directory)"},
		{"-a", "--all-apks", "", "Install every APK (in the current or given directory)"},
		{"-f", "--filter", "WORDS", "Pick among APKs whose names contain all the words"},
		{"-r", "", "", "Replace the existing app, keeping its data (always on)"},
		{"-t", "", "", "Allow test-only APKs"},
		{"-d", "", "", "Allow version downgrade"},
		flagYes,
	},
	Examples: []string{
		"ak install", "ak install app-debug.apk", "ak install -l build/outputs/apk/debug",
		"ak install -f \"myapp release\"", "ak install app.apk --all", "ak install app.apk -s 46171FDAP000P8 -y",
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
	o := installOptions{latest: c.Bool("--latest"), all: c.Bool("--all-apks"), filter: c.String("--filter"), args: c.Args}
	o.filtered = c.Bool("--filter")
	o.adbOpts = []string{"-r"}
	for _, f := range []string{"-t", "-d"} {
		if c.Bool(f) {
			o.adbOpts = append(o.adbOpts, f)
		}
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
		return usageErr(cmd, "Use only one of -l, -a and -f")
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
			return usageErr(cmd, "-l, -a and -f choose APKs from a directory; they cannot be used with APK files (%s)", a)
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

const hintChooseAPK = "Pass APK files, or use -l (newest) or -a (all)"

// pickAPKs asks which APKs to install (or uses the only one).
// scanned says the list came from a directory, so a single result is announced.
func pickAPKs(list, sources, location []string, prompt string, scanned bool) ([]string, error) {
	if len(list) == 1 {
		if scanned {
			ui.Step("Using %s %s", filepath.Base(list[0]), ui.Err.Dim("(the only APK found)"))
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
	o := installOptionsFrom(c)
	if err := validateInstallOptions(c.Cmd, o); err != nil {
		return err
	}
	files, err := selectInstallAPKs(o)
	if err != nil {
		return err
	}
	devices, err := c.Devices()
	if err != nil {
		return err
	}
	if len(devices) > 1 {
		return installParallel(devices, files, o.adbOpts)
	}
	r := &directReporter{}
	ok := true
	for _, f := range files {
		if !installOne(r, devices[0], o.adbOpts, f) {
			ok = false
		}
	}
	if !ok {
		return errReported
	}
	return nil
}

// ── Installing one APK ───────────────────────────────

// reporter receives the messages of one device's installs. The single-device
// reporter prints immediately; the parallel one buffers and drives the status board.
type reporter interface {
	progress(format string, args ...any) // current activity (spinner text)
	done()                               // activity finished
	success(format string, args ...any)
	warn(format string, args ...any)
	fail(format string, args ...any)
	detail(text string)
	confirm(question string) (bool, error)
}

type directReporter struct{ sp *ui.Spinner }

func (r *directReporter) progress(f string, a ...any) { r.done(); r.sp = ui.Spin(f, a...) }
func (r *directReporter) done() {
	if r.sp != nil {
		r.sp.Stop()
		r.sp = nil
	}
}
func (r *directReporter) success(f string, a ...any)     { r.done(); ui.Success(f, a...) }
func (r *directReporter) warn(f string, a ...any)        { r.done(); ui.Warn(f, a...) }
func (r *directReporter) fail(f string, a ...any)        { r.done(); ui.Errorf(f, a...) }
func (r *directReporter) detail(t string)                { ui.Detail("%s", t) }
func (r *directReporter) confirm(q string) (bool, error) { r.done(); return ui.Confirm(q, true) }

var reFailure = regexp.MustCompile(`\[?(INSTALL_[A-Z_]+|DELETE_[A-Z_]+)`)

// failureCode extracts e.g. INSTALL_FAILED_INSUFFICIENT_STORAGE from adb output.
func failureCode(out string) string {
	if m := reFailure.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return "unknown error"
}

func adbInstall(serial string, opts []string, apkFile string) string {
	out, _ := adb.Combined(append(append([]string{"-s", serial, "install"}, opts...), apkFile)...)
	return strings.TrimRight(out, "\n")
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
	attempt := func(o []string) string {
		r.progress("Installing %s on %s", base, deviceName(serial))
		return adbInstall(serial, o, apkFile)
	}
	finish := func(result string) bool {
		if strings.Contains(result, "Success") {
			r.success("Installed %s on %s %s", base, deviceName(serial), ui.Err.Dim("("+elapsed(start)+")"))
			return true
		}
		r.fail("Couldn't install %s on %s: %s", base, deviceName(serial), failureCode(result))
		r.detail(result)
		return false
	}

	result := attempt(opts)
	switch {
	case strings.Contains(result, "INSTALL_FAILED_TEST_ONLY"):
		r.warn("%s is a test-only APK, retrying with -t", base)
		opts = append(opts, "-t")
		result = attempt(opts)
		if strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE") {
			return reinstallAfterDowngrade(r, serial, opts, apkFile, finish)
		}
		return finish(result)
	case strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE"):
		if !hasOpt(opts, "-d") {
			r.warn("%s is older than the installed version, retrying with -d", base)
			result = attempt(append(opts, "-d"))
			if !strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE") {
				return finish(result)
			}
		}
		return reinstallAfterDowngrade(r, serial, opts, apkFile, finish)
	case strings.Contains(result, "INSTALL_FAILED_UPDATE_INCOMPATIBLE"):
		pkg := conflictPackage(result)
		if pkg == "" {
			pkg, _ = apk.PackageName(apkFile)
		}
		r.warn("The signature of %s differs from %s installed on %s", base, pkg, deviceName(serial))
		return uninstallThenInstall(r, serial, pkg, opts, apkFile, finish)
	}
	return finish(result)
}

func reinstallAfterDowngrade(r reporter, serial string, opts []string, apkFile string, finish func(string) bool) bool {
	pkg, err := apk.PackageName(apkFile)
	if err != nil {
		r.fail("Couldn't read the package name of %s: %v", filepath.Base(apkFile), err)
		return false
	}
	r.warn("%s does not allow downgrading %s without uninstalling it", deviceName(serial), pkg)
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

func uninstallThenInstall(r reporter, serial, pkg string, opts []string, apkFile string, finish func(string) bool) bool {
	ok, err := r.confirm("Uninstall " + pkg + " from " + deviceName(serial) + " (removes its data) and reinstall?")
	switch {
	case errors.Is(err, ui.ErrNoInput):
		r.fail("Skipped %s on %s: uninstalling needs confirmation (pass -y)", filepath.Base(apkFile), deviceName(serial))
		return false
	case errors.Is(err, ui.ErrInterrupted):
		r.fail("Skipped %s on %s: interrupted", filepath.Base(apkFile), deviceName(serial))
		return false
	case err != nil || !ok:
		r.fail("Skipped %s on %s", filepath.Base(apkFile), deviceName(serial))
		return false
	}
	r.progress("Uninstalling %s from %s", pkg, deviceName(serial))
	if out, err := adb.Combined("-s", serial, "uninstall", pkg); err != nil || !strings.Contains(out, "Success") {
		r.fail("Couldn't uninstall %s from %s", pkg, deviceName(serial))
		r.detail(out)
		return false
	}
	r.progress("Reinstalling %s on %s", filepath.Base(apkFile), deviceName(serial))
	return finish(adbInstall(serial, opts, apkFile))
}
