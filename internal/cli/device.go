package cli

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

// selectFn is the picker; tests replace it.
var selectFn = tui.Select

// pick runs the picker and exits on Ctrl+C (130) or when no terminal is available.
func pick(o tui.Options) tui.Result {
	r, err := selectFn(o)
	switch {
	case errors.Is(err, tui.ErrInterrupted):
		exit(130)
	case err != nil:
		say("%s %v", Error, err)
		exit(1)
	}
	return r
}

// capitalize upper-cases the first letter and lower-cases the rest.
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

// prettyDevice formats a device for display. level: "minimal", "short" or "normal".
func prettyDevice(serial, level string) string {
	for _, d := range adb.ListDevices() {
		if d.Serial == serial && (d.State == "unauthorized" || d.State == "offline") {
			return "(" + serial + ")"
		}
	}
	out := adb.Shell(serial, "getprop")
	if out == "" {
		if level == "minimal" {
			return "Unknown Device"
		}
		return fmt.Sprintf("Unknown Device (%s): device not responding", serial)
	}
	p := adb.ParseGetprop(out)
	brand := orDefault(capitalize(p["ro.product.brand"]), "Unknown")
	model := orDefault(p["ro.product.model"], "Device")
	switch level {
	case "minimal":
		return brand + " " + model
	case "short":
		return fmt.Sprintf("%s %s (%s)", brand, model, serial)
	}
	return fmt.Sprintf("%s %s (%s) Android %s, API %s", brand, model, serial,
		orDefault(p["ro.build.version.release"], "Unknown"), orDefault(p["ro.build.version.sdk"], "Unknown"))
}

// selectDevice returns the single target device, asking when several are connected.
func selectDevice() string {
	serials := adb.ReadySerials()
	switch len(serials) {
	case 0:
		say("%s No connected devices found.", Error)
		exit(1)
	case 1:
		return serials[0]
	}
	items := make([]string, len(serials))
	for i, s := range serials {
		items[i] = prettyDevice(s, "normal")
	}
	r := pick(tui.Options{NoFilter: true, Prompt: "Select a device", Items: items})
	return serials[r.Index]
}

// selectDevicesMulti returns target devices for install: all with -m, otherwise asks.
func selectDevicesMulti(all bool) []string {
	serials := adb.ReadySerials()
	switch {
	case len(serials) == 0:
		say("%s No connected devices found.", Error)
		exit(1)
	case len(serials) == 1 || all:
		return serials
	}
	items := make([]string, len(serials))
	for i, s := range serials {
		items[i] = prettyDevice(s, "normal")
	}
	r := pick(tui.Options{Multi: true, NoFilter: true, Prompt: "Select devices for installation", Items: items})
	var picked []string
	for _, i := range r.Indices {
		picked = append(picked, serials[i])
	}
	return picked
}

// width counts runes, matching ${#var} in a UTF-8 Bash locale.
func width(s string) int { return utf8.RuneCountInString(s) }

func pad(n int) string {
	if n < 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
