package cli

import (
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Hidden: not in the global help or the zsh completion.
var cmdVerifier = &Command{
	Name: "verifier", Summary: "Show or turn Play Protect scans of adb installs on or off", Args: "[on|off]",
	Long: "Show or change whether Google Play Protect scans apps installed with adb\n" +
		"(settings global " + verifierKey + "). The setting stays after a reboot.\n" +
		"'off' sets it to 0; 'on' deletes it, which restores the default (scans on).",
	ArgHelp:  [][2]string{{"on", "Scan adb installs again (default)"}, {"off", "Skip scans of adb installs"}},
	Examples: []string{"ak verifier", "ak verifier off --all", "ak verifier on -s emulator-5554"},
	Device:   manyDevices, Hidden: true, Run: runVerifier,
}

const verifierKey = "verifier_verify_adb_installs"

func runVerifier(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	action := ""
	if len(c.Args) == 1 {
		action = c.Args[0]
		if action != "on" && action != "off" {
			return usageErr(c.Cmd, "Expected 'on' or 'off': %s", action)
		}
	}
	serials, err := c.Devices()
	if err != nil {
		return err
	}
	failed := false
	var rows [][]ui.Cell
	for _, s := range serials {
		var out string
		switch action {
		case "off":
			out, err = adb.Combined("-s", s, "shell", "settings", "put", "global", verifierKey, "0")
		case "on":
			out, err = adb.Combined("-s", s, "shell", "settings", "delete", "global", verifierKey)
		}
		if err != nil {
			ui.Errorf("Couldn't change %s on %s", q(verifierKey), deviceName(s))
			ui.Detail("%s", strings.TrimSpace(out))
			failed = true
			continue
		}
		val := strings.TrimSpace(adb.Shell(s, "settings get global "+verifierKey))
		state := ui.Cell{Text: "on"}
		if val == "0" {
			state = ui.Cell{Text: "off", Color: ui.Out.Yellow}
		}
		rows = append(rows, []ui.Cell{{Text: deviceName(s)}, state, {Text: val, Color: ui.Out.Dim}})
	}
	if len(rows) > 0 {
		ui.Table([]string{"DEVICE", "PLAY_PROTECT_SCAN", "VALUE"}, rows)
	}
	if failed {
		return errReported
	}
	return nil
}
