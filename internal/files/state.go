package files

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// minColumnsWidth is the narrowest terminal that shows one pane's list in
// two columns; narrower, names would be cut too short to read.
const minColumnsWidth = 80

// minSplitWidth is the narrowest terminal that shows two panes. It is the
// same as for two columns: both split the width in half, so a window that
// shows two columns can always show the PC pane instead.
const minSplitWidth = minColumnsWidth

// columns is how many columns the list runs in: two, newspaper style, in
// one pane at least minColumnsWidth wide; one in two panes.
func (s *state) columns() int {
	if !s.split && !s.oneColumn && s.width >= minColumnsWidth {
		return 2
	}
	return 1
}

type sortKey int

const (
	sortName sortKey = iota
	sortNewest
	sortSize
)

func (k sortKey) label() string {
	return [...]string{"name ↑", "newest ↓", "size ↓"}[k]
}

// pane is one side's folder view. Each side keeps its folder, cursor and
// selection while the other side is shown.
type pane struct {
	fs      FS
	dir     string
	entries []Entry // the folder's listing
	err     error   // the listing failed
	rows    []Entry // shown: ".." first, then entries after hidden, filter and sort
	// Each pane sorts on its own (2), and its recent view apart from its
	// folders: the recent files start newest first.
	sortBy, recentSort sortKey
	cursor             int
	top                int // first row on screen
	selected           map[string]bool
	filter             string

	recent      bool    // device: recent files instead of the folder
	recentList  []Entry // nil until loaded
	recentErr   error
	folderPath  string // row under the cursor in the folder view, kept while recent shows
	loadingNote string // shown in the list area while a listing loads
	free        int64  // free bytes where dir is (device: df when it opens); -1 unknown
}

type mode int

const (
	modeNormal   mode = iota
	modeFilter        // typing a filter after '/'
	modeInput         // typing a name (rename, new folder) or a path (go to)
	modeDelete        // delete confirmation
	modeConflict      // a name exists where we push or pull
	modeFailures      // the failure list of the last task
	modeHelp          // all keys
)

type inputKind int

const (
	inputRename inputKind = iota
	inputMkdir
	inputGoto
)

type msgKind int

const (
	msgInfo msgKind = iota
	msgSuccess
	msgWarn
	msgError
)

// message is a one-off line above the key help, cleared by the next key.
type message struct {
	text     string
	kind     msgKind
	failures []failure // Enter opens the list
	done     []string  // and shows what went through too
}

// clipboard is ak's own clipboard (not the system's): items taken with
// 3/c (copy) or 4/x (cut) on one side, pasted with 6/v where the cursor is.
// It stays while folders and panes change, which a selection does not; a
// new copy or cut replaces it.
type clipboard struct {
	side  Side
	items []Entry
	cut   bool
}

// task is work that runs off the key loop: a push or pull, a delete,
// opening a device file, loading the recent list. run returns a function
// that applies the result to the state on the key loop.
type task struct {
	label      string
	route      string // push and pull: "Pixel:/sdcard/Download ← Computer"
	cancelable bool
	started    time.Time // when it began running: the spinner shows how long
	run        func(ctx context.Context, report func(progress)) func(*state)
	// tick, when set, runs on the key loop's ticks while run is running, to
	// show what run has done so far.
	tick func(*state)
}

type exitKind int

const (
	exitNone exitKind = iota
	exitQuit
	exitInterrupt
)

// state is the whole screen. It does no terminal I/O; storage calls that
// take a moment (listing a folder, renaming) run inline, longer ones are
// handed to the runner as a task.
type state struct {
	panes      [2]*pane // by Side
	active     Side
	split      bool // two panes on screen now
	wantSplit  bool // two panes when the window is wide enough: on at start, 9 toggles it
	width      int
	height     int
	showHidden bool
	oneColumn  bool // v: one pane lists in one wide column (long names), not two
	// The cursor's row when its name does not fit (render.go): it shows the
	// whole name, and scrolls one longer than the row.
	focusKey      string
	focusSince    time.Time
	focusExpanded bool      // set while drawing: the status line shows its size and date
	scrolling     bool      // set while drawing: the loop redraws to move it
	scrollNext    time.Time // when the scrolling name next moves
	mode          mode
	msg           *message

	// modeInput / modeFilter
	inputKind   inputKind
	inputText   string
	inputCursor int // byte offset
	// Go to: what Tab found, in the key line until the next key
	inputMatches []string           // the folders, each with its separator
	inputHint    string             // or why there are none
	gotoLists    map[string][]Entry // folders Tab read while this Go to is open
	// Tab's menu over inputMatches, as zsh's: Tab again picks the next
	// folder into the text, which is menuText with the name at menuAt.
	menuSel    int    // the picked folder; -1 before the second Tab
	menuTop    int    // the first grid row drawn (set while drawing)
	menuText   string // the text when the menu opened
	menuCursor int    // its cursor
	menuAt     int    // where the folder name starts in it
	menuDrawn  bool   // set while drawing: the menu has its panel

	// modeDelete
	delTargets []Entry

	// modeConflict
	plan    *copyJob
	push    bool // a send that pushes (PC to device); else it pulls
	pasting bool // the plan is a paste of the clipboard, not a send (5)

	clip *clipboard // what c or x took, kept across folders and panes
	ci   int        // op being asked about
	// toAll: a was pressed while a name is asked about, so the answer
	// covers the rest too. A letter, not Shift: Caps Lock flips the case.
	toAll bool

	failures []failure // modeFailures
	failDone []string  // modeFailures: the items that went through
	helpTop  int       // modeHelp: first line shown

	task     *task // running
	pending  *task // to start
	prog     progress
	spin     int
	canceled bool // Esc pressed on the running task
	exit     exitKind

	summary   Summary
	now       func() time.Time
	openLocal func(string) error
	tempDir   string
	opened    int
}

func newState(device, pc FS, deviceDir, pcDir string, openLocal func(string) error) *state {
	// The PC pane is on from the start (when the window is wide enough).
	s := &state{now: time.Now, openLocal: openLocal, tempDir: tempDir(), width: 80, height: 24, wantSplit: true}
	s.panes[Device] = &pane{fs: device, selected: map[string]bool{}, free: -1, recentSort: sortNewest}
	s.panes[PC] = &pane{fs: pc, selected: map[string]bool{}, free: -1, recentSort: sortNewest}
	s.open(s.panes[Device], deviceDir, "")
	s.open(s.panes[PC], pcDir, "")
	return s
}

func (s *state) cur() *pane { return s.panes[s.active] }

func lowerFirst(t string) string {
	if t == "" {
		return t
	}
	return strings.ToLower(t[:1]) + t[1:]
}

// upperFirst starts an error's text (lower case, as Go errors are) with a
// capital when it is the whole message.
func upperFirst(t string) string {
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

func (s *state) setMsg(kind msgKind, format string, args ...any) {
	s.msg = &message{text: fmt.Sprintf(format, args...), kind: kind}
}

// setSize records the terminal size. Two panes that no longer fit become one,
// showing the side the cursor was on.
// setSize follows the terminal. Two panes show whenever they are wanted and
// fit, so a window made narrow and then wide again gets the PC pane back.
func (s *state) setSize(w, h int) {
	s.width, s.height = w, h
	s.split = s.wantSplit && w >= minSplitWidth
}

// ---- listing ----

// open shows dir in p, with the cursor on the row named focus.
func (s *state) open(p *pane, dir, focus string) {
	entries, err := p.fs.List(dir)
	s.show(p, dir, focus, entries, err)
}

// show puts a listing of dir in p (open, or a go to that listed it already).
func (s *state) show(p *pane, dir, focus string, entries []Entry, err error) {
	if dir != p.dir {
		p.selected = map[string]bool{}
		p.filter = ""
		p.cursor, p.top = 0, 0
	}
	p.dir, p.entries, p.err = dir, entries, err
	p.recent = false
	s.refreshRows(p, focus)
	readFree(p)
}

// readFree reads the free space where p's folder is, on storages that tell
// it cheaply (the device: one df); otherwise it is unknown.
func readFree(p *pane) {
	p.free = -1
	if f, ok := p.fs.(interface {
		Free(dir string) (int64, error)
	}); ok && p.err == nil {
		if n, err := f.Free(p.dir); err == nil {
			p.free = n
		}
	}
}

// reload lists p's folder again, keeping the cursor on the same row.
func (s *state) reload(p *pane) {
	if p.recent {
		s.refreshRows(p, s.focusPath(p))
		return
	}
	focus := s.focusPath(p)
	p.entries, p.err = p.fs.List(p.dir)
	s.refreshRows(p, focus)
}

func (s *state) focusPath(p *pane) string {
	if p.cursor < len(p.rows) {
		return p.rows[p.cursor].Path
	}
	return ""
}

// refreshRows rebuilds p.rows and puts the cursor on focus (a path or a name)
// if it is still shown.
func (s *state) refreshRows(p *pane, focus string) {
	var src []Entry
	if p.recent {
		src = p.recentList
	} else {
		src = p.entries
	}
	f := fold(p.filter)
	var rows []Entry
	for _, e := range src {
		if !s.showHidden && strings.HasPrefix(e.Name, ".") {
			continue
		}
		if f != "" && !strings.Contains(fold(e.Name), f) {
			continue
		}
		rows = append(rows, e)
	}
	if p.recent {
		sortEntries(rows, p.recentSort)
	} else {
		sortEntries(rows, p.sortBy)
		if f == "" && p.fs.Dir(p.dir) != p.dir {
			rows = append([]Entry{{Name: "..", Path: p.fs.Dir(p.dir), Dir: true}}, rows...)
		}
	}
	p.rows = rows
	// Drop selections that are gone from the listing.
	for k := range p.selected {
		if !containsPath(src, k) {
			delete(p.selected, k)
		}
	}
	p.cursor = min(p.cursor, max(len(rows)-1, 0))
	if focus != "" {
		for i, e := range rows {
			if e.Path == focus || (e.Name == focus && e.Name != "..") {
				p.cursor = i
				break
			}
		}
	}
}

func containsPath(list []Entry, p string) bool {
	for _, e := range list {
		if e.Path == p {
			return true
		}
	}
	return false
}

// sortEntries puts folders first, then sorts by key (names break ties).
func sortEntries(list []Entry, key sortKey) {
	sort.SliceStable(list, func(a, b int) bool {
		x, y := list[a], list[b]
		if x.Dir != y.Dir {
			return x.Dir
		}
		switch key {
		case sortNewest:
			if !x.ModTime.Equal(y.ModTime) {
				return x.ModTime.After(y.ModTime)
			}
		case sortSize:
			if x.Size != y.Size {
				return x.Size > y.Size
			}
		}
		return fold(x.Name) < fold(y.Name)
	})
}

// ---- layout numbers the key handling needs ----

// listHeight is how many rows the list shows (render.go draws the rest).
func (s *state) listHeight() int { return s.layout().list }

// ---- keys ----

func (s *state) handle(k tui.Key) {
	if k.Kind == tui.KeyCtrlC {
		s.quit(exitInterrupt)
		return
	}
	switch s.mode {
	case modeFilter, modeInput:
		s.handleInput(k)
		return
	case modeDelete:
		s.handleDelete(k)
		return
	case modeConflict:
		s.handleConflict(k)
		return
	case modeHelp:
		s.handleHelp(k)
		return
	case modeFailures:
		s.mode = modeNormal
		return
	}
	msg := s.msg
	s.msg = nil
	p := s.cur()
	switch k.Kind {
	case tui.KeyUp:
		s.move(p, -1)
	case tui.KeyDown:
		s.move(p, 1)
	case tui.KeyPgUp:
		s.move(p, -s.listHeight()*s.columns())
	case tui.KeyPgDn:
		s.move(p, s.listHeight()*s.columns())
	case tui.KeyHome:
		p.cursor = 0
	case tui.KeyEnd:
		p.cursor = max(len(p.rows)-1, 0)
	case tui.KeyEnter:
		if msg != nil && len(msg.failures) > 0 {
			s.failures, s.failDone, s.mode = msg.failures, msg.done, modeFailures
			return
		}
		s.enter(p)
	case tui.KeyRight:
		if s.columns() == 2 {
			s.sideways(p, 1)
		} else if e, ok := s.cursorEntry(p); !ok || e.Dir {
			s.enter(p) // into a folder; a file opens with Enter only, → is too easy to hit
		}
	case tui.KeyLeft:
		if s.columns() == 2 {
			s.sideways(p, -1)
		} else {
			s.up(p)
		}
	case tui.KeyBackspace:
		s.up(p)
	case tui.KeyTab:
		s.active = 1 - s.active
	case tui.KeyEsc:
		s.escape(p, msg)
	case tui.KeyDelete:
		if k.Shift {
			s.deleteUnasked(p)
		} else {
			s.askDelete(p)
		}
	case tui.KeyRune:
		s.handleRune(p, k.Rune)
	}
}

// numberKeys maps the number key bar (mc's F keys as digits: macOS
// terminals take F keys for the system) to the letter keys, which stay as
// aliases.
var numberKeys = map[byte]byte{'1': 's', '2': 't', '3': 'c', '4': 'x', '6': 'v', '7': 'n', '8': 'd', '9': 'w', '0': 'r'}

func (s *state) handleRune(p *pane, r byte) {
	if r >= 'A' && r <= 'Z' {
		r |= 0x20 // Caps Lock on: the same keys
	}
	if r >= 0x80 {
		// UTF-8 text: a Korean (or other) input source is on. Its letters
		// are not mapped to keys: the input method holds a jamo until the
		// next key and sends a syllable, and d then y as one syllable must
		// not delete unasked. Say why nothing happened.
		s.setMsg(msgWarn, "Letter keys need English input; the number keys work in any input")
		return
	}
	if r == '5' { // copy to the other side: pull on the device, push on the PC
		r = map[Side]byte{Device: 'g', PC: 'p'}[p.fs.Side()]
	}
	if alias, ok := numberKeys[r]; ok {
		r = alias
	}
	switch r {
	case 'q':
		s.quit(exitQuit)
	case ' ':
		if e, ok := s.cursorEntry(p); ok {
			p.selected[e.Path] = !p.selected[e.Path]
			if !p.selected[e.Path] {
				delete(p.selected, e.Path)
			}
			s.move(p, 1)
		}
	case 'a': // as in ak's other pickers (install, pull, push)
		s.toggleAll(p)
	case '/':
		s.mode, s.inputText, s.inputCursor = modeFilter, p.filter, len(p.filter)
	case 'c', 'x':
		s.toClipboard(p, r == 'x')
	case 'v':
		s.paste(p)
	case 'b':
		switch {
		case s.split:
			s.setMsg(msgInfo, "Two panes list in one column each: 9 hides the computer pane")
			return
		case s.width < minColumnsWidth:
			s.setMsg(msgInfo, "Window too narrow for two columns")
			return // nothing to switch: the label stays as it is
		}
		key := s.focusPath(p)
		s.oneColumn = !s.oneColumn
		s.refreshRows(p, key) // the cursor stays on its item; scrolling follows
	case 'w':
		switch {
		case s.split:
			s.wantSplit, s.split = false, false
		case s.width < minSplitWidth:
			s.setMsg(msgWarn, "Window too narrow for two panes")
		default:
			s.wantSplit, s.split = true, true
		}
	case 'p':
		if p.fs.Side() == PC { // the device side has nothing to push
			s.send(p, true)
		}
	case 'g':
		if p.fs.Side() == Device {
			s.send(p, false)
		}
	case 'd':
		s.askDelete(p)
	case 'r':
		s.askRename(p)
	case 'n':
		s.askInput(p, inputMkdir, "")
	case ':':
		s.askInput(p, inputGoto, displayPath(p.fs, p.dir))
	case 't':
		if p.fs.Side() == PC {
			// Recent files are the device's: show the device pane in its
			// recent view, from the PC side too.
			s.active = Device
			if d := s.panes[Device]; !d.recent {
				s.toggleRecent(d)
			}
			return
		}
		s.toggleRecent(p)
	case 's':
		// The pane with the cursor, and in it the view shown: sorting the
		// recent files leaves the folders' order, and the other pane, alone.
		if p.recent {
			p.recentSort = (p.recentSort + 1) % 3
		} else {
			p.sortBy = (p.sortBy + 1) % 3
		}
		s.refreshRows(p, s.focusPath(p))
	case '.':
		s.showHidden = !s.showHidden
		s.refreshAll()
	case '?':
		s.mode = modeHelp
	}
}

func (s *state) refreshAll() {
	for _, p := range s.panes {
		s.refreshRows(p, s.focusPath(p))
	}
}

func (s *state) quit(how exitKind) {
	if s.exit == exitNone || how == exitInterrupt {
		s.exit = how
	}
	s.cancelTask()
}

// sideways moves to the same row of the next (dir 1) or previous (-1)
// column; when the next column is shorter, to its last row.
func (s *state) sideways(p *pane, dir int) {
	h := max(s.listHeight(), 1)
	switch to := p.cursor + dir*h; {
	case to >= 0 && to < len(p.rows):
		p.cursor = to
	case dir > 0 && p.cursor/h < (len(p.rows)-1)/h:
		p.cursor = len(p.rows) - 1
	}
}

func (s *state) move(p *pane, delta int) {
	if len(p.rows) == 0 {
		return
	}
	p.cursor = min(max(p.cursor+delta, 0), len(p.rows)-1)
}

// cursorEntry is the entry under the cursor, if it is not "..".
func (s *state) cursorEntry(p *pane) (Entry, bool) {
	if p.cursor >= len(p.rows) || s.isUp(p, p.cursor) {
		return Entry{}, false
	}
	return p.rows[p.cursor], true
}

func (s *state) isUp(p *pane, i int) bool { return !p.recent && p.rows[i].Name == ".." && i == 0 }

func (s *state) toggleAll(p *pane) {
	all := true
	n := 0
	for i, e := range p.rows {
		if !s.isUp(p, i) {
			n++
			all = all && p.selected[e.Path]
		}
	}
	if n == 0 {
		return
	}
	for i, e := range p.rows {
		if s.isUp(p, i) {
			continue
		}
		if all {
			delete(p.selected, e.Path)
		} else {
			p.selected[e.Path] = true
		}
	}
}

// targets are what c, x and d act on: the selection, or the cursor's entry.
func (s *state) targets(p *pane) []Entry {
	list := s.targetsSelected(p)
	if len(list) == 0 {
		if e, ok := s.cursorEntry(p); ok {
			list = append(list, e)
		}
	}
	return list
}

// blocked refuses an action key while a task runs.
func (s *state) blocked() bool {
	t := s.task
	if t == nil {
		t = s.pending
	}
	if t != nil {
		hint := ""
		if t.cancelable {
			hint = ", or press Esc to cancel"
		}
		s.setMsg(msgWarn, "Wait until %s finishes%s", lowerFirst(t.label), hint)
		return true
	}
	return false
}

// escape cancels the running task, else turns the filter off, else empties
// the clipboard.
func (s *state) escape(p *pane, msg *message) {
	switch {
	case s.task != nil && s.task.cancelable:
		s.cancelTask()
	case p.filter != "":
		p.filter = ""
		s.refreshRows(p, s.focusPath(p))
	case msg != nil:
		// the key only cleared the message
	case len(p.selected) > 0:
		p.selected = map[string]bool{} // this pane's; the clipboard stays for the next Esc
	default:
		s.clip = nil
	}
}

func (s *state) cancelTask() {
	if s.task != nil && s.task.cancelable {
		s.canceled = true
	}
	if s.pending != nil {
		s.pending = nil
	}
}

// ---- open / navigate ----

func (s *state) enter(p *pane) {
	if p.cursor >= len(p.rows) {
		return
	}
	e := p.rows[p.cursor]
	if s.isUp(p, p.cursor) {
		s.up(p)
		return
	}
	if e.Dir {
		s.open(p, e.Path, "")
		return
	}
	// Open takes the file under the cursor only, never the selection: a
	// hundred selected files must not open a hundred windows.
	note := len(s.targetsSelected(p)) > 0
	if p.fs.Side() == PC {
		s.openFile(e.Path, note)
		return
	}
	if s.blocked() {
		return
	}
	s.openDeviceFile(p, e, note)
}

func (s *state) up(p *pane) {
	if p.recent {
		s.toggleRecent(p)
		return
	}
	parent := p.fs.Dir(p.dir)
	if parent == p.dir {
		return
	}
	s.open(p, parent, p.fs.Base(p.dir))
}

// openFile opens path on this PC; note says files were selected, which open
// leaves alone, so the message tells what the selection is for.
func (s *state) openFile(path string, note bool) {
	if s.openLocal == nil {
		s.setMsg(msgError, "Couldn't open %s here", ui.Quote(shown(filepath.Base(path))))
		return
	}
	if err := s.openLocal(path); err != nil {
		s.setMsg(msgError, "Couldn't open %s: %v", ui.Quote(shown(filepath.Base(path))), err)
		return
	}
	if note {
		s.setMsg(msgSuccess, "Opened %s (open takes the file under the cursor; the selection is for 3, 4, 5 and 8)", ui.Quote(shown(filepath.Base(path))))
		return
	}
	s.setMsg(msgSuccess, "Opened %s", ui.Quote(shown(filepath.Base(path))))
}

// openDeviceFile pulls a device file to the temp folder and opens it on the
// PC (files-design.md §5).
func (s *state) openDeviceFile(p *pane, e Entry, note bool) {
	s.opened++
	dir := filepath.Join(s.tempDir, strconv.FormatInt(s.now().UnixNano(), 36)+"-"+strconv.Itoa(s.opened))
	dst := filepath.Join(dir, e.Name)
	from := p.fs
	name := ui.Quote(shown(e.Name))
	s.pending = &task{label: "Opening " + name, cancelable: true,
		run: func(ctx context.Context, report func(progress)) func(*state) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return func(s *state) { s.setMsg(msgError, "Couldn't open %s: %v", name, err) }
			}
			job := &copyJob{from: from, to: localFS{PC, computerLabel}, dstDir: dir, ops: []copyOp{{src: e, name: e.Name}}}
			res := job.run(ctx, report)
			return func(s *state) {
				switch {
				case res.canceled:
					s.setMsg(msgInfo, "Canceled opening %s", name)
				case len(res.failed) > 0:
					s.setMsg(msgError, "Couldn't open %s: %s", name, res.failed[0].msg)
				default:
					s.openFile(dst, note)
				}
			}
		}}
}

// toggleRecent switches the device pane between its folder and the recent
// files (files-design.md §7).
func (s *state) toggleRecent(p *pane) {
	lister, ok := p.fs.(interface{ Recent() ([]Entry, error) })
	if !ok {
		s.setMsg(msgInfo, "Recent files are shown for the device only")
		return
	}
	if p.recent {
		p.recent = false
		p.filter = ""
		p.selected = map[string]bool{}
		// List the folder again: a delete or a copy in the recent view
		// only reloaded the recent rows.
		p.entries, p.err = p.fs.List(p.dir)
		s.refreshRows(p, p.folderPath)
		readFree(p)
		return
	}
	if s.blocked() {
		return
	}
	p.folderPath = s.focusPath(p)
	p.recent, p.filter, p.cursor, p.top = true, "", 0, 0
	p.recentSort = sortNewest // each visit starts with what is new; s only sorts this visit
	p.selected = map[string]bool{}
	p.recentList, p.recentErr = nil, nil
	p.loadingNote = "Reading recent files"
	s.refreshRows(p, "")
	s.pending = &task{label: "Reading recent files on " + p.fs.Label(), run: func(context.Context, func(progress)) func(*state) {
		list, err := lister.Recent()
		return func(s *state) {
			p.loadingNote = ""
			p.recentList, p.recentErr = list, err
			if p.recentList == nil {
				p.recentList = []Entry{}
			}
			if p.recent {
				s.refreshRows(p, "")
			}
		}
	}}
}

// ---- push and pull ----

// send copies the targets of p to the other side's current folder: push
// from the PC to the device, pull from the device to the PC. That folder is
// the one the other pane remembers, shown or not (one pane). Only names that
// exist there are asked about (files-design.md §6).
func (s *state) send(p *pane, push bool) {
	if s.blocked() {
		return
	}
	items := s.targets(p)
	if len(items) == 0 {
		return
	}
	dst := s.panes[1-p.fs.Side()]
	entries, err := dst.fs.List(dst.dir) // what is there now, even if the pane shows recent files
	if err != nil {
		verb := map[bool]string{true: "push", false: "pull"}[push]
		s.setMsg(msgError, "Couldn't %s to %s: %s", verb, place(dst.fs, dst.dir), errText(err))
		return
	}
	inDir := nameLookup(dst.fs, dst.dir, entries)
	used := map[string]bool{} // names this copy gives, ignoring case to be safe
	taken := func(n string) bool { _, ok := inDir(n); return ok || used[fold(n)] }
	job := &copyJob{from: p.fs, to: dst.fs, dstDir: dst.dir, inDir: inDir}
	for _, it := range items {
		name := it.Name
		if push {
			// Composed, as phones expect: a macOS (NFD) name kept as is shows
			// as loose jamo in many apps. A folder's inner names stay as adb
			// pushes them (files-design.md §6).
			name = norm.NFC.String(name)
		}
		op := copyOp{src: it, name: name}
		if e, ok := inDir(name); ok && !used[fold(name)] {
			op.conflict, op.dstDir, op.existing = true, e.Dir, e.Path
		} else if used[fold(name)] { // same names from different folders (recent files)
			op.name = UniqueName(name, it.Dir, taken)
		}
		used[fold(op.name)] = true
		job.ops = append(job.ops, op)
	}
	s.plan, s.push, s.pasting, s.ci, s.toAll = job, push, false, -1, false
	s.nextConflict()
}

// route is where a push or pull goes, the device always on the left as on
// screen: "Pixel:/sdcard/Download ← Computer", "Pixel → ~/Downloads".
func route(job *copyJob, push bool) string {
	if push {
		return location(job.to, job.dstDir) + " ← " + computerLabel
	}
	return job.from.Label() + " → " + displayPath(job.to, job.dstDir)
}

// nameLookup finds a name in dir: the entry of the same name, or, when only
// the case differs, the one the file system itself finds (a Stat of the name
// on /sdcard, macOS or Windows, which ignore case; nothing on ext4).
func nameLookup(f FS, dir string, entries []Entry) func(string) (Entry, bool) {
	exact := map[string]Entry{}
	composed := map[string]Entry{} // NFC: names that differ only in normalization are the same name
	folded := map[string]bool{}
	for _, e := range entries {
		exact[e.Name] = e
		composed[norm.NFC.String(e.Name)] = e
		folded[fold(e.Name)] = true
	}
	stats := map[string]bool{}
	return func(n string) (Entry, bool) {
		if e, ok := exact[n]; ok {
			return e, true
		}
		if e, ok := composed[norm.NFC.String(n)]; ok {
			return e, true
		}
		if !folded[fold(n)] {
			return Entry{}, false
		}
		found, asked := stats[n]
		var e Entry
		if !asked {
			var err error
			e, err = f.Stat(f.Join(dir, n))
			found = err == nil
			stats[n] = found
		}
		return e, found
	}
}

// fold is a name as case-insensitive file systems compare it.
func fold(name string) string { return FoldName(name) }

// FoldName is a name for comparing: composed (NFC, so a macOS NFD name
// equals the same name typed) and lower case. Paths themselves keep their
// bytes: macOS opens an NFD name only as NFD.
func FoldName(name string) string { return strings.ToLower(norm.NFC.String(name)) }

// takenFold reports names in use, ignoring case.
func takenFold(names []string) func(string) bool {
	set := map[string]bool{}
	for _, n := range names {
		set[fold(n)] = true
	}
	return func(n string) bool { return set[fold(n)] }
}

// nextConflict asks about the next unresolved name, or starts copying.
func (s *state) nextConflict() {
	for s.ci++; s.ci < len(s.plan.ops); s.ci++ {
		if s.plan.ops[s.ci].conflict {
			s.mode = modeConflict
			return
		}
	}
	s.mode = modeNormal
	if s.pasting {
		s.startPaste(s.plan)
	} else {
		s.startSend(s.plan, s.push)
	}
	s.plan = nil
}

// cancelPlan drops a push, pull or paste while its names are asked about.
func (s *state) cancelPlan() {
	what := map[bool]string{true: "pushing", false: "pulling"}[s.push]
	if s.pasting {
		what = map[bool]string{true: "moving", false: "copying"}[s.plan.cut]
	}
	s.mode, s.plan = modeNormal, nil
	s.setMsg(msgInfo, "Canceled %s", what)
}

func (s *state) handleConflict(k tui.Key) {
	var choice byte
	switch k.Kind {
	case tui.KeyEnter:
		choice = 'r'
	case tui.KeyEsc:
		s.cancelPlan()
		return
	case tui.KeyRune:
		choice = k.Rune
		if choice >= 'A' && choice <= 'Z' {
			choice |= 0x20 // Caps Lock on: the same keys
		}
		switch choice {
		case 'q': // like Esc; a second q quits
			s.cancelPlan()
			return
		case 'a':
			s.toAll = !s.toAll
			return
		}
	default:
		return
	}
	if choice != 'o' && choice != 's' && choice != 'r' {
		return
	}
	end := s.ci + 1
	if s.toAll {
		end = len(s.plan.ops)
	}
	s.toAll = false
	for i := s.ci; i < end; i++ {
		s.resolve(i, choice)
	}
	s.nextConflict()
}

func (s *state) resolve(i int, choice byte) {
	op := &s.plan.ops[i]
	if !op.conflict {
		return
	}
	op.conflict = false
	switch choice {
	case 'o':
		op.overwrite = true
	case 's':
		op.skip = true
	case 'r':
		var names []string
		for j, o := range s.plan.ops {
			if j != i && !o.skip {
				names = append(names, o.name)
			}
		}
		others := takenFold(names)
		op.name = UniqueName(op.name, op.src.Dir, func(n string) bool {
			_, ok := s.plan.inDir(n)
			return ok || others(n)
		})
	}
}

func (s *state) startSend(job *copyJob, push bool) {
	n := 0
	for _, op := range job.ops {
		if !op.skip {
			n++
		}
	}
	if n == 0 {
		s.setMsg(msgInfo, "Nothing copied: all skipped")
		return
	}
	s.panes[job.from.Side()].selected = map[string]bool{} // copying starts: the selection is used
	label := map[bool]string{true: "Pushing", false: "Pulling"}[push]
	s.pending = &task{label: label, route: route(job, push), cancelable: true,
		run: func(ctx context.Context, report func(progress)) func(*state) {
			res := job.run(ctx, report)
			return func(s *state) { s.finishSend(job, push, res) }
		},
		tick: func(s *state) { s.showFinished(job) }}
}

// showFinished puts the items a copy has finished into the panes as they
// finish, without listing the folder again (no adb call per item, and no
// half-copied temporary file shown): a copy into the folder shown, and for
// a move its original out. The job's end lists the folders again for their
// exact dates and permissions.
func (s *state) showFinished(job *copyJob) {
	done := job.takeFinished()
	if len(done) == 0 {
		return
	}
	for _, p := range s.panes {
		if p.recent || p.err != nil {
			continue
		}
		changed := false
		if p.fs == job.to && p.fs.Join(p.dir, "") == job.to.Join(job.dstDir, "") {
			for _, op := range done {
				e := Entry{Name: op.name, Path: job.to.Join(job.dstDir, op.name), Dir: op.src.Dir, Size: op.src.Size, ModTime: s.now()}
				if e.Dir {
					e.Size = 0
				}
				p.entries = append(withoutNamed(p.entries, e.Path, op.existing), e) // an overwrite replaces the old row
			}
			changed = true
		}
		if job.cut && p.fs == job.from {
			for _, op := range done {
				if containsPath(p.entries, op.src.Path) {
					p.entries = withoutNamed(p.entries, op.src.Path)
					changed = true
				}
			}
		}
		if changed {
			s.refreshRows(p, s.focusPath(p))
		}
	}
}

// withoutNamed is list without the entries at the paths given.
func withoutNamed(list []Entry, paths ...string) []Entry {
	var kept []Entry
	for _, e := range list {
		if e.Path == "" || !slices.Contains(paths, e.Path) {
			kept = append(kept, e)
		}
	}
	return kept
}

// finishSend reports a push or pull: "Pushed 'a.txt' to '/sdcard' on Pixel",
// "Pulled 3 files from Pixel to '~/Downloads'".
func (s *state) finishSend(job *copyJob, push bool, res copyResult) {
	for _, p := range s.panes {
		if p.fs == job.to && !p.recent {
			s.reload(p)
			readFree(p) // the copy took space
		}
	}
	verb, past, ing := "push", "Pushed", "pushing"
	if !push {
		verb, past, ing = "pull", "Pulled", "pulling"
	}
	s.reportCopy(job, res, verb, past, ing)
}

// reportCopy puts a finished push, pull or paste into the status line and
// the summary, naming the place the same way in each: "Pulled 3 files from
// Pixel to '~/Downloads'", "Canceled pulling from Pixel to '~/Downloads': 1
// of 3 pulled", "Couldn't pull 'a.txt' from Pixel to '~/Downloads': …".
func (s *state) reportCopy(job *copyJob, res copyResult, verb, past, ing string) {
	where := job.where()
	what := pastedItems(res.done, res.doneAs)
	if len(res.done) > 0 {
		s.addDone(past+" "+what+" "+where, itemNames(res.done, res.doneAs))
	}
	if res.canceled {
		s.summary.Canceled = append(s.summary.Canceled,
			fmt.Sprintf("Canceled %s %s: %d of %d %s", ing, where, len(res.done), res.todo, strings.ToLower(past)))
	}
	for _, f := range res.failed {
		s.summary.Failed = append(s.summary.Failed, fmt.Sprintf("Couldn't %s %s %s: %s", verb, ui.Quote(f.name), where, f.msg))
	}
	switch {
	case res.canceled:
		s.setMsg(msgInfo, "Canceled %s: %d of %d %s", ing, len(res.done), res.todo, strings.ToLower(past))
	case len(res.failed) > 0:
		s.msg = &message{kind: msgError, failures: res.failed, done: itemNames(res.done, res.doneAs),
			text: fmt.Sprintf("Couldn't %s %d of %s", verb, len(res.failed), ui.Plural(len(res.done)+len(res.failed), itemWord(job.sources())))}
	default:
		s.setMsg(msgSuccess, "%s %s %s", past, what, where)
	}
}

// pastedItems is countItems with the names items were saved as: "'a.txt'
// as 'a (1).txt'", or "3 files (1 renamed)".
func pastedItems(done []Entry, as []string) string {
	// A name that only changed form (NFD to NFC on the way to the device)
	// looks the same: it is not "renamed".
	renamed := 0
	for i, e := range done {
		if i < len(as) && shown(as[i]) != shown(e.Name) {
			renamed++
		}
	}
	switch {
	case renamed == 0:
		return countItems(done)
	case len(done) == 1:
		return ui.Quote(shown(done[0].Name)) + " as " + ui.Quote(as[0])
	}
	return fmt.Sprintf("%s (%d renamed)", countItems(done), renamed)
}

func withoutPaths(list, gone []Entry) []Entry {
	var kept []Entry
	for _, e := range list {
		if !containsPath(gone, e.Path) {
			kept = append(kept, e)
		}
	}
	return kept
}

// countItems is "'a.txt'", "3 files", "2 folders" or "4 items".
func countItems(list []Entry) string {
	if len(list) == 1 {
		return ui.Quote(shown(list[0].Name))
	}
	return ui.Plural(len(list), itemWord(list))
}

// itemWord is what list holds: "file", "folder", or "item" for both.
func itemWord(list []Entry) string {
	dirs := 0
	for _, e := range list {
		if e.Dir {
			dirs++
		}
	}
	switch dirs {
	case 0:
		return "file"
	case len(list):
		return "folder"
	}
	return "item"
}

// ---- delete ----

func (s *state) askDelete(p *pane) {
	if s.blocked() {
		return
	}
	s.delTargets = s.targets(p)
	if len(s.delTargets) > 0 {
		s.mode = modeDelete
	}
}

// deleteUnasked deletes the targets without the [y/N] question (Shift+Delete):
// a key apart from the letters, so Caps Lock can't make d into it.
func (s *state) deleteUnasked(p *pane) {
	if s.blocked() {
		return
	}
	if targets := s.targets(p); len(targets) > 0 {
		s.startDelete(p, targets, true)
	}
}

// handleDelete takes y; anything else keeps the files ([y/N]).
func (s *state) handleDelete(k tui.Key) {
	s.mode = modeNormal
	if k.Kind != tui.KeyRune || (k.Rune != 'y' && k.Rune != 'Y') {
		s.setMsg(msgInfo, "Nothing deleted")
		return
	}
	s.startDelete(s.cur(), s.delTargets, false)
}

// startDelete deletes targets in p; unasked marks a Shift+Delete in the
// messages, so a delete nobody confirmed is never silent.
func (s *state) startDelete(p *pane, targets []Entry, unasked bool) {
	f := p.fs
	// Esc stops between items: the item being deleted (one rm -rf) runs to
	// its end, so nothing is left half deleted by ak.
	s.pending = &task{label: "Deleting " + countItems(targets) + " from " + f.Label(), cancelable: true,
		run: func(ctx context.Context, report func(progress)) func(*state) {
			var done []Entry
			var failed []failure
			for i, e := range targets {
				if ctx.Err() != nil {
					break
				}
				report(progress{index: i + 1, count: len(targets), name: e.Name, total: -1})
				if err := f.Remove(e.Path); err != nil {
					failed = append(failed, failure{shown(e.Name), err.Error()})
				} else {
					done = append(done, e)
				}
			}
			flushMedia(f)
			stopped := len(done)+len(failed) < len(targets)
			return func(s *state) { s.finishDelete(p, targets, done, failed, stopped, unasked) }
		}}
}

// finishDelete reports a delete, with "from <device or computer>" as the
// question said it: "Deleted 3 files from Pixel".
func (s *state) finishDelete(p *pane, targets, done []Entry, failed []failure, stopped, unasked bool) {
	total := len(targets)
	p.recentList = withoutPaths(p.recentList, done)
	if s.clip != nil && s.clip.side == p.fs.Side() { // deleted items are no longer there to paste
		if s.clip.items = withoutPaths(s.clip.items, done); len(s.clip.items) == 0 {
			s.clip = nil
		}
	}
	for _, q := range s.panes {
		if q.fs == p.fs {
			s.reload(q)
			readFree(q) // the delete gave space back
		}
	}
	where := p.fs.Label()
	how := ""
	if unasked {
		how = " (Shift+Delete, not asked)"
	}
	if len(done) > 0 {
		s.addDone(fmt.Sprintf("Deleted %s from %s%s", countItems(done), where, how), itemNames(done, nil))
	}
	for _, f := range failed {
		s.summary.Failed = append(s.summary.Failed, fmt.Sprintf("Couldn't delete %s from %s: %s", ui.Quote(f.name), where, f.msg))
	}
	if stopped {
		s.summary.Canceled = append(s.summary.Canceled, fmt.Sprintf("Canceled deleting from %s: %d of %d deleted", where, len(done), total))
	}
	switch {
	case len(failed) > 0:
		s.msg = &message{kind: msgError, failures: failed, done: itemNames(done, nil),
			text: fmt.Sprintf("Couldn't delete %d of %s", len(failed), ui.Plural(len(done)+len(failed), itemWord(targets)))}
	case stopped:
		s.setMsg(msgInfo, "Canceled deleting: %d of %d deleted", len(done), total)
	default:
		s.setMsg(msgSuccess, "Deleted %s%s", countItems(done), how)
	}
}

// ---- rename, new folder, go to ----

func (s *state) askRename(p *pane) {
	if e, ok := s.cursorEntry(p); ok && !s.blocked() {
		s.askInput(p, inputRename, shown(e.Name)) // composed: Backspace takes a syllable, not a jamo
	}
}

func (s *state) askInput(p *pane, kind inputKind, text string) {
	if kind != inputGoto && (s.blocked() || (p.err != nil && !p.recent) || (p.recent && kind == inputMkdir)) {
		if p.recent && kind == inputMkdir {
			s.setMsg(msgInfo, "Make folders in the folder view: 2 shows it")
		}
		return
	}
	s.mode, s.inputKind, s.inputText, s.inputCursor = modeInput, kind, text, len(text)
	s.gotoLists = nil
}

// handleInput edits the filter or name line. The filter applies as it is
// typed; Enter keeps it, Esc clears it.
func (s *state) handleInput(k tui.Key) {
	p := s.cur()
	if len(s.inputMatches) > 0 && s.menuKey(k) {
		return
	}
	s.inputHint, s.inputMatches = "", nil
	switch k.Kind {
	case tui.KeyTab:
		if s.mode == modeInput && s.inputKind == inputGoto {
			s.completeGoto(p)
		}
		return
	case tui.KeyEsc:
		if s.mode == modeFilter {
			p.filter = ""
			s.refreshRows(p, s.focusPath(p))
		}
		s.mode = modeNormal
		return
	case tui.KeyEnter:
		wasFilter := s.mode == modeFilter
		s.mode = modeNormal
		if !wasFilter {
			s.submitInput(p)
		}
		return
	case tui.KeyUp, tui.KeyDown:
		if s.mode == modeFilter {
			s.move(p, map[bool]int{true: -1, false: 1}[k.Kind == tui.KeyUp])
		}
		return
	case tui.KeyLeft:
		if s.inputCursor > 0 {
			_, n := lastRune(s.inputText[:s.inputCursor])
			s.inputCursor -= n
		}
		return
	case tui.KeyRight:
		if s.inputCursor < len(s.inputText) {
			s.inputCursor += firstRuneLen(s.inputText[s.inputCursor:])
		}
		return
	case tui.KeyHome, tui.KeyCtrlA:
		s.inputCursor = 0
		return
	case tui.KeyEnd, tui.KeyCtrlE:
		s.inputCursor = len(s.inputText)
		return
	case tui.KeyBackspace:
		if s.inputCursor > 0 {
			_, n := lastRune(s.inputText[:s.inputCursor])
			s.inputText = s.inputText[:s.inputCursor-n] + s.inputText[s.inputCursor:]
			s.inputCursor -= n
		}
	case tui.KeyDelete:
		if s.inputCursor < len(s.inputText) {
			n := firstRuneLen(s.inputText[s.inputCursor:])
			s.inputText = s.inputText[:s.inputCursor] + s.inputText[s.inputCursor+n:]
		}
	case tui.KeyRune:
		s.insertText(string([]byte{k.Rune}))
	case tui.KeyPaste:
		s.insertText(strings.ReplaceAll(k.Text, "\n", ""))
	default:
		return
	}
	if s.mode == modeFilter {
		p.filter = s.inputText
		p.cursor = 0
		s.refreshRows(p, "")
	}
}

// gotoPath is the folder a typed Go to path names. A relative path is from
// the folder shown, as in a shell's cd.
func gotoPath(p *pane, text string) string {
	dir := expandHome(p.fs, text)
	if p.fs.Side() == PC {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(p.dir, dir)
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs // paths reach adb whole: never "-x" or relative
		}
	} else if !strings.HasPrefix(dir, "/") {
		dir = p.fs.Join(p.dir, dir)
	}
	return dir
}

// completeGoto completes the folder name before the cursor, as a shell's
// Tab: one match is filled in with a separator after it, several are filled
// to the part they share and listed in the key line. A name with the same
// case wins; only when none has it does case not matter. Hidden folders
// show only for a name begun with a dot.
func (s *state) completeGoto(p *pane) {
	before := s.inputText[:s.inputCursor]
	seps, sep := "/", "/"
	if p.fs.Side() == PC && filepath.Separator == '\\' {
		seps, sep = `/\`, `\`
	}
	i := strings.LastIndexAny(before, seps)
	if i >= 0 {
		sep = before[i : i+1] // the separator the path already uses
	}
	dirPart, prefix := before[:i+1], before[i+1:]
	if dirPart == "" && prefix == "~" && p.fs.Side() == PC {
		s.insertText(sep)
		return
	}
	entries, err := s.gotoList(p, gotoPath(p, strings.TrimSpace(dirPart)))
	if err != nil {
		s.inputHint = fmt.Sprintf("Couldn't read %s: %s", ui.Quote(strings.TrimSpace(dirPart)), errText(err))
		if dirPart == "" {
			s.inputHint = "Couldn't read this folder: " + errText(err)
		}
		return
	}
	var exact, folded []string
	for _, e := range entries {
		if !e.Dir || e.Name == "." || e.Name == ".." || (strings.HasPrefix(e.Name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		if strings.HasPrefix(e.Name, prefix) {
			exact = append(exact, e.Name)
		} else if len(e.Name) >= len(prefix) && strings.EqualFold(e.Name[:len(prefix)], prefix) {
			folded = append(folded, e.Name)
		}
	}
	match := exact
	if len(match) == 0 {
		match = folded
	}
	switch len(match) {
	case 0:
		s.inputHint = "No folder starts with " + ui.Quote(prefix)
		if prefix == "" {
			s.inputHint = "No folders here"
		}
		return
	case 1:
		s.replaceBeforeCursor(len(prefix), match[0]+sep)
		return
	}
	sort.Slice(match, func(a, b int) bool { return strings.ToLower(match[a]) < strings.ToLower(match[b]) })
	if common := sharedPrefix(match); len(common) >= len(prefix) {
		s.replaceBeforeCursor(len(prefix), common)
	}
	for _, m := range match {
		s.inputMatches = append(s.inputMatches, m+sep)
	}
	s.menuSel, s.menuTop = -1, 0
	s.menuText, s.menuCursor, s.menuAt = s.inputText, s.inputCursor, i+1
}

// menuKey handles a key while Tab's menu is open: Tab and Shift+Tab pick
// the next and previous folder, the arrows move in the grid (filled
// column by column, as ls lists), Esc closes it and puts the text back. Any other key
// closes it, keeping the folder picked, and does what it does (Enter goes).
func (s *state) menuKey(k tui.Key) bool {
	n := len(s.inputMatches)
	_, rows, _ := s.menuGrid()
	sel := s.menuSel
	switch k.Kind {
	case tui.KeyTab:
		switch {
		case k.Shift && sel <= 0:
			sel = n - 1
		case k.Shift:
			sel--
		default:
			sel = (sel + 1) % n
		}
	case tui.KeyDown, tui.KeyUp, tui.KeyRight, tui.KeyLeft:
		// Up and down stay in the column; left and right in the row.
		step := map[tui.KeyKind]int{tui.KeyDown: 1, tui.KeyUp: -1, tui.KeyRight: rows, tui.KeyLeft: -rows}[k.Kind]
		t, r := sel+step, sel%rows
		switch {
		case sel < 0:
			sel = 0
		case k.Kind == tui.KeyDown && r == rows-1, k.Kind == tui.KeyUp && r == 0:
		case t >= 0 && t < n:
			sel = t
		}
	case tui.KeyEsc:
		s.inputText, s.inputCursor = s.menuText, s.menuCursor
		s.inputMatches = nil
		return true
	default:
		return false
	}
	s.menuSel = sel
	name := s.inputMatches[sel]
	s.inputText = s.menuText[:s.menuAt] + name + s.menuText[s.menuCursor:]
	s.inputCursor = s.menuAt + len(name)
	return true
}

// menuGrid is how Tab's folders lay out across the screen: columns of
// colW, rows deep, filled column by column.
func (s *state) menuGrid() (cols, rows, colW int) {
	for _, m := range s.inputMatches {
		colW = max(colW, ui.TextWidth(m)+2)
	}
	colW = min(colW, max(s.width-1, 1))
	cols = max((s.width-1)/max(colW, 1), 1)
	rows = (len(s.inputMatches) + cols - 1) / cols
	return cols, rows, colW
}

// gotoList lists dir for Tab without asking adb again for what is known:
// the folder the pane shows is its own listing, and a folder Tab read is
// kept while this Go to is open (closing it forgets them, so nothing goes
// stale). A failure is not kept.
func (s *state) gotoList(p *pane, dir string) ([]Entry, error) {
	dir = p.fs.Join(dir, "") // one spelling: "/sdcard/" and "/sdcard" are the same folder
	if dir == p.fs.Join(p.dir, "") && !p.recent && p.err == nil {
		return p.entries, nil
	}
	if e, ok := s.gotoLists[dir]; ok {
		return e, nil
	}
	entries, err := p.fs.List(dir)
	if err != nil {
		return nil, err
	}
	if s.gotoLists == nil {
		s.gotoLists = map[string][]Entry{}
	}
	s.gotoLists[dir] = entries
	return entries, nil
}

// replaceBeforeCursor replaces the n bytes before the cursor with t.
func (s *state) replaceBeforeCursor(n int, t string) {
	s.inputText = s.inputText[:s.inputCursor-n] + t + s.inputText[s.inputCursor:]
	s.inputCursor += len(t) - n
}

// sharedPrefix is the longest start all names share, cut at a whole rune.
func sharedPrefix(names []string) string {
	common := names[0]
	for _, n := range names[1:] {
		j := 0
		for j < len(common) && j < len(n) && common[j] == n[j] {
			j++
		}
		common = common[:j]
	}
	for len(common) > 0 && !utf8.ValidString(common) {
		common = common[:len(common)-1]
	}
	return common
}

// insertText inserts typed bytes. Keys arrive a byte at a time, so a
// Hangul syllable typed through an input method arrives as several KeyRune
// bytes; they are joined as they come.
func (s *state) insertText(t string) {
	s.inputText = s.inputText[:s.inputCursor] + t + s.inputText[s.inputCursor:]
	s.inputCursor += len(t)
}

func (s *state) submitInput(p *pane) {
	text := s.inputText
	switch s.inputKind {
	case inputGoto:
		dir := gotoPath(p, strings.TrimSpace(text))
		entries, err := p.fs.List(dir) // once: a failure keeps the folder shown
		if err != nil {
			s.setMsg(msgError, "Couldn't go to %s: %s", ui.Quote(text), errText(err))
			return
		}
		s.show(p, dir, "", entries, nil)
		return
	case inputMkdir:
		if err := s.checkNewName(p, text, ""); err != nil {
			s.setMsg(msgError, "%s", upperFirst(err.Error()))
			return
		}
		if err := p.fs.Mkdir(p.fs.Join(p.dir, text)); err != nil {
			s.setMsg(msgError, "Couldn't make folder %s: %v", ui.Quote(text), err)
			s.summary.Failed = append(s.summary.Failed, fmt.Sprintf("Couldn't make folder %s in %s: %v", ui.Quote(text), place(p.fs, p.dir), err))
			return
		}
		s.addDone(fmt.Sprintf("Made folder %s in %s", ui.Quote(text), place(p.fs, p.dir)), nil)
		s.reload(p)
		s.refreshRows(p, p.fs.Join(p.dir, text))
		s.setMsg(msgSuccess, "Made folder %s", ui.Quote(text))
	case inputRename:
		e, ok := s.cursorEntry(p)
		if !ok || shown(text) == shown(e.Name) { // unchanged (an NFD name pre-filled composed counts as the same)
			return
		}
		dir, entries := p.dir, p.entries
		if p.recent { // the item's own folder, read now
			dir = p.fs.Dir(e.Path)
			var err error
			if entries, err = p.fs.List(dir); err != nil {
				s.setMsg(msgError, "Couldn't rename %s: %s", ui.Quote(shown(e.Name)), errText(err))
				return
			}
		}
		if err := checkNameIn(p.fs, dir, entries, text, e.Name); err != nil {
			s.setMsg(msgError, "%s", upperFirst(err.Error()))
			return
		}
		to := p.fs.Join(dir, text)
		if err := renameEntry(p.fs, e, to); err != nil {
			s.setMsg(msgError, "Couldn't rename %s: %v", ui.Quote(shown(e.Name)), err)
			s.summary.Failed = append(s.summary.Failed, fmt.Sprintf("Couldn't rename %s to %s on %s: %v", ui.Quote(shown(e.Name)), ui.Quote(text), p.fs.Label(), err))
			return
		}
		s.addDone(fmt.Sprintf("Renamed %s to %s on %s", ui.Quote(shown(e.Name)), ui.Quote(text), p.fs.Label()), nil)
		s.renamedOnClipboard(p.fs, e.Path, to)
		if p.recent {
			s.renamedInRecent(p, e, text, to)
		} else {
			s.reload(p)
		}
		s.refreshRows(p, to)
		s.setMsg(msgSuccess, "Renamed %s to %s", ui.Quote(shown(e.Name)), ui.Quote(text))
	}
}

// renamedInRecent puts a rename done in the recent view into its list
// (MediaStore is not asked again) and into the folder views that show the
// item's folder.
func (s *state) renamedInRecent(p *pane, e Entry, name, to string) {
	for i := range p.recentList {
		if p.recentList[i].Path == e.Path {
			p.recentList[i].Name, p.recentList[i].Path = name, to
		}
	}
	if p.selected[e.Path] {
		delete(p.selected, e.Path)
		p.selected[to] = true
	}
	dir := p.fs.Dir(e.Path)
	for _, q := range s.panes {
		if q.fs == p.fs && q.dir == dir {
			q.entries, q.err = q.fs.List(q.dir) // its folder view, shown now or later
			if !q.recent {
				s.refreshRows(q, s.focusPath(q))
			}
		}
	}
}

// checkNewName refuses empty names, names with '/', and names in use
// (by the file system's own rule for case, see nameLookup).
func (s *state) checkNewName(p *pane, name, self string) error {
	return checkNameIn(p.fs, p.dir, p.entries, name, self)
}

// checkNameIn is checkNewName for any folder and its listing.
func checkNameIn(f FS, dir string, entries []Entry, name, self string) error {
	if err := validName(name); err != nil {
		return err
	}
	var others []Entry
	for _, e := range entries {
		if e.Name != self {
			others = append(others, e)
		}
	}
	if _, ok := nameLookup(f, dir, others)(name); ok { // self out: a.txt -> A.txt is fine
		return fmt.Errorf("%s already exists", ui.Quote(name))
	}
	return nil
}

// renameEntry renames e to to. A change of case only (a.txt -> A.txt) goes
// through a temporary name: on a case-insensitive file system the target
// "exists" already.
func renameEntry(f FS, e Entry, to string) error {
	if fold(e.Name) != fold(f.Base(to)) {
		return f.Rename(e.Path, to)
	}
	tmp := f.Join(f.Dir(e.Path), ".ak-tmp-"+e.Name)
	if _, err := f.Stat(tmp); err == nil {
		return errExists
	}
	if err := f.Rename(e.Path, tmp); err != nil {
		return err
	}
	if err := f.Rename(tmp, to); err != nil {
		_ = f.Rename(tmp, e.Path)
		return err
	}
	return nil
}

func lastRune(s string) (rune, int) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] < 0x80 || s[i] >= 0xc0 {
			return rune(s[i]), len(s) - i
		}
	}
	return 0, len(s)
}

func firstRuneLen(s string) int {
	n := 1
	for n < len(s) && s[n] >= 0x80 && s[n] < 0xc0 {
		n++
	}
	return n
}

// ---- paths shown to people ----

func expandHome(f FS, p string) string {
	if f.Side() != PC {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// displayPath shortens the home folder to ~ on the PC.
func displayPath(f FS, p string) string {
	if f.Side() != PC {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rel, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rel
	}
	return p
}

// location is where a copy went: "~/Downloads" or "Pixel 9 Pro:/sdcard/DCIM".
func location(f FS, dir string) string {
	if f.Side() == PC {
		return displayPath(f, dir)
	}
	return f.Label() + ":" + dir
}

// place names a folder in a message, quoted as names are and with the
// device after it: "'/sdcard/DCIM' on Pixel 9 Pro", or "'~/Downloads'" on
// the computer.
func place(f FS, dir string) string {
	if f.Side() == PC {
		return ui.Quote(displayPath(f, dir))
	}
	return ui.Quote(dir) + " on " + f.Label()
}

func errText(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "no such folder"
	}
	return err.Error()
}

// targetsSelected are the selected rows that are shown: a filter or hidden
// files hide a selection from d, p and g.
func (s *state) targetsSelected(p *pane) []Entry {
	var list []Entry
	for i, e := range p.rows {
		if p.selected[e.Path] && !s.isUp(p, i) {
			list = append(list, e)
		}
	}
	return list
}

// handleHelp scrolls a help longer than the screen; any other key closes it.
func (s *state) handleHelp(k tui.Key) {
	page := max(s.listHeight(), 1)
	switch k.Kind {
	case tui.KeyUp:
		s.helpTop--
	case tui.KeyDown:
		s.helpTop++
	case tui.KeyPgUp:
		s.helpTop -= page
	case tui.KeyPgDn:
		s.helpTop += page
	case tui.KeyHome:
		s.helpTop = 0
	case tui.KeyEnd:
		s.helpTop = 1 << 20 // clamped when drawn
	default:
		s.mode, s.helpTop = modeNormal, 0
	}
}

// addDone records a summary line and, when it covers several items, their
// names for the lines under it.
func (s *state) addDone(line string, items []string) {
	s.summary.Done = append(s.summary.Done, line)
	for len(s.summary.Items) < len(s.summary.Done)-1 {
		s.summary.Items = append(s.summary.Items, nil)
	}
	if len(items) < 2 {
		items = nil // one item is named in the line itself
	}
	s.summary.Items = append(s.summary.Items, items)
}

// itemNames are the names a summary line covers, folders with a '/', and a
// new name after an arrow: "a.txt → a (1).txt".
func itemNames(done []Entry, as []string) []string {
	names := make([]string, len(done))
	for i, e := range done {
		n := shown(e.Name)
		if e.Dir {
			n += "/"
		}
		if i < len(as) && shown(as[i]) != shown(e.Name) {
			n += " → " + as[i]
		}
		names[i] = n
	}
	return names
}

// shown is a name as it is printed: composed (NFC), so a macOS NFD name
// does not show as loose jamo. Paths keep their own bytes.
func shown(name string) string { return norm.NFC.String(name) }
