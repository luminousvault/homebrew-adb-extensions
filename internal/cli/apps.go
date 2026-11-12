package cli

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var argPackage = [2]string{"package", "Package name (default: the foreground app)"}

// ── info ─────────────────────────────────────────────

var cmdInfo = &Command{
	Name: "info", Summary: "Show package information", Args: "[package]",
	Long:     "Show the version, SDK levels, debuggable flag and installer of a package.",
	ArgHelp:  [][2]string{argPackage},
	Examples: []string{"ak info", "ak info com.example.app", "ak info --json | jq .versionName"},
	Device:   oneDevice, JSON: true, Run: runInfo,
}

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

// afterEquals returns the text between the first and second '=', trimmed.
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
	if a.Installer == "null" {
		a.Installer = ""
	}
	return a
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
		return failErr("", "Could not read versionName, versionCode or targetSdk of %s", t.pkg)
	}
	if c.JSON() {
		return ui.JSON(map[string]any{
			"package": t.pkg, "versionName": a.VersionName, "versionCode": intOrNil(a.VersionCode),
			"minSdk": intOrNil(a.MinSdk), "targetSdk": intOrNil(a.TargetSdk),
			"debuggable": a.Debuggable, "installer": strOrNil(a.Installer),
		})
	}
	ui.Step("%s", t.describe(serial))
	ui.KeyValues([][2]string{
		{"versionName", a.VersionName},
		{"versionCode", a.VersionCode},
		{"minSdk", a.MinSdk},
		{"targetSdk", a.TargetSdk},
		{"debuggable", strconv.FormatBool(a.Debuggable)},
		{"installerPackageName", orDefault(a.Installer, "-")},
	})
	return nil
}

// ── permissions ──────────────────────────────────────

var cmdPermissions = &Command{
	Name: "permissions", Summary: "List package permissions", Args: "[package]",
	Long:     "List the permissions granted to a package.",
	ArgHelp:  [][2]string{argPackage},
	Examples: []string{"ak permissions", "ak permissions com.example.app --json"},
	Device:   oneDevice, JSON: true, Run: runPermissions,
}

func parseGrantedPermissions(dumpsys string) []string {
	perms := []string{}
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.Contains(l, "granted=true") {
			name, _, _ := strings.Cut(l, ":")
			perms = append(perms, strings.TrimSpace(name))
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
	Name: "launch", Summary: "Launch package", Args: "<package>",
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
	result, _ := adb.Combined("-s", serial, "shell", "am", "start", "--user", "0", "-n", lastLine(resolved))
	if strings.Contains(result, "Exception occurred while executing 'start'") || strings.Contains(result, "Error:") {
		ui.Errorf("Couldn't launch %s", pkg)
		ui.Detail("%s", result)
		return errReported
	}
	ui.Success("Launched %s on %s", pkg, deviceName(serial))
	return nil
}

// ── kill / clear ─────────────────────────────────────

var cmdKill = &Command{
	Name: "kill", Summary: "Force-stop package", Args: "[package...]",
	Long:     "Force-stop one or more packages.",
	ArgHelp:  [][2]string{{"package", "Package names (default: the foreground app)"}},
	Examples: []string{"ak kill", "ak kill com.example.app com.example.other"},
	Device:   oneDevice, Run: runKill,
}

var cmdClear = &Command{
	Name: "clear", Summary: "Clear app data and cache", Args: "[package...]",
	Long:     "Clear the data and cache of one or more packages.",
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
		if !action(serial, t.pkg, t.pkg+" (foreground app)") {
			return errReported
		}
		return nil
	}
	failed := false
	seen := map[string]bool{}
	for _, pkg := range c.Args {
		switch {
		case seen[pkg]:
			ui.Warn("Skipping duplicate package %s", pkg)
		case !validPackageName(pkg):
			ui.Errorf("Invalid package name: %s", pkg)
			failed = true
		case !packageInstalled(serial, pkg):
			ui.Errorf("Package not installed on %s: %s", deviceName(serial), pkg)
			failed = true
		default:
			if !action(serial, pkg, pkg) {
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
			ui.Errorf("Couldn't stop %s", pkg)
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
			ui.Errorf("Couldn't clear data of %s", pkg)
			ui.Detail("%s", out)
			return false
		}
		ui.Success("Cleared data of %s", label)
		return true
	})
}

// ── uninstall ────────────────────────────────────────

var cmdUninstall = &Command{
	Name: "uninstall", Summary: "Uninstall package", Args: "[package]",
	Long:     "Uninstall a package.",
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
		ui.Errorf("Couldn't uninstall %s", t.pkg)
		ui.Detail("%s", out)
		return errReported
	}
	ui.Success("Uninstalled %s", t.describe(serial))
	return nil
}

// ── extract ──────────────────────────────────────────

var cmdExtract = &Command{
	Name: "extract", Summary: "Extract APK file from device", Args: "[package] [file]",
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
	remote, err := apkPathForPackage(serial, t.pkg)
	if err != nil {
		return err
	}
	start := time.Now()
	sp := ui.Spin("Extracting %s", t.describe(serial))
	out, err := adb.Combined("-s", serial, "pull", remote, file)
	sp.Stop()
	if err != nil {
		ui.Errorf("Couldn't pull the APK of %s", t.pkg)
		ui.Detail("%s", out)
		return errReported
	}
	size := ""
	if st, err := os.Stat(file); err == nil {
		size = humanSize(strconv.FormatInt(st.Size(), 10)) + ", "
	}
	ui.Success("Saved %s %s", file, ui.Err.Dim("("+size+elapsed(start)+")"))
	return nil
}

// elapsed formats the time since start like "3.2s".
func elapsed(start time.Time) string {
	return strconv.FormatFloat(time.Since(start).Seconds(), 'f', 1, 64) + "s"
}
