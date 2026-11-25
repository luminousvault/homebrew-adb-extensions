package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdSysinfo = &Command{
	Name: "sysinfo", Summary: "Show device details and identifiers (IMEI, MAC, Android ID)",
	Long: "Show a device's \"About phone\" details in one place:\n" +
		"  Device       adb serial, model, manufacturer and the name set on the device\n" +
		"  Software     Android version, security patch and build\n" +
		"  Identifiers  Hardware serial number, IMEI (and the second IMEI on dual-SIM devices), Android ID\n" +
		"  Network      Wi-Fi IP address and MAC addresses\n" +
		"  Hardware     CPU ABI and screen size\n" +
		"Values the device doesn't expose are shown as - (null in JSON).",
	Examples: []string{"ak sysinfo", "ak sysinfo --all", "ak sysinfo --json | jq '.[].imei'"},
	Device:   manyDevices, JSON: true, Run: runSysinfo,
}

// hintIMEI shows the IMEI on the device itself: the dialer with *#06#.
func hintIMEI(serial string) string {
	return "Run 'adb" + serialArg(serial) + " shell am start -a android.intent.action.DIAL -d tel:*%2306%23' " +
		"to show the IMEI on the device (the dialer with *#06#)"
}

var reParcelWord = regexp.MustCompile(`^[0-9a-fA-F]{8}$`)

// parseParcelString reads the String16 reply of `service call`:
//
//	Result: Parcel(
//	0x00000000: 00000000 0000000f 00350033 ...  '....3.5...'
//
// The first word is the exception code (0 = none), the second the length in
// UTF-16 units (-1 = null), then two units per word, low half first. It returns
// "" for exceptions, null strings and anything it can't read.
func parseParcelString(out string) string {
	var words []uint32
	for _, l := range strings.Split(out, "\n") {
		l, _, _ = strings.Cut(l, "'") // drop the ASCII column
		l = strings.TrimPrefix(strings.TrimSpace(l), "Result: Parcel(")
		if _, rest, ok := strings.Cut(l, ": "); ok && strings.HasPrefix(l, "0x") {
			l = rest
		}
		for _, f := range strings.Fields(l) {
			if !reParcelWord.MatchString(f) {
				continue
			}
			w, _ := strconv.ParseUint(f, 16, 32)
			words = append(words, uint32(w))
		}
	}
	if len(words) < 2 || words[0] != 0 {
		return ""
	}
	n := int(words[1])
	if words[1] == 0xffffffff || 2+(n+1)/2 > len(words) {
		return ""
	}
	units := make([]rune, n)
	for i := range units {
		w := words[2+i/2]
		if i%2 == 1 {
			w >>= 16
		}
		units[i] = rune(w & 0xffff)
	}
	return string(units)
}

// validIMEI accepts 15 digits with a valid Luhn check digit.
func validIMEI(s string) bool {
	if len(s) != 15 {
		return false
	}
	sum := 0
	for i := range 15 {
		c := s[14-i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// readIMEIs asks the iphonesubinfo service (the shell has
// READ_PRIVILEGED_PHONE_STATE). Transaction codes follow IPhoneSubInfo.aidl and
// differ by Android version and OEM, so a result counts only if it is a valid
// IMEI. Checked on Android 15 (Samsung, dual SIM): code 1 getDeviceId(pkg) is
// the first IMEI; code 4 getDeviceIdForPhone(phoneId, pkg) gives each slot.
func readIMEIs(serial string) (imei, imei2 string) {
	call := func(args string) string {
		v := parseParcelString(adb.Shell(serial, "service call iphonesubinfo "+args+" s16 com.android.shell"))
		if !validIMEI(v) {
			return ""
		}
		return v
	}
	if imei = call("1"); imei == "" {
		imei = call("4 i32 0")
	}
	if imei2 = call("4 i32 1"); imei2 == imei {
		imei2 = ""
	}
	if imei == "" {
		imei, imei2 = imei2, ""
	}
	return imei, imei2
}

// settingValue reads a setting; "null" (unset) becomes "".
func settingValue(serial, namespace, key string) string {
	v := strings.TrimSpace(adb.Shell(serial, "settings get "+namespace+" "+key))
	if v == "null" || strings.ContainsAny(v, "\n") {
		return ""
	}
	return v
}

type screenInfo struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	Density int `json:"density"`
}

var (
	reWMSize    = regexp.MustCompile(`(Physical|Override) size: (\d+)x(\d+)`)
	reWMDensity = regexp.MustCompile(`(Physical|Override) density: (\d+)`)
)

// parseWM reads `wm size; wm density`: the physical screen, and the size and
// density in use when the user changed them (0 when not overridden).
func parseWM(out string) (phys screenInfo, overW, overH, overD int, ok bool) {
	for _, m := range reWMSize.FindAllStringSubmatch(out, -1) {
		w, _ := strconv.Atoi(m[2])
		h, _ := strconv.Atoi(m[3])
		if m[1] == "Physical" {
			phys.Width, phys.Height = w, h
		} else {
			overW, overH = w, h
		}
	}
	for _, m := range reWMDensity.FindAllStringSubmatch(out, -1) {
		d, _ := strconv.Atoi(m[2])
		if m[1] == "Physical" {
			phys.Density = d
		} else {
			overD = d
		}
	}
	return phys, overW, overH, overD, phys.Width > 0 && phys.Height > 0
}

type sysInfo struct {
	Serial         string      `json:"serial"`
	Device         string      `json:"device"`
	Model          any         `json:"model"`
	Manufacturer   any         `json:"manufacturer"`
	Name           any         `json:"name"`
	Android        any         `json:"android"`
	API            any         `json:"api"`
	SecurityPatch  any         `json:"securityPatch"`
	Build          any         `json:"build"`
	HardwareSerial any         `json:"hardwareSerial"`
	IMEI           any         `json:"imei"`
	IMEI2          any         `json:"imei2"`
	AndroidID      any         `json:"androidId"`
	IP             any         `json:"ip"`
	WifiMAC        any         `json:"wifiMac"`
	WifiSSID       any         `json:"wifiSsid"`
	WifiRandomized bool        `json:"wifiRandomized"`
	WifiFactory    any         `json:"wifiFactory"`
	Bluetooth      any         `json:"bluetooth"`
	ABI            any         `json:"abi"`
	Screen         *screenInfo `json:"screen"`

	screenNote string // the size/density in use, when changed from the physical one
}

func collectSysinfo(serial string) sysInfo {
	p := deviceProps(serial)
	imei, imei2 := readIMEIs(serial)
	m := collectMAC(serial)
	s := sysInfo{
		Serial: serial, Device: deviceName(serial),
		Model: strOrNil(p["ro.product.model"]), Manufacturer: strOrNil(capitalize(p["ro.product.manufacturer"])),
		Name:    strOrNil(settingValue(serial, "global", "device_name")),
		Android: strOrNil(p["ro.build.version.release"]), API: intOrNil(p["ro.build.version.sdk"]),
		SecurityPatch: strOrNil(p["ro.build.version.security_patch"]), Build: strOrNil(p["ro.build.display.id"]),
		HardwareSerial: strOrNil(p["ro.serialno"]), IMEI: strOrNil(imei), IMEI2: strOrNil(imei2),
		AndroidID: strOrNil(settingValue(serial, "secure", "android_id")), IP: strOrNil(deviceIP(serial)),
		WifiMAC: m.Wifi, WifiSSID: m.WifiSSID, WifiRandomized: m.Randomized, WifiFactory: m.WifiFactory,
		Bluetooth: m.Bluetooth, ABI: strOrNil(p["ro.product.cpu.abi"]),
	}
	if phys, w, h, d, ok := parseWM(adb.Shell(serial, "wm size; wm density")); ok {
		s.Screen = &phys
		var in []string
		if w > 0 && (w != phys.Width || h != phys.Height) {
			in = append(in, strconv.Itoa(w)+"x"+strconv.Itoa(h))
		}
		if d > 0 && d != phys.Density {
			in = append(in, strconv.Itoa(d)+" dpi")
		}
		if len(in) > 0 {
			s.screenNote = "in use: " + strings.Join(in, ", ")
		}
	}
	return s
}

func printSysinfo(s sysInfo) {
	o := ui.Out
	dash := func(v any) string { return orDefault(str(v), "-") }
	ui.Step("%s", s.Device)
	type block struct {
		title string
		rows  [][2]string
	}
	var blocks []block
	section := func(title string, rows ...[2]string) { blocks = append(blocks, block{title, rows}) }
	section("Device", [2]string{"serial", s.Serial}, [2]string{"model", dash(s.Model)},
		[2]string{"manufacturer", dash(s.Manufacturer)}, [2]string{"name", dash(s.Name)})

	android := dash(s.Android)
	if s.API != nil {
		android += " (API " + strconv.FormatInt(s.API.(int64), 10) + ")"
	}
	section("Software", [2]string{"android", android}, [2]string{"securityPatch", dash(s.SecurityPatch)},
		[2]string{"build", dash(s.Build)})

	ids := [][2]string{{"hardwareSerial", dash(s.HardwareSerial)}, {"imei", dash(s.IMEI)}}
	if s.IMEI2 != nil {
		ids = append(ids, [2]string{"imei2", str(s.IMEI2)})
	}
	section("Identifiers", append(ids, [2]string{"androidId", dash(s.AndroidID)})...)

	wifi := dash(s.WifiMAC)
	var notes []string
	if s.WifiMAC == nil {
		notes = append(notes, "Wi-Fi is off or not connected")
	}
	if s.WifiRandomized {
		notes = append(notes, "randomized")
	}
	if ssid := str(s.WifiSSID); ssid != "" {
		notes = append(notes, `SSID "`+ssid+`"`)
	}
	if len(notes) > 0 {
		wifi += " " + o.Dim("("+strings.Join(notes, ", ")+")")
	}
	section("Network", [2]string{"ip", dash(s.IP)}, [2]string{"wifiMac", wifi},
		[2]string{"wifiFactory", dash(s.WifiFactory)}, [2]string{"bluetooth", dash(s.Bluetooth)})

	screen := "-"
	if sc := s.Screen; sc != nil {
		screen = strconv.Itoa(sc.Width) + "x" + strconv.Itoa(sc.Height)
		if sc.Density > 0 {
			screen += ", " + strconv.Itoa(sc.Density) + " dpi"
		}
		if s.screenNote != "" {
			screen += " " + o.Dim("("+s.screenNote+")")
		}
	}
	section("Hardware", [2]string{"abi", dash(s.ABI)}, [2]string{"screen", screen})

	// One key column for all sections, so values line up down the whole result.
	w := 0
	for _, b := range blocks {
		w = max(w, ui.KeyWidth(b.rows))
	}
	for i, b := range blocks {
		if i > 0 {
			o.Println("")
		}
		o.Println(o.Bold(b.title))
		ui.KeyValuesWidth(b.rows, w)
	}
}

func runSysinfo(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	serials, err := c.Devices()
	if err != nil {
		return err
	}
	infos := make([]sysInfo, len(serials))
	noIMEI := "" // the first device whose IMEI ak could not read
	for i, s := range serials {
		infos[i] = collectSysinfo(s)
		if infos[i].IMEI == nil && noIMEI == "" {
			noIMEI = s
		}
	}
	if c.JSON() {
		return ui.JSON(infos) // always an array, like mac, so scripts see one shape for 1 or n devices
	}
	for i, s := range infos {
		if i > 0 {
			ui.Out.Println("")
		}
		printSysinfo(s)
	}
	if noIMEI != "" {
		ui.Hint("%s", hintIMEI(noIMEI))
	}
	return nil
}
