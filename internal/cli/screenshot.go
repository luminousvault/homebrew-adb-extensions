package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdScreenshot = &Command{
	Name: "screenshot", Summary: "Save a screenshot", Args: "[file|directory]",
	Long: "Capture the screen and save it as PNG on this computer (nothing is left on the device).\n" +
		"The default name is screenshot-<model>-<time>.png in the current directory.\n" +
		"With several devices, one file per device is saved. Saved paths are printed to stdout.",
	ArgHelp:  [][2]string{{"file", "Output .png file (one device)"}, {"directory", "Save into this directory"}},
	Flags:    []Flag{flagOpen},
	Examples: []string{"ak screenshot", "ak screenshot -o", "ak screenshot home.png", "ak screenshot ~/Desktop --all"},
	Device:   manyDevices, Run: runScreenshot,
}

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileStem turns a device name into a file-name-safe stem ("Google Pixel 9 Pro" -> "Pixel-9-Pro").
func fileStem(serial string) string {
	name := deviceName(serial)
	if p := deviceProps(serial); p != nil && p["ro.product.model"] != "" {
		name = p["ro.product.model"]
	}
	return strings.Trim(reUnsafe.ReplaceAllString(name, "-"), "-")
}

// outputPath decides where a capture goes: an explicit file (one device),
// a directory, or the current directory with a generated name.
func outputPath(arg, serial, prefix, ext string, now time.Time, many bool) (string, error) {
	name := fmt.Sprintf("%s-%s-%s%s", prefix, fileStem(serial), now.Format("20060102-150405"), ext)
	if arg == "" {
		return name, nil
	}
	if st, err := os.Stat(arg); err == nil && st.IsDir() {
		return filepath.Join(arg, name), nil
	}
	if many {
		return "", failErr("Pass a directory to save one file per device", "%s is not a directory", q(arg))
	}
	if filepath.Ext(arg) == "" {
		arg += ext
	}
	return arg, nil
}

func runScreenshot(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	serials, err := c.Devices()
	if err != nil {
		return err
	}
	arg := ""
	if len(c.Args) == 1 {
		arg = c.Args[0]
	}
	now := time.Now()
	failed := false
	for _, s := range serials {
		path, err := outputPath(arg, s, "screenshot", ".png", now, len(serials) > 1)
		if err != nil {
			return err
		}
		sp := ui.Spin("Capturing %s", deviceName(s))
		data, err := adb.Output("-s", s, "exec-out", "screencap", "-p")
		sp.Stop()
		if err != nil || !bytes.HasPrefix(data, pngMagic) {
			ui.Errorf("Couldn't capture the screen of %s", deviceName(s))
			if len(data) > 0 && !bytes.HasPrefix(data, pngMagic) {
				ui.Detail("%s", strings.TrimSpace(string(data[:min(len(data), 300)])))
			}
			failed = true
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			ui.Errorf("Couldn't save %s: %v", q(path), err)
			failed = true
			continue
		}
		ui.Success("Saved %s %s", q(path), ui.Err.Dim("("+deviceName(s)+", "+humanSize(strconv.Itoa(len(data)))+")"))
		ui.Out.Println(path)
		openSaved(c, path)
	}
	if failed {
		return errReported
	}
	return nil
}
