package cli

import (
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

var cmdDevices = &Command{
	Name: "devices", Desc: "Show connected devices",
	Usage: "devices", Help: "List connected devices",
	Flags: helpFlags, Run: runDevices,
}

var showHelpDevices = helpWith("ak devices [-h|--help]",
	"Description: Show connected device list with model and Android version.")

type deviceRow struct {
	id, model, android, cpu, status string
}

// cpuLabel appends 64/32-bit support from the ABI lists (Android 5.0+).
func cpuLabel(p map[string]string) string {
	cpu := orDefault(p["ro.product.cpu.abi"], "Unknown")
	has32, has64 := p["ro.product.cpu.abilist32"] != "", p["ro.product.cpu.abilist64"] != ""
	switch {
	case has64 && has32:
		return cpu + " (64/32)"
	case has64:
		return cpu + " (64only)"
	case has32:
		return cpu + " (32only)"
	}
	return cpu
}

func deviceRowFor(d adb.DeviceEntry) deviceRow {
	row := deviceRow{id: d.Serial, model: "-", android: "-", cpu: "-"}
	switch d.State {
	case "device":
		row.status = "Connected"
		out := adb.Shell(d.Serial, "getprop")
		if out == "" {
			row.model, row.android, row.cpu = "Unknown Device", "Unknown (API Unknown)", "Unknown"
			return row
		}
		p := adb.ParseGetprop(out)
		brand := orDefault(capitalize(p["ro.product.brand"]), "Unknown")
		row.model = brand + " " + orDefault(p["ro.product.model"], "Device")
		row.android = orDefault(p["ro.build.version.release"], "Unknown") + " (API " + orDefault(p["ro.build.version.sdk"], "Unknown") + ")"
		row.cpu = cpuLabel(p)
	case "unauthorized":
		row.status = "Unauthorized"
	case "offline":
		row.status = "Offline"
	default:
		row.status = "Unknown"
	}
	return row
}

func runDevices(args []string) {
	parseLeading("devices", args, showHelpDevices, nil, false)
	entries := adb.ListDevices()
	if len(entries) == 0 {
		say("%sERROR: No connected devices found.%s", Red, NC)
		return
	}
	idW, modelW, androidW, cpuW, statusW := 15, 20, 12, 10, 11
	rows := make([]deviceRow, len(entries))
	for i, e := range entries {
		r := deviceRowFor(e)
		rows[i] = r
		idW = max(idW, width(r.id))
		if e.State == "device" {
			modelW, androidW, cpuW = max(modelW, width(r.model)), max(androidW, width(r.android)), max(cpuW, width(r.cpu))
		}
	}

	say("")
	say("%s%sConnected Devices:%s", Cyan, Bold, NC)
	say("")
	head := func(label string, w int) string { return Dim + Bold + label + NC + pad(w-width(label)) }
	say("%s  %s  %s  %s  %s", head("Device ID", idW), head("Model", modelW), head("Android", androidW), head("CPU", cpuW), Dim+Bold+"Status"+NC)
	say("%s%s%s", Dim, strings.Repeat("─", idW+modelW+androidW+cpuW+statusW+8), NC)
	for _, r := range rows {
		cpu := r.cpu
		if strings.HasSuffix(cpu, "(64only)") {
			cpu = Yellow + cpu + NC
		}
		status := r.status
		switch status {
		case "Unauthorized":
			status = Red + status + NC
		case "Offline", "Unknown":
			status = Dim + status + NC
		}
		say("%s%s  %s%s  %s%s  %s%s  %s%s",
			Bold+r.id+NC, pad(idW-width(r.id)),
			r.model, pad(modelW-width(r.model)),
			r.android, pad(androidW-width(r.android)),
			cpu, pad(cpuW-width(r.cpu)),
			status, pad(statusW-width(r.status)))
	}
	say("")
}
