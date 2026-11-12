package cli

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

const pullRecentLimit = 300

var cmdPull = &Command{
	Name: "pull", Desc: "Pull recent files from device",
	Usage: "pull [directory]", Help: "Pull recent files from device",
	Flags: helpFlags, Run: runPull,
}

func showHelpPull() {
	say("%s%sUsage:%s ak pull [-h|--help] [directory]", Cyan, Bold, NC)
	say("")
	say("Description: Lists recently modified files on the device (like the Files app")
	say("'Recents' tab, via MediaStore), lets you pick one or more interactively,")
	say("then pulls them with adb. Files are saved with their original names.")
	say("")
	say("%s%sExamples:%s", Cyan, Bold, NC)
	say("  ak pull                  # Pull selected files into the current directory")
	say("  ak pull /path/to/folder  # Pull into the given directory (created if needed)")
	say("")
	say("Notes:")
	say("  - Shows the %d most recently modified files (newest first).", pullRecentLimit)
	say("  - Existing local files with the same name are overwritten.")
	say("  - Selected files sharing the same name are saved with a ' (N)' suffix.")
	say("")
	say("Options:")
	say("  -h, --help  Show this help message")
	say("")
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

func confirmCreateDir(dest string) {
	say("%sDirectory does not exist:%s %s", Yellow, NC, dest)
	sayf("Create it? [y/N] ")
	reply, _ := stdin.ReadString('\n')
	switch strings.TrimRight(reply, "\r\n") {
	case "y", "Y", "yes", "YES":
	default:
		say("Aborted.")
		exit(1)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		say("%s Cannot create directory: %s", Error, dest)
		exit(1)
	}
}

func runPull(args []string) {
	args = parseLeading("pull", args, showHelpPull, nil, true)
	if len(args) > 1 {
		say("%s Too many arguments", Error)
		say("Try 'ak pull --help' for more information.")
		exit(1)
	}
	dest := "."
	if len(args) == 1 && args[0] != "" {
		dest = args[0]
	}
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		confirmCreateDir(dest)
	}

	serial := selectDevice()
	query := "content query --uri content://media/external/file" +
		" --projection _data:date_modified:_size" +
		` --where "(format IS NULL OR format != 12289) AND _size > 0 AND _data NOT LIKE '%/Android/%' AND _data NOT LIKE '%/.%'"` +
		" --sort 'date_modified DESC' 2>&1 | head -n " + strconv.Itoa(pullRecentLimit)
	raw := adb.Shell(serial, query)
	files := parseRecentFiles(raw)
	if len(files) == 0 {
		if raw == "" || strings.Contains(raw, "No result found") {
			say("%s No recent files found on device.", Error)
		} else {
			say("%s Failed to query recent files from device:", Error)
			lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
			say("%s", strings.Join(lines[:min(5, len(lines))], "\n"))
		}
		exit(1)
	}

	items := make([]string, len(files))
	for i, f := range files {
		items[i] = path.Base(f.path) + "\t(" + fileMeta(f.path, f.size) + ")"
	}
	r := pick(tui.Options{Multi: true, Prompt: "Select files to pull",
		Location: []string{"Recent files (newest first)"}, Items: items})
	if len(r.Indices) == 0 {
		say("%sNo files selected.%s", Yellow, NC)
		exit(0)
	}

	used := map[string]bool{}
	ok, fail := 0, 0
	for _, i := range r.Indices {
		remote := files[i].path
		say("")
		base := uniqueName(path.Base(remote), used)
		if base != path.Base(remote) {
			say("%sWARNING: Duplicate file name, saving as:%s %s", Yellow, NC, base)
		}
		used[base] = true
		say("%s==> Pulling:%s %s", Yellow, NC, path.Base(remote))
		if err := adb.Default.Passthrough("-s", serial, "pull", remote, strings.TrimSuffix(dest, "/")+"/"+base); err == nil {
			ok++
		} else {
			fail++
			say("%sERROR: Failed to pull:%s %s", Red, NC, remote)
		}
	}
	say("")
	if fail > 0 {
		say("%sDone:%s %d pulled, %s%d failed%s", Green, NC, ok, Red, fail, NC)
		exit(1)
	}
	say("%sDone:%s %d file(s) pulled to %s", Green, NC, ok, dest)
}
