package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var errSizeMismatch = errors.New("the copy's size differs from the source; the existing item was kept")

// pollInterval is how often a copy's destination size is read for progress.
var pollInterval = 300 * time.Millisecond

// progress is what the status line shows while a task runs.
type progress struct {
	index, count int    // current item (1-based) and how many
	name         string // current item
	done, total  int64  // bytes of the current item; total < 0 when unknown (a spinner only)
}

// transfer copies src on from to dst on to: adb pull or adb push between
// the device and the computer, cp -r on one device, a file copy on the
// computer (files-design.md §6).
func transfer(ctx context.Context, from FS, src string, to FS, dst string) error {
	fd, fromDevice := from.(*deviceFS)
	td, toDevice := to.(*deviceFS)
	switch {
	case fromDevice && toDevice && fd == td:
		return fd.copyWithin(ctx, src, dst)
	case fromDevice && !toDevice:
		return adbTransfer(ctx, fd.serial, "pull", src, dst)
	case !fromDevice && toDevice:
		if err := adbTransfer(ctx, td.serial, "push", src, dst); err != nil {
			return err
		}
		return td.mkdirs(localDirs(src, dst)) // adb push skips empty folders
	case fromDevice && toDevice:
		return errors.New("copying between two devices is not supported")
	}
	return copyLocal(ctx, src, dst) // the computer; in tests a local folder stands in for the device
}

// adbTransfer runs adb pull or push. adb prints "adb: error: ..." on failure;
// older versions exit 0 even then.
func adbTransfer(ctx context.Context, serial, verb, src, dst string) error {
	out, err := runAdb(ctx, "-s", serial, verb, src, dst)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// adb's own error lines start with "adb: error:"; "error:" elsewhere may
	// be part of a file name. Old adb exits 0 even on errors, so look for
	// the line as well as the exit code.
	for _, line := range strings.Split(out, "\n") {
		if msg, ok := strings.CutPrefix(strings.TrimSpace(line), "adb: error: "); ok {
			return errors.New(firstLines(msg))
		}
	}
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(firstLines(msg))
	}
	return nil
}

// localDirs lists the folders at or under src as device paths under dst
// (none when src is a file).
func localDirs(src, dst string) []string {
	var dirs []string
	_ = filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
		if err != nil || !de.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		dirs = append(dirs, path.Join(dst, filepath.ToSlash(rel)))
		return nil
	})
	return dirs
}

// progressSize is how much of a copy has arrived, the cheapest way the
// storage knows: one du on the device instead of a stat per file.
func progressSize(f FS, p string) (int64, error) {
	if q, ok := f.(interface{ progressSize(string) (int64, error) }); ok {
		return q.progressSize(p)
	}
	b, _, err := f.Usage(p)
	return b, err
}

// pollEvery is how often a copy's progress is read: pollInterval for a
// file, 1 to 3 seconds for a folder by its number of files.
func pollEvery(u usage) time.Duration {
	if u.files <= 1 {
		return pollInterval
	}
	return min(time.Second+time.Duration(u.files)*time.Millisecond, 3*time.Second)
}

// pollSize reports the size of dst on to every pollInterval until stop is
// closed. report gets the bytes copied so far, capped at want.
func pollSize(to FS, dst string, want int64, every time.Duration, report func(int64)) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if n, err := progressSize(to, dst); err == nil && n >= 0 {
					report(min(n, want))
				}
			}
		}
	}()
	return func() { close(done); wg.Wait() }
}

// copyOp is one item to copy and its name at the destination.
type copyOp struct {
	src       Entry
	name      string // name in the destination folder
	conflict  bool   // name exists there; resolved into overwrite, skip or a new name
	overwrite bool
	skip      bool
	dstDir    bool   // the existing destination is a folder
	existing  string // its path: it may differ from name in case or normalization
}

// copyJob copies (or, cut, moves) the ops into a folder: from one storage
// to the other, or within one.
type copyJob struct {
	from, to FS
	dstDir   string
	inDir    func(string) (Entry, bool) // names in dstDir when it was planned (send's lookup)
	ops      []copyOp
	cut      bool // move: the original goes once the copy is complete
}

// sameFS reports whether the job stays on one storage (a paste within the
// device or within the computer).
func (j *copyJob) sameFS() bool { return j.from == j.to }

type failure struct{ name, msg string }

type copyResult struct {
	done     []Entry  // sources that were copied or moved
	doneAs   []string // their names at the destination
	todo     int      // items to copy (not skipped)
	failed   []failure
	skipped  int
	canceled bool
}

func (j *copyJob) run(ctx context.Context, report func(progress)) copyResult {
	defer flushMedia(j.from, j.to)
	var res copyResult
	var todo []copyOp
	for _, op := range j.ops {
		if op.skip {
			res.skipped++
			continue
		}
		todo = append(todo, op)
	}
	res.todo = len(todo)
	// Sizes first, for byte progress and to check a copy before it replaces
	// anything.
	sizes := make([]usage, len(todo))
	for i, op := range todo {
		report(progress{index: i + 1, count: len(todo), name: op.src.Name, total: -1})
		sizes[i] = usage{op.src.Size, 1}
		if j.cut && j.sameFS() {
			continue // a rename: nothing to measure, nothing to wait for
		}
		if op.src.Dir || op.src.Size < 0 {
			b, n, err := j.from.Usage(op.src.Path)
			if err != nil {
				b, n = -1, -1
			}
			sizes[i] = usage{b, n}
		}
		if ctx.Err() != nil {
			res.canceled = true
			return res
		}
	}
	for i, op := range todo {
		if ctx.Err() != nil {
			res.canceled = true
			break
		}
		p := progress{index: i + 1, count: len(todo), name: op.src.Name, total: sizes[i].bytes}
		report(p)
		err := j.one(ctx, op, sizes[i], func(n int64) {
			q := p
			q.done = n
			report(q)
		})
		if err == nil {
			res.done = append(res.done, op.src) // finished, even if Esc came meanwhile
			res.doneAs = append(res.doneAs, op.name)
		} else if ctx.Err() == nil {
			res.failed = append(res.failed, failure{shown(op.src.Name), err.Error()})
		}
		if ctx.Err() != nil {
			res.canceled = true
			break
		}
	}
	return res
}

// usage is the size and file count of an item; -1 when unknown.
type usage struct{ bytes, files int64 }

var errExists = errors.New("already exists")

var (
	errInSource   = errors.New("can't replace a folder that holds what is pasted")
	errNoSize     = errors.New("couldn't check the size; the original was kept")
	errIntoItself = errors.New("can't paste a folder into itself")
)

// one copies (or moves) a single item. It never deletes anything it did not
// make, except the item an overwrite replaces, and that only after the new
// copy is complete: the copy goes to a temporary name next to it first. A
// move deletes its original only when the copy has the same size and file
// count. On failure or cancel only what this copy wrote is removed.
func (j *copyJob) one(ctx context.Context, op copyOp, size usage, report func(int64)) error {
	dst := j.to.Join(j.dstDir, op.name)
	existing := op.existing
	if existing == "" {
		existing = dst
	}
	if op.overwrite && j.sameFS() && holds(j.to, existing, op.src.Path) {
		return errInSource // replacing /x/A with /x/A/A would delete what is pasted
	}
	if op.src.Dir && j.sameFS() && holds(j.to, op.src.Path, j.dstDir) {
		return errIntoItself // paste checks this too; a copy into itself never ends
	}
	target := dst // where the new copy is written
	if op.overwrite {
		var err error
		if target, err = j.freeName(ctx, ".ak-tmp-"+op.name); err != nil {
			return err
		}
	} else if _, err := j.to.Stat(dst); err == nil {
		return errExists // appeared since the listing: never write over it
	}
	if j.cut && j.sameFS() {
		err := j.to.Rename(op.src.Path, target) // mv: the item itself, at once
		switch {
		case err == nil && op.overwrite:
			if err = j.replace(ctx, target, existing, dst); err != nil {
				_ = j.to.Rename(target, op.src.Path) // put the original back
			}
			return err
		case err == nil:
			return nil
		case !errors.Is(err, syscall.EXDEV):
			return err
		}
		// Another volume of the computer: copy, check, then delete below.
	}
	var stop func()
	if size.bytes > 0 {
		stop = pollSize(j.to, target, size.bytes, pollEvery(size), report)
	}
	err := transfer(ctx, j.from, op.src.Path, j.to, target)
	if stop != nil {
		stop()
	}
	if err == nil && (op.overwrite || j.cut) {
		switch {
		case j.cut && size.bytes < 0:
			err = errNoSize // a move deletes its original only after a size check
		default:
			err = j.check(target, size) // before anything is replaced or deleted
		}
	}
	if err == nil && op.overwrite {
		err = j.replace(ctx, target, existing, dst)
		target = "" // in place now: not ours to remove
	}
	if err != nil {
		if target != "" {
			_ = j.to.Remove(target) // only what this copy wrote
		}
		return err
	}
	if j.cut {
		if err := j.from.Remove(op.src.Path); err != nil {
			return fmt.Errorf("copied, but couldn't delete the original: %w", err)
		}
	}
	return nil
}

// holds reports whether path p is root or inside it, by the items
// themselves (symbolic links and aliases such as /sdcard resolved).
func holds(f FS, root, p string) bool {
	real := realPaths(f, root, p)
	return under(f, real[1], real[0])
}

// realPaths resolves symbolic links and aliases: one shell call on the
// device, EvalSymlinks on the computer. "" for a path that can't be resolved.
func realPaths(f FS, paths ...string) []string {
	if r, ok := f.(interface{ realPaths([]string) []string }); ok {
		return r.realPaths(paths)
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			out[i], _ = filepath.Abs(real)
		}
	}
	return out
}

// check compares a copy with its source's size; with an unknown size it
// only needs to exist.
func (j *copyJob) check(p string, want usage) error {
	b, n, err := j.to.Usage(p)
	switch {
	case err != nil:
		return errSizeMismatch
	case want.bytes >= 0 && (b != want.bytes || n != want.files):
		return errSizeMismatch
	}
	return nil
}

// replace puts the complete copy tmp in place of dst: dst is renamed aside,
// tmp takes its name, and only then the old item is deleted. If a step
// fails, dst is put back.
func (j *copyJob) replace(ctx context.Context, tmp, existing, dst string) error {
	old, err := j.freeName(ctx, ".ak-old-"+j.to.Base(dst))
	if err != nil {
		return err
	}
	if err := j.to.Rename(existing, old); err != nil {
		return fmt.Errorf("couldn't replace the existing item: %w", err)
	}
	if err := j.to.Rename(tmp, dst); err != nil {
		_ = j.to.Rename(old, existing)
		return fmt.Errorf("couldn't replace the existing item: %w", err)
	}
	if err := j.to.Remove(old); err != nil {
		return fmt.Errorf("replaced, but couldn't delete the old item %s: %w", old, err)
	}
	return nil
}

// freeName is a name in the destination folder that nothing uses.
func (j *copyJob) freeName(ctx context.Context, name string) (string, error) {
	p := j.to.Join(j.dstDir, name)
	for n := 2; n < 1000; n++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		_, err := j.to.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return p, nil
		case err != nil: // the device went away, adb failed: don't guess
			return "", fmt.Errorf("couldn't check %s: %w", p, err)
		}
		p = j.to.Join(j.dstDir, fmt.Sprintf("%s-%d", name, n))
	}
	return "", fmt.Errorf("no free temporary name for %s", name)
}

// tempDir is where device files opened with Enter are pulled. It is emptied
// when ak files starts, not when it ends: the app showing a file may still
// be reading it.
func tempDir() string {
	if cache, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cache, "ak", "files") // per user: /tmp is shared on Linux
	}
	return filepath.Join(os.TempDir(), "ak-files")
}

// cleanTempDir removes the folders of files opened more than a day ago.
// Newer ones stay: another ak files may have just opened them.
func cleanTempDir(dir string, now time.Time) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, de := range des {
		if fi, err := de.Info(); err == nil && now.Sub(fi.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(dir, de.Name()))
		}
	}
}

// mediaForgetter is a storage that drops index entries of deleted folders
// (the device: MediaStore keeps rows for folders removed from the shell).
type mediaForgetter interface{ forgetRemoved() }

// flushMedia lets each storage drop the index entries of folders it deleted
// since the last flush, in one call per storage.
func flushMedia(list ...FS) {
	for _, f := range list {
		if m, ok := f.(mediaForgetter); ok {
			m.forgetRemoved()
		}
	}
}
