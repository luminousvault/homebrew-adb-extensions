package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdRestart = &Command{
	Name: "restart", Summary: "Restart an app and time its launch", Args: "[package]",
	Long: "Force-stop an app, start its launcher activity again and show how long the launch took\n" +
		"(am start -W: TotalTime). With --repeat, restart several times and show min/avg/max.",
	ArgHelp:  [][2]string{argPackage},
	Flags:    []Flag{{"-n", "--repeat", "N", "Restart N times (default 1)"}},
	Examples: []string{"ak restart", "ak restart com.example.app -n 5", "ak restart --json"},
	Device:   oneDevice, JSON: true, Run: runRestart,
}

var (
	reTotalTime   = regexp.MustCompile(`TotalTime: (\d+)`)
	reWaitTime    = regexp.MustCompile(`WaitTime: (\d+)`)
	reLaunchState = regexp.MustCompile(`LaunchState: (\w+)`)
)

type launchResult struct {
	LaunchState string `json:"launchState,omitempty"`
	TotalTime   int    `json:"totalTimeMs"`
	WaitTime    int    `json:"waitTimeMs"`
}

func parseLaunch(out string) (launchResult, bool) {
	var r launchResult
	m := reTotalTime.FindStringSubmatch(out)
	if m == nil {
		return r, false
	}
	r.TotalTime, _ = strconv.Atoi(m[1])
	if m := reWaitTime.FindStringSubmatch(out); m != nil {
		r.WaitTime, _ = strconv.Atoi(m[1])
	}
	if m := reLaunchState.FindStringSubmatch(out); m != nil {
		r.LaunchState = strings.ToLower(m[1])
	}
	return r, true
}

func runRestart(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	repeat := 1
	if v := c.String("--repeat"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return usageErr(c.Cmd, "--repeat must be 1-100: %s", v)
		}
		repeat = n
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	t, err := resolvePackage(serial, c.Args)
	if err != nil {
		return err
	}
	resolved, _ := adb.Combined("-s", serial, "shell", "cmd", "package", "resolve-activity", "--brief", t.pkg)
	component := lastLine(resolved)
	if !strings.Contains(component, "/") {
		return failErr("", "%s has no launcher activity", t.pkg)
	}

	var runs []launchResult
	for i := 0; i < repeat; i++ {
		sp := ui.Spin("Restarting %s (%d/%d)", t.describe(serial), i+1, repeat)
		_, _ = adb.Combined("-s", serial, "shell", "am", "force-stop", t.pkg)
		time.Sleep(300 * time.Millisecond)
		out, _ := adb.Combined("-s", serial, "shell", "am", "start", "-W", "--user", "0", "-n", component)
		sp.Stop()
		r, ok := parseLaunch(out)
		if !ok || strings.Contains(out, "Error") {
			ui.Errorf("Couldn't start %s", component)
			ui.Detail("%s", out)
			return errReported
		}
		runs = append(runs, r)
		if i < repeat-1 {
			time.Sleep(time.Second)
		}
	}

	lo, hi, sum := runs[0].TotalTime, runs[0].TotalTime, 0
	for _, r := range runs {
		lo, hi, sum = min(lo, r.TotalTime), max(hi, r.TotalTime), sum+r.TotalTime
	}
	avg := sum / len(runs)
	if c.JSON() {
		return ui.JSON(map[string]any{"package": t.pkg, "activity": component, "runs": runs,
			"minMs": lo, "avgMs": avg, "maxMs": hi})
	}
	ui.Success("Restarted %s", t.describe(serial))
	if repeat == 1 {
		ui.KeyValues([][2]string{
			{"activity", component},
			{"launchState", orDefault(runs[0].LaunchState, "-")},
			{"totalTime", fmt.Sprintf("%d ms", runs[0].TotalTime)},
			{"waitTime", fmt.Sprintf("%d ms", runs[0].WaitTime)},
		})
		return nil
	}
	var rows [][]ui.Cell
	for i, r := range runs {
		rows = append(rows, []ui.Cell{{Text: strconv.Itoa(i + 1)}, {Text: orDefault(r.LaunchState, "-")},
			{Text: fmt.Sprintf("%d ms", r.TotalTime)}, {Text: fmt.Sprintf("%d ms", r.WaitTime)}})
	}
	ui.Table([]string{"RUN", "LAUNCH", "TOTAL", "WAIT"}, rows)
	ui.Out.Println("")
	ui.KeyValues([][2]string{{"min", fmt.Sprintf("%d ms", lo)}, {"avg", fmt.Sprintf("%d ms", avg)}, {"max", fmt.Sprintf("%d ms", hi)}})
	return nil
}
