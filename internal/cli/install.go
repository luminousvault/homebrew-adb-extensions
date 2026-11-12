package cli

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/apk"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

var cmdInstall = &Command{
	Name: "install", Desc: "Install APK files",
	Usage: "install [options] [apk_files...]", Help: "Install APK files",
	Flags: []string{"-h", "--help", "-l", "-a", "-f", "-m", "-r", "-t", "-d"}, Run: runInstall,
}

func showHelpInstall() {
	say("%sUsage:%s ak install [options] [apk_files...]", Bold, NC)
	say("Install APK files on a selected Android device using adb.")
	say("")
	say("%sGeneral Options:%s", Bold, NC)
	say("  -h\t\tShow this help message and exit.")
	say("")
	say("%sAPK Selection Options (mutually exclusive):%s", Bold, NC)
	say("  (none)\tSelect APK files interactively from the current directory (default).")
	say("  <directories>\tSelect APK files interactively from the specified directories.")
	say("  <apk files>\tDirectly specify APK files to install.")
	say("  -l\t\tInstall the latest APK file (from current directory or specified directory).")
	say("  -a\t\tInstall all APK files (from current directory or specified directory).")
	say("  -f <filter>\tFilter and select APK files matching the filter interactively.")
	say("\t\t\tFilter is REQUIRED. Can be used with directory.")
	say("\t\t\tExamples:")
	say("\t\t\t  -f debug\t\t\tFind APKs containing 'debug' in current dir")
	say("\t\t\t  -f \"myapp release\"\t\tFind APKs containing both 'myapp' and 'release'")
	say("\t\t\t  -f debug /path/to/folder\tFind APKs in specified folder")
	say("")
	say("%sDevice Options:%s", Bold, NC)
	say("  -m\t\tInstall APK files on all connected devices.")
	say("")
	say("%sADB Install Options:%s", Bold, NC)
	say("  -r\t\tReplace an existing application without removing its data (default).")
	say("  -t\t\tAllow test APKs to be installed.")
	say("  -d\t\tAllow version code downgrade (requires 'pm' permission).")
	say("")
	say("%sCompatibility Notes:%s", Bold, NC)
	say("  If a '.idsig' file is present for the APK, the '--no-incremental' option is added to")
	say("  the install command to ensure compatibility.")
}

type installOptions struct {
	latest, all, filtered, allDevices bool
	filter                            string
	adbOpts                           []string // starts with -r
	args                              []string // positional: APK files, directories
}

func parseInstallOptions(args []string) installOptions {
	o := installOptions{adbOpts: []string{"-r"}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			showHelpInstall()
			exit(0)
		case "-f":
			o.filtered = true
			i++
			if i >= len(args) {
				say("%s Option -f requires a filter argument.", Error)
				say("")
				say("%sUsage:%s ak install -f <filter> [directory]", Bold, NC)
				say("%sExample:%s", Bold, NC)
				say("  ak install -f debug")
				say("  ak install -f \"myapp release\"")
				say("  ak install -f debug /path/to/folder")
				say("  ak install /path/to/folder -f debug")
				say("")
				say("For interactive selection of all APKs, use: ak install")
				exit(1)
			}
			o.filter = args[i]
		case "-l":
			o.latest = true
		case "-a":
			o.all = true
		case "-m":
			o.allDevices = true
		case "-t", "-d":
			o.adbOpts = append(o.adbOpts, a)
		case "-r":
			// default
		default:
			if strings.HasPrefix(a, "-") {
				// Bash printed the error line to stderr and the hint to stdout.
				sayErr("%s Invalid option: %s", Error, a)
				say("Try 'ak install --help' for more information.")
				exit(1)
			}
			o.args = append(o.args, a)
		}
	}
	return o
}

func validateInstallOptions(o installOptions) {
	switch {
	case o.latest && o.all && o.filtered:
		say("%s Options -l, -a, and -f cannot be used together.", Error)
		exit(1)
	case o.latest && o.all:
		say("%s Options -l and -a cannot be used together.", Error)
		exit(1)
	case o.latest && o.filtered:
		say("%s Options -l and -f cannot be used together.", Error)
		exit(1)
	case o.all && o.filtered:
		say("%s Options -a and -f cannot be used together.", Error)
		exit(1)
	}
	for _, a := range o.args {
		if st, err := os.Stat(a); err == nil && st.Mode().IsRegular() {
			if filepath.Ext(a) != ".apk" {
				say("%s Invalid file detected: '%s'. Only APK files are allowed.", Error, a)
				exit(1)
			}
			if o.latest || o.all || o.filtered {
				say("%s Options -l, -a, or -f cannot be used with APK file arguments: '%s'.", Error, a)
				exit(1)
			}
		}
	}
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
		if matchesFilter(p, filter) {
			out = append(out, p)
			keep = append(keep, i)
		}
	}
	return out, keep
}

func onlyOneAPK(p string) {
	say("%s Only one APK file found: %s%s%s", BArrow, Yellow, filepath.Base(p), NC)
}

// displayDir returns an absolute, ~-abbreviated directory for display.
func displayDir(dir string) string {
	if a, err := filepath.Abs(dir); err == nil {
		dir = a
	}
	return tildeHome(dir)
}

// selectAPKsInCurrentDir is the default: pick from ./*.apk (newest first).
func selectAPKsInCurrentDir(filter string) []string {
	say("%s %sScanning APK files in the current directory...%s", BArrow, Bold, NC)
	list := listAPKs(".", sortNewest)
	if len(list) == 0 {
		say("%s No APK files found in the current directory.", Error)
		exit(1)
	}
	if filter != "" {
		list, _ = filterAPKs(list, filter)
		if len(list) == 0 {
			say("%s No APK files found matching all filters: '%s'", Error, filter)
			exit(1)
		}
	}
	if len(list) == 1 {
		onlyOneAPK(list[0])
		return list
	}
	items := make([]string, len(list))
	for i, p := range list {
		items[i] = filepath.Base(p)
	}
	r := pick(tui.Options{Multi: true, Prompt: "Select APK files to install", Location: []string{displayDir(".")}, Items: items})
	var picked []string
	for _, i := range r.Indices {
		picked = append(picked, list[i])
	}
	if len(picked) == 0 {
		say("%s No valid APK files selected.", Error)
		exit(1)
	}
	return picked
}

// collectAPKArgs resolves APK file and directory arguments, asking when several APKs match.
func collectAPKArgs(args []string, filter string) []string {
	var list, sources, dirs, missing []string
	found := false
	for _, a := range args {
		st, err := os.Stat(a)
		switch {
		case err != nil:
			missing = append(missing, a)
		case st.Mode().IsRegular():
			if !strings.HasSuffix(a, ".apk") {
				say("%s Invalid file detected: '%s'. Only APK files are allowed.", Error, a)
				exit(1)
			}
			found = true
			list = append(list, a)
			sources = append(sources, displayDir(filepath.Dir(a)))
		case st.IsDir():
			found = true
			dirs = append(dirs, a)
			for _, p := range listAPKs(a, sortNewest) {
				list = append(list, p)
				sources = append(sources, displayDir(a))
			}
		}
	}
	if !found {
		return nil
	}
	if len(list) == 0 {
		say("%s No APK files found in the specified directories.", Error)
		exit(1)
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
			say("%s No APK files found matching all filters: %s", Error, filter)
			exit(1)
		}
	}
	if len(list) == 1 {
		onlyOneAPK(list[0])
		return list
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
	var location []string
	for _, d := range dirs {
		label := displayDir(d)
		if len(listAPKs(d, sortNewest)) == 0 {
			label += " (empty)"
		}
		location = append(location, label)
	}
	for _, m := range missing {
		p := m
		if home, err := os.UserHomeDir(); err == nil && (p == "~" || strings.HasPrefix(p, "~/")) {
			p = home + strings.TrimPrefix(p, "~")
		}
		location = append(location, displayDir(p)+" (not found)")
	}

	say("%s %sSelect APK files to install%s", BArrow, Bold, NC)
	say("")
	r := pick(tui.Options{Multi: true, Prompt: "Select APK files", Location: location, Items: items})
	var picked []string
	for _, i := range r.Indices {
		picked = append(picked, list[i])
	}
	return picked
}

func selectInstallAPKs(o installOptions) []string {
	var files []string
	if o.filtered && len(o.args) == 0 {
		files = selectAPKsInCurrentDir(o.filter)
	}
	if o.latest || o.all {
		dir := "."
		for _, a := range o.args {
			if st, err := os.Stat(a); err == nil && st.IsDir() {
				dir = a
				break
			}
		}
		if list := listAPKs(dir, sortNewest); len(list) > 0 {
			if o.latest {
				files = list[:1]
			} else {
				files = list
			}
		}
	}
	if len(files) == 0 {
		files = collectAPKArgs(o.args, o.filter)
	}
	if len(files) == 0 && len(o.args) == 0 {
		files = selectAPKsInCurrentDir(o.filter)
	}
	if len(files) == 0 {
		say("%s No APK files found.", Error)
		exit(1)
	}
	return files
}

func runInstall(args []string) {
	o := parseInstallOptions(args)
	validateInstallOptions(o)
	files := selectInstallAPKs(o)
	devices := selectDevicesMulti(o.allDevices)

	if len(devices) > 1 {
		say("")
		say("%s %sSelected devices for installation:%s", BArrow, Bold, NC)
		for i, d := range devices {
			say("%d. %s", i+1, prettyDevice(d, "normal"))
		}
	}
	say("")
	say("%s %sThe APK files to install.%s", BArrow, Bold, NC)
	for i, f := range files {
		say("%d. %s", i+1, filepath.Base(f))
	}
	say("")
	say("%s═══════════════════════════════════════════════════════════════════════%s", Cyan, NC)
	say("%s%s    🚀 Starting the install process for the selected devices... 🚀%s", Bold, Purple, NC)
	say("%s═══════════════════════════════════════════════════════════════════════%s", Cyan, NC)

	for _, d := range devices {
		say("")
		say("%s %sSelected device: %s%s%s", BArrow, Bold, Cyan, prettyDevice(d, "normal"), NC)
		for _, f := range files {
			opts := append([]string(nil), o.adbOpts...)
			if st, err := os.Stat(f + ".idsig"); err == nil && !st.IsDir() {
				say("")
				say("%s Detected an .idsig file associated with %s'%s'%s.", GArrow, Yellow, filepath.Base(f), NC)
				say("    Applying the %s%s'--no-incremental'%s option for compatibility.%s", Cyan, Bold, NC, NC)
				opts = append(opts, "--no-incremental")
			}
			installWithRecovery(d, opts, f)
		}
	}
}

func adbInstall(serial string, opts []string, apkFile string) string {
	out, _ := adb.Combined(append(append([]string{"-s", serial, "install"}, opts...), apkFile)...)
	return strings.TrimRight(out, "\n")
}

func printInstallCommand(opts []string, apkFile string) {
	say("%s Install command: %sadb install %s %s%s", BArrow, Bold, strings.Join(opts, " "), filepath.Base(apkFile), NC)
}

func hasOpt(opts []string, opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

// installWithRecovery installs one APK and handles common failures automatically.
func installWithRecovery(serial string, opts []string, apkFile string) {
	say("")
	printInstallCommand(opts, apkFile)
	result := adbInstall(serial, opts, apkFile)
	switch {
	case strings.Contains(result, "INSTALL_FAILED_TEST_ONLY"):
		retryInstall(serial, "INSTALL_FAILED_TEST_ONLY", "-t", opts, apkFile)
	case strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE"):
		if hasOpt(opts, "-d") {
			resolveDowngrade(serial, opts, apkFile)
		} else {
			retryInstall(serial, "INSTALL_FAILED_VERSION_DOWNGRADE", "-d", opts, apkFile)
		}
	case strings.Contains(result, "INSTALL_FAILED_UPDATE_INCOMPATIBLE"):
		resolveConflict(serial, opts, apkFile, result)
	default:
		say("%s", result)
	}
}

func retryInstall(serial, reason, retryOpt string, opts []string, apkFile string) {
	inner := append(append([]string(nil), opts...), retryOpt)
	say("")
	say("%s Installation failed due to %s'%s'%s. Retrying with %s%s'%s'%s option.", GArrow, Yellow, reason, NC, Cyan, Bold, retryOpt, NC)
	say("")
	printInstallCommand(inner, apkFile)
	result := adbInstall(serial, inner, apkFile)
	if strings.Contains(result, "INSTALL_FAILED_VERSION_DOWNGRADE") {
		resolveDowngrade(serial, opts, apkFile)
		return
	}
	say("%s", result)
}

// confirmYes asks a [Y/n] question; Enter, y and Y mean yes.
func confirmYes(question string) bool {
	sayf("%s", question)
	c := readKey()
	say("%s", c)
	return c == "" || c == "y" || c == "Y"
}

func uninstallAndReinstall(serial, pkg string, opts []string, apkFile string) {
	say("")
	say("%s Uninstalling package: %s%s%s", BArrow, Bold, pkg, NC)
	if _, err := adb.Combined("-s", serial, "uninstall", pkg); err != nil {
		say("%s Failed to uninstall the existing application.", Error)
		return
	}
	say("%s Uninstallation successful.", GArrow)
	say("")
	printInstallCommand(opts, apkFile)
	say("%s", adbInstall(serial, opts, apkFile))
}

func resolveDowngrade(serial string, opts []string, apkFile string) {
	say("")
	say("%s%sApplication Installation Failed%s", Red, Bold, NC)
	say("")
	say("The adb install -d option is not supported on newer Android OS versions.")
	say("You need to uninstall the existing application before reinstalling it.")
	say("")
	say("%s%sWARNING:%s Uninstalling will remove all application data!", Yellow, Bold, NC)
	say("")
	if !confirmYes("Do you want to uninstall and reinstall the application? [Y/n]: ") {
		say("%s Installation aborted by user.", GArrow)
		return
	}
	pkg, err := apk.PackageName(apkFile)
	if err != nil {
		say("")
		say("%s Could not read the package name from %s: %v", Error, filepath.Base(apkFile), err)
		say("")
		return
	}
	uninstallAndReinstall(serial, pkg, opts, apkFile)
}

var reConflictPackage = regexp.MustCompile(`^.*package ([^ ]*)`)

func resolveConflict(serial string, opts []string, apkFile, result string) {
	say("")
	say("%s%sApplication Installation Failed%s", Red, Bold, NC)
	say("")
	say("The device already has an application with the same package but a different signature.")
	say("In order to proceed, you will have to uninstall the existing application.")
	say("")
	say("%s%sWARNING:%s Uninstalling will remove the application data!", Yellow, Bold, NC)
	say("")
	if !confirmYes("Do you want to uninstall the existing application? [Y/n]: ") {
		say("%s Installation aborted by user.", GArrow)
		return
	}
	pkg := ""
	for _, l := range strings.Split(result, "\n") {
		if m := reConflictPackage.FindStringSubmatch(l); m != nil {
			pkg = m[1]
			break
		}
	}
	if pkg == "" {
		pkg, _ = apk.PackageName(apkFile)
	}
	uninstallAndReinstall(serial, pkg, opts, apkFile)
}
