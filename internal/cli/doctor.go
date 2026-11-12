package cli

import (
	"runtime"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

var cmdDoctor = &Command{
	Name: "doctor", Desc: "Diagnose adb setup",
	Usage: "doctor", Help: "Diagnose adb setup and conflicts",
	Flags: []string{"-h", "--help"}, NoADB: true, Run: runDoctor,
}

func showHelpDoctor() {
	say("%s%sUsage:%s ak doctor [-h|--help]", Cyan, Bold, NC)
	say("")
	say("Description: Show which adb binary ak uses and every other adb found.")
	say("Lookup order: AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH,")
	say("Android Studio default SDK, ~/.ak/platform-tools.")
	say("")
	say("Using adb binaries with different versions at the same time makes them")
	say("restart each other's adb server, which drops device connections.")
	say("")
	say("Options:")
	say("  -h, --help  Show this help message")
	say("")
}

func runDoctor(args []string) {
	parseLeading("doctor", args, showHelpDoctor, nil, false)

	cands := adb.Candidates()
	say("")
	if len(cands) == 0 {
		say("%s ADB is not installed.", Error)
		say("")
		printADBInstallGuide()
		exit(1)
	}

	say("%s%sadb in use:%s", Cyan, Bold, NC)
	builds := map[string]bool{}
	for i, c := range cands {
		proto, build := adb.VersionInfo(c.Path)
		if build == "" {
			build = "unknown"
		}
		builds[build] = true
		if i == 0 {
			say("  %s%s%s", Bold, c.Path, NC)
			say("  %sSource:%s %s, %sVersion:%s %s (%s)", Dim, NC, c.Source, Dim, NC, build, proto)
			if len(cands) > 1 {
				say("")
				say("%s%sOther adb found:%s", Cyan, Bold, NC)
			}
			continue
		}
		say("  %s  %s(%s, %s)%s", c.Path, Dim, c.Source, build, NC)
	}
	say("")
	if len(builds) > 1 {
		say("%sWARNING:%s adb binaries with different versions were found.", Yellow, NC)
		say("  Running them side by side (e.g. ak and Android Studio) restarts the adb")
		say("  server and drops device connections. Point ak at the same adb with:")
		if runtime.GOOS == "windows" {
			say("  %sset AK_ADB=<path to adb.exe>%s", Bold, NC)
		} else {
			say("  %sexport AK_ADB=<path to adb>%s", Bold, NC)
		}
		say("")
		return
	}
	say("%sOK:%s no adb version conflict.", Green, NC)
	say("")
}
