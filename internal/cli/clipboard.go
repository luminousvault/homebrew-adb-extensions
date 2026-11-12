package cli

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var flagCopy = Flag{"-c", "--copy", "", "Copy the image to the clipboard (alone: no file is saved)"}

// copyImageFn puts a PNG file on the system clipboard as an image; tests
// replace it.
var copyImageFn = copyImage

func copyImage(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// The path goes in as an argument, so quotes in it need no escaping.
		cmd = exec.Command("osascript",
			"-e", "on run argv",
			"-e", "set the clipboard to (read (POSIX file (item 1 of argv)) as «class PNGf»)",
			"-e", "end run", path)
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-STA", "-Command",
			"Add-Type -AssemblyName System.Windows.Forms, System.Drawing; "+
				"$img = [System.Drawing.Image]::FromFile($env:AK_CLIPBOARD_FILE); "+
				"[System.Windows.Forms.Clipboard]::SetImage($img); $img.Dispose()")
		cmd.Env = append(os.Environ(), "AK_CLIPBOARD_FILE="+path)
	default:
		tool, args := linuxClipboard()
		if tool == "" {
			return errors.New("no clipboard tool found (install wl-clipboard or xclip)")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		cmd = exec.Command(tool, args...)
		cmd.Stdin = f
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// linuxClipboard picks wl-copy on Wayland, else xclip.
func linuxClipboard() (string, []string) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", []string{"--type", "image/png"}
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}
	}
	return "", nil
}
