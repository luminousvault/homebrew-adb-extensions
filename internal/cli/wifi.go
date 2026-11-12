package cli

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdWifi = &Command{
	Name: "wifi", Summary: "Debug over Wi-Fi", Args: "[off]",
	Long: "Switch a USB-connected device to Wi-Fi debugging (adb tcpip) and connect to it, so the\n" +
		"cable can be unplugged. The computer and the device must be on the same network.\n" +
		"'ak wifi off' switches Wi-Fi devices back to USB mode and disconnects them.\n" +
		"The connected address is printed to stdout.",
	ArgHelp:  [][2]string{{"off", "Switch back to USB and disconnect"}},
	Flags:    []Flag{{"-p", "--port", "PORT", "TCP port (default 5555)"}},
	Examples: []string{"ak wifi", "ak wifi -s emulator-5554", "ak wifi off"},
	Device:   oneDevice, Run: runWifi,
}

var reInet = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)

func isNetworkSerial(s string) bool { return strings.Contains(s, ":") }

// deviceIP returns the IPv4 address of wlan0.
func deviceIP(serial string) string {
	if m := reInet.FindStringSubmatch(adb.Shell(serial, "ip -f inet addr show wlan0")); m != nil {
		return m[1]
	}
	return ""
}

func runWifi(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	if len(c.Args) == 1 {
		if c.Args[0] != "off" {
			return usageErr(c.Cmd, "Unknown argument: %s", c.Args[0])
		}
		return wifiOff(c)
	}
	port := 5555
	if v := c.String("--port"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return usageErr(c.Cmd, "Invalid port: %s", v)
		}
		port = n
	}
	serial, err := c.Device()
	if err != nil {
		return err
	}
	if isNetworkSerial(serial) {
		ui.Success("%s is already connected over the network (%s)", deviceName(serial), serial)
		ui.Out.Println(serial)
		return nil
	}
	ip := deviceIP(serial)
	if ip == "" {
		return failErr("Connect the device to Wi-Fi first", "%s has no Wi-Fi IP address", deviceName(serial))
	}
	name := deviceName(serial)
	addr := ip + ":" + strconv.Itoa(port)

	sp := ui.Spin("Switching %s to Wi-Fi debugging on port %d", name, port)
	if out, err := adb.Combined("-s", serial, "tcpip", strconv.Itoa(port)); err != nil {
		sp.Stop()
		ui.Errorf("Couldn't switch %s to TCP mode", name)
		ui.Detail("%s", out)
		return errReported
	}
	sp.SetText("Connecting to %s at %s", name, addr)
	var last string
	connected := false
	for i := 0; i < 10 && !connected; i++ {
		time.Sleep(time.Second)
		last, _ = adb.Combined("connect", addr)
		connected = strings.Contains(last, "connected to") && !strings.Contains(last, "failed")
	}
	sp.Stop()
	if !connected {
		ui.Errorf("Couldn't connect to %s at %s", name, addr)
		ui.Detail("%s", last)
		ui.Hint("Check that this computer can reach %s (same network, no client isolation). 'ak wifi off' switches the device back", ip)
		return errReported
	}
	ui.Success("Connected to %s over Wi-Fi at %s", name, addr)
	ui.Hint("You can unplug the USB cable. Use '-s %s' while both connections exist; 'ak wifi off' switches back to USB", addr)
	ui.Out.Println(addr)
	return nil
}

// wifiOff switches network devices (or the one given with -s) back to USB and disconnects them.
func wifiOff(c *Ctx) error {
	targets := c.Strings("--serial")
	if len(targets) == 0 {
		for _, d := range adb.ListDevices() {
			if isNetworkSerial(d.Serial) {
				targets = append(targets, d.Serial)
			}
		}
	}
	if len(targets) == 0 {
		ui.Info("No devices connected over Wi-Fi")
		return nil
	}
	failed := false
	for _, t := range targets {
		if !isNetworkSerial(t) {
			ui.Errorf("%s is not a network connection", t)
			failed = true
			continue
		}
		name := deviceName(t)
		_, _ = adb.Combined("-s", t, "usb") // restart adbd in USB mode
		out, _ := adb.Combined("disconnect", t)
		if strings.Contains(out, "disconnected") {
			ui.Success("Switched %s back to USB and disconnected %s", name, t)
		} else {
			ui.Errorf("Couldn't disconnect %s", t)
			ui.Detail("%s", out)
			failed = true
		}
	}
	if failed {
		return errReported
	}
	return nil
}
