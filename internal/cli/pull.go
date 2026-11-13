package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

const pullRecentLimit = 300

var cmdPull = &Command{
	Name: "pull", Summary: "Pick recent files on a device and copy them here", Args: "[directory]",
	Long: "Pick recently modified files on the device (like the Files app 'Recents' tab, via MediaStore)\n" +
		"and pull them. Files keep their names; existing local files are overwritten, and selected\n" +
		"files with the same name get a ' (N)' suffix. Shows the " + strconv.Itoa(pullRecentLimit) + " newest files.",
	ArgHelp:  [][2]string{{"directory", "Where to save (default: current directory; created after confirmation)"}},
	Flags:    []Flag{{"-y", "--yes", "", "Create the destination directory without asking if it does not exist"}},
	Examples: []string{"ak pull", "ak pull ~/Downloads/device", "ak pull new-dir -y"},
	Device:   oneDevice, Run: runPull,
}

// humanSize formats bytes like the Bash version (rounded B/KB, one decimal MB/GB).
func humanSize(size string) string {
	n, err := strconv.ParseInt(size, 10, 64)
	if err != nil || n < 0 || size == "" {
		return "?"
	}
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1048576:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	case n < 1073741824:
		t := (n*10 + 524288) / 1048576
		return fmt.Sprintf("%d.%d MB", t/10, t%10)
	}
	t := (n*10 + 536870912) / 1073741824
	return fmt.Sprintf("%d.%d GB", t/10, t%10)
}

// fileMeta returns "<folder relative to sdcard>, <size>" for the picker suffix.
func fileMeta(p, size string) string {
	dir := path.Dir(p)
	rel := strings.TrimPrefix(dir, "/storage/emulated/0/")
	if rel == "/storage/emulated/0" {
		rel = "sdcard"
	}
	return rel + ", " + humanSize(size)
}

type recentFile struct{ path, size string }

// parseRecentFiles parses `content query` rows:
// "Row: 0 _data=<path>, date_modified=<ts>, _size=<bytes>".
func parseRecentFiles(raw string) []recentFile {
	var files []recentFile
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "Row:") {
			continue
		}
		_, p, ok := strings.Cut(line, " _data=")
		if !ok {
			continue
		}
		if i := strings.Index(p, ", date_modified="); i >= 0 {
			p = p[:i]
		}
		size := line
		if i := strings.LastIndex(line, ", _size="); i >= 0 {
			size = line[i+len(", _size="):]
		}
		if p == "" {
			continue
		}
		files = append(files, recentFile{p, size})
	}
	return files
}

// uniqueName adds " (N)" before the extension when name was already used.
func uniqueName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	name, ext := base, ""
	if i := strings.LastIndex(base, "."); i >= 0 {
		name, ext = base[:i], base[i:]
	}
	for n := 1; ; n++ {
		c := fmt.Sprintf("%s (%d)%s", name, n, ext)
		if !used[c] {
			return c
		}
	}
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
	query := "content query --uri content://media/external/file" +
		" --projection _data:date_modified:_size" +
		` --where "(format IS NULL OR format != 12289) AND _size > 0 AND _data NOT LIKE '%/Android/%' AND _data NOT LIKE '%/.%'"` +
		" --sort 'date_modified DESC' 2>&1 | head -n " + strconv.Itoa(pullRecentLimit)
	raw := adb.Shell(serial, query)
	files := parseRecentFiles(raw)
	if len(files) == 0 {
		if raw == "" || strings.Contains(raw, "No result found") {
			return failErr("", "No recent files found on %s", deviceName(serial))
		}
		lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
		release()
		ui.Errorf("Couldn't query recent files on %s", deviceName(serial))
		ui.Detail("%s", strings.Join(lines[:min(5, len(lines))], "\n"))
		return errReported
	}

	items := make([]string, len(files))
	for i, f := range files {
		items[i] = path.Base(f.path) + "\t(" + fileMeta(f.path, f.size) + ")"
	}
	r, err := pick(tui.Options{Multi: true, Prompt: "Select files to pull",
		Location: []string{"Recent files on " + deviceName(serial) + " (newest first)"}, Items: items}, "")
	if err != nil {
		return err
	}
	release()

	used := map[string]bool{}
	ok, failed := 0, 0
	for _, i := range r.Indices {
		remote := files[i].path
		base := uniqueName(path.Base(remote), used)
		used[base] = true
		if base != path.Base(remote) {
			ui.Warn("Duplicate file name, saving %s as %s", q(path.Base(remote)), q(base))
		}
		local := filepath.Join(dest, base)
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
		ui.Success("Pulled %s %s", q(local), ui.Err.Dim("("+fileMeta(remote, files[i].size)+", "+elapsed(start)+")"))
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
