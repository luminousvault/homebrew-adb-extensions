package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdMac = &Command{
	Name: "mac", Summary: "Show device MAC addresses",
	Long: "Show the MAC addresses of a device.\n" +
		"  Wi-Fi (current)  MAC of the current connection (per network, may be randomized)\n" +
		"  Wi-Fi (factory)  The device's own Wi-Fi MAC, if exposed (also when Wi-Fi is off)\n" +
		"  Bluetooth        Bluetooth MAC",
	Examples: []string{"ak mac", "ak mac --all", "ak mac --json"},
	Device:   manyDevices, JSON: true, Run: runMac,
}

var (
	reMAC  = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)
	reSSID = regexp.MustCompile(`(^|[ ,])SSID: ("[^"]*"|[^,]*)`)
)

// normalizeMAC lowercases the first line and rejects malformed or placeholder MACs.
func normalizeMAC(s string) (string, bool) {
	line, _, _ := strings.Cut(s, "\n")
	mac := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(line, "\r", "")))
	if !reMAC.MatchString(mac) || mac == "02:00:00:00:00:00" || mac == "00:00:00:00:00:00" {
		return "", false
	}
	return mac, true
}

func grepF(out, filter string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, filter) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// macAfter finds the first "<prefix><hex/colons>" in out (grep -o, then last field).
func macAfter(out, prefix string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(prefix) + `[0-9a-fA-F:]*`)
	m := re.FindString(out)
	f := strings.Fields(m)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

type wifiSource struct{ cmd, prefix, filter string }

// wifiMAC returns the MAC used by the current Wi-Fi connection and the raw output
// it came from (for SSID). Only "Supplicant state: COMPLETED" lines count, since
// WifiInfo keeps the last MAC even when Wi-Fi is off.
func wifiMAC(serial string) (mac, raw string) {
	api, _ := strconv.Atoi(strings.TrimSpace(adb.Shell(serial, "getprop ro.build.version.sdk")))
	var sources []wifiSource
	if api >= 30 {
		sources = append(sources, wifiSource{"cmd wifi status", "MAC: ", "Supplicant state: COMPLETED"})
	}
	sources = append(sources,
		wifiSource{"dumpsys wifi | grep 'MAC: '", "MAC: ", "Supplicant state: COMPLETED"},
		wifiSource{"ip link show wlan0", "link/ether ", ""},
		wifiSource{"cat /sys/class/net/wlan0/address", "", ""},
		wifiSource{"ifconfig wlan0", "HWaddr ", ""},
	)
	for _, src := range sources {
		out := adb.Shell(serial, src.cmd+" 2>/dev/null")
		if src.filter != "" {
			out = grepF(out, src.filter)
		}
		if m, ok := normalizeMAC(macAfter(out, src.prefix)); ok {
			debuglog.Logf("Wi-Fi MAC from '%s': %s", src.cmd, m)
			return m, out
		}
	}
	return "", ""
}

// wifiSSID extracts the SSID from the line that reported mac.
func wifiSSID(raw, mac string) string {
	if mac == "" {
		return ""
	}
	for _, l := range strings.Split(raw, "\n") {
		if !strings.Contains(strings.ToLower(l), strings.ToLower("MAC: "+mac)) {
			continue
		}
		m := reSSID.FindStringSubmatch(l)
		if m == nil {
			return ""
		}
		ssid := strings.TrimSuffix(strings.TrimPrefix(m[2], `"`), `"`)
		if ssid == "<unknown ssid>" {
			return ""
		}
		return ssid
	}
	return ""
}

func factoryMAC(serial string) string {
	out := adb.Shell(serial, "dumpsys wifi 2>/dev/null | grep -m1 wifi_sta_factory_mac_address")
	line, _, _ := strings.Cut(out, "\n")
	if i := strings.LastIndex(line, "="); i >= 0 {
		line = line[i+1:]
	}
	m, _ := normalizeMAC(line)
	return m
}

func btMAC(serial string) string {
	if m, ok := normalizeMAC(adb.Shell(serial, "settings get secure bluetooth_address")); ok {
		return m
	}
	for _, l := range strings.Split(adb.Shell(serial, "dumpsys bluetooth_manager"), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToLower(t), "address:") {
			if f := strings.Fields(t); len(f) > 1 {
				m, _ := normalizeMAC(f[1])
				return m
			}
			return ""
		}
	}
	return ""
}

// randomizedMAC checks the locally administered bit of the first octet.
func randomizedMAC(mac string) bool {
	b, err := strconv.ParseUint(mac[:2], 16, 8)
	return err == nil && b&0x02 != 0
}

type macInfo struct {
	Serial      string `json:"serial"`
	Device      string `json:"device"`
	Wifi        any    `json:"wifi"`
	WifiSSID    any    `json:"wifiSsid"`
	Randomized  bool   `json:"wifiRandomized"`
	WifiFactory any    `json:"wifiFactory"`
	Bluetooth   any    `json:"bluetooth"`
}

func collectMAC(serial string) macInfo {
	wifi, raw := wifiMAC(serial)
	m := macInfo{Serial: serial, Device: deviceName(serial), Wifi: strOrNil(wifi),
		WifiSSID: strOrNil(wifiSSID(raw, wifi)), WifiFactory: strOrNil(factoryMAC(serial)), Bluetooth: strOrNil(btMAC(serial))}
	m.Randomized = wifi != "" && randomizedMAC(wifi)
	return m
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func printMAC(m macInfo) {
	o := ui.Out
	o.Println(o.Bold(m.Device) + " " + o.Dim("("+m.Serial+")"))
	wifi := str(m.Wifi)
	if wifi == "" {
		o.Printf("  %-16s n/a %s\n", "Wi-Fi (current)", o.Dim("(Wi-Fi is off or not connected)"))
	} else {
		var notes []string
		if m.Randomized {
			notes = append(notes, "randomized")
		}
		if ssid := str(m.WifiSSID); ssid != "" {
			notes = append(notes, `SSID "`+ssid+`"`)
		}
		note := ""
		if len(notes) > 0 {
			note = " " + o.Dim("("+strings.Join(notes, ", ")+")")
		}
		o.Printf("  %-16s %s%s\n", "Wi-Fi (current)", wifi, note)
	}
	o.Printf("  %-16s %s\n", "Wi-Fi (factory)", orDefault(str(m.WifiFactory), "n/a"))
	o.Printf("  %-16s %s\n", "Bluetooth", orDefault(str(m.Bluetooth), "n/a"))
	if str(m.WifiFactory) == "" && m.Randomized {
		ui.Hint("To see the factory MAC of %s, set Wi-Fi privacy to 'Use device MAC' and run again", m.Device)
	}
}

func runMac(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	serials, err := c.Devices()
	if err != nil {
		return err
	}
	infos := make([]macInfo, len(serials))
	for i, s := range serials {
		infos[i] = collectMAC(s)
	}
	if c.JSON() {
		return ui.JSON(infos)
	}
	for i, m := range infos {
		if i > 0 {
			ui.Out.Println("")
		}
		printMAC(m)
	}
	return nil
}
