package cli

import (
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

func TestDoctorReport(t *testing.T) {
	ui.Out = &ui.Stream{}
	t.Setenv("HOME", "/home/u")
	found := []adbFound{
		{Path: "/home/u/sdk/platform-tools/adb", Source: "ANDROID_HOME", Version: "37.0.1", Protocol: "1.0.41", InUse: true},
		{Path: "/usr/local/bin/adb", Source: "PATH", Version: "35.0.0", Protocol: "1.0.41"},
	}
	devices := []deviceFound{
		{Serial: "emulator-5554", State: "device", Name: "Pixel 7"},
		{Serial: "emulator-5556", State: "unauthorized"},
	}
	sections := doctorChecks(found, devices, "emulator-5558", "")
	got := strings.Join(doctorLines(sections), "\n")
	want := strings.Join([]string{
		"adb",
		"  ✓ 37.0.1 (protocol 1.0.41)",
		"      ~/sdk/platform-tools/adb  (ANDROID_HOME)",
		"  ✗ Other adb with a different version",
		"      /usr/local/bin/adb  35.0.0  (PATH)",
		"      Hint: Running them side by side (e.g. ak and Android Studio) restarts the adb server. " +
			"Point ak at the same adb: " + setEnv("AK_ADB", "<path to adb>"),
		"",
		"Devices",
		"  ✓ 1 connected",
		"      Pixel 7  emulator-5554",
		"  ✗ emulator-5556 is unauthorized",
		"      Hint: Unlock the device and allow USB debugging in the prompt",
		"  ✗ ANDROID_SERIAL=emulator-5558 is not connected",
		"      Hint: Connect it, or unset ANDROID_SERIAL",
		"",
		"Optional tools",
		"  – apksigner not found",
		"      Only needed when 'ak signature' cannot parse an APK; comes with SDK build-tools",
	}, "\n")
	if got != want {
		t.Errorf("report:\n%s\nwant:\n%s", got, want)
	}

	// Same version elsewhere is fine; no devices is not a problem.
	found[1].Version = "37.0.1"
	sections = doctorChecks(found, nil, "", "/sdk/build-tools/37.0.0/apksigner")
	got = strings.Join(doctorLines(sections), "\n")
	for _, w := range []string{"  ✓ 1 other adb, same version", "  – No devices connected", "  ✓ apksigner"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if strings.Contains(got, "✗") {
		t.Errorf("unexpected problem:\n%s", got)
	}
}
