package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

var cmdMac = &Command{
	Name: "mac", Desc: "Show device MAC addresses",
	Usage: "mac [options]", Help: "Show device MAC addresses",
	Flags: []string{"-h", "--help", "-m"}, Run: runMac,
}

func showHelpMac() {
	say("%s%sUsage:%s ak mac [-m] [-h|--help]", Cyan, Bold, NC)
	say("")
	say("Description: Show MAC addresses of the device.")
	say("  Wi-Fi (current)  MAC used for the current connection (per-network, may be randomized)")
	say("  Wi-Fi (factory)  Device's own Wi-Fi MAC (available even when Wi-Fi is off, if exposed)")
	say("  Bluetooth        Bluetooth MAC")
	say("")
	say("Options:")
	say("  -m          Show all connected devices")
	say("  -h, --help  Show this help message")
	say("")
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

func printMACDetail(serial, wifi, ssid, factory, bt string) {
	say("")
	say("%s%s%s", Bold, prettyDevice(serial, "normal"), NC)
	if wifi != "" {
		var notes []string
		if randomizedMAC(wifi) {
			notes = append(notes, "randomized")
		}
		if ssid != "" {
			notes = append(notes, `SSID "`+ssid+`"`)
		}
		sayf("  %-16s %s", "Wi-Fi (current)", wifi)
		if len(notes) > 0 {
			sayf("   %s(%s)%s", Dim, strings.Join(notes, " · "), NC)
		}
		say("")
	} else {
		sayf("  %-16s %s", "Wi-Fi (current)", "n/a")
		say("   %s(Wi-Fi is off or not connected)%s", Dim, NC)
	}
	say("  %-16s %s", "Wi-Fi (factory)", orDefault(factory, "n/a"))
	say("  %-16s %s", "Bluetooth", orDefault(bt, "n/a"))
	if factory == "" && wifi != "" && randomizedMAC(wifi) {
		say("  %s↳ Factory MAC: set Wi-Fi privacy to 'Use device MAC' and run again%s", Dim, NC)
	}
}

func runMac(args []string) {
	multi := false
	parseLeading("mac", args, showHelpMac, map[string]*bool{"-m": &multi}, false)
	var serials []string
	if multi {
		serials = adb.ReadySerials()
		if len(serials) == 0 {
			say("%s No connected devices found.", Error)
			exit(1)
		}
	} else {
		serials = []string{selectDevice()}
	}
	for _, s := range serials {
		wifi, raw := wifiMAC(s)
		printMACDetail(s, wifi, wifiSSID(raw, wifi), factoryMAC(s), btMAC(s))
	}
	say("")
}
