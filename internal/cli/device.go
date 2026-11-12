package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// selectFn is the picker; tests replace it.
var selectFn = tui.Select

// canPick reports whether the interactive picker can be shown.
func canPick() bool { return ui.InTTY && ui.Err.TTY }

// pick runs the picker. Without a terminal it fails with a hint.
func pick(o tui.Options, hint string) (tui.Result, error) {
	if !canPick() {
		return tui.Result{Index: -1}, failErr(hint, "%s: interactive selection needs a terminal", o.Prompt)
	}
	return selectFn(o)
}

var (
	propsMu    sync.Mutex
	propsCache = map[string]map[string]string{}
)

// deviceProps returns cached getprop values (nil if the device does not respond).
func deviceProps(serial string) map[string]string {
	propsMu.Lock()
	defer propsMu.Unlock()
	if p, ok := propsCache[serial]; ok {
		return p
	}
	var p map[string]string
	if out := adb.Shell(serial, "getprop"); out != "" {
		p = adb.ParseGetprop(out)
	}
	propsCache[serial] = p
	return p
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// deviceName returns "Brand Model" (e.g. "Google Pixel 9 Pro").
func deviceName(serial string) string {
	p := deviceProps(serial)
	if p == nil {
		return serial
	}
	return orDefault(capitalize(p["ro.product.brand"]), "Unknown") + " " + orDefault(p["ro.product.model"], "Device")
}

// deviceLabel is the picker label: "Brand Model (serial) Android X, API N".
func deviceLabel(serial string) string {
	p := deviceProps(serial)
	if p == nil {
		return serial + " (not responding)"
	}
	return fmt.Sprintf("%s (%s) Android %s, API %s", deviceName(serial), serial,
		orDefault(p["ro.build.version.release"], "?"), orDefault(p["ro.build.version.sdk"], "?"))
}

const hintConnect = "Connect a device with USB debugging enabled, then run 'ak devices'"

// checkSerial verifies that serial is connected and ready.
func checkSerial(serial string) error {
	for _, d := range adb.ListDevices() {
		if d.Serial != serial {
			continue
		}
		switch d.State {
		case "device":
			return nil
		case "unauthorized":
			return failErr("Accept the USB debugging prompt on the device", "Device %s is unauthorized", serial)
		default:
			return failErr("Reconnect the device and run 'ak devices'", "Device %s is %s", serial, d.State)
		}
	}
	return failErr("Run 'ak devices' to see connected devices", "Device not found: %s", serial)
}

// Device resolves the single target: -s, ANDROID_SERIAL, the only device, or the picker.
func (c *Ctx) Device() (string, error) {
	serials := c.Strings("--serial")
	if len(serials) > 1 {
		return "", usageErr(c.Cmd, "This command works on one device; got %d (-s %s)", len(serials), strings.Join(serials, ", "))
	}
	if len(serials) == 0 {
		if env := os.Getenv("ANDROID_SERIAL"); env != "" {
			serials = []string{env}
		}
	}
	if len(serials) == 1 {
		return serials[0], checkSerial(serials[0])
	}
	ready := adb.ReadySerials()
	switch len(ready) {
	case 0:
		return "", failErr(hintConnect, "No devices connected")
	case 1:
		return ready[0], nil
	}
	if !canPick() {
		return "", failErr("Use -s <serial> or set ANDROID_SERIAL", "Multiple devices connected: %s", strings.Join(ready, ", "))
	}
	items := make([]string, len(ready))
	for i, s := range ready {
		items[i] = deviceLabel(s)
	}
	r, err := pick(tui.Options{NoFilter: true, Prompt: "Select a device", Items: items}, "")
	if err != nil {
		return "", err
	}
	return ready[r.Index], nil
}

// Devices resolves targets for multi-device commands: -s (repeatable), --all,
// ANDROID_SERIAL, the only device, or the picker.
func (c *Ctx) Devices() ([]string, error) {
	serials := c.Strings("--serial")
	if len(serials) == 0 && !c.Bool("--all") {
		if env := os.Getenv("ANDROID_SERIAL"); env != "" {
			serials = []string{env}
		}
	}
	if len(serials) > 0 {
		for _, s := range serials {
			if err := checkSerial(s); err != nil {
				return nil, err
			}
		}
		return serials, nil
	}
	ready := adb.ReadySerials()
	switch {
	case len(ready) == 0:
		return nil, failErr(hintConnect, "No devices connected")
	case len(ready) == 1 || c.Bool("--all"):
		return ready, nil
	}
	if !canPick() {
		return nil, failErr("Use -s <serial>, --all, or set ANDROID_SERIAL", "Multiple devices connected: %s", strings.Join(ready, ", "))
	}
	items := make([]string, len(ready))
	for i, s := range ready {
		items[i] = deviceLabel(s)
	}
	r, err := pick(tui.Options{Multi: true, NoFilter: true, Prompt: "Select devices", Items: items}, "")
	if err != nil {
		return nil, err
	}
	var picked []string
	for _, i := range r.Indices {
		picked = append(picked, ready[i])
	}
	return picked, nil
}

// resetDeviceCache is for tests.
func resetDeviceCache() {
	propsMu.Lock()
	propsCache = map[string]map[string]string{}
	propsMu.Unlock()
}
