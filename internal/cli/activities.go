package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdActivities = &Command{
	Name: "activities", Summary: "Show activity stack",
	Long:     "Show the activity stack of the foreground task, or of every task with --all-tasks.",
	Flags:    []Flag{{"-a", "--all-tasks", "", "Show every task"}},
	Examples: []string{"ak activities", "ak activities -a", "ak activities --json"},
	Device:   oneDevice, JSON: true, Run: runActivities,
}

var (
	// Ported from sed expressions; groups after "}" keep any trailing text like sed does.
	reTaskID   = regexp.MustCompile(`^.* t([0-9]*)}(.*)$`)
	reHistNum  = regexp.MustCompile(`^.*Hist.*#([0-9]*)`)
	reActivity = regexp.MustCompile(`^.* u0 (.*) t[0-9]*}(.*)$`)
)

func histLines(dumpsys string) []string {
	var lines []string
	for _, l := range strings.Split(dumpsys, "\n") {
		if strings.Contains(strings.ToLower(l), "hist") {
			lines = append(lines, l)
		}
	}
	return lines
}

func taskID(line string) string {
	if m := reTaskID.FindStringSubmatch(line); m != nil {
		return m[1] + m[2]
	}
	return ""
}

func histNumber(line string) (int, bool) {
	m := reHistNum.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// linesForTask keeps lines of task N, matching " tN}" exactly.
func linesForTask(lines []string, task string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, " t"+task+"}") {
			out = append(out, l)
		}
	}
	return out
}

type activityJSON struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

type taskJSON struct {
	Task       string         `json:"task"`
	Activities []activityJSON `json:"activities"`
}

func parseTask(lines []string, task string) taskJSON {
	t := taskJSON{Task: task, Activities: []activityJSON{}}
	for _, l := range lines {
		m := reActivity.FindStringSubmatch(l)
		if m == nil || m[1]+m[2] == "" {
			continue
		}
		n, _ := histNumber(l)
		t.Activities = append(t.Activities, activityJSON{n, m[1] + m[2]})
	}
	return t
}

func printTask(t taskJSON) {
	ui.Out.Println(ui.Out.Bold("Task #" + t.Task))
	top := -1
	for _, a := range t.Activities {
		top = max(top, a.Index)
	}
	for _, a := range t.Activities {
		mark := ""
		switch a.Index {
		case top:
			mark = ui.Out.Dim("  (top)")
		case 0:
			mark = ui.Out.Dim("  (root)")
		}
		ui.Out.Printf("  #%-3d %s%s\n", a.Index, a.Name, mark)
	}
}

func runActivities(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	dumpsys := adb.Shell(serial, "dumpsys activity activities")
	foreground := ""
	for _, l := range strings.Split(dumpsys, "\n") {
		if id := taskID(l); id != "" {
			foreground = id
			break
		}
	}
	if foreground == "" {
		return failErr("", "No foreground activity found on %s", deviceName(serial))
	}
	hist := histLines(dumpsys)
	tasks := []string{foreground}
	if c.Bool("--all-tasks") {
		tasks = nil
		seen := map[string]bool{}
		for _, l := range hist {
			if id := taskID(l); id != "" && !seen[id] {
				seen[id] = true
				tasks = append(tasks, id)
			}
		}
	}
	var result []taskJSON
	for _, id := range tasks {
		if t := parseTask(linesForTask(hist, id), id); len(t.Activities) > 0 {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		return failErr("", "No activities found on %s", deviceName(serial))
	}
	if c.JSON() {
		return ui.JSON(result)
	}
	for i, t := range result {
		if i > 0 {
			ui.Out.Println("")
		}
		printTask(t)
	}
	return nil
}
