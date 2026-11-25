package cli

import (
	"errors"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

var cmdFiles = &Command{
	Name: "files", Summary: "Browse device and computer folders and push, pull or delete files", Args: "[path]",
	Long: "A file manager for the device and this computer, with number keys like mc's F keys. Starts\n" +
		"with the device and the computer side by side (80 columns or more); 9 hides the computer\n" +
		"pane, and then the list runs in two columns and Tab switches between device and computer.\n" +
		"5 copies the selection to the other side at once: on the device it pulls to the computer\n" +
		"side's folder, on the computer it pushes to the device side's folder. 3 copies or 4 cuts\n" +
		"items into a clipboard that stays while you change folders; 6 pastes them where the cursor\n" +
		"is (a cut moves). Enter opens a folder or a file (device files are copied here first).\n" +
		"Press ? for every key.\n" +
		"Needs a terminal; without one, use ak pull and ak push.",
	ArgHelp:  [][2]string{{"path", "Device folder to start in (default: /sdcard)"}},
	Examples: []string{"ak files", "ak files /sdcard/DCIM/Camera"},
	Device:   oneDevice, Run: runFiles,
}

func runFiles(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	start := ""
	if len(c.Args) == 1 {
		start = c.Args[0]
		if !strings.HasPrefix(start, "/") {
			return usageErr(c.Cmd, "%s is not an absolute device path, e.g. /sdcard/DCIM", q(start))
		}
	}
	if !canPick() {
		return failErr("Run 'ak pull' or 'ak push' instead", "ak files needs a terminal")
	}
	release := holdPickers() // device picker, then the file manager, on one screen
	defer release()
	serial, err := c.Device()
	if err != nil {
		return err
	}
	tui.ClearHeld()
	sum, err := files.Run(files.Options{Serial: serial, DeviceName: deviceName(serial),
		DevicePath: start, OpenLocal: openFn})
	release()
	sum.Print()
	if errors.Is(err, tui.ErrNoTerminal) {
		return failErr("Run 'ak pull' or 'ak push' instead", "ak files needs a terminal")
	}
	if err != nil {
		return err
	}
	if sum.HasFailures() {
		return errReported
	}
	return nil
}
