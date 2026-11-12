package cli

import (
	"regexp"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

var helpFlags = []string{"-h", "--help"}

// helpWith builds the common help layout used by package commands.
func helpWith(usage string, lines ...string) func() {
	return func() {
		say("%s%sUsage:%s %s", Cyan, Bold, NC, usage)
		say("")
		for _, l := range lines {
			say("%s", l)
		}
		say("")
		say("Options:")
		say("  -h, --help  Show this help message")
		say("")
	}
}

// ── info ─────────────────────────────────────────────

var cmdInfo = &Command{
	Name: "info", Desc: "Show package information",
	Usage: "info [package]", Help: "Show app information",
	Flags: helpFlags, Run: runInfo,
}

var showHelpInfo = helpWith("ak info [-h|--help] [packageName]",
	"Description: Show detailed information of the package (version, SDK, debuggable, etc.).",
	"If 'packageName' is not provided, the script will auto-detect the current foreground app.")

var (
	reMinSdk    = regexp.MustCompile(`minSdk=([0-9]*)`)
	reTargetSdk = regexp.MustCompile(`targetSdk=([0-9]*)`)
)

// AppInfo is what `ak info` reports from `dumpsys package`.
type AppInfo struct {
	VersionName, VersionCode, MinSdk, TargetSdk, Installer string
	Debuggable                                             bool
}

func firstLine(out, substr string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	return ""
}

// afterEquals mimics `cut -d= -f2 | xargs`: the text between the first and
// second '=', with whitespace collapsed.
func afterEquals(line string) string {
	parts := strings.Split(line, "=")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(strings.Fields(parts[1]), " ")
}

func parseAppInfo(dumpsys string) AppInfo {
	var a AppInfo
	a.VersionName = afterEquals(firstLine(dumpsys, "versionName"))
	if l := firstLine(dumpsys, "versionCode"); l != "" {
		if f := strings.Fields(l); len(f) > 0 {
			a.VersionCode = afterEquals(f[0])
		}
	}
	if m := reMinSdk.FindStringSubmatch(firstLine(dumpsys, "minSdk=")); m != nil {
		a.MinSdk = m[1]
	}
	if m := reTargetSdk.FindStringSubmatch(firstLine(dumpsys, "targetSdk=")); m != nil {
		a.TargetSdk = m[1]
	}
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.Contains(strings.ToLower(l), "flags=[") && strings.Contains(l, "DEBUGGABLE") {
			a.Debuggable = true
			break
		}
	}
	a.Installer = afterEquals(firstLine(dumpsys, "installerPackageName"))
	return a
}

func runInfo(args []string) {
	args = parseLeading("info", args, showHelpInfo, nil, true)
	serial := selectDevice()
	pkg := packageArgOrForeground(serial, args)

	a := parseAppInfo(adb.Out("-s", serial, "shell", "dumpsys", "package", pkg))
	if a.VersionName == "" || a.VersionCode == "" || a.TargetSdk == "" {
		say("%sERROR: Critical app info missing for package:%s %s", Red, NC, pkg)
		say("%s  ==> versionName, versionCode, or targetSdk could not be retrieved.%s", Red, NC)
		say("")
		exit(1)
	}
	installer := a.Installer
	if installer == "" {
		installer = Yellow + "(not available)" + NC
	}
	debuggable := "false"
	if a.Debuggable {
		debuggable = "true"
	}
	say("%sAPK Detailed Info:%s", Cyan, NC)
	say("  %sversionName:%s %s", Green, NC, a.VersionName)
	say("  %sversionCode:%s %s", Green, NC, a.VersionCode)
	say("  %sminSdk:%s %s", Green, NC, a.MinSdk)
	say("  %stargetSdk:%s %s", Green, NC, a.TargetSdk)
	say("  %sdebuggable:%s %s", Green, NC, debuggable)
	say("  %sinstallerPackageName:%s %s", Green, NC, installer)
	say("")
}

// ── permissions ──────────────────────────────────────

var cmdPermissions = &Command{
	Name: "permissions", Desc: "List package permissions",
	Usage: "permissions [package]", Help: "List app permissions",
	Flags: helpFlags, Run: runPermissions,
}

var showHelpPermissions = helpWith("ak permissions [-h|--help] [packageName]",
	"Description: List all permissions granted to the specified package.",
	"If 'packageName' is not provided, the script will auto-detect the current foreground app.")

func parseGrantedPermissions(dumpsys string) []string {
	var perms []string
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.Contains(l, "granted=true") {
			name, _, _ := strings.Cut(l, ":")
			perms = append(perms, strings.TrimLeft(name, " "))
		}
	}
	return perms
}

func runPermissions(args []string) {
	args = parseLeading("permissions", args, showHelpPermissions, nil, true)
	serial := selectDevice()
	pkg := packageArgOrForeground(serial, args)

	perms := parseGrantedPermissions(adb.Out("-s", serial, "shell", "dumpsys", "package", pkg))
	if len(perms) == 0 {
		say("%sERROR: No permissions found or failed to retrieve permissions for package:%s %s", Red, NC, pkg)
		say("")
		exit(1)
	}
	say("%sPermissions for package %s:%s", Cyan, pkg, NC)
	say("%s", strings.Join(perms, "\n"))
	say("")
}

// ── launch ───────────────────────────────────────────

var cmdLaunch = &Command{
	Name: "launch", Desc: "Launch package",
	Usage: "launch <package>", Help: "Launch app",
	Flags: helpFlags, Run: runLaunch,
}

var showHelpLaunch = helpWith("ak launch [-h|--help] <packageName>",
	"Description: Launch the specified package using its launcher activity.",
	"Package name is required for this command.")

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

func runLaunch(args []string) {
	args = parseLeading("launch", args, showHelpLaunch, nil, true)
	if len(args) == 0 || args[0] == "" {
		showHelpLaunch()
		exit(1)
	}
	pkg := args[0]
	serial := selectDevice()
	if !packageInstalled(serial, pkg) {
		say("")
		say("%sERROR: Package not installed on the device:%s %s", Red, NC, pkg)
		exit(1)
	}
	resolved, _ := adb.Combined("-s", serial, "shell", "cmd", "package", "resolve-activity", "--brief", pkg)
	result, _ := adb.Combined("-s", serial, "shell", "am", "start", "--user", "0", "-n", lastLine(resolved))
	if strings.Contains(result, "Exception occurred while executing 'start'") {
		say("%sERROR: Failed to launch app. Reason:%s", Red, NC)
		say("  %s", strings.ReplaceAll(strings.TrimRight(result, "\n"), "\n", "\n  "))
		say("")
		exit(1)
	}
	say("%sSuccessfully launched package:%s %s", Green, NC, pkg)
	say("")
}

// ── kill / clear ─────────────────────────────────────

var cmdKill = &Command{
	Name: "kill", Desc: "Force-stop package",
	Usage: "kill [packages...]", Help: "Force-stop app",
	Flags: helpFlags, Run: runKill,
}

var showHelpKill = helpWith("ak kill [-h|--help] [packageName1] [packageName2] ...",
	"Description: Force-stops one or more specified packages on the connected device.",
	"If no packageName is provided, the script will auto-detect the current foreground app.")

var cmdClear = &Command{
	Name: "clear", Desc: "Clear app data and cache",
	Usage: "clear [packages...]", Help: "Clear app data",
	Flags: helpFlags, Run: runClear,
}

var showHelpClear = helpWith("ak clear [-h|--help] [packageName1] [packageName2] ...",
	"Description: Clear app data and cache for one or more specified packages.",
	"If no packageName is provided, the script will auto-detect the current foreground app.")

func killPackage(serial, pkg string) {
	say("")
	say("%s==> Attempting to kill:%s %s", Yellow, NC, pkg)
	if err := adb.Default.Passthrough("-s", serial, "shell", "am", "force-stop", pkg); err == nil {
		say("%sSuccessfully killed%s", Green, NC)
	} else {
		say("%sERROR: Failed to kill:%s %s (may not be installed or running)", Red, NC, pkg)
	}
}

func clearPackage(serial, pkg string) {
	say("")
	say("%s==> Attempting to clear data for:%s %s", Yellow, NC, pkg)
	say("%s==> Clearing app data...%s", Blue, NC)
	out, _ := adb.Combined("-s", serial, "shell", "pm", "clear", pkg)
	out = strings.TrimRight(out, "\n")
	if strings.Contains(out, "Success") {
		say("%sSuccessfully cleared data%s", Green, NC)
	} else {
		say("%sERROR: Failed to clear app data:%s %s", Red, NC, pkg)
		say("  ==> %s", out)
	}
}

// forEachPackage runs action for the foreground app (no args) or each valid,
// installed, non-duplicate package.
func forEachPackage(serial string, args []string, startMsg string, action func(serial, pkg string)) {
	if len(args) == 0 {
		pkg := foregroundPackage(serial)
		say("")
		say("%sAuto-detected package:%s %s", Yellow, NC, pkg)
		requirePackage(serial, pkg)
		action(serial, pkg)
		return
	}
	say("%s%s%s", Cyan, startMsg, NC)
	seen := map[string]bool{}
	for _, pkg := range args {
		switch {
		case !validPackageName(pkg):
			say("")
			say("%sERROR: Invalid package name format:%s %s", Red, NC, pkg)
		case !packageInstalled(serial, pkg):
			say("")
			say("%sERROR: Package not installed on the device:%s %s", Red, NC, pkg)
		case seen[pkg]:
			say("")
			say("%sWARNING: Skipping duplicate package:%s %s", Yellow, NC, pkg)
		default:
			seen[pkg] = true
			action(serial, pkg)
		}
	}
}

func runKill(args []string) {
	args = parseLeading("kill", args, showHelpKill, nil, true)
	forEachPackage(selectDevice(), args, "Starting to kill packages...", killPackage)
}

func runClear(args []string) {
	args = parseLeading("clear", args, showHelpClear, nil, true)
	forEachPackage(selectDevice(), args, "Starting to clear app data...", clearPackage)
}

// ── uninstall ────────────────────────────────────────

var cmdUninstall = &Command{
	Name: "uninstall", Desc: "Uninstall package",
	Usage: "uninstall [package]", Help: "Uninstall app",
	Flags: helpFlags, Run: runUninstall,
}

var showHelpUninstall = helpWith("ak uninstall [-h|--help] [packageName]",
	"Description: Uninstall the specified package from the device.",
	"If 'packageName' is not provided, the script will auto-detect the current foreground app.")

func runUninstall(args []string) {
	args = parseLeading("uninstall", args, showHelpUninstall, nil, true)
	serial := selectDevice()
	pkg := packageArgOrForeground(serial, args)
	out, _ := adb.Combined("-s", serial, "uninstall", pkg)
	out = strings.TrimRight(out, "\n")
	if strings.Contains(out, "Success") {
		say("%sSuccessfully uninstalled.%s", Green, NC)
	} else {
		say("%sERROR: Failed to uninstall:%s", Red, NC)
		say("  ==> %s", out)
	}
	say("")
}

// ── extract ──────────────────────────────────────────

var cmdExtract = &Command{
	Name: "extract", Desc: "Extract APK file from device",
	Usage: "extract [package] [filename]", Help: "Extract APK from device",
	Flags: helpFlags, Run: runExtract,
}

func showHelpExtract() {
	say("%s%sUsage:%s ak extract [-h|--help] [packageName|saveApkFileName] [saveApkFileName|packageName]", Cyan, Bold, NC)
	say("")
	say("Description: Extract the APK file of the specified package to local storage.")
	say("The order of arguments is flexible - the script automatically detects which is the package name and which is the file name.")
	say("")
	say("%s%sExamples:%s", Cyan, Bold, NC)
	say("  ak extract                          # Auto-detect current app, save as <packageName>.apk")
	say("  ak extract myapp.apk                # Auto-detect current app, save as myapp.apk")
	say("  ak extract com.example.app          # Specific package, save as com.example.app.apk")
	say("  ak extract com.example.app out.apk  # Specific package, save as out.apk")
	say("  ak extract out.apk com.example.app  # Same as above (order flexible)")
	say("")
	say("Options:")
	say("  -h, --help  Show this help message")
	say("")
}

func looksLikeFile(s string) bool { return strings.HasSuffix(s, ".apk") || strings.Contains(s, "/") }

// splitExtractArgs detects which argument is the package and which the file name.
func splitExtractArgs(args []string) (pkg, file string) {
	switch len(args) {
	case 1:
		if looksLikeFile(args[0]) {
			return "", args[0]
		}
		return args[0], ""
	case 2:
		if looksLikeFile(args[0]) {
			return args[1], args[0]
		}
		return args[0], args[1]
	}
	return "", ""
}

func runExtract(args []string) {
	args = parseLeading("extract", args, showHelpExtract, nil, true)
	serial := selectDevice()
	if len(args) > 2 {
		say("%s Too many arguments", Error)
		say("Try 'ak extract --help' for more information.")
		exit(1)
	}
	pkg, file := splitExtractArgs(args)
	pkg = packageArgOrForeground(serial, []string{pkg})

	switch {
	case file == "":
		file = pkg + ".apk"
	case !strings.HasSuffix(file, ".apk"):
		file += ".apk"
	}
	path, ok := apkPathForPackage(serial, pkg)
	if !ok {
		exit(1)
	}
	if err := adb.Default.Passthrough("-s", serial, "pull", path, file); err != nil {
		say("")
		say("%sERROR: Failed to pull APK from device. Check device connection and permissions.%s", Red, NC)
		exit(1)
	}
	say("%sAPK file saved as:%s %s", Green, NC, file)
}
