package cli

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdPull = &Command{
	Name: "pull", Summary: "Pick recent files on a device and copy them here", Args: "[directory]",
	Long: "Pick recently modified files on the device (like the Files app 'Recents' tab, via MediaStore)\n" +
		"and pull them. Files keep their names; a file already in the directory (in any case) is kept\n" +
		"and the pulled one gets a ' (N)' suffix, as do selected files with the same name. Lists up\n" +
		"to the " + strconv.Itoa(files.RecentFilesLimit) + " newest files.\n" +
		"Without a terminal, choose with -l or with a -f that matches one file.",
	ArgHelp: [][2]string{{"directory", "Where to save (default: current directory; created after confirmation)"}},
	Flags: []Flag{
		{"-l", "--latest", "", "Pull the newest file without asking (with -f: the newest match)"},
		{"-f", "--filter", "WORDS", "Only files whose names contain all the words; one match is pulled without asking"},
		{"-y", "--yes", "", "Create the destination directory without asking if it does not exist"},
	},
	Examples: []string{"ak pull", "ak pull ~/Downloads/device", "ak pull new-dir -y", "ak pull -l", "ak pull -l -f Screenshot"},
	Device:   oneDevice, Run: runPull,
}

// fileMeta returns "<folder relative to sdcard>, <size>" for the picker suffix.
func fileMeta(p string, size int64) string {
	dir := path.Dir(p)
	rel := strings.TrimPrefix(dir, "/storage/emulated/0/")
	if rel == "/storage/emulated/0" {
		rel = "sdcard"
	}
	return rel + ", " + files.HumanSize(size)
}

// filterRecentFiles keeps the files whose names match every word of filter (see matchesFilter).
func filterRecentFiles(list []files.RecentFile, filter string) []files.RecentFile {
	var out []files.RecentFile
	for _, f := range list {
		if matchesFilter(path.Base(f.Path), filter) {
			out = append(out, f)
		}
	}
	return out
}

func ensureDir(dest string) error {
	if st, err := os.Stat(dest); err == nil {
		if !st.IsDir() {
			return failErr("", "Not a directory: %s", q(dest))
		}
		return nil
	}
	ok, err := ui.Confirm("Directory "+q(dest)+" does not exist. Create it?", false)
	switch {
	case errors.Is(err, ui.ErrNoInput):
		return failErr("Pass -y to create it", "Directory does not exist: %s", q(dest))
	case err != nil:
		return err
	case !ok:
		return failErr("", "Aborted")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return failErr("", "Cannot create directory %s: %v", q(dest), err)
	}
	return nil
}

func runPull(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	dest := "."
	if len(c.Args) == 1 && c.Args[0] != "" {
		dest = c.Args[0]
	}
	if err := ensureDir(dest); err != nil {
		return err
	}
	release := holdPickers() // device picker, then file picker, on one screen
	defer release()
	serial, err := c.Device()
	if err != nil {
		return err
	}
	tui.ClearHeld()
	sp := ui.Spin("Reading recent files on %s", deviceName(serial))
	raw := adb.Shell(serial, files.RecentFilesQuery) // ~1s even for few rows: `content` starts a JVM
	recent := files.ParseRecentFiles(raw)
	recent = files.FillUnknownSizes(serial, recent) // files MediaStore has not scanned yet
	sp.Stop()
	if len(recent) == 0 {
		if raw == "" || strings.Contains(raw, "No result found") {
			return failErr("", "No recent files found on %s", deviceName(serial))
		}
		lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
		release()
		ui.Errorf("Couldn't query recent files on %s", deviceName(serial))
		ui.Detail("%s", strings.Join(lines[:min(5, len(lines))], "\n"))
		return errReported
	}

	if filter := c.String("--filter"); c.Bool("--filter") {
		recent = filterRecentFiles(recent, filter)
		if len(recent) == 0 {
			return failErr("", "No recent files on %s match %s", deviceName(serial), q(filter))
		}
	}
	chosen := []int{0} // -l, or -f with one match
	if !c.Bool("--latest") && (!c.Bool("--filter") || len(recent) > 1) {
		items := make([]string, len(recent))
		for i, f := range recent {
			items[i] = path.Base(f.Path) + "\t(" + fileMeta(f.Path, f.Size) + ")"
		}
		r, err := pick(tui.Options{Multi: true, Prompt: "Select files to pull",
			Location: []string{"Recent files on " + deviceName(serial) + " (newest first)"}, Items: items},
			"Use -l for the newest file, or -f to narrow it to one")
		if err != nil {
			return err
		}
		chosen = r.Indices
	}
	release()

	// Names already in dest (case and NFC/NFD ignored: macOS and Windows
	// ignore them) and
	// names given in this pull are not written over.
	here := map[string]bool{}
	if des, err := os.ReadDir(dest); err == nil {
		for _, de := range des {
			here[files.FoldName(de.Name())] = true
		}
	}
	used := map[string]bool{}
	taken := func(n string) bool { return here[files.FoldName(n)] || used[files.FoldName(n)] }
	ok, failed := 0, 0
	for _, i := range chosen {
		remote := recent[i].Path
		name := path.Base(remote)
		base := files.UniqueName(name, false, taken)
		switch {
		case base == name:
		case here[files.FoldName(name)]:
			ui.Warn("%s already exists in %s, saving as %s", q(name), q(dest), q(base))
		default:
			ui.Warn("Duplicate file name, saving %s as %s", q(name), q(base))
		}
		used[files.FoldName(base)] = true
		local := filepath.Join(dest, base)
		if strings.HasPrefix(local, "-") { // dest ".": adb would read "-x.txt" as an option
			local = "." + string(filepath.Separator) + local
		}
		start := time.Now()
		sp := ui.Spin("Pulling %s", q(path.Base(remote)))
		out, err := adb.Combined("-s", serial, "pull", remote, local)
		sp.Stop()
		if err != nil {
			failed++
			ui.Errorf("Couldn't pull %s", q(remote))
			ui.Detail("%s", out)
			continue
		}
		ok++
		ui.Success("Pulled %s %s", q(local), ui.Err.Dim("("+fileMeta(remote, recent[i].Size)+", "+elapsed(start)+")"))
	}
	if failed > 0 {
		ui.Errorf("%d of %d files failed", failed, ok+failed)
		return errReported
	}
	if ok > 1 {
		ui.Success("Pulled %d files to %s", ok, q(dest))
	}
	return nil
}
