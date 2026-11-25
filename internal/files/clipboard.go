package files

import (
	"context"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// toClipboard takes the targets of p (3/c copy, 4/x cut). The clipboard
// stays while folders and panes change (a folder's selection does not), so
// the items can be pasted after going elsewhere; a new c or x replaces it,
// so it holds one folder's items at a time. The folder's selection is used
// up.
func (s *state) toClipboard(p *pane, cut bool) {
	if s.blocked() {
		return
	}
	items := s.targets(p)
	if len(items) == 0 {
		return
	}
	s.clip = &clipboard{side: p.fs.Side(), items: items, cut: cut}
	p.selected = map[string]bool{}
}

// paste plans pasting the clipboard into p's folder (6/v): a pull, a push,
// a copy within the device or within the computer, or with a cut a move.
// Names already there are asked about; a folder never goes into itself
// (files-design.md §6).
func (s *state) paste(p *pane) {
	switch {
	case s.blocked():
		return
	case s.clip == nil:
		s.setMsg(msgInfo, "Nothing to paste: 3 copies or 4 cuts items first")
		return
	case p.recent:
		s.setMsg(msgInfo, "Paste in the folder view: 2 shows it")
		return
	}
	from, to := s.panes[s.clip.side].fs, p.fs
	entries, err := to.List(p.dir) // what is there now
	if err != nil {
		s.setMsg(msgError, "Couldn't paste into %s: %s", place(to, p.dir), errText(err))
		return
	}
	items := s.clip.items
	same := from == to
	var real []string // the folder, then each item and its folder, resolved
	if same {
		paths := []string{p.dir}
		for _, it := range items {
			paths = append(paths, it.Path, from.Dir(it.Path))
		}
		real = realPaths(to, paths...)
	}
	push := !same && from.Side() == PC && to.Side() == Device
	inDir := nameLookup(to, p.dir, entries)
	used := map[string]bool{}
	taken := func(n string) bool { _, ok := inDir(n); return ok || used[fold(n)] }
	job := &copyJob{from: from, to: to, dstDir: p.dir, inDir: inDir, cut: s.clip.cut}
	for i, it := range items {
		sameDir := false
		if same {
			item, dir := real[1+2*i], real[2+2*i]
			if it.Dir && (under(to, real[0], item) || under(to, p.dir, it.Path)) { // the folder is the item or inside it
				s.setMsg(msgError, "Couldn't paste %s into itself", ui.Quote(shown(it.Name)))
				return
			}
			sameDir = from.Dir(it.Path) == p.dir || (dir != "" && dir == real[0])
		}
		if sameDir && s.clip.cut {
			continue // moving to where it is: nothing to do
		}
		name := it.Name
		if push {
			name = norm.NFC.String(name) // as 5 does: phones expect composed names
		}
		op := copyOp{src: it, name: name}
		if e, ok := inDir(name); ok && !sameDir && !used[fold(name)] {
			op.conflict, op.dstDir, op.existing = true, e.Dir, e.Path
		} else if sameDir || used[fold(name)] {
			op.name = UniqueName(name, it.Dir, taken) // a copy next to the original: ' (N)'
		}
		used[fold(op.name)] = true
		job.ops = append(job.ops, op)
	}
	if len(job.ops) == 0 {
		s.setMsg(msgInfo, "Already here: nothing to move")
		return
	}
	s.plan, s.pasting, s.ci, s.toAll = job, true, -1, false
	s.nextConflict()
}

// under reports whether path p is root or inside it, by the text of the
// paths ("" never is).
func under(f FS, p, root string) bool {
	if p == "" || root == "" {
		return false
	}
	sep := "/"
	if _, ok := f.(*deviceFS); !ok {
		sep = string(filepath.Separator)
	}
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, sep)+sep)
}

func (s *state) startPaste(job *copyJob) {
	n := 0
	for _, op := range job.ops {
		if !op.skip {
			n++
		}
	}
	if n == 0 {
		s.setMsg(msgInfo, "Nothing pasted: all skipped")
		return
	}
	label := map[bool]string{true: "Moving", false: "Copying"}[job.cut]
	s.pending = &task{label: label, route: pasteRoute(job), cancelable: true,
		run: func(ctx context.Context, report func(progress)) func(*state) {
			res := job.run(ctx, report)
			return func(s *state) { s.finishPaste(job, res) }
		},
		tick: func(s *state) { s.showFinished(job) }}
}

// pasteRoute is where a paste goes: across, the device on the left as for
// 5; within one side, the folder.
func pasteRoute(job *copyJob) string {
	switch {
	case job.sameFS():
		return location(job.to, job.dstDir)
	case job.to.Side() == Device:
		return route(job, true)
	}
	return route(job, false)
}

// finishPaste reports a paste: "Copied 3 files to '~/Downloads'", "Moved
// 'a.txt' to '/sdcard/DCIM' on Pixel". Moved items leave the clipboard.
func (s *state) finishPaste(job *copyJob, res copyResult) {
	for _, p := range s.panes {
		if job.cut && p.fs == job.from {
			p.recentList = withoutPaths(p.recentList, res.done)
		}
		if (p.fs == job.to || (job.cut && p.fs == job.from)) && !p.recent {
			s.reload(p)
		}
		if p.fs == job.to || (job.cut && p.fs == job.from) {
			readFree(p)
		}
	}
	if job.cut && s.clip != nil {
		s.clip.items = withoutPaths(s.clip.items, res.done)
		if len(s.clip.items) == 0 {
			s.clip = nil
		}
	}
	verb, past, ing := "copy", "Copied", "copying"
	if job.cut {
		verb, past, ing = "move", "Moved", "moving"
	}
	s.reportCopy(job, res, verb, past, ing)
}

// clipParts is the clipboard line's text: "Copy 3 files (12.0 MB) from " and
// the source, "Pixel:/sdcard/DCIM".
func (s *state) clipParts() (head, src string) {
	verb := "Copy"
	if s.clip.cut {
		verb = "Cut"
	}
	return verb + " " + clipItems(s.clip.items) + " from ", clipSource(s.panes[s.clip.side].fs, s.clip.items)
}

// clipItems is "2 files (433.2 MB)"; with folders or unknown sizes the size
// is left out (it is measured when pasting).
func clipItems(list []Entry) string {
	var total int64
	for _, e := range list {
		if e.Dir || e.Size < 0 {
			return countItems(list)
		}
		total += e.Size
	}
	return countItems(list) + " (" + HumanSize(total) + ")"
}

// clipSource is "Pixel:/sdcard/DCIM" when the items share a folder, else
// the storage's name.
func clipSource(f FS, list []Entry) string {
	dir := f.Dir(list[0].Path)
	for _, e := range list[1:] {
		if f.Dir(e.Path) != dir {
			return f.Label()
		}
	}
	return location(f, dir)
}

// renamedOnClipboard follows a rename: clipboard items at or under the old
// path now live at the new one, so a later paste still finds them.
func (s *state) renamedOnClipboard(f FS, from, to string) {
	if s.clip == nil || s.clip.side != f.Side() {
		return
	}
	for i, it := range s.clip.items {
		if !under(f, it.Path, from) {
			continue
		}
		s.clip.items[i].Path = to + it.Path[len(from):]
		if it.Path == from {
			s.clip.items[i].Name = f.Base(to)
		}
	}
}
