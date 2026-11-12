package cli

import (
	"runtime"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdDoctor = &Command{
	Name: "doctor", Summary: "Diagnose adb setup",
	Long: "Show which adb binary ak uses and every other adb found.\n" +
		"Lookup order: AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH, Android Studio default SDK, ~/.ak/platform-tools.\n" +
		"Running adb binaries of different versions side by side restarts the adb server and drops device connections.",
	Examples: []string{"ak doctor", "ak doctor --json"},
	NoADB:    true, JSON: true, Run: runDoctor,
}

type adbFound struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
	InUse    bool   `json:"inUse"`
}

func runDoctor(c *Ctx) error {
	if err := tooMany(c, 0); err != nil {
		return err
	}
	var found []adbFound
	builds := map[string]bool{}
	for i, cand := range adb.Candidates() {
		proto, build := adb.VersionInfo(cand.Path)
		found = append(found, adbFound{cand.Path, cand.Source, orDefault(build, "unknown"), proto, i == 0})
		builds[orDefault(build, "unknown")] = true
	}
	conflict := len(builds) > 1
	if c.JSON() {
		if found == nil {
			found = []adbFound{}
		}
		return ui.JSON(map[string]any{"adb": found, "versionConflict": conflict})
	}
	if len(found) == 0 {
		return failErr(adbInstallHint(), "adb not found")
	}
	o := ui.Out
	o.Println(o.Bold("adb in use"))
	o.Printf("  %s\n  %s\n", found[0].Path, o.Dim(found[0].Source+", version "+found[0].Version+" (protocol "+found[0].Protocol+")"))
	if len(found) > 1 {
		o.Println("")
		o.Println(o.Bold("Other adb found"))
		for _, f := range found[1:] {
			o.Printf("  %s  %s\n", f.Path, o.Dim("("+f.Source+", "+f.Version+")"))
		}
	}
	if !conflict {
		ui.Success("No adb version conflict")
		return nil
	}
	ui.Warn("adb binaries with different versions were found")
	ui.Info("Running them side by side (e.g. ak and Android Studio) restarts the adb server and drops connections.")
	if runtime.GOOS == "windows" {
		ui.Hint("Point ak at the same adb: set AK_ADB=<path to adb.exe>")
	} else {
		ui.Hint("Point ak at the same adb: export AK_ADB=<path to adb>")
	}
	return nil
}
