package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
)

// deviceFS is the device's storage, reached through the adb shell.
type deviceFS struct {
	serial, name string

	mu      sync.Mutex
	removed []string // folders deleted since forgetRemoved
}

func (d *deviceFS) Side() Side                   { return Device }
func (d *deviceFS) Label() string                { return d.name }
func (d *deviceFS) Join(dir, name string) string { return path.Join(dir, name) }
func (d *deviceFS) Dir(p string) string          { return path.Dir(p) }
func (d *deviceFS) Base(p string) string         { return path.Base(p) }

// ShellQuote quotes s for the device shell (spaces, Hangul, ' and $ are safe).
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// okMark ends the output of a shell command that succeeded. adb before
// Android 7 (no shell protocol) always exits 0, so the exit code alone is
// not enough.
const okMark = "ak-ok"

// run runs a shell command and fails with its output unless it printed okMark.
func (d *deviceFS) run(cmd string) error {
	out, _ := adb.Combined("-s", d.serial, "shell", cmd+" && echo "+okMark)
	return checkOK(out)
}

func checkOK(out string) error {
	out = strings.TrimRight(out, "\n")
	if out == okMark || strings.HasSuffix(out, "\n"+okMark) {
		return nil
	}
	return errors.New(firstLines(out))
}

// firstLines keeps an error message to its first lines, without the
// "stat: " style tool prefixes doubled up.
func firstLines(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "failed"
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, " / ")
}

// statFormat is "type|size|mtime|path". The path is last, so cutting at the
// first three '|' keeps names that contain '|'.
const statFormat = `'%F|%s|%Y|%n'`

// accessMark starts the second part of listCmd's output: what the shell may
// do with each entry, "rwxl|path" (any of read, write, run, link).
const accessMark = "ak:access"

// listCmd lists dir in one adb call (~0.1s per folder). -L follows symlinks
// so linked folders (/sdcard) open like folders.
func listCmd(dir string) string {
	q := ShellQuote(dir)
	prefix := ShellQuote(strings.TrimSuffix(dir, "/")) // "/" -> '' so the globs are /* and /.*
	return "if [ ! -d " + q + " ]; then echo ak:notdir; " +
		"elif [ ! -r " + q + " ] || [ ! -x " + q + " ]; then echo ak:denied; " +
		"else stat -L -c " + statFormat + " " + prefix + "/* " + prefix + "/.* 2>&1; " +
		// The shell's own tests ask the file system (FUSE on /sdcard does not
		// go by the mode bits); they are built in, so no process per file.
		"echo " + accessMark + "; for f in " + prefix + "/* " + prefix + "/.*; do " +
		`[ -e "$f" ] || [ -L "$f" ] || continue; a=; [ -r "$f" ] && a=r; [ -w "$f" ] && a=${a}w; ` +
		`[ -x "$f" ] && a=${a}x; [ -L "$f" ] && a=${a}l; echo "$a|$f"; done; fi`
}

func (d *deviceFS) List(dir string) ([]Entry, error) {
	out := adb.Out("-s", d.serial, "shell", listCmd(dir))
	return parseStat(out, dir)
}

// parseStat parses listCmd output. Error lines for the globs that matched
// nothing (an empty folder) are ignored; lines for names with a newline,
// which stat splits over two lines, are dropped.
func parseStat(out, dir string) ([]Entry, error) {
	switch strings.TrimSpace(out) {
	case "ak:notdir":
		return nil, fs.ErrNotExist
	case "ak:denied":
		return nil, fs.ErrPermission
	}
	// The marker is a line of its own; a name may end in the same text.
	access := ""
	if rest, ok := strings.CutPrefix(out, accessMark+"\n"); ok {
		out, access = "", rest
	} else if i := strings.Index(out, "\n"+accessMark+"\n"); i >= 0 {
		out, access = out[:i+1], out[i+len(accessMark)+2:]
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	var list []Entry
	broken := false // the previous entry's name went on past its line
	denied := false
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		e, ok := parseStatLine(line)
		if ok && strings.HasPrefix(e.Path, prefix) && !strings.Contains(e.Path[len(prefix):], "/") {
			if e.Name != "." && e.Name != ".." {
				list = append(list, e)
			}
			broken = false
			continue
		}
		if strings.HasPrefix(line, "stat: ") {
			if strings.HasSuffix(line, "Permission denied") &&
				(strings.HasSuffix(line, "/*': Permission denied") || strings.HasSuffix(line, "/.*': Permission denied")) {
				denied = true
			}
			continue // a glob that matched nothing, or one entry stat may not read
		}
		// The rest of a name with a newline: drop the entry it belongs to.
		if !broken && len(list) > 0 {
			list = list[:len(list)-1]
		}
		broken = true
	}
	if denied && len(list) == 0 {
		return nil, fs.ErrPermission
	}
	applyAccess(list, access)
	return list, nil
}

// applyAccess sets what the shell may do with each entry from the "rwxl|path"
// lines of listCmd. An entry without a line keeps the zero value (ordinary).
func applyAccess(list []Entry, access string) {
	flags := map[string]string{}
	for _, line := range strings.Split(access, "\n") {
		if f, p, ok := strings.Cut(line, "|"); ok && strings.Trim(f, "rwxl") == "" {
			flags[p] = f
		}
	}
	for i := range list {
		f, ok := flags[list[i].Path]
		if !ok {
			continue
		}
		e := &list[i]
		e.NoRead = !strings.Contains(f, "r") || (e.Dir && !strings.Contains(f, "x"))
		e.ReadOnly = !e.NoRead && !strings.Contains(f, "w")
		e.Exec = !e.Dir && strings.Contains(f, "x")
		e.Link = strings.Contains(f, "l")
	}
}

// parseStatLine parses one "type|size|mtime|path" line.
func parseStatLine(line string) (Entry, bool) {
	f := strings.SplitN(line, "|", 4) // the name may hold '|'
	if len(f) != 4 || f[3] == "" {
		return Entry{}, false
	}
	size, err1 := strconv.ParseInt(f[1], 10, 64)
	mtime, err2 := strconv.ParseInt(f[2], 10, 64)
	if err1 != nil || err2 != nil {
		return Entry{}, false
	}
	e := Entry{Name: path.Base(f[3]), Path: f[3], Dir: f[0] == "directory", Size: size, ModTime: time.Unix(mtime, 0)}
	if e.Dir {
		e.Size = 0
	}
	return e, true
}

func (d *deviceFS) Stat(p string) (Entry, error) {
	out := adb.Out("-s", d.serial, "shell", "stat -L -c "+statFormat+" "+ShellQuote(p)+" 2>&1")
	line, _, _ := strings.Cut(out, "\n")
	if e, ok := parseStatLine(line); ok {
		return e, nil
	}
	if strings.Contains(out, "No such file") {
		return Entry{}, fs.ErrNotExist
	}
	return Entry{}, errors.New(firstLines(out))
}

func (d *deviceFS) Mkdir(p string) error { return d.run("mkdir " + ShellQuote(p) + " 2>&1") }

// Remove deletes p with rm -rf, then checks that it is gone: -f hides errors.
// A deleted folder is remembered for forgetRemoved.
func (d *deviceFS) Remove(p string) error {
	q := ShellQuote(p)
	out, _ := adb.Combined("-s", d.serial, "shell", "[ -d "+q+" ] && echo ak-dir; rm -rf "+q+" 2>&1; [ ! -e "+q+" ] && [ ! -L "+q+" ] && echo "+okMark)
	wasDir := strings.HasPrefix(out, "ak-dir\n")
	out = strings.TrimPrefix(out, "ak-dir\n")
	if wasDir && checkOK(out) == nil {
		d.mu.Lock()
		d.removed = append(d.removed, p)
		d.mu.Unlock()
	}
	if err := checkOK(out); err != nil {
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out), okMark)) == "" {
			return fmt.Errorf("couldn't delete %s", p)
		}
		return err
	}
	return nil
}

// Rename fails when to exists: mv would move into a folder of that name, or
// write over a file. The check and the mv are one shell call.
func (d *deviceFS) Rename(from, to string) error {
	qt := ShellQuote(to)
	return d.run("if [ -e " + qt + " ] || [ -L " + qt + " ]; then echo " + ShellQuote(to+": already exists") + "; exit 1; fi; " +
		"mv " + ShellQuote(from) + " " + qt + " 2>&1")
}

// Usage sums the regular files under p (p itself if it is a file), the same
// way localFS does, so a copy can be checked. Folder entries themselves are
// not counted: their sizes differ between file systems.
func (d *deviceFS) Usage(p string) (bytes, files int64, err error) {
	q := ShellQuote(p)
	out := adb.Out("-s", d.serial, "shell", "[ -e "+q+" ] && { find "+q+" -type f -exec stat -c %s {} + 2>/dev/null; echo "+okMark+"; }")
	if checkOK(out) != nil {
		return -1, -1, fs.ErrNotExist
	}
	for _, line := range strings.Split(out, "\n") {
		if n, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64); err == nil {
			bytes += n
			files++
		}
	}
	return bytes, files, nil
}

// runAdb runs adb until it ends or ctx is canceled, and returns stdout and
// stderr together. Real adb goes through adb.Start so it can be killed; the
// test fake, which cannot stream, runs through adb.Default.
func runAdb(ctx context.Context, args ...string) (string, error) {
	cmd, out, err := adb.Start(args...)
	if errors.Is(err, adb.ErrNoStreaming) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return adb.Combined(args...)
	}
	if err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		case <-done:
		}
	}()
	b, _ := io.ReadAll(out)
	err = cmd.Wait()
	close(done)
	if ctx.Err() != nil {
		return string(b), ctx.Err()
	}
	return strings.ReplaceAll(string(b), "\r", ""), err
}

// Recent lists the newest files in MediaStore (the recent view, `t`).
func (d *deviceFS) Recent() ([]Entry, error) {
	raw := adb.Shell(d.serial, RecentFilesQuery)
	rows := ParseRecentFiles(raw)
	if len(rows) == 0 && raw != "" && !strings.Contains(raw, "No result found") {
		return nil, errors.New(firstLines(raw))
	}
	FillUnknownSizes(d.serial, rows)
	return recentEntries(rows), nil
}

// mkdirs makes folders (and their parents); adb push skips empty folders.
func (d *deviceFS) mkdirs(dirs []string) error {
	for len(dirs) > 0 {
		n := min(len(dirs), 100) // keep the command line short
		q := make([]string, n)
		for i, p := range dirs[:n] {
			q[i] = ShellQuote(p)
		}
		if err := d.run("mkdir -p " + strings.Join(q, " ") + " 2>&1"); err != nil {
			return err
		}
		dirs = dirs[n:]
	}
	return nil
}

// forgetRemoved drops the MediaStore rows of the folders deleted since the
// last call: MediaStore keeps a folder's row (format 12289) after the shell
// deletes it, so it would still show in gallery and file apps. One content
// call (~1s) for all of them; a failure is only logged, the files are gone
// either way.
func (d *deviceFS) forgetRemoved() {
	d.mu.Lock()
	dirs := d.removed
	d.removed = nil
	d.mu.Unlock()
	where := mediaWhere(dirs)
	if where == "" {
		return
	}
	out, err := adb.Combined("-s", d.serial, "shell", "content delete --uri content://media/external/file --where "+ShellQuote(where))
	debuglog.Logf("forget media rows of %q: %v %s", dirs, err, strings.TrimSpace(out))
}

// mediaWhere matches the MediaStore rows of folders and everything under
// them. Folders outside shared storage have no rows and are left out.
func mediaWhere(dirs []string) string {
	var parts []string
	for _, p := range dirs {
		m, ok := mediaPath(p)
		if !ok {
			continue
		}
		sq := strings.ReplaceAll(m, "'", "''")
		like := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(sq)
		parts = append(parts, "_data='"+sq+"' OR _data LIKE '"+like+"/%' ESCAPE '\\'")
	}
	return strings.Join(parts, " OR ")
}

// Free reads the free space of the file system holding dir (df -k).
func (d *deviceFS) Free(dir string) (int64, error) {
	return parseDF(adb.Out("-s", d.serial, "shell", "df -k "+ShellQuote(dir)+" 2>/dev/null"))
}

// parseDF reads "Available" (1K blocks, the fourth column) from the last
// line of df -k.
func parseDF(out string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if len(lines) < 2 || len(f) < 4 {
		return -1, errors.New("no df output")
	}
	n, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return -1, err
	}
	return n * 1024, nil
}

// progressSize is a copy's size so far for progress bars: one du -s -b for
// a whole folder. It counts folder entries too, so it is near, not equal to,
// Usage; Usage checks the copy.
func (d *deviceFS) progressSize(p string) (int64, error) {
	f := strings.Fields(adb.Out("-s", d.serial, "shell", "du -s -b "+ShellQuote(p)+" 2>/dev/null"))
	if len(f) == 0 {
		return -1, fs.ErrNotExist
	}
	return strconv.ParseInt(f[0], 10, 64)
}
