// Package cli implements the ak commands.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

// Build metadata, set from main via ldflags.
var (
	Version     = "dev"
	ReleaseDate = "unknown"
)

// Command is one `ak <name>` subcommand.
type Command struct {
	Name  string
	Desc  string   // short description (zsh completion)
	Usage string   // "<name> <args>" column in the global help
	Help  string   // description column in the global help
	Flags []string // accepted options; must match completions/_ak
	NoADB bool     // runs without adb (e.g. doctor)
	Run   func(args []string)
}

// Commands lists every subcommand in global-help order.
var Commands = []*Command{
	cmdInstall, cmdPull, cmdExtract, cmdInfo, cmdPermissions, cmdUninstall, cmdClear,
	cmdKill, cmdLaunch, cmdSignature, cmdActivities, cmdDevices, cmdMac, cmdDoctor,
}

func find(name string) *Command {
	for _, c := range Commands {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Output and input streams; tests replace them.
var (
	stdout io.Writer     = os.Stdout
	stderr io.Writer     = os.Stderr
	stdin  *bufio.Reader = bufio.NewReader(os.Stdin)
)

func say(format string, args ...any)  { fmt.Fprintf(stdout, format+"\n", args...) }
func sayf(format string, args ...any) { fmt.Fprintf(stdout, format, args...) }

// exitCode is panicked by exit() and recovered in Main, so deferred
// terminal restores run and tests can observe the code.
type exitCode int

func exit(code int) { panic(exitCode(code)) }

// Main runs ak with args (without the program name) and returns the exit code.
func Main(args []string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			c, ok := r.(exitCode)
			if !ok {
				panic(r)
			}
			code = int(c)
		}
	}()

	if len(args) == 0 {
		showHelp()
		return 0
	}
	name, rest := args[0], args[1:]
	switch name {
	case "--version", "-v":
		showVersion()
		return 0
	case "--help", "-h":
		showHelp()
		return 0
	}
	c := find(name)
	if c == nil {
		say("%s Unknown command: %s", Error, name)
		say("")
		showHelp()
		return 1
	}
	if !c.NoADB && !ensureADB() {
		return 1
	}
	c.Run(rest)
	return 0
}

// ensureADB locates adb and installs the default runner.
func ensureADB() bool {
	if adb.Default != nil {
		return true
	}
	p, err := adb.Locate()
	if err != nil {
		say("")
		say("%s ADB is not installed.", Error)
		say("")
		printADBInstallGuide()
		return false
	}
	adb.Default = adb.Exec{Path: p}
	return true
}

func printADBInstallGuide() {
	say("%sInstallation:%s", Yellow, NC)
	say("  %smacOS:%s   brew install --cask android-platform-tools", Bold, NC)
	say("  %sUbuntu:%s  sudo apt install adb", Bold, NC)
	say("  %sManual:%s  https://developer.android.com/tools/releases/platform-tools", Bold, NC)
	say("  %sCustom:%s  export AK_ADB=/path/to/adb", Bold, NC)
	say("")
}

func showHelp() {
	say("")
	say("%s%sUsage:%s ak <command> [arguments...]", Cyan, Bold, NC)
	say("")
	say("%sCommands:%s", Bold, NC)
	for _, c := range Commands {
		cmd, args, _ := strings.Cut(c.Usage, " ")
		pad := 33 - len(c.Usage)
		if pad < 1 {
			pad = 1
		}
		say("  %s%s%s%s%s%s", Bold, cmd, NC, prefixSpace(args), strings.Repeat(" ", pad), c.Help)
	}
	say("")
	say("%sGlobal Options:%s", Bold, NC)
	say("  %s--version, -v%s    Show version", Bold, NC)
	say("  %s--help, -h%s       Show this help", Bold, NC)
	say("")
	say("Run '%sak <command> --help%s' for command-specific help.", Bold, NC)
	say("")
}

func prefixSpace(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

func showVersion() {
	adbVersion := "Not found"
	if p, err := adb.Locate(); err == nil {
		if v, _ := adb.VersionInfo(p); v != "" {
			adbVersion = v
		}
	}
	G, Y, R, C, B := Green, Yellow, Red, Cyan, Blue
	say("")
	say("                                        %sak%s %s%s%s - Released on %s", Bold, NC, G, Version, NC, ReleaseDate)
	say("        %s::%s                   %s.::%s        ------------------------------------", G, NC, G, NC)
	say("       %s:#*+.%s                %s:+*#.%s       %s%sADB Version:%s %s", G, NC, G, NC, Bold, Y, NC, adbVersion)
	say("        %s:**+:%s    %s......%s    %s-+**.%s        %s%sAuthor:%s Claude Hwang", G, NC, G, NC, G, NC, Bold, Y, NC)
	say("         %s.*+=---::::::::---+*+%s          %s%sLicense:%s MIT", G, NC, Bold, Y, NC)
	say("        %s:-=----:--------:---==-.%s        %s%sLanguage:%s Go", Y, NC, Bold, Y, NC)
	say("      %s:++=---==============---=+=.%s      %s%sSupported OS:%s macOS, Linux, Windows", Y, NC, Bold, Y, NC)
	say("    %s.+*+=+*%%#++++++++++++++##+=+**-%s     %s%sDependencies:%s adb", R, NC, Bold, Y, NC)
	say("   %s.****+%%@@%%+++++++++++++*@@@#+**#+%s    %s%sRepository:%s https://github.com/luminousvault/homebrew-adb-extensions", R, NC, Bold, Y, NC)
	say("   %s*#*****#*+++++++++++++++*##*****#=%s   ", C, NC)
	say("  %s-%%#*****++++++++++++++++++++****##%%.%s  %s%sPurpose:%s ADB extensions kit - Essential ADB utilities", B, NC, Bold, Y, NC)
	say("                                        %s%sFeatures:%s APK install, Device management, App control", Bold, Y, NC)
	say("")
}

// parseLeading parses options before the first positional argument, like the
// Bash `while case` loops: -h/--help prints help, flags set bools, any other
// "-x" is an invalid option. If positional is false, positional args are errors.
func parseLeading(name string, args []string, help func(), flags map[string]*bool, positional bool) []string {
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "-h" || a == "--help":
			help()
			exit(1)
		case flags[a] != nil:
			*flags[a] = true
			args = args[1:]
		case strings.HasPrefix(a, "-"):
			say("%s Invalid option: %s", Error, a)
			say("Try 'ak %s --help' for more information.", name)
			exit(1)
		case !positional:
			say("%s Unexpected argument: %s", Error, a)
			say("Try 'ak %s --help' for more information.", name)
			exit(1)
		default:
			return args
		}
	}
	return args
}

func sayErr(format string, args ...any) { fmt.Fprintf(stderr, format+"\n", args...) }
