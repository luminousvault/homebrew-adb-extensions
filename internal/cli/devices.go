package cli

import (
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdDevices = &Command{
	Name: "devices", Summary: "Show connected devices",
	Long:     "List connected devices with model, Android version, CPU ABI and state.\nABI shows 32/64-bit app support: (64/32), (64only) or (32only).",
	Examples: []string{"ak devices", "ak devices --json"},
	JSON:     true, Run: runDevices,
}

// abiBits returns "64/32", "64only", "32only" or "" from the ABI lists (Android 5.0+).
func abiBits(p map[string]string) string {
	has32, has64 := p["ro.product.cpu.abilist32"] != "", p["ro.product.cpu.abilist64"] != ""
	switch {
	case has64 && has32:
		return "64/32"
	case has64:
		return "64only"
	case has32:
		return "32only"
	}
	return ""
}

type deviceInfo struct {
	Serial  string `json:"serial"`
	State   string `json:"state"`
	Brand   string `json:"brand,omitempty"`
	Model   string `json:"model,omitempty"`
	Android string `json:"android,omitempty"`
	API     any    `json:"api,omitempty"`
	ABI     string `json:"abi,omitempty"`
	Bits    string `json:"bits,omitempty"`
}

func collectDevice(d adb.DeviceEntry) deviceInfo {
	info := deviceInfo{Serial: d.Serial, State: d.State}
	if d.State != "device" {
		return info
	}
	p := deviceProps(d.Serial)
	if p == nil {
		info.State = "not responding"
		return info
	}
	info.Brand = capitalize(p["ro.product.brand"])
	info.Model = p["ro.product.model"]
	info.Android = p["ro.build.version.release"]
	info.API = intOrNil(p["ro.build.version.sdk"])
	info.ABI = p["ro.product.cpu.abi"]
	info.Bits = abiBits(p)
	return info
}

func stateLabel(state string) (string, func(string) string) {
	switch state {
	case "device":
		return "connected", ui.Out.Green
	case "unauthorized":
		return state, ui.Out.Red
	}
	return state, ui.Out.Dim
}

func runDevices(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	entries := adb.ListDevices()
	infos := make([]deviceInfo, len(entries))
	for i, e := range entries {
		infos[i] = collectDevice(e)
	}
	if c.JSON() {
		return ui.JSON(infos)
	}
	if len(infos) == 0 {
		ui.Info("No devices connected")
		return nil
	}
	var rows [][]ui.Cell
	for _, d := range infos {
		model, android, abi := "-", "-", "-"
		var abiColor func(string) string
		if d.State == "device" {
			model = strings.TrimSpace(d.Brand + " " + d.Model)
			android = d.Android
			if d.API != nil {
				android += " (API " + strconv.FormatInt(d.API.(int64), 10) + ")"
			}
			abi = d.ABI
			if d.Bits != "" {
				abi += " (" + d.Bits + ")"
			}
			if d.Bits == "64only" {
				abiColor = ui.Out.Yellow // 32-bit apps cannot be installed
			}
		}
		state, stateColor := stateLabel(d.State)
		rows = append(rows, []ui.Cell{{Text: d.Serial}, {Text: model}, {Text: android}, {Text: abi, Color: abiColor}, {Text: state, Color: stateColor}})
	}
	ui.Table([]string{"SERIAL", "MODEL", "ANDROID", "ABI", "STATE"}, rows)
	return nil
}
