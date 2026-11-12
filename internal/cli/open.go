package cli

import (
	"os/exec"
	"runtime"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var flagOpen = Flag{"-o", "--open", "", "Open the saved file with the default app"}

// openFn opens a file with the system's default app; tests replace it.
var openFn = func(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// openSaved opens a saved capture when --open was given. The file is already
// saved, so failing to open it is only a warning.
func openSaved(c *Ctx, path string) {
	if !c.Bool("--open") {
		return
	}
	if err := openFn(path); err != nil {
		ui.Warn("Couldn't open %s: %v", q(path), err)
	}
}
