package cli

import (
	"golang.org/x/text/unicode/norm"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

const pushDefaultDest = "/sdcard/Download"

var cmdPush = &Command{
	Name: "push", Summary: "Copy files from here to a device", Args: "[file]...",
	Long: "Copy files to the device, into " + pushDefaultDest + " unless -d gives another directory\n" +
		"(created if it does not exist). Files already on the device are never overwritten: the\n" +
		"copy gets a ' (N)' suffix. Without files, pick from the current directory (newest first).\n" +
		"Only the main user's storage (user 0) is reachable; adb cannot access a work profile.",
	ArgHelp: [][2]string{{"file", "Files to push (directories are not supported)"}},
	Flags: []Flag{
		{"-d", "--dest", "DIR", "Device directory to push into (default: " + pushDefaultDest + ")"},
	},
	Examples: []string{"ak push", "ak push report.pdf", "ak push *.png -d /sdcard/Pictures", "ak push notes.txt --all"},
	Device:   manyDevices, Run: runPush,
}

// shellArg quotes a path for a shell only when it needs it, so a hint can
// be pasted as is.
func shellArg(p string) string {
	if strings.Trim(p, "/._-~abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
		return files.ShellQuote(p)
	}
	return p
}

type localFile struct {
	path string
	size int64
}

// pushArgs checks that every argument is an existing regular file.
func pushArgs(args []string) ([]localFile, error) {
	var files []localFile
	for _, a := range args {
		st, err := os.Stat(a)
		switch {
		case err != nil:
			return nil, failErr("Check the path, or run 'ak push' without files to pick from the current directory", "No such file: %s", q(a))
		case st.IsDir():
			return nil, failErr("Pass the files inside it instead, e.g. ak push "+shellArg(a)+"/*", "%s is a directory; ak push copies files only", q(a))
		case !st.Mode().IsRegular():
			return nil, failErr("", "Not a regular file: %s", q(a))
		}
		files = append(files, localFile{a, st.Size()})
	}
	return files, nil
}

// filesHere lists the regular, non-hidden files of the current directory, newest first.
func filesHere() []localFile {
	entries, _ := os.ReadDir(".")
	type entry struct {
		localFile
		mod time.Time
	}
	var es []entry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		if info, err := e.Info(); err == nil {
			es = append(es, entry{localFile{e.Name(), info.Size()}, info.ModTime()})
		}
	}
	sort.SliceStable(es, func(i, j int) bool { return es[i].mod.After(es[j].mod) })
	files := make([]localFile, len(es))
	for i, e := range es {
		files[i] = e.localFile
	}
	return files
}

func runPush(c *Ctx) error {
	dest := pushDefaultDest
	if c.Bool("--dest") {
		dest = c.String("--dest")
	}
	if !strings.HasPrefix(dest, "/") {
		return usageErr(c.Cmd, "-d takes an absolute device path, e.g. /sdcard/Pictures: %s", q(dest))
	}
	dest = path.Clean(dest)

	list, err := pushArgs(c.Args)
	if err != nil {
		return err
	}
	var here []localFile
	if len(list) == 0 {
		if here = filesHere(); len(here) == 0 {
			return failErr("Pass the files to push", "No files in the current directory")
		}
	}
	release := holdPickers() // device picker, then file picker, on one screen
	defer release()
	serials, err := c.Devices()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		items := make([]string, len(here))
		for i, f := range here {
			items[i] = f.path + "\t(" + files.HumanSize(f.size) + ")"
		}
		wd, _ := os.Getwd()
		r, err := pick(tui.Options{Multi: true, Prompt: "Select files to push",
			Location: []string{"Files in " + wd + " (newest first)"}, Items: items},
			"Pass the files to push")
		if err != nil {
			return err
		}
		for _, i := range r.Indices {
			list = append(list, here[i])
		}
	}
	release()

	failed := 0
	for _, s := range serials {
		failed += pushTo(s, list, dest)
	}
	if failed > 0 {
		if total := len(list) * len(serials); total > 1 {
			ui.Errorf("%d of %d files failed", failed, total)
		}
		return errReported
	}
	return nil
}

// pushTo pushes files into dest on one device, creating dest when missing and
// renaming instead of overwriting. It returns how many files failed.
func pushTo(s string, list []localFile, dest string) int {
	name := deviceName(s)
	// The names already there, so nothing is written over. A missing dest is
	// made; one that can't be listed is an error, not "missing".
	qd := files.ShellQuote(dest)
	out, _ := adb.Combined("-s", s, "shell", "if [ ! -e "+qd+" ] && [ ! -L "+qd+" ]; then echo ak:missing; "+
		"elif [ ! -d "+qd+" ]; then echo ak:notdir; else cd "+qd+" && ls -A && echo ak:listed; fi")
	used := map[string]string{} // lower-case name -> the name on the device (/sdcard ignores case)
	switch lines := strings.Split(strings.TrimRight(out, "\n"), "\n"); {
	case lines[len(lines)-1] == "ak:listed":
		for _, n := range lines[:len(lines)-1] {
			used[files.FoldName(n)] = n
		}
	case strings.TrimSpace(out) == "ak:missing":
		if out, err := adb.Combined("-s", s, "shell", "mkdir -p "+qd); err != nil {
			ui.Errorf("Couldn't create %s on %s", q(dest), name)
			ui.Detail("%s", strings.TrimSpace(out))
			return len(list)
		}
		ui.Step("Created %s on %s", q(dest), name)
	case strings.TrimSpace(out) == "ak:notdir":
		ui.Errorf("%s on %s is not a directory", q(dest), name)
		ui.Hint("Push into a directory with -d")
		return len(list)
	default:
		ui.Errorf("Couldn't list %s on %s", q(dest), name)
		ui.Detail("%s", strings.TrimSpace(out))
		ui.Hint("Without its file names ak could overwrite them; push somewhere else with -d")
		return len(list)
	}
	sdk, _ := strconv.Atoi(deviceProps(s)["ro.build.version.sdk"])

	ok, failed := 0, 0
	for _, f := range list {
		// Composed on the device: phone apps show a macOS (NFD) name as loose jamo.
		base := norm.NFC.String(filepath.Base(f.path))
		target := files.UniqueName(base, false, func(n string) bool { _, ok := used[files.FoldName(n)]; return ok })
		there := used[files.FoldName(base)]
		used[files.FoldName(target)] = target
		if target != base {
			ui.Warn("%s already exists on %s, pushing as %s", q(path.Join(dest, there)), name, q(target))
		}
		remote := path.Join(dest, target)
		start := time.Now()
		sp := ui.Spin("Pushing %s to %s", q(base), name)
		local := f.path
		if abs, err := filepath.Abs(local); err == nil {
			local = abs // "-x.txt" would be read as an adb option
		}
		out, err := adb.Combined("-s", s, "push", local, remote)
		sp.Stop()
		if err != nil {
			failed++
			ui.Errorf("Couldn't push %s to %s", q(f.path), name)
			ui.Detail("%s", strings.TrimSpace(out))
			continue
		}
		ok++
		// From Android 11 (API 30) writes go through FUSE and MediaStore sees them at once.
		if sdk > 0 && sdk < 30 {
			uri := (&url.URL{Scheme: "file", Path: remote}).String()
			scan := adb.Shell(s, "am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d "+files.ShellQuote(uri))
			debuglog.Logf("media scan %s: %s", uri, strings.TrimSpace(scan))
		}
		ui.Success("Pushed %s to %s:%s %s", q(f.path), name, remote,
			ui.Err.Dim("("+files.HumanSize(f.size)+", "+elapsed(start)+")"))
	}
	if ok > 1 {
		ui.Success("Pushed %d files to %s:%s", ok, name, dest)
	}
	return failed
}
