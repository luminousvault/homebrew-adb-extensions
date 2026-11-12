package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

var cmdActivities = &Command{
	Name: "activities", Desc: "Show activity stack",
	Usage: "activities [--all]", Help: "Show activity stack",
	Flags: []string{"-h", "--help", "-a", "--all"}, Run: runActivities,
}

func showHelpActivities() {
	say("%s%sUsage:%s ak activities [-h|--help] [-a|--all]", Cyan, Bold, NC)
	say("")
	say("Description: Display the activity stack of running applications.")
	say("")
	say("Options:")
	say("  -h, --help  Show this help message")
	say("  (none)      Show foreground task's activity stack (default)")
	say("  -a, --all   Show all tasks' activity stacks")
	say("")
}

var (
	// Ported from sed expressions; groups after "}" keep any trailing text like sed does.
	reTaskID   = regexp.MustCompile(`^.* t([0-9]*)}(.*)$`)
	reHistNum  = regexp.MustCompile(`^.*Hist.*#([0-9]*)`)
	reActivity = regexp.MustCompile(`^.* u0 (.*) t[0-9]*}(.*)$`)
)

// histLines returns `dumpsys activity activities` lines containing "hist" (any case).
func histLines(dumpsys string) []string {
	var lines []string
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.Contains(strings.ToLower(l), "hist") {
			lines = append(lines, l)
		}
	}
	return lines
}

// taskID extracts N from a line ending in " tN}".
func taskID(line string) string {
	if m := reTaskID.FindStringSubmatch(line); m != nil {
		return m[1] + m[2]
	}
	return ""
}

// histNumber extracts N from "Hist ... #N".
func histNumber(line string) (int, bool) {
	m := reHistNum.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

func printActivityStack(lines []string, task string) {
	say("%s==> Activity Stack (Task %s#%s%s)", Bold, Cyan, task, NC)
	maxHist := 0
	for _, l := range lines {
		if n, ok := histNumber(l); ok && n > maxHist {
			maxHist = n
		}
	}
	count := 0
	for _, l := range lines {
		m := reActivity.FindStringSubmatch(l)
		if m == nil || m[1]+m[2] == "" {
			continue
		}
		count++
		n, _ := histNumber(l)
		color := Yellow
		switch n {
		case maxHist:
			color = Cyan
		case 0:
			color = Green
		}
		say("  %s[#%d]%s %s", color, n, NC, m[1]+m[2])
	}
	say("%sTotal: %d activities in this task%s", Dim, count, NC)
	say("")
}

// linesForTask keeps lines of task N. Matches " tN}" (with the leading space)
// so task 2 does not also pick up task 12, unlike the Bash grep.
func linesForTask(lines []string, task string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, " t"+task+"}") {
			out = append(out, l)
		}
	}
	return out
}

func runActivities(args []string) {
	all := false
	parseLeading("activities", args, showHelpActivities, map[string]*bool{"-a": &all, "--all": &all}, false)
	serial := selectDevice()

	say("")
	say("%s==> %sFetching activity stack...%s", Blue, Bold, NC)
	say("")
	dumpsys := adb.Shell(serial, "dumpsys activity activities")

	foreground := ""
	for _, l := range strings.Split(dumpsys, "\n") {
		if id := taskID(l); id != "" {
			foreground = id
			break
		}
	}
	if foreground == "" {
		say("%s No foreground activity found.", Error)
		exit(1)
	}
	hist := histLines(dumpsys)
	selected := hist
	if !all {
		selected = linesForTask(hist, foreground)
	}
	if len(selected) == 0 {
		say("%s No activities found.", Error)
		exit(1)
	}
	if !all {
		printActivityStack(selected, foreground)
		return
	}
	var tasks []string
	seen := map[string]bool{}
	for _, l := range hist {
		if id := taskID(l); id != "" && !seen[id] {
			seen[id] = true
			tasks = append(tasks, id)
		}
	}
	if len(tasks) == 0 {
		say("%s No tasks found.", Error)
		exit(1)
	}
	for _, t := range tasks {
		if ls := linesForTask(hist, t); len(ls) > 0 {
			printActivityStack(ls, t)
		}
	}
}
