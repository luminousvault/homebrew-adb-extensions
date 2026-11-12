// Package cli implements the ak commands.
// Output rules: docs/design/cli-output.md.
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Build metadata, set from main via ldflags.
var (
	Version     = "dev"
	ReleaseDate = "unknown"
)

type deviceMode int

const (
	noDevice    deviceMode = iota
	oneDevice              // -s/--serial
	manyDevices            // -s/--serial (repeatable), -a/--all
)

// Flag is one option of a command.
type Flag struct {
	Short, Long string // "-t", "--json"; either may be empty
	Value       string // placeholder when the option takes a value
	Help        string
}

func (f Flag) key() string {
	if f.Long != "" {
		return f.Long
	}
	return f.Short
}

// Command is one `ak <name>` subcommand. Parsing, help and the zsh completion
// check are all driven by this definition.
type Command struct {
	Name     string
	Summary  string      // one line for the global help and zsh completion
	Args     string      // usage arguments, e.g. "[package]"
	Long     string      // help description (defaults to Summary)
	ArgHelp  [][2]string // argument name, description
	Flags    []Flag
	Examples []string
	Device   deviceMode
	JSON     bool // adds --json
	NoADB    bool
	Hidden   bool // left out of the global help and the zsh completion
	Run      func(c *Ctx) error
}

var (
	flagHelp   = Flag{"-h", "--help", "", "Show this help"}
	flagJSON   = Flag{"", "--json", "", "Print the result as JSON"}
	flagSerial = Flag{"-s", "--serial", "ID", "Target device (default: $ANDROID_SERIAL, or ask)"}
	flagSerals = Flag{"-s", "--serial", "ID", "Target device; repeat for more (default: $ANDROID_SERIAL, or ask)"}
	flagAll    = Flag{"-a", "--all", "", "All connected devices"}
	flagYes    = Flag{"-y", "--yes", "", "Answer yes to confirmations"}
)

// AllFlags returns the command's options including the common ones.
func (cmd *Command) AllFlags() []Flag {
	fs := append([]Flag(nil), cmd.Flags...)
	switch cmd.Device {
	case oneDevice:
		fs = append(fs, flagSerial)
	case manyDevices:
		fs = append(fs, flagSerals, flagAll)
	}
	if cmd.JSON {
		fs = append(fs, flagJSON)
	}
	return append(fs, flagHelp)
}

// Commands lists every subcommand in global-help order.
var Commands = []*Command{
	cmdInstall, cmdPull, cmdExtract, cmdInfo, cmdPermissions, cmdUninstall, cmdClear,
	cmdKill, cmdLaunch, cmdRestart, cmdSignature, cmdActivities, cmdLogcat, cmdScreenshot, cmdRecord,
	cmdDevices, cmdMac, cmdWifi, cmdDoctor, cmdVerifier,
}

func find(name string) *Command {
	for _, c := range Commands {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ── Errors and exit codes ────────────────────────────

// cliError carries the exit code and message printed by Main.
type cliError struct {
	code      int
	msg, hint string
	usageOf   string // command name for "Run 'ak <cmd> --help'"
}

func (e *cliError) Error() string { return e.msg }

// usageErr is a usage mistake (exit 2).
func usageErr(cmd *Command, format string, args ...any) error {
	return &cliError{code: 2, msg: fmt.Sprintf(format, args...), usageOf: cmd.Name}
}

// failErr is a failure with an optional hint (exit 1).
func failErr(hint, format string, args ...any) error {
	return &cliError{code: 1, msg: fmt.Sprintf(format, args...), hint: hint}
}

// errReported means failures were already printed; just exit 1.
var errReported = &cliError{code: 1}

// Main runs ak with args (without the program name) and returns the exit code.
func Main(args []string) int {
	noColor := false
	var rest []string
	for _, a := range args {
		if a == "--no-color" {
			noColor = true
			continue
		}
		rest = append(rest, a)
	}
	ui.Setup(noColor)
	return run(rest)
}

func run(args []string) int {
	if len(args) == 0 {
		showHelp()
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		showHelp()
		return 0
	case "-v", "--version":
		showVersion()
		return 0
	}
	cmd := find(args[0])
	if cmd == nil {
		return report(&cliError{code: 2, msg: "Unknown command: " + args[0], hint: "Run 'ak --help' to see all commands"})
	}
	c, help, err := parse(cmd, args[1:])
	if err != nil {
		return report(err)
	}
	if help {
		showCommandHelp(cmd)
		return 0
	}
	if !cmd.NoADB {
		if err := ensureADB(); err != nil {
			return report(err)
		}
	}
	return report(cmd.Run(c))
}

// report prints err (if any) and returns the exit code.
func report(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, tui.ErrInterrupted) || errors.Is(err, ui.ErrInterrupted) {
		return 130
	}
	var ce *cliError
	if !errors.As(err, &ce) {
		ui.Errorf("%v", err)
		return 1
	}
	if ce.msg != "" {
		ui.Errorf("%s", ce.msg)
	}
	if ce.hint != "" {
		ui.Hint("%s", ce.hint)
	}
	if ce.usageOf != "" {
		ui.Err.Println("Run 'ak " + ce.usageOf + " --help' for usage.")
	}
	return ce.code
}

// ensureADB locates adb and installs the default runner.
func ensureADB() error {
	if adb.Default != nil {
		return nil
	}
	p, err := adb.Locate()
	if err != nil {
		return failErr(adbInstallHint(), "adb not found")
	}
	adb.Default = adb.Exec{Path: p}
	return nil
}

func adbInstallHint() string {
	return "Install it with 'brew install --cask android-platform-tools' (macOS), " +
		"'sudo apt install adb' (Ubuntu) or from https://developer.android.com/tools/releases/platform-tools, " +
		"or point AK_ADB at an adb binary"
}

// ── Parsing ──────────────────────────────────────────

// Ctx is a parsed invocation.
type Ctx struct {
	Cmd    *Command
	Args   []string
	values map[string][]string // flag key -> values ("" for switches)
}

// Bool reports whether a switch was given.
func (c *Ctx) Bool(key string) bool { return len(c.values[key]) > 0 }

// String returns the last value of an option.
func (c *Ctx) String(key string) string {
	v := c.values[key]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// Strings returns every value of a repeatable option (comma-separated values are split).
func (c *Ctx) Strings(key string) []string {
	var out []string
	for _, v := range c.values[key] {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// JSON reports whether --json was given.
func (c *Ctx) JSON() bool { return c.Bool("--json") }

// parse splits options and arguments. Options may appear anywhere; "--" ends them.
func parse(cmd *Command, args []string) (*Ctx, bool, error) {
	c := &Ctx{Cmd: cmd, values: map[string][]string{}}
	flags := cmd.AllFlags()
	lookup := func(name string) (Flag, bool) {
		for _, f := range flags {
			if name != "" && (name == f.Short || name == f.Long) {
				return f, true
			}
		}
		return Flag{}, false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			c.Args = append(c.Args, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			c.Args = append(c.Args, a)
			continue
		}
		// Bundled short options, as getopt allows: -ea is -e -a, -tl5 is -t -l=5,
		// -cl=W is -c -l=W (the "=value" goes to the last option of the bundle).
		if head, val, hasVal := strings.Cut(a, "="); len(head) > 2 && head[1] != '-' {
			split, err := splitShort(cmd, head, lookup)
			if err != nil {
				return nil, false, err
			}
			if hasVal {
				split[len(split)-1] += "=" + val
			}
			args = append(append(append([]string{}, args[:i]...), split...), args[i+1:]...)
			a = args[i]
		}
		name, value, hasValue := strings.Cut(a, "=")
		f, ok := lookup(name)
		if !ok {
			return nil, false, usageErr(cmd, "Unknown option: %s", name)
		}
		if f == flagHelp {
			return c, true, nil
		}
		switch {
		case f.Value == "" && hasValue:
			return nil, false, usageErr(cmd, "Option %s does not take a value", name)
		case f.Value != "" && !hasValue:
			if i+1 >= len(args) {
				return nil, false, usageErr(cmd, "Option %s requires a value (%s)", name, f.Value)
			}
			i++
			value = args[i]
		}
		c.values[f.key()] = append(c.values[f.key()], value)
	}
	if c.Bool("--yes") {
		ui.AssumeYes = true
	}
	return c, false, nil
}

// splitShort expands bundled short options: "-ea" -> "-e", "-a". An option
// that takes a value ends the bundle: the rest is its value ("-lW" -> "-l=W"),
// or the next argument when nothing is left ("-tl W").
func splitShort(cmd *Command, a string, lookup func(string) (Flag, bool)) ([]string, error) {
	var out []string
	rs := []rune(a[1:])
	for j, r := range rs {
		name := "-" + string(r)
		f, ok := lookup(name)
		if !ok {
			if j == 0 {
				return nil, usageErr(cmd, "Unknown option: %s", a)
			}
			return nil, usageErr(cmd, "Unknown option: %s (in %s)", name, a)
		}
		if f.Value != "" {
			if rest := string(rs[j+1:]); rest != "" {
				name += "=" + rest
			}
			return append(out, name), nil
		}
		out = append(out, name)
	}
	return out, nil
}

// ── Help ─────────────────────────────────────────────

func showHelp() {
	o := ui.Out
	o.Println("ak - ADB extensions kit")
	o.Println("")
	o.Println(o.Bold("Usage:"))
	o.Println("  ak <command> [options] [arguments]")
	o.Println("")
	o.Println(o.Bold("Commands:"))
	for _, c := range Commands {
		if c.Hidden {
			continue
		}
		o.Printf("  %-12s %s\n", c.Name, c.Summary)
	}
	o.Println("")
	o.Println(o.Bold("Global options:"))
	o.Println("  -h, --help      Show help")
	o.Println("  -v, --version   Show version")
	o.Println("      --no-color  Disable colored output")
	o.Println("")
	o.Println(o.Bold("Environment:"))
	o.Println("  ANDROID_SERIAL  Default target device")
	o.Println("  AK_ADB          Path to the adb binary")
	o.Println("  NO_COLOR        Disable colored output")
	o.Println("")
	o.Println("Run 'ak <command> --help' for details.")
}

func flagLabel(f Flag) string {
	var s string
	switch {
	case f.Short != "" && f.Long != "":
		s = f.Short + ", " + f.Long
	case f.Short != "":
		s = f.Short
	default:
		s = "    " + f.Long
	}
	if f.Value != "" {
		s += " " + f.Value
	}
	return s
}

func showCommandHelp(cmd *Command) {
	o := ui.Out
	desc := cmd.Long
	if desc == "" {
		desc = cmd.Summary + "."
	}
	o.Println(desc)
	o.Println("")
	o.Println(o.Bold("Usage:"))
	usage := "  ak " + cmd.Name + " [options]"
	if cmd.Args != "" {
		usage += " " + cmd.Args
	}
	o.Println(usage)
	if len(cmd.ArgHelp) > 0 {
		o.Println("")
		o.Println(o.Bold("Arguments:"))
		w := 0
		for _, a := range cmd.ArgHelp {
			w = max(w, len(a[0]))
		}
		for _, a := range cmd.ArgHelp {
			o.Printf("  %-*s  %s\n", w, a[0], a[1])
		}
	}
	o.Println("")
	o.Println(o.Bold("Options:"))
	flags := cmd.AllFlags()
	w := 0
	for _, f := range flags {
		w = max(w, len(flagLabel(f)))
	}
	for _, f := range flags {
		o.Printf("  %-*s  %s\n", w, flagLabel(f), f.Help)
	}
	if len(cmd.Examples) > 0 {
		o.Println("")
		o.Println(o.Bold("Examples:"))
		for _, e := range cmd.Examples {
			o.Println("  " + e)
		}
	}
}

func showVersion() {
	o := ui.Out
	adbVersion, adbPath := "Not found", "Not found"
	if c := adb.Candidates(); len(c) > 0 {
		if v, _ := adb.VersionInfo(c[0].Path); v != "" {
			adbVersion = v
		}
		adbPath = tildeHome(c[0].Path) + " " + o.Dim("("+c[0].Source+")")
	}
	G, Y, R, C, B := o.Green, o.Yellow, o.Red, o.Cyan, func(s string) string { return o.Paint("\033[34m", s) }
	key := func(s string) string { return o.Paint("\033[1m\033[33m", s) }
	lines := []string{
		"",
		"                                        " + o.Bold("ak") + " " + G(Version) + " - Released on " + ReleaseDate,
		"        " + G("::") + "                   " + G(".::") + "        ------------------------------------",
		"       " + G(":#*+.") + "                " + G(":+*#.") + "       " + key("ADB Version:") + " " + adbVersion,
		"        " + G(":**+:") + "    " + G("......") + "    " + G("-+**.") + "        " + key("ADB Path:") + " " + adbPath,
		"         " + G(".*+=---::::::::---+*+") + "          " + key("Author:") + " Claude Hwang",
		"        " + Y(":-=----:--------:---==-.") + "        " + key("License:") + " MIT",
		"      " + Y(":++=---==============---=+=.") + "      " + key("Language:") + " Go",
		"    " + R(".+*+=+*%#++++++++++++++##+=+**-") + "     " + key("Supported OS:") + " macOS, Linux, Windows",
		"   " + R(".****+%@@%+++++++++++++*@@@#+**#+") + "    " + key("Repository:") + " https://github.com/luminousvault/homebrew-adb-extensions",
		"   " + C("*#*****#*+++++++++++++++*##*****#=") + "   " + key("Purpose:") + " ADB extensions kit - Essential ADB utilities",
		"  " + B("-%#*****++++++++++++++++++++****##%.") + "  " + key("Features:") + " APK install, Device management, App control",
		"",
	}
	for _, l := range lines {
		o.Println(l)
	}
}

// q quotes a package name, file name or path inside a message so it stands
// apart from the words around it: Installed 'app.apk' on Pixel 7.
func q(s string) string { return "'" + s + "'" }
