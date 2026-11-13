package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdLogcat = &Command{
	Name: "logcat", Summary: "Show logs of one or more apps", Args: "[package...]",
	Long: "Show the logcat lines of one app and keep following it when it restarts (new process IDs\n" +
		"are picked up automatically; processes like 'pkg:remote' are included). The app's lines still\n" +
		"in the device log buffer are shown first (use -c to start empty). Press Ctrl+C to stop.\n" +
		"With several packages their lines are merged in time order, each marked with its app.\n" +
		"Android framework code running in the app (views, input method, graphics, class loading)\n" +
		"logs under its own tags; those lines are hidden except errors (E, F), and GPU driver lines\n" +
		"except F. -e shows them. -t and -i take a tag name; '*' matches any text (-t 'My*').",
	ArgHelp: [][2]string{argPackage},
	Flags: []Flag{
		{"-l", "--level", "LEVEL", "Minimum level: V, D, I, W, E or F (default V)"},
		{"-c", "--clear", "", "Clear the device log buffer first"},
		{"-t", "--tag", "TAG", "Show only this tag, repeatable (framework tags too)"},
		{"-i", "--ignore-tag", "TAG", "Hide this tag, repeatable"},
		{"-e", "--every-tag", "", "Also show framework and driver logs"},
	},
	Examples: []string{"ak logcat", "ak logcat com.example.app -l W", "ak logcat com.example.app com.example.store",
		"ak logcat -t OkHttp -t 'MyApp*'",
		"ak logcat -i Glide", "ak logcat -e", "ak logcat -c > app.log"},
	Device: oneDevice, Run: runLogcat,
}

const levels = "VDIWEF"

var (
	// threadtime: "11-12 10:31:22.123  1234  1250 I Tag: message"
	reLogLine   = regexp.MustCompile(`^(\d\d-\d\d) (\d\d:\d\d:\d\d\.\d+)\s+(\d+)\s+(\d+) ([VDIWEFS]) (.*?)\s*: (.*)$`)
	reProcStart = regexp.MustCompile(`Start proc (\d+):([^/ ]+)`)
	reProcDied  = regexp.MustCompile(`Process (\S+) \(pid (\d+)\) has died`)
	reProcKill  = regexp.MustCompile(`Killing (\d+):([^/ ]+)/\S+ \(adj -?\d+\): (.*)$`)
)

type logLine struct {
	time, level, tag, msg string
	pid                   int
	app                   string // the app's label, with several packages
	appColor              string // its color: one of appColors, by the package's place on the command line
}

func parseLogLine(s string) (logLine, bool) {
	m := reLogLine.FindStringSubmatch(s)
	if m == nil {
		return logLine{}, false
	}
	pid, _ := strconv.Atoi(m[3])
	return logLine{time: m[2], pid: pid, level: m[5], tag: m[6], msg: m[7]}, true
}

// belongs reports whether a process name is the app or one of its processes ("pkg:remote").
func belongs(name, pkg string) bool { return name == pkg || strings.HasPrefix(name, pkg+":") }

// owner returns which of pkgs a process name belongs to, or "".
func owner(name string, pkgs []string) string {
	for _, p := range pkgs {
		if belongs(name, p) {
			return p
		}
	}
	return ""
}

// appPIDs maps the process IDs of pkgs to their package, from `ps -A -o PID,NAME`.
func appPIDs(serial string, pkgs []string) map[int]string {
	pids := map[int]string{}
	for _, l := range strings.Split(adb.Shell(serial, "ps -A -o PID,NAME"), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		if p := owner(f[1], pkgs); p != "" {
			if n, err := strconv.Atoi(f[0]); err == nil {
				pids[n] = p
			}
		}
	}
	return pids
}

// pidList lists the process IDs of pkg, or "" when it is not running.
func pidList(pids map[int]string, pkg string) string {
	var l []string
	for pid, p := range pids {
		if p == pkg {
			l = append(l, strconv.Itoa(pid))
		}
	}
	sort.Strings(l)
	return strings.Join(l, ", ")
}

// appColors tell apps apart, in the order the packages are given: hues far
// from each other and from the level colors (a hash could give two blues).
var appColors = []string{"\033[38;5;39m", "\033[38;5;205m", "\033[38;5;214m", "\033[38;5;78m", "\033[38;5;141m", "\033[38;5;51m"}

// fitAppLabels names each package for the app column. To a pipe or file
// (cols 0) it is the whole name. In a terminal the column gets a fifth of the
// width, between 12 and 16 columns (the color bar already tells the apps
// apart; the name only confirms which one): whole names when they fit,
// otherwise leading parts are dropped ("…demo.android"), never below the part
// that tells the apps apart (appLabels).
func fitAppLabels(pkgs []string, cols int) map[string]string {
	labels := map[string]string{}
	floors := appLabels(pkgs)
	budget := min(max(12, cols/5), 16)
	for _, p := range pkgs {
		labels[p] = p
		if cols == 0 || ui.TextWidth(p) <= budget {
			continue
		}
		parts := strings.Split(p, ".")
		floor := len(strings.Split(floors[p], "."))
		labels[p] = "…" + floors[p]
		for k := len(parts) - 1; k > floor; k-- {
			if l := "…" + strings.Join(parts[len(parts)-k:], "."); ui.TextWidth(l) <= budget {
				labels[p] = l
				break
			}
		}
	}
	return labels
}

// appLabels names each package by its last parts, at least two (one alone is
// often a word like "android" or "app") and as many as tell it apart from the
// others: com.onestorecorp.sdk.demo.android is "demo.android".
func appLabels(pkgs []string) map[string]string {
	labels := map[string]string{}
	for _, p := range pkgs {
		parts := strings.Split(p, ".")
		for k := min(2, len(parts)); k <= len(parts); k++ {
			label := strings.Join(parts[len(parts)-k:], ".")
			unique := true
			for _, o := range pkgs {
				if o != p && (o == label || strings.HasSuffix(o, "."+label)) {
					unique = false
				}
			}
			if unique || k == len(parts) {
				labels[p] = label
				break
			}
		}
	}
	return labels
}

// Colors follow Android Studio's Logcat (Darcula scheme), in xterm 256 colors:
// the level as a badge, the message in the level's color, and each tag in a
// color picked from its name.
var levelStyles = map[string]struct{ badge, msg string }{
	"V": {"\033[38;5;16;48;5;252m", "\033[38;5;250m"},
	"D": {"\033[38;5;231;48;5;32m", "\033[38;5;74m"}, // brighter than Studio's, which is hard to read on dark terminals
	"I": {"\033[38;5;255;48;5;65m", "\033[38;5;148m"},
	"W": {"\033[38;5;16;48;5;142m", "\033[38;5;142m"},
	"E": {"\033[38;5;16;48;5;167m", "\033[38;5;203m"},
	"F": {"\033[38;5;231;48;5;88m", "\033[38;5;203m"},
}

// tagColors is Android Studio's tag palette (logcat-tags-palette.json, dark
// foregrounds) in the same order: Gray, Light Blue, Orange, Green, Light Purple,
// Pink, Turquoise, Tan, Pacific Blue, Teal, Purple, Dark Orange, Indigo, Lime,
// Magenta, Vivid Blue.
var tagColors = []int{246, 146, 173, 148, 140, 174, 72, 221, 38, 36, 139, 137, 68, 142, 169, 69}

// tagColor picks the tag's color as Android Studio does (Java's String.hashCode
// into the palette), so a tag has the same hue in both.
func tagColor(tag string) string {
	var h int32
	for _, c := range utf16.Encode([]rune(tag)) {
		h = 31*h + int32(c)
	}
	i := int64(h)
	if i < 0 {
		i = -i
	}
	return fmt.Sprintf("\033[38;5;%dm", tagColors[i%int64(len(tagColors))])
}

// maxTagWidth caps the tag column, as Android Studio's default; longer tags
// are shortened in the middle.
const maxTagWidth = 23

// logLayout is how lines are laid out: the terminal width (0 for a pipe or
// file) and the widths of the tag and app columns.
type logLayout struct{ cols, tagW, appW int }

// formatLogLine is one output line: "time L [app] tag: msg". In a terminal
// (cols > 0) the app and tag are padded so messages line up, and a long
// message wraps with its next lines indented to where it starts. To a pipe or
// file the line is left as it is.
func formatLogLine(o *ui.Stream, l logLine, lay logLayout) string {
	cols := lay.cols
	badge := l.level
	if o.Color {
		badge = " " + l.level + " "
	}
	app, bar, tag, pad := "", "", l.tag+":", " "
	if l.app != "" {
		app = l.app + " "
		if o.Color {
			bar = "▌" // in the app's color; the name itself stays plain so it never looks like a tag
		}
	}
	if cols > 0 {
		tag = shortenMiddle(l.tag, maxTagWidth) + ":"
		pad += strings.Repeat(" ", max(0, lay.tagW+1-ui.TextWidth(tag)))
		if l.app != "" {
			app = ui.Pad(l.app, lay.appW) + " "
		}
	}
	indent := ui.TextWidth(l.time+" "+badge+" "+bar+app+tag) + len(pad)
	parts := []string{l.msg}
	if avail := cols - indent - 1; cols > 0 && avail >= 20 { // -1: some terminals wrap at the last column
		parts = wrapText(l.msg, avail)
	}
	st := levelStyles[l.level]
	msg := func(s string) string { return s }
	var b strings.Builder
	if o.Color {
		msg = func(s string) string { return o.Paint(st.msg, s) }
		b.WriteString(o.Dim(l.time) + " " + o.Paint(st.badge, badge) + " " + o.Paint(l.appColor, bar) + app +
			o.Paint(tagColor(l.tag)+"\033[1m", tag))
	} else {
		b.WriteString(l.time + " " + badge + " " + app + tag)
	}
	b.WriteString(pad + msg(parts[0]))
	for _, p := range parts[1:] {
		b.WriteString("\n" + strings.Repeat(" ", indent) + msg(p))
	}
	return b.String()
}

// shortenMiddle cuts s to cols terminal columns with "…" in the middle.
func shortenMiddle(s string, cols int) string {
	if ui.TextWidth(s) <= cols {
		return s
	}
	r := []rune(s)
	h, w := 0, 0 // head: as many runes as fit in half the room
	for ; h < len(r) && w+ui.TextWidth(string(r[h])) <= cols/2; h++ {
		w += ui.TextWidth(string(r[h]))
	}
	t, w := len(r), 0 // tail: the rest of the room after "…"
	for ; t > h && w+ui.TextWidth(string(r[t-1])) <= (cols-1)/2; t-- {
		w += ui.TextWidth(string(r[t-1]))
	}
	return string(r[:h]) + "…" + string(r[t:])
}

// wrapText cuts s into pieces of at most cols terminal columns.
func wrapText(s string, cols int) []string {
	var parts []string
	var cur strings.Builder
	used := 0
	for _, r := range s {
		w := ui.TextWidth(string(r))
		if used+w > cols && used > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
			used = 0
		}
		cur.WriteRune(r)
		used += w
	}
	return append(parts, cur.String())
}

// frameworkTags are tags that Android framework code (AOSP and device makers')
// logs from inside the app's process: useful to platform developers, noise to
// app developers. Lines below E are hidden unless -e. Crashes (AndroidRuntime,
// DEBUG, System.err) and dropped frames (Choreographer) are not listed.
var frameworkTags = []string{
	// views, windows, insets, back navigation
	"VRI[*", "ViewRootImpl*", "View", "DecorView", "PhoneWindow", "Dialog", "WindowManager",
	"WindowOnBackDispatcher", "CoreBackPreview", "InsetsController", "InsetsSourceConsumer", "SurfaceView*",
	"SurfaceControl", "DesktopModeFlags", "DesktopExperienceFlags", "DisplayManager", "ContentCaptureHelper",
	"ContentCapture", "AutofillManager", "AssistStructure", "InteractionJankMonitor", "FrameTracker",
	// input method
	"ImeTracker", "ImeFocusController", "InputMethodManager*", "IInputConnectionWrapper",
	"RemoteInputConnectionImpl", "InputEventReceiver", "InputTransport",
	// graphics
	"HWUI", "OpenGLRenderer", "HardwareRenderer", "BLASTBufferQueue*", "BufferQueueProducer",
	"SurfaceComposerClient", "GraphicsEnvironment", "vulkan", "libEGL", "EGL_emulation",
	// process start, class loading, system properties
	"Zygote", "ActivityThread", "LoadedApk", "ApplicationLoaders", "nativeloader", "ziparchive", "libc",
	"CompatChangeReporter", "CompatibilityChangeReporter", "ProfileInstaller", "NetworkSecurityConfig",
	"TrafficStats", "ResourcesManager", "studio.deploy",
}

// driverTags are GPU and vendor drivers and services: nothing an app developer
// can act on, so only F is shown unless -e.
var driverTags = []string{
	"Adreno*", "qdgralloc", "Gralloc*", "IDS_TAG", "NativeCustomFrequencyManager",
}

// tagMatch reports whether tag matches pattern, where '*' matches any text.
func tagMatch(pattern, tag string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return tag == pattern
	}
	if !strings.HasPrefix(tag, parts[0]) {
		return false
	}
	tag = tag[len(parts[0]):]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(tag, p)
		if i < 0 {
			return false
		}
		tag = tag[i+len(p):]
	}
	return strings.HasSuffix(tag, parts[len(parts)-1])
}

func matchAny(patterns []string, tag string) bool {
	for _, p := range patterns {
		if tagMatch(p, tag) {
			return true
		}
	}
	return false
}

// tagFilter decides which tags are shown (-t, -i, -e).
type tagFilter struct {
	only, ignore []string
	every        bool
	runtimeTags  []string // ART logs (GC, JIT) under the process name, cut to its last 15 characters
}

func newTagFilter(c *Ctx, pkgs []string) tagFilter {
	f := tagFilter{only: c.Strings("--tag"), ignore: c.Strings("--ignore-tag"), every: c.Bool("--every-tag")}
	for _, p := range pkgs {
		f.runtimeTags = append(f.runtimeTags, p[max(0, len(p)-15):])
	}
	return f
}

// framework reports whether the line is framework or driver noise: a framework
// tag below E or a driver tag below F.
func (f tagFilter) framework(l logLine) bool {
	if f.every || len(f.only) > 0 {
		return false
	}
	lv := strings.Index(levels, l.level)
	if matchAny(driverTags, l.tag) {
		return lv < strings.Index(levels, "F")
	}
	return lv < strings.Index(levels, "E") && (slices.Contains(f.runtimeTags, l.tag) || matchAny(frameworkTags, l.tag))
}

func (f tagFilter) show(l logLine) bool {
	if len(f.only) > 0 && !matchAny(f.only, l.tag) {
		return false
	}
	return !matchAny(f.ignore, l.tag) && !f.framework(l)
}

// tracker follows the apps' PIDs as processes start and die.
type tracker struct {
	mu   sync.Mutex
	pkgs []string
	pids map[int]string // pid -> package
}

// app returns the package a PID belongs to, or "" when it is not followed.
func (t *tracker) app(pid int) string { t.mu.Lock(); defer t.mu.Unlock(); return t.pids[pid] }

// observe updates PIDs from ActivityManager lines and returns an event to show.
func (t *tracker) observe(l logLine) string {
	if m := reProcStart.FindStringSubmatch(l.msg); m != nil && owner(m[2], t.pkgs) != "" {
		pid, _ := strconv.Atoi(m[1])
		t.mu.Lock()
		t.pids[pid] = owner(m[2], t.pkgs)
		t.mu.Unlock()
		return fmt.Sprintf("%s started (pid %d)", m[2], pid)
	}
	if m := reProcDied.FindStringSubmatch(l.msg); m != nil && owner(m[1], t.pkgs) != "" {
		return fmt.Sprintf("%s died (pid %s)", m[1], m[2])
	}
	if m := reProcKill.FindStringSubmatch(l.msg); m != nil && owner(m[2], t.pkgs) != "" {
		return fmt.Sprintf("%s killed (pid %s): %s", m[2], m[1], m[3])
	}
	return ""
}

func (t *tracker) merge(pids map[int]string) {
	t.mu.Lock()
	for pid, p := range pids {
		t.pids[pid] = p
	}
	t.mu.Unlock()
}

func runLogcat(c *Ctx) error {
	minLevel := 0
	if v := strings.ToUpper(c.String("--level")); v != "" {
		if len(v) != 1 || !strings.Contains(levels, v) {
			return usageErr(c.Cmd, "--level must be one of V, D, I, W, E, F: %s", v)
		}
		minLevel = strings.Index(levels, v)
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	var pkgs []string
	for _, p := range c.Args {
		if !validPackageName(p) {
			return failErr("", "Invalid package name: %s", q(p))
		}
		if !slices.Contains(pkgs, p) {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		pkg := foregroundPackage(serial)
		if err := requirePackage(serial, pkg); err != nil {
			return err
		}
		pkgs = []string{pkg}
	}
	if c.Bool("--clear") {
		_, _ = adb.Combined("-s", serial, "logcat", "-c")
	}
	t := &tracker{pkgs: pkgs, pids: appPIDs(serial, pkgs)}
	if len(pkgs) == 1 {
		if pids := pidList(t.pids, pkgs[0]); pids == "" {
			ui.Step("Waiting for %s to start on %s (Ctrl+C to stop)", q(pkgs[0]), deviceName(serial))
		} else {
			ui.Step("Logs of %s (pid %s) on %s (Ctrl+C to stop)", q(pkgs[0]), pids, deviceName(serial))
		}
	} else {
		ui.Step("Logs of %d apps on %s (Ctrl+C to stop)", len(pkgs), deviceName(serial))
		for _, p := range pkgs {
			state := "waiting for it to start"
			if pids := pidList(t.pids, p); pids != "" {
				state = "pid " + pids
			}
			ui.Detail("%s (%s)", q(p), state)
		}
	}

	cmd, out, err := adb.Start("-s", serial, "logcat", "-v", "threadtime")
	if err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
		case <-done:
		}
		_ = cmd.Process.Kill()
	}()
	// Refresh PIDs in case a start line was missed (e.g. the buffer was cleared).
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				t.merge(appPIDs(serial, pkgs))
			}
		}
	}()

	tags, hinted, lay := newTagFilter(c, pkgs), false, logLayout{}
	labels, labelCols := map[string]string{}, -1
	o := ui.Out
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l, ok := parseLogLine(sc.Text())
		if !ok {
			continue
		}
		if ev := t.observe(l); ev != "" {
			o.Println(o.Dim("--- " + ev))
			continue
		}
		app := t.app(l.pid)
		if app == "" || strings.Index(levels, l.level) < minLevel {
			continue
		}
		if len(pkgs) > 1 {
			l.appColor = appColors[slices.Index(pkgs, app)%len(appColors)]
		}
		if !tags.show(l) {
			if !hinted && tags.framework(l) {
				hinted = true
				ui.Hint("Framework and driver logs are hidden (errors are kept); -e shows them")
			}
			continue
		}
		if o.TTY {
			lay.cols = ui.Width() // per line, so a resized window wraps right
			// The tag column grows to the longest tag so far (up to maxTagWidth);
			// it settles within the first lines, which come from the log buffer.
			lay.tagW = max(lay.tagW, min(ui.TextWidth(l.tag), maxTagWidth))
		}
		if len(pkgs) > 1 {
			if lay.cols != labelCols { // first line, or the window was resized
				labels, labelCols, lay.appW = fitAppLabels(pkgs, lay.cols), lay.cols, 0
				for _, label := range labels {
					lay.appW = max(lay.appW, ui.TextWidth(label))
				}
			}
			l.app = labels[app]
		}
		o.Println(formatLogLine(o, l, lay))
	}
	close(done)
	_ = cmd.Wait()
	return nil
}
