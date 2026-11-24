package cli

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var argPackage = [2]string{"package", "Package name (default: the foreground app)"}

// ── info ─────────────────────────────────────────────

var cmdInfo = &Command{
	Name: "info", Summary: "Show app version and SDK levels", Args: "[package]",
	Long:     "Show the version, SDK levels, debuggable and system flags, enabled state, installer, install and update times, APK path, UID and PID (when running) of a package.",
	ArgHelp:  [][2]string{argPackage},
	Examples: []string{"ak info", "ak info com.example.app", "ak info --json | jq .versionName"},
	Device:   oneDevice, JSON: true, Run: runInfo,
}

var (
	reMinSdk    = regexp.MustCompile(`minSdk=([0-9]*)`)
	reTargetSdk = regexp.MustCompile(`targetSdk=([0-9]*)`)
	reEnabled   = regexp.MustCompile(`^\s*User \d+:.* enabled=(\d)`)
	reAppID     = regexp.MustCompile(`\bappId=(\d+)`)
	reUserID    = regexp.MustCompile(`\buserId=(\d+)`)
)

// AppInfo is what `ak info` reports from `dumpsys package`.
type AppInfo struct {
	VersionName, VersionCode, MinSdk, TargetSdk, Installer string
	Debuggable, System                                     bool
	// Enabled is the enabled= digit of the first `User N:` line: 0 default,
	// 1 enabled, 2 disabled, 3 disabled by user, 4 disabled until used; "" if missing.
	Enabled                                string
	FirstInstallTime, LastUpdateTime, Path string
	Splits                                 []string // including base; nil if missing
	UID                                    string
}

func firstLine(out, substr string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	return ""
}

// afterEquals returns the text between the first and second '=', trimmed.
func afterEquals(line string) string {
	parts := strings.Split(line, "=")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(strings.Fields(parts[1]), " ")
}

// restAfter returns the rest of the first line containing key, after key,
// trimmed. Unlike afterEquals it keeps '=' (a /data/app/~~…==/ path has it).
func restAfter(out, key string) string {
	l := firstLine(out, key)
	if l == "" {
		return ""
	}
	return strings.TrimSpace(l[strings.Index(l, key)+len(key):])
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
	if a.Installer == "null" {
		a.Installer = ""
	}
	// An updated system app has a second, hidden package block further down;
	// firstLine keeps every field from the first (current) one.
	if l := firstLine(dumpsys, "pkgFlags=["); l != "" {
		for _, f := range strings.Fields(l) {
			if f == "SYSTEM" {
				a.System = true
			}
		}
	}
	for _, l := range strings.Split(dumpsys, "\n") {
		if m := reEnabled.FindStringSubmatch(l); m != nil {
			a.Enabled = m[1]
			break
		}
	}
	a.FirstInstallTime = restAfter(dumpsys, "firstInstallTime=")
	a.LastUpdateTime = restAfter(dumpsys, "lastUpdateTime=")
	a.Path = restAfter(dumpsys, "codePath=")
	if l := firstLine(dumpsys, "splits=["); l != "" {
		inner := l[strings.Index(l, "splits=[")+len("splits=["):]
		inner, _, _ = strings.Cut(inner, "]")
		for _, s := range strings.Split(inner, ",") {
			if s = strings.TrimSpace(s); s != "" {
				a.Splits = append(a.Splits, s)
			}
		}
	}
	// Android 9 and older print userId= instead of appId=.
	for _, re := range []*regexp.Regexp{reAppID, reUserID} {
		if m := re.FindStringSubmatch(dumpsys); m != nil {
			a.UID = m[1]
			break
		}
	}
	return a
}

// enabledText is the `enabled` row: "yes", "no", "no (by user)", "no (until used)" or "-".
func (a AppInfo) enabledText() string {
	switch a.Enabled {
	case "0", "1":
		return "yes"
	case "2":
		return "no"
	case "3":
		return "no (by user)"
	case "4":
		return "no (until used)"
	}
	return "-"
}

// enabledJSON is true, false, or nil when the state is unknown.
func (a AppInfo) enabledJSON() any {
	switch a.Enabled {
	case "0", "1":
		return true
	case "2", "3", "4":
		return false
	}
	return nil
}

// pathText is the APK path with " (+N splits)" when there are splits besides base.
func (a AppInfo) pathText() string {
	if a.Path == "" {
		return "-"
	}
	switch n := len(a.Splits) - 1; {
	case n == 1:
		return a.Path + " (+1 split)"
	case n > 1:
		return a.Path + " (+" + strconv.Itoa(n) + " splits)"
	}
	return a.Path
}

// parsePidof returns the first PID printed by `pidof`, or "" when not running.
func parsePidof(out string) string {
	f := strings.Fields(out)
	if len(f) == 0 {
		return ""
	}
	if _, err := strconv.Atoi(f[0]); err != nil {
		return ""
	}
	return f[0]
}

func intOrNil(s string) any {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return nil
}

func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func tooMany(c *Ctx, max int) error {
	if len(c.Args) > max {
		return usageErr(c.Cmd, "Too many arguments: %s", strings.Join(c.Args[max:], " "))
	}
	return nil
}

func runInfo(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	t, err := resolvePackage(serial, c.Args)
	if err != nil {
		return err
	}
	a := parseAppInfo(adb.Out("-s", serial, "shell", "dumpsys", "package", t.pkg))
	if a.VersionName == "" || a.VersionCode == "" || a.TargetSdk == "" {
		return failErr("", "Could not read versionName, versionCode or targetSdk of %s", q(t.pkg))
	}
	pid := parsePidof(adb.Out("-s", serial, "shell", "pidof", t.pkg))
	if c.JSON() {
		var splits any
		if a.Splits != nil {
			splits = a.Splits
		}
		return ui.JSON(map[string]any{
			"package": t.pkg, "versionName": a.VersionName, "versionCode": intOrNil(a.VersionCode),
			"minSdk": intOrNil(a.MinSdk), "targetSdk": intOrNil(a.TargetSdk),
			"debuggable": a.Debuggable, "installer": strOrNil(a.Installer),
			"system": a.System, "enabled": a.enabledJSON(),
			"firstInstallTime": strOrNil(a.FirstInstallTime), "lastUpdateTime": strOrNil(a.LastUpdateTime),
			"path": strOrNil(a.Path), "splits": splits, "uid": intOrNil(a.UID), "pid": intOrNil(pid),
		})
	}
	title := "App on "
	if t.foreground {
		title = "Foreground app on "
	}
	ui.Step("%s%s", title, deviceName(serial))
	rows := [][2]string{
		{"package", t.pkg},
		{"version", a.VersionName + " (" + a.VersionCode + ")"},
		{"minSdk", sdkWithRelease(a.MinSdk)},
		{"targetSdk", sdkWithRelease(a.TargetSdk)},
		{"debuggable", strconv.FormatBool(a.Debuggable)},
		{"system", strconv.FormatBool(a.System)},
		{"enabled", a.enabledText()},
		{"installer", orDefault(a.Installer, "-")},
		{"installed", orDefault(a.FirstInstallTime, "-")},
		{"updated", orDefault(a.LastUpdateTime, "-")},
		{"path", a.pathText()},
		{"uid", orDefault(a.UID, "-")},
	}
	if pid != "" {
		rows = append(rows, [2]string{"pid", pid})
	}
	ui.KeyValues(rows)
	return nil
}

// androidReleases maps an API level to the Android version it shipped with.
var androidReleases = map[string]string{
	"14": "4.0", "15": "4.0.3", "16": "4.1", "17": "4.2", "18": "4.3", "19": "4.4", "20": "4.4W",
	"21": "5.0", "22": "5.1", "23": "6", "24": "7.0", "25": "7.1", "26": "8.0", "27": "8.1",
	"28": "9", "29": "10", "30": "11", "31": "12", "32": "12L", "33": "13", "34": "14",
	"35": "15", "36": "16",
}

// sdkWithRelease returns "34 (Android 14)", or the level alone when the
// version is not known.
func sdkWithRelease(level string) string {
	if v, ok := androidReleases[level]; ok {
		return level + " (Android " + v + ")"
	}
	return level
}

// ── permissions ──────────────────────────────────────

var cmdPermissions = &Command{
	Name: "permissions", Summary: "List granted permissions of an app", Args: "[package]",
	Long:     "List the permissions granted to a package.",
	ArgHelp:  [][2]string{argPackage},
	Examples: []string{"ak permissions", "ak permissions com.example.app --json"},
	Device:   oneDevice, JSON: true, Run: runPermissions,
}

// parseGrantedPermissions lists granted permissions once each. It stops at
// "Hidden system packages:": an updated system app also lists its factory
// version there, with that version's grants.
func parseGrantedPermissions(dumpsys string) []string {
	perms := []string{}
	seen := map[string]bool{}
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.HasPrefix(l, "Hidden system packages:") {
			break
		}
		if strings.Contains(l, "granted=true") {
			name, _, _ := strings.Cut(l, ":")
			if name = strings.TrimSpace(name); !seen[name] {
				seen[name] = true
				perms = append(perms, name)
			}
		}
	}
	return perms
}

func runPermissions(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	t, err := resolvePackage(serial, c.Args)
	if err != nil {
		return err
	}
	perms := parseGrantedPermissions(adb.Out("-s", serial, "shell", "dumpsys", "package", t.pkg))
	if c.JSON() {
		return ui.JSON(map[string]any{"package": t.pkg, "granted": perms})
	}
	ui.Step("Granted permissions of %s", t.describe(serial))
	if len(perms) == 0 {
		ui.Info("(none)")
	}
	for _, p := range perms {
		ui.Out.Println(p)
	}
	return nil
}

// ── launch ───────────────────────────────────────────

var cmdLaunch = &Command{
	Name: "launch", Summary: "Launch an app", Args: "<package>",
	Long:     "Start the launcher activity of a package.",
	ArgHelp:  [][2]string{{"package", "Package name"}},
	Examples: []string{"ak launch com.example.app"},
	Device:   oneDevice, Run: runLaunch,
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

func runLaunch(c *Ctx) error {
	if len(c.Args) == 0 {
		return usageErr(c.Cmd, "Missing package name")
	}
	if err := tooMany(c, 1); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	pkg := c.Args[0]
	if err := requirePackage(serial, pkg); err != nil {
		return err
	}
	resolved, _ := adb.Combined("-s", serial, "shell", "cmd", "package", "resolve-activity", "--brief", pkg)
	component := lastLine(resolved) // "No activity found" when there is none
	if !strings.Contains(component, "/") {
		return failErr("", "%s has no launcher activity", q(pkg))
	}
	result, _ := adb.Combined("-s", serial, "shell", "am", "start", "--user", "0", "-n", component)
	if strings.Contains(result, "Exception occurred while executing 'start'") || strings.Contains(result, "Error:") {
		ui.Errorf("Couldn't launch %s", q(pkg))
		ui.Detail("%s", result)
		return errReported
	}
	ui.Success("Launched %s on %s", q(pkg), deviceName(serial))
	return nil
}

// ── kill / clear ─────────────────────────────────────

var cmdKill = &Command{
	Name: "kill", Summary: "Force-stop apps", Args: "[package...]",
	Long:     "Force-stop one or more apps. Without a package name, the app on screen is used.",
	ArgHelp:  [][2]string{{"package", "Package names (default: the foreground app)"}},
	Examples: []string{"ak kill", "ak kill com.example.app com.example.other"},
	Device:   oneDevice, Run: runKill,
}

var cmdClear = &Command{
	Name: "clear", Summary: "Clear app data and cache", Args: "[package...]",
	Long: "Clear the data and cache of one or more apps, like reinstalling them fresh.\n" +
		"Without a package name, the app on screen is used. It does not ask first.",
	ArgHelp:  [][2]string{{"package", "Package names (default: the foreground app)"}},
	Examples: []string{"ak clear", "ak clear com.example.app"},
	Device:   oneDevice, Run: runClear,
}

// forEachPackage runs action for the foreground app (no args) or each package.
// Invalid, missing and duplicate packages are reported and skipped; any failure exits 1.
func forEachPackage(c *Ctx, action func(serial, pkg, label string) bool) error {
	serial, err := c.Device()
	if err != nil {
		return err
	}
	if len(c.Args) == 0 {
		t, err := resolvePackage(serial, nil)
		if err != nil {
			return err
		}
		if !action(serial, t.pkg, q(t.pkg)+" (foreground app)") {
			return errReported
		}
		return nil
	}
	failed := false
	seen := map[string]bool{}
	for _, pkg := range c.Args {
		switch {
		case seen[pkg]:
			ui.Warn("Skipping duplicate package %s", q(pkg))
		case !validPackageName(pkg):
			ui.Errorf("Invalid package name: %s", q(pkg))
			failed = true
		case !packageInstalled(serial, pkg):
			ui.Errorf("Package not installed on %s: %s", deviceName(serial), q(pkg))
			failed = true
		default:
			if !action(serial, pkg, q(pkg)) {
				failed = true
			}
		}
		seen[pkg] = true
	}
	if failed {
		return errReported
	}
	return nil
}

func runKill(c *Ctx) error {
	return forEachPackage(c, func(serial, pkg, label string) bool {
		if out, err := adb.Combined("-s", serial, "shell", "am", "force-stop", pkg); err != nil {
			ui.Errorf("Couldn't stop %s", q(pkg))
			ui.Detail("%s", out)
			return false
		}
		ui.Success("Stopped %s", label)
		return true
	})
}

func runClear(c *Ctx) error {
	return forEachPackage(c, func(serial, pkg, label string) bool {
		out, _ := adb.Combined("-s", serial, "shell", "pm", "clear", pkg)
		if !strings.Contains(out, "Success") {
			ui.Errorf("Couldn't clear data of %s", q(pkg))
			ui.Detail("%s", out)
			return false
		}
		ui.Success("Cleared data of %s", label)
		return true
	})
}

// ── uninstall ────────────────────────────────────────

var cmdUninstall = &Command{
	Name: "uninstall", Summary: "Uninstall an app", Args: "[package]",
	Long: "Uninstall an app (its data is removed too).\n" +
		"Without a package name, the app on screen is used. It does not ask first.",
	ArgHelp:  [][2]string{argPackage},
	Examples: []string{"ak uninstall", "ak uninstall com.example.app"},
	Device:   oneDevice, Run: runUninstall,
}

func runUninstall(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	t, err := resolvePackage(serial, c.Args)
	if err != nil {
		return err
	}
	out, _ := adb.Combined("-s", serial, "uninstall", t.pkg)
	if !strings.Contains(out, "Success") {
		ui.Errorf("Couldn't uninstall %s", q(t.pkg))
		ui.Detail("%s", out)
		return errReported
	}
	ui.Success("Uninstalled %s", t.describe(serial))
	return nil
}

// ── extract ──────────────────────────────────────────

var cmdExtract = &Command{
	Name: "extract", Summary: "Save the APK of an installed app", Args: "[package] [file]",
	Long: "Save the APK of an installed package to a local file.\n" +
		"The arguments can be given in either order: a value ending in .apk or containing '/' is the file.",
	ArgHelp: [][2]string{argPackage, {"file", "Output file (default: <package>.apk)"}},
	Examples: []string{"ak extract", "ak extract myapp.apk", "ak extract com.example.app",
		"ak extract com.example.app out.apk"},
	Device: oneDevice, Run: runExtract,
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

func runExtract(c *Ctx) error {
	if err := tooMany(c, 2); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	pkg, file := splitExtractArgs(c.Args)
	t, err := resolvePackage(serial, []string{pkg})
	if err != nil {
		return err
	}
	switch {
	case file == "":
		file = t.pkg + ".apk"
	case !strings.HasSuffix(file, ".apk"):
		file += ".apk"
	}
	remote, splits, err := apkPathForPackage(serial, t.pkg)
	if err != nil {
		return err
	}
	start := time.Now()
	sp := ui.Spin("Extracting %s", t.describe(serial))
	out, err := adb.Combined("-s", serial, "pull", remote, file)
	sp.Stop()
	if err != nil {
		ui.Errorf("Couldn't pull the APK of %s", q(t.pkg))
		ui.Detail("%s", out)
		return errReported
	}
	size := ""
	if st, err := os.Stat(file); err == nil {
		size = files.HumanSize(st.Size()) + ", "
	}
	ui.Success("Saved %s %s", q(file), ui.Err.Dim("("+size+elapsed(start)+")"))
	if splits > 0 {
		ui.Warn("%s also has %d split APKs; the base APK alone may not install", q(t.pkg), splits)
		ui.Hint("List them with 'adb -s %s shell pm path %s', pull each, and install them together with 'adb install-multiple'", serial, t.pkg)
	}
	return nil
}

// elapsed formats the time since start like "3.2s".
func elapsed(start time.Time) string {
	return strconv.FormatFloat(time.Since(start).Seconds(), 'f', 1, 64) + "s"
}
