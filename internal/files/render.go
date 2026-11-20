package files

import (
	"fmt"
	"strings"
	"time"

	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// visWidth is the columns s takes on screen (escape sequences excluded).
func visWidth(s string) int { return ui.TextWidth(ui.Plain(s)) }

// fitPad cuts s to w columns and pads it to exactly w.
func fitPad(s string, w int) string {
	s = ui.Fit(s, w)
	if d := w - visWidth(s); d > 0 {
		s += strings.Repeat(" ", d)
	}
	return s
}

// truncate cuts plain text to w columns, ending with … when cut.
func truncate(t string, w int) string {
	if ui.TextWidth(t) <= w {
		return t
	}
	if w <= 1 {
		return strings.Repeat("…", max(w, 0))
	}
	var b strings.Builder
	used := 0
	for _, r := range t {
		rw := ui.TextWidth(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

// frameLayout says which fixed lines fit and how many list rows remain.
type frameLayout struct {
	header, topRule, bottomRule bool
	barGap                      bool     // a blank row between the status line and the number bar
	clip                        bool     // the clipboard row over the status line, blank when empty
	bar                         int      // rows of the number key bar: 2 when narrow, 1, or 0
	keys                        []string // the letter keys, under the header: one or two rows
	list                        int
}

// layout keeps at least three list rows and the status line: when the
// terminal is too short it drops the number key bar, then the rules, then
// the header.
func (s *state) layout() frameLayout {
	// The rows depend on the size only, never on a copy running, a question
	// or an input, so the list does not jump when they come and go.
	l := frameLayout{header: true, topRule: true, bottomRule: true, barGap: true, clip: true, bar: 1}
	if s.width < barOneRowWidth {
		l.bar = 2 // 1-5 over 6-0, so the labels are not cut
	}
	// The letter keys change only with what the user switches on purpose
	// (two panes, the recent view, hidden files), never with a copy, a
	// question or a filter, so their rows are as many as they need.
	l.keys = wrapKeys(s.letterKeys(), s.width-1)
	fixed := func() int {
		// The rule between the header and the letter keys comes with the keys;
		// the final 1 is the status line.
		keysRule := b2i(l.header && len(l.keys) > 0)
		return b2i(l.header) + keysRule + b2i(l.topRule) + b2i(l.bottomRule) + len(l.keys) + b2i(l.barGap && l.bar > 0) + b2i(l.clip) + l.bar + 1
	}
	// Too short: the gap over the bar, the letter keys (second row, then
	// all), the bar's second row (back to one cut row), the bar, the rules,
	// then the header go, so three list rows and the status line stay.
	for s.height-fixed() < 3 {
		switch {
		case l.barGap:
			l.barGap = false
		case l.clip:
			l.clip = false
		case len(l.keys) > 0:
			l.keys = l.keys[:len(l.keys)-1]
		case l.bar > 0:
			l.bar--
		case l.bottomRule:
			l.bottomRule = false
		case l.topRule:
			l.topRule = false
		case l.header:
			l.header = false
		default:
			l.list = 1
			return l
		}
	}
	l.list = max(s.height-fixed(), 1)
	return l
}

// frame draws the whole screen as at most s.height lines of at most s.width
// columns (cli-design.md §10).
func (s *state) frame() []string {
	w := s.width
	l := s.layout()
	s.focusExpanded, s.scrolling = false, false // the rows set them again
	if s.mode == modeHelp || s.mode == modeFailures {
		// The help and the failure list take everything over the bottom rule.
		rows := b2i(l.header) + b2i(l.header && len(l.keys) > 0) + len(l.keys) + b2i(l.topRule) + l.list
		var lines []string
		if s.mode == modeHelp {
			lines = s.helpPage(w, rows)
		} else {
			lines = s.overlay(failureLines(s.failures, s.failDone, w, rows), w, rows)
		}
		if l.bottomRule {
			lines = append(lines, s.bottomRuleLine(w))
		}
		if l.clip {
			lines = append(lines, s.clipLine())
		}
		lines = append(lines, " "+s.statusText())
		if l.bar > 0 {
			if l.barGap {
				lines = append(lines, "")
			}
			lines = append(lines, s.bottomBar(w, l.bar)...)
		}
		return lines[:min(len(lines), max(s.height, 0))]
	}
	var area []string // header, rule, list rows
	switch {
	case s.split:
		lw := (w - 1) / 2
		rw := w - 1 - lw
		left := s.paneLines(s.panes[Device], lw, s.active == Device, l.list, true)
		right := s.paneLines(s.panes[PC], rw, s.active == PC, l.list, true)
		for i := range left {
			sep := Dim + "│" + NC
			if i == 1 { // where the rules cross: the way p or g copies from this side
				sep = Cyan + map[Side]string{Device: "▶", PC: "◀"}[s.active] + NC
			}
			area = append(area, fitPad(left[i], lw)+sep+right[i])
		}
	default:
		area = s.paneLines(s.cur(), w, true, l.list, false)
	}
	var lines []string
	if l.header {
		lines = append(lines, area[0])
		if len(l.keys) > 0 {
			lines = append(lines, s.headerRule(w))
		}
	}
	for _, k := range l.keys { // the letter keys under the header, over the list; the number keys at the bottom
		lines = append(lines, " "+k)
	}
	if l.topRule {
		lines = append(lines, area[1])
	}
	lines = append(lines, area[2:]...)
	if l.bottomRule {
		lines = append(lines, s.bottomRuleLine(w))
	}
	if l.clip {
		lines = append(lines, s.clipLine())
	}
	lines = append(lines, " "+s.statusText())
	if l.bar > 0 {
		if l.barGap {
			lines = append(lines, "")
		}
		lines = append(lines, s.bottomBar(w, l.bar)...)
	}
	if len(lines) > s.height {
		lines = lines[:max(s.height, 0)] // never scroll the screen
	}
	return lines
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// overlay shows text lines in the header-and-list area.
func (s *state) overlay(text []string, w, h int) []string {
	out := make([]string, 0, h)
	for _, t := range text {
		if len(out) == h {
			break
		}
		out = append(out, ui.Fit(t, w))
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// helpItem is one line of the help: a key with its letter alias, and what
// it does.
type helpItem struct{ key, alias, text string }

// helpSection is a titled group of the help.
type helpSection struct {
	title string
	items []helpItem
}

// helpSections is the whole help (1 or ?): every key, with its letter
// alias next to it, and how opening, copying, deleting and the recent list
// work.
func helpSections() []helpSection {
	tmp := displayPath(localFS{PC, "PC"}, tempDir())
	return []helpSection{
		{"Move", []helpItem{
			{"↑ ↓", "", "One item; in two columns they run on into the next column"},
			{"PgUp PgDn", "", "A screen (in two columns, both columns)"},
			{"Home End", "", "First and last item"},
			{"← →", "", "Two columns: the other column. One column: parent folder, open"},
			{"Enter", "", "Go into a folder or open a file (see How things work)"},
			{"Backspace", "", "Parent folder"},
			{":", "", "Go to a path you type"},
		}},
		{"Select", []helpItem{
			{"Space", "", "Select or unselect, then move down"},
			{"a", "", "Select or unselect everything here (as in the install and pull pickers)"},
			{"/", "", "Filter by name as you type; Esc turns it off"},
			{"", "", "A selection is used by 3, 4, 5 and 8; selected items a filter or hidden files hide are left out"},
		}},
		{"Copy", []helpItem{
			{"5 To computer", "g", "On the device side: copy to the computer side's current folder now (adb pull)"},
			{"5 To device", "p", "On the computer side: copy to the device side's current folder now (adb push)"},
			{"3 Copy", "c", "Take the items into ak's clipboard to paste later: it stays while you go to other folders and panes (a selection does not). A new copy or cut replaces it"},
			{"4 Cut", "x", "The same, and the paste moves them: the originals go once each copy is complete (on one side, a rename)"},
			{"6 Paste", "v", "Paste the clipboard into the folder where the cursor is: device ↔ computer, or within the device or the computer. A copy stays in the clipboard for more pastes; what a cut moved leaves it"},
			{"", "", "What 3, 4, 5 take: the selected items shown, or else the item under the cursor. In the same folder a copy gets ' (N)' and a cut does nothing; a folder never goes into itself"},
			{"o s r", "", "A name already there: overwrite, skip, or rename with ' (N)'. A capital letter does all the rest; Esc or q cancels"},
			{"", "", "Overwrite copies to a temporary name first, checks the size, then swaps it in; if that fails or is canceled, the old item stays. A move keeps the original when the size can't be checked"},
			{"Esc", "", "Stops copying (a progress bar shows it) and removes the half-copied item; else turns the filter off; else empties the clipboard"},
		}},
		{"How things work", []helpItem{
			{"Open", "", "A folder: go into it. A device file is copied to " + tmp + " and opened with its default app; a big file takes a while (progress bar, Esc cancels). Files there are removed on a later start, a day after. A computer file opens at once. With a selection, only the file under the cursor opens"},
			{"Delete", "", "Always asks. On the device it is rm -rf; on the computer it is permanent too, not the Trash"},
			{"Recent", "", "The device's 300 newest files from MediaStore, the list of the gallery and the Files app 'Recent'; about a second the first time. From the computer side too"},
			{"Free space", "", "The device's df, read when a folder opens and after a copy or a delete"},
			{"Device", "", "Reached with adb's shell rights: the main user's storage (user 0) only, not a work profile"},
		}},
		{"Manage", []helpItem{
			{"0 Rename", "r", "Rename the item under the cursor (in the recent view too)"},
			{"7 Mkdir", "n", "New folder"},
			{"8 Delete", "d", "Delete the selection or the item (Delete key too)"},
		}},
		{"View", []helpItem{
			{"Tab", "", "Device ↔ computer (two panes: the other pane)"},
			{"b", "", "One pane: one wide column (whole long names) or two columns"},
			{"9 Show computer", "w", "The computer pane beside the device on or off"},
			{"2 Recent", "t", "The device's recent files ↔ its folder"},
			{"1 Sort", "s", "Name → newest → size, for the pane with the cursor only; the recent view keeps its own order (newest first to start). Each pane's header shows its order"},
			{".", "", "Hidden files on or off"},
			{"?", "", "This help"},
			{"q", "", "Quit (Ctrl+C too)"},
		}},
		{"Colors", []helpItem{
			{Red + "red" + NC, "", "can't be read"},
			{Purple + "purple" + NC, "", "a symbolic link"},
			{Yellow + "yellow" + NC, "", "read-only"},
			{Blue + "blue" + NC, "", "a folder"},
			{Green + "green" + NC, "", "a file that can be run"},
			{Dim + "dim" + NC, "", "hidden"},
			{Cyan + Bold + "cyan" + NC, "", "the cursor"},
		}},
		{"Status line", []helpItem{
			{"", "", "2/48 where the cursor is · folders, files and their size here (free space on the device) · N selected · the filter · the sort"},
		}},
	}
}

// helpColumnWidth is the width from which the help is laid out in two
// columns.
const helpColumnWidth = 120

// helpText lays the sections out for w columns: one column, or two when
// wide. A key is never cut; text wraps under its description.
func helpText(w int) []string {
	secs := helpSections()
	if w < helpColumnWidth {
		var lines []string
		for _, sec := range secs {
			lines = append(lines, sectionLines(sec, w)...)
		}
		return lines
	}
	cw := (w - 3) / 2
	var all [][]string
	total := 0
	for _, sec := range secs {
		l := sectionLines(sec, cw)
		all = append(all, l)
		total += len(l)
	}
	// Sections in order, split where the taller column is shortest.
	best, bestH, n := 0, total, 0
	for i := range all {
		n += len(all[i])
		if h := max(n, total-n); h < bestH {
			best, bestH = i+1, h
		}
	}
	var left, right []string
	for i, l := range all {
		if i < best {
			left = append(left, l...)
		} else {
			right = append(right, l...)
		}
	}
	lines := make([]string, max(len(left), len(right)))
	for i := range lines {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = fitPad(l, cw) + "   " + r
	}
	return lines
}

// sectionLines is a section in w columns: a bold title, then the items with
// their keys in a column (cyan key, dim alias in parentheses) and the text
// wrapped beside them, or under them when w is narrow.
func sectionLines(sec helpSection, w int) []string {
	keyW := 0
	for _, it := range sec.items {
		keyW = max(keyW, visWidth(helpKey(it)))
	}
	textX := keyW + 3 // " key  text"
	under := w-textX < 24
	lines := []string{" " + Bold + sec.title + NC}
	for _, it := range sec.items {
		key := helpKey(it)
		if key == "" {
			for _, t := range wrapText(it.text, w-3) {
				lines = append(lines, "   "+t)
			}
			continue
		}
		if under {
			lines = append(lines, " "+key)
			for _, t := range wrapText(it.text, w-5) {
				lines = append(lines, "     "+t)
			}
			continue
		}
		for i, t := range wrapText(it.text, w-textX) {
			if i == 0 {
				lines = append(lines, " "+key+strings.Repeat(" ", keyW-visWidth(key))+"  "+t)
			} else {
				lines = append(lines, strings.Repeat(" ", textX)+t)
			}
		}
	}
	return append(lines, "")
}

// helpKey is "8 Delete (d)": the key in cyan (a color sample as it is),
// the alias dim.
func helpKey(it helpItem) string {
	if it.key == "" {
		return ""
	}
	k := it.key
	if !strings.Contains(k, "\033") {
		k = Cyan + k + NC
	}
	if it.alias != "" {
		k += Dim + " (" + it.alias + ")" + NC
	}
	return k
}

// wrapText breaks plain text into lines of at most w columns at spaces; a
// word longer than a line is cut.
func wrapText(t string, w int) []string {
	w = max(w, 1)
	var lines []string
	cur := ""
	for _, word := range strings.Fields(t) {
		switch {
		case cur == "":
			cur = word
		case ui.TextWidth(cur)+1+ui.TextWidth(word) <= w:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
		for ui.TextWidth(cur) > w {
			cut := ui.Plain(ui.Fit(cur, w))
			lines = append(lines, cut)
			cur = cur[len(cut):]
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// helpPage is the help in rows lines from s.helpTop; when it is longer
// than that, the last line says how to scroll.
func (s *state) helpPage(w, rows int) []string {
	text := helpText(w)
	foot := " " + Dim + "Any key closes" + NC
	if len(text) > rows {
		foot = " " + Dim + "↑↓ PgUp PgDn scroll · any other key closes" + NC
		s.helpTop = min(max(s.helpTop, 0), len(text)-(rows-1))
		text = text[s.helpTop:]
	} else {
		s.helpTop = 0
	}
	out := make([]string, 0, rows)
	for _, t := range text {
		if len(out) == rows-1 {
			break
		}
		out = append(out, ui.Fit(t, w))
	}
	for len(out) < rows-1 {
		out = append(out, "")
	}
	if rows > 0 {
		out = append(out, ui.Fit(foot, w))
	}
	return out
}

// failureLines is what Enter shows after "3 of 5 pulled, 2 failed" in h
// rows: the failures, then the items that went through side by side in at
// most two rows, so a partly failed job does not look failed as a whole.
// Failures come first: the done names give way to a count when they would
// leave the failures fewer than three rows, and failures that don't fit are
// counted on their last row. The whole
// list is in the summary after quitting.
func failureLines(list []failure, done []string, w, h int) []string {
	var tail []string
	if len(done) > 0 {
		tail = []string{"", " " + Bold + fmt.Sprintf("Done (%d)", len(done)) + NC}
		for i, r := range packItems(done, w-3, 2) {
			mark := "   "
			if i == 0 {
				mark = " " + Green + "✓" + NC + " "
			}
			tail = append(tail, mark+Dim+r+NC)
		}
		need := min(len(list), 3) // rows the failures get before the done names
		if h-1-len(tail) < need {
			tail = []string{"", " " + Green + "✓" + NC + " " + fmt.Sprintf("%d done", len(done))}
		}
		if h-1-len(tail) < 1 {
			tail = nil
		}
	}
	room := max(h-1-len(tail), 1)
	lines := []string{" " + Bold + fmt.Sprintf("Failed (%d)", len(list)) + NC}
	for i, f := range list {
		if i == room-1 && len(list) > room {
			lines = append(lines, "   "+Dim+fmt.Sprintf("… and %d more failed", len(list)-i)+NC)
			break
		}
		lines = append(lines, " "+Red+"✗"+NC+" "+f.name+"  "+Dim+f.msg+NC)
	}
	return append(lines, tail...)
}

// paneLines is a pane's header, rule and h list rows. With two columns the
// rows run down the left column and on in the right one, like a newspaper.
func (s *state) paneLines(p *pane, w int, active bool, h int, compact bool) []string {
	cols := 1
	if !compact && s.columns() == 2 {
		cols = 2
	}
	lw := w
	color := Dim
	if active && s.split {
		color = Cyan // two panes: the rules of the pane with the cursor stand out
	}
	rule := color + strings.Repeat("─", max(w, 1)) + NC
	if cols == 2 {
		lw = (w - 1) / 2
		rule = color + strings.Repeat("─", lw) + "┬" + strings.Repeat("─", max(w-1-lw, 0)) + NC
		s.scrollColumns(p, h)
	} else {
		s.scroll(p, h)
	}
	lines := []string{s.header(p, w, active), rule}
	var cells []string
	switch {
	case p.recent && p.recentList == nil:
		cells = append(cells, " "+Dim+string(spinFrames[s.spin%len(spinFrames)])+" "+p.loadingNote+"…"+NC)
	case p.recent && p.recentErr != nil:
		cells = append(cells, " "+Yellow+"Couldn't read recent files: "+p.recentErr.Error()+NC)
	default:
		for i := p.top; i < len(p.rows) && i < p.top+cols*h; i++ {
			cells = append(cells, s.row(p, i, lw, active, compact, cols == 2))
		}
		if p.err != nil {
			cells = append(cells, "      "+Yellow+errText(p.err)+NC)
		} else if len(p.rows) == 0 || (len(p.rows) == 1 && s.isUp(p, 0)) {
			note := "(empty)"
			if p.filter != "" {
				note = "(no names match)"
			} else if p.recent {
				note = "(no recent files)"
			}
			cells = append(cells, "      "+Dim+note+NC)
		}
	}
	cell := func(i int) string {
		if i < len(cells) {
			return cells[i]
		}
		return ""
	}
	for r := 0; r < h; r++ {
		if cols == 2 {
			lines = append(lines, fitPad(cell(r), lw)+Dim+"│"+NC+cell(h+r))
		} else {
			lines = append(lines, cell(r))
		}
	}
	return lines
}

// scroll keeps the cursor on screen.
func (s *state) scroll(p *pane, h int) {
	if p.cursor < p.top {
		p.top = p.cursor
	}
	if p.cursor >= p.top+h {
		p.top = p.cursor - h + 1
	}
	if p.top > max(len(p.rows)-h, 0) {
		p.top = max(len(p.rows)-h, 0)
	}
}

// scrollColumns keeps the cursor on screen in two columns of h rows,
// shifting a whole column at a time (mc's brief view): rows 1-10 | 11-20,
// then 11-20 | 21-30 once the cursor reaches 21.
func (s *state) scrollColumns(p *pane, h int) {
	h = max(h, 1)
	first := p.top / h // the left column, as a column number
	switch col := p.cursor / h; {
	case col < first:
		first = col
	case col > first+1:
		first = col - 1
	}
	p.top = first * h
}

func (s *state) header(p *pane, w int, active bool) string {
	label := p.fs.Label()
	where := shown(displayPath(p.fs, p.dir))
	if p.recent {
		where = "Recent files"
	}
	color := Bold
	if active && s.split {
		color = Cyan + Bold
	}
	// The filter and the sort order of this pane: with two panes each shows
	// its own, as each sorts on its own.
	key := p.sortBy
	if p.recent {
		key = p.recentSort
	}
	r := key.label() + " "
	if p.filter != "" {
		r = "/" + p.filter + "  " + r
	}
	room := w - 3 - ui.TextWidth(label) // " label  "
	if room-ui.TextWidth(r)-2 >= min(ui.TextWidth(where), 20) {
		room -= ui.TextWidth(r) + 2
	} else {
		r = ""
	}
	left := " " + color + label + NC + "  " + truncateLeft(where, max(room, 1))
	if r == "" {
		return left
	}
	return left + strings.Repeat(" ", max(w-visWidth(left)-ui.TextWidth(r), 2)) + Dim + r + NC
}

// truncateLeft keeps the end of a path: "…/DCIM/Camera".
func truncateLeft(t string, w int) string {
	if ui.TextWidth(t) <= w {
		return t
	}
	r := []rune(t)
	used := 1 // the …
	i := len(r)
	for i > 0 && used+ui.TextWidth(string(r[i-1])) <= w {
		i--
		used += ui.TextWidth(string(r[i]))
	}
	return "…" + string(r[i:])
}

// row is one list line: marks, name, size and age, fitted to w. brief is a
// column of the two-column view: short sizes ("3.2M") and times ("11-03").
func (s *state) row(p *pane, i, w int, active, compact, brief bool) string {
	e := p.rows[i]
	focused := active && i == p.cursor
	mark := "  "
	if focused {
		mark = " " + Cyan + "➤" + NC
	}
	check := "    "
	if p.selected[e.Path] {
		check = Green + "[✓]" + NC + " "
	}
	size, when := "", ""
	if !e.Dir {
		size = HumanSize(e.Size)
		if brief {
			size = shortSize(e.Size)
		}
	}
	if !s.isUp(p, i) {
		when = fileTime(e.ModTime, s.now(), compact || brief)
	}
	sizeW, ageW := 9, 11 // "430.0 MB", "11-03 14:05"
	if compact || brief {
		ageW = 5 // "11-03"
	}
	if brief {
		sizeW = 5 // "430M"
	}
	right := fmt.Sprintf("%*s %*s ", sizeW, size, ageW, when)
	nameW := w - 6 - len(right) // marks take 6 columns
	name := shown(e.Name)       // a macOS NFD name shows as its syllables
	if e.Dir && !s.isUp(p, i) {
		name += "/"
	}
	suffix := ""
	if e.Folder != "" {
		suffix = "  " + shown(e.Folder)
	}
	if focused && ui.TextWidth(name) > nameW && w-6 > nameW {
		// The cursor's row shows the whole name over the size and date
		// columns (the status line shows those); a name longer than the
		// row scrolls left after a moment.
		full := w - 6
		s.focusExpanded = true
		shown := s.marquee(p.fs.Side(), e.Path, name, full)
		return mark + check + Cyan + Bold + shown + NC + strings.Repeat(" ", max(full-ui.TextWidth(shown), 0))
	}
	name = truncateMiddle(name, max(nameW, 1))
	suffix = truncate(suffix, max(nameW-ui.TextWidth(name), 0))
	pad := strings.Repeat(" ", max(nameW-ui.TextWidth(name)-ui.TextWidth(suffix), 0))
	styled := name
	switch {
	case focused:
		styled = Cyan + Bold + name + NC
	default:
		if c := nameColor(e); c != "" {
			styled = c + name + NC
		}
	}
	if nameW < 4 { // too narrow for sizes: the name alone
		return mark + check + styled
	}
	return mark + check + styled + Dim + suffix + NC + pad + Dim + right + NC
}

// statusText is the status line: a question or an input line, the progress,
// a message, or else where the cursor is and what the folder holds.
func (s *state) statusText() string {
	key := func(k, desc string) string { return "  " + Cyan + k + NC + " " + desc }
	switch s.mode {
	case modeDelete:
		return fmt.Sprintf("%sDelete %s from %s? This cannot be undone.%s [y/N]",
			Yellow, countItems(s.delTargets), s.cur().fs.Label(), NC)
	case modeConflict:
		op := s.plan.ops[s.ci]
		return fmt.Sprintf("%s'%s' already exists.%s", Yellow, op.name, NC) +
			key("o", "Overwrite") + key("s", "Skip") + key("r", "Rename (N)") + "  " + Dim + "· Shift = all" + NC
	case modeFilter:
		return "Filter: " + s.inputLine()
	case modeInput:
		prompt := [...]string{"Rename to: ", "New folder: ", "Go to: "}[s.inputKind]
		return prompt + s.inputLine()
	}
	if s.task != nil {
		return s.progressText(s.width - 1) // the status line starts with a space
	}
	if s.msg != nil {
		t := s.msg.text
		switch s.msg.kind {
		case msgSuccess:
			t = Green + "✓" + NC + " " + t
		case msgWarn:
			t = Yellow + t + NC
		case msgError:
			t = Red + t + NC
		}
		if len(s.msg.failures) > 0 {
			t += key("Enter", "Details")
		}
		return t
	}
	return s.infoLine(s.width - 2) // a column of margin on each side, like the left edge
}

// infoLine is "2/48  3 folders  45 files  1.8 GB  1 selected 3.2 MB  filter: x"
// with "54.0 GB free" at the right on the device. It counts the folder's own
// entries (hidden ones only when shown), not what is under them; the
// position leaves ".." out.
func (s *state) infoLine(w int) string {
	p := s.cur()
	n, pos := len(p.rows), p.cursor+1
	if n > 0 && s.isUp(p, 0) {
		n, pos = n-1, pos-1
	}
	free := "" // the device's free space, right after the size of what is here
	if p.free >= 0 {
		free = " " + Dim + "(" + HumanSize(p.free) + " free)" + NC
	}
	var parts []string
	// The cursor's row covers its own size and date when its name is long:
	// they show here instead, in the cursor's color.
	cursorInfo := ""
	if e, ok := s.cursorEntry(p); ok && s.focusExpanded {
		var info []string
		if !e.Dir && e.Size >= 0 { // unknown (not scanned by MediaStore yet): left out, not "?"
			info = append(info, HumanSize(e.Size))
		}
		if !e.ModTime.IsZero() {
			info = append(info, e.ModTime.In(s.now().Location()).Format("2006-01-02 15:04"))
		}
		if p.recent && e.Folder != "" {
			info = append(info, shown(e.Folder)) // the expanded row has no room for it
		}
		cursorInfo = Cyan + strings.Join(info, "  ") + NC
	}
	if p.recent {
		parts = append(parts, fmt.Sprintf("Recent %d/%d", pos, n)+free)
		if cursorInfo != "" {
			parts = append(parts, cursorInfo)
		}
	} else {
		parts = append(parts, fmt.Sprintf("%d/%d", pos, n))
		if cursorInfo != "" {
			parts = append(parts, cursorInfo)
		}
		var dirs, files int
		var bytes int64
		for _, e := range p.entries {
			switch {
			case !s.showHidden && strings.HasPrefix(e.Name, "."):
			case e.Dir:
				dirs++
			default:
				files++
				bytes += max(e.Size, 0)
			}
		}
		parts = append(parts, plural(dirs, "folder"), plural(files, "file"), HumanSize(bytes)+free)
	}
	var sel int
	var selBytes int64
	for _, e := range s.targetsSelected(p) {
		sel++
		if !e.Dir {
			selBytes += max(e.Size, 0)
		}
	}
	if sel > 0 {
		parts = append(parts, Green+fmt.Sprintf("%d selected %s", sel, HumanSize(selBytes))+NC)
	}
	if p.filter != "" {
		parts = append(parts, "filter: "+p.filter+" "+Dim+"(Esc clears)"+NC)
	}
	return strings.Join(parts, "  ") // each pane's sort order is in its header
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// progressText is "Pulling 2/3  huge.bin  Pixel → ~/Downloads  ████░░░░ 42%
// (180 / 430 MB)  Esc Cancel" in at most width columns. When it is too
// wide, the name and route are cut first, then the sizes go, then the bar
// shrinks to its 10 columns; the percentage always stays. Without a total
// size it is a spinner instead of the bar.
func (s *state) progressText(width int) string {
	p := s.prog
	t := s.task
	head := t.label
	if p.count > 1 {
		head += fmt.Sprintf(" %d/%d", max(p.index, 1), p.count) // one item: no "1/1"
	}
	name, rt := p.name, t.route
	if t.route == "" {
		name = "" // "Opening x" and "Deleting 3 items" name it in the label
	}
	tail := ""
	if t.cancelable {
		tail = "  " + Cyan + "Esc" + NC + " Cancel"
	}
	join := func(name, rt, rest string) string {
		out := head
		for _, part := range []string{name, rt, rest} {
			if part != "" {
				out += "  " + part
			}
		}
		return out + tail
	}
	done, total := p.done, p.total
	sizes := fmt.Sprintf(" (%s / %s)", HumanSize(done), HumanSize(total))
	switch {
	case p.measuring:
		return string(spinFrames[s.spin%len(spinFrames)]) + " " + join(name, rt, "")
	case p.count > 1 && p.all > 0:
		// Several items of known size: one bar for the whole job by bytes,
		// closest to the time left and moving during a big file too (a bar
		// per file barely moves for small files, which finish between two
		// size checks). The current item's name still shows.
		head = t.label + fmt.Sprintf(" %d/%d", max(p.index, 1), p.count)
		done, total = p.before+max(p.done, 0), p.all
		sizes = fmt.Sprintf(" (%s / %s)", HumanSize(done), HumanSize(total))
	case p.count > 1:
		// Several items, some of unknown size (or a delete): the bar counts
		// items, so the "2/10" after the label would only repeat it.
		head = t.label
		done, total = int64(max(p.index-1, 0)), int64(p.count)
		sizes = fmt.Sprintf(" (%d / %d)", done, total)
	case total <= 0:
		// One item of unknown size (a delete, a folder rm -rf): a spinner
		// and, past a few seconds, how long it has run.
		spin := string(spinFrames[s.spin%len(spinFrames)]) + " " + join(name, rt, "")
		if d := s.now().Sub(t.started); !t.started.IsZero() && d >= 3*time.Second {
			spin += "  " + Dim + d.Truncate(time.Second).String() + NC
		}
		return spin
	}
	// One item of known size: the bar counts its bytes.
	pct := fmt.Sprintf(" %d%%", min(done*100/total, 100))
	const minBar, maxBar = 10, 40
	// Give the bar 20 columns if cutting the name and the path allows it,
	// else 10; drop the sizes before cutting too much. The percent stays.
	for _, withSizes := range []bool{true, false} {
		if !withSizes {
			sizes = ""
		}
		used := visWidth(join(name, rt, "x")) - 1 + ui.TextWidth(pct+sizes) // everything but the bar
		free := width - used
		for _, want := range []int{2 * minBar, minBar} {
			if free >= want {
				return join(name, rt, bar(min(free, maxBar), done, total)+pct+sizes)
			}
			room := ui.TextWidth(name) + ui.TextWidth(rt) - (want - free)
			if room < 18 && (withSizes || want > minBar) {
				continue // too tight even cut: a shorter bar, then no sizes
			}
			rw := min(ui.TextWidth(rt), max(room/2, room-ui.TextWidth(name)))
			nw := max(room-rw, 1)
			return join(truncate(name, nw), truncateRoute(rt, max(rw, 1)), bar(want, done, total)+pct+sizes)
		}
	}
	return "" // not reached
}

// truncateRoute cuts a route ("Pixel → ~/Downloads", "Pixel:/sdcard/x ← Computer")
// to w columns by shortening its PC or device path from the left, so the
// device name and the arrow, which say where the files go, stay.
func truncateRoute(rt string, w int) string {
	if ui.TextWidth(rt) <= w {
		return rt
	}
	if l, path, ok := strings.Cut(rt, " → "); ok { // pull: the PC path is on the right
		if keep := ui.TextWidth(l + " → "); w-keep >= 4 {
			return l + " → " + truncateLeft(path, w-keep)
		}
	} else if left, r, ok := strings.Cut(rt, " ← "); ok { // push: the device path is on the left
		label, path, _ := strings.Cut(left, ":")
		if keep := ui.TextWidth(label + ": ← " + r); w-keep >= 4 {
			return label + ":" + truncateLeft(path, w-keep) + " ← " + r
		}
	}
	return truncateLeft(rt, w)
}

// bar is a w-column progress bar: █ done, ░ to go.
func bar(w int, done, total int64) string {
	n := int(min(max(done*int64(w)/total, 0), int64(w)))
	return Green + strings.Repeat("█", n) + NC + Dim + strings.Repeat("░", w-n) + NC
}

func (s *state) inputLine() string {
	t, c := s.inputText, s.inputCursor
	if c >= len(t) {
		return t + "\033[7m \033[0m"
	}
	n := firstRuneLen(t[c:])
	return t[:c] + "\033[7m" + t[c:c+n] + "\033[0m" + t[c+n:]
}

// modeKeys is the key line of a question or an input, in the number key
// bar's place.
func (s *state) modeKeys() string {
	k := func(key, desc string) string { return Cyan + key + NC + " " + Dim + desc + NC + "  " }
	switch s.mode {
	case modeFilter:
		return k("Enter", "Keep filter") + k("Esc", "Clear filter") + k("↑↓", "Move")
	case modeInput:
		return k("Enter", "OK") + k("Esc", "Cancel")
	case modeDelete:
		return k("y", "Delete") + k("n Esc q", "Keep")
	case modeConflict:
		return k("Enter", "Rename") + k("Esc q", "Cancel")
	case modeHelp:
		return k("↑↓ PgUp PgDn", "Scroll") + k("Any other key", "Close")
	case modeFailures:
		return k("Any key", "Close")
	}
	return ""
}

// barLabels are the number keys 1-9 and 0 as mc shows its F keys, and
// whether each does something here; one that does not keeps its label,
// drawn dim (barRow).
func (s *state) barLabels() (labels [10]string, on [10]bool) {
	p := s.cur()
	idle := s.task == nil
	_, onEntry := s.cursorEntry(p)
	hasTargets := onEntry || len(s.targetsSelected(p)) > 0
	labels = [10]string{"Sort", "Recent", "Copy", "Cut", "", "Paste", "Mkdir", "Delete", "Show computer", "Rename"}
	labels[4] = map[Side]string{Device: "To computer", PC: "To device"}[p.fs.Side()]
	if s.split {
		labels[8] = "Hide computer"
	}
	if p.fs.Side() == Device && p.recent {
		labels[1] = "Folder"
	}
	on = [10]bool{
		true, true,
		idle && hasTargets, idle && hasTargets, idle && hasTargets, // copy, cut, to the other side
		idle && s.clip != nil && !p.recent, // paste
		idle && !p.recent,                  // mkdir
		idle && hasTargets,                 // delete
		s.split || s.width >= minSplitWidth,
		idle && onEntry, // rename (the recent view too)
	}
	return labels, on
}

// barOneRowWidth is the width from which the number key bar fits on one
// row with every label and alias ("9 Show computer (w)" needs 20 columns a
// cell).
const barOneRowWidth = 200

// bottomBar is the number key bar, mc style: the width split into cells, each
// a bold digit, a space and a dim label cut to fit. On two rows the keys 1-5
// go over 6-0 with wider cells. In a question or an input, that mode's keys
// instead.
func (s *state) bottomBar(w, rows int) []string {
	if mk := s.modeKeys(); mk != "" {
		out := []string{" " + mk}
		for len(out) < rows {
			out = append(out, "")
		}
		return out
	}
	labels, on := s.barLabels()
	aliases := s.barAliases()
	if rows < 2 {
		return []string{barRow(labels[:], aliases[:], on[:], 0, w)}
	}
	return []string{barRow(labels[:5], aliases[:5], on[:5], 0, w), barRow(labels[5:], aliases[5:], on[5:], 5, w)}
}

// barAliases are the letter keys that do the same as each number key, shown
// after the label ("9 Show computer (w)"): the keys of before the number bar still
// work.
func (s *state) barAliases() [10]string {
	var a [10]string
	for d, letter := range numberKeys {
		a[(int(d-'0')+9)%10] = string(letter)
	}
	a[4] = map[Side]string{Device: "g", PC: "p"}[s.cur().fs.Side()]
	return a
}

// barRow draws labels as equal cells over w columns; first is the index of
// the first key (its digit is (first+i+1)%10). A label is followed by its
// letter alias, dim in parentheses, when the cell has room: "9 Show computer (w)".
func barRow(labels, aliases []string, on []bool, first, w int) string {
	n := len(labels)
	var b strings.Builder
	for i, label := range labels {
		cw := w / n
		if i < w%n {
			cw++ // the first cells take the remainder
		}
		if cw < 1 {
			continue
		}
		digit := string(rune('0' + (first+i+1)%10))
		cell := Cyan + Bold + digit + NC
		if !on[i] {
			// A key that does nothing here: all dim, and no alias. Without
			// color an × before the label says it (writeFrame drops colors).
			cell = Dim + digit + NC
			if !ui.Err.Color && label != "" {
				label = "×" + label
			}
		}
		gap := " " // between the digit and its label; dropped when cells are tiny
		if cw < 6 {
			gap = ""
		}
		used := 1
		if room := cw - 2 - len(gap); label != "" && room > 0 { // and a space before the next digit
			cut := ui.Plain(ui.Fit(label, room))
			if on[i] {
				cell += gap + cut
			} else {
				cell += gap + Dim + cut + NC
			}
			used += len(gap) + ui.TextWidth(cut)
			if alias := " (" + aliases[i] + ")"; on[i] && aliases[i] != "" && cut == label && used+len(alias) <= cw-1 {
				cell += Dim + alias + NC
				used += len(alias)
			}
		}
		b.WriteString(cell + strings.Repeat(" ", max(cw-used, 0)))
	}
	return b.String()
}

// letterKeys are the keys without a number, for the row under the header,
// right over the list (the number keys are at the bottom): every key is on
// screen, and ? explains them at more length.
func (s *state) letterKeys() []string {
	k := func(key, desc string) string { return Cyan + key + NC + " " + Dim + desc + NC }
	p := s.cur()
	var out []string
	switch {
	case s.split:
		out = append(out, k("Tab", "Other pane"))
	case s.active == Device:
		out = append(out, k("Tab", "Computer"))
	default:
		out = append(out, k("Tab", "Device"))
	}
	if s.columns() == 2 {
		out = append(out, k("←→", "Column"))
	} else {
		out = append(out, k("←→", "Up/Open"))
	}
	out = append(out, k("Space", "Select"), k("a", "All"), k("/", "Filter"))
	out = append(out, k(".", map[bool]string{true: "Hide hidden", false: "Show hidden"}[s.showHidden]))
	out = append(out, k("?", "Help"), k("q", "Quit"))
	// Keys that work only in some views come last and are left out where
	// they do nothing, so the row stays short and the keys before them
	// never move: b needs one pane wide enough for two columns, : Go to a
	// folder (not the recent view).
	if !s.split && s.width >= minColumnsWidth {
		out = append(out, k("b", map[bool]string{true: "2 columns", false: "1 column"}[s.oneColumn]))
	}
	if !p.recent {
		out = append(out, k(":", "Go to"))
	}
	// Esc is not here: it shows where it acts, "Esc Cancel" on the progress
	// line and "Esc clears" beside the filter on the status line. A key that
	// came and went here would change how many rows the keys take.
	return out
}

// wrapKeys puts key hints on one row, or on two of about the same length
// when they do not fit in w columns (a hint is never split; a row still too
// long is cut by ui.Fit).
func wrapKeys(items []string, w int) []string {
	join := func(list []string) string { return strings.Join(list, "  ") }
	if len(items) == 0 {
		return nil
	}
	if visWidth(join(items)) <= w || len(items) == 1 {
		return []string{join(items)}
	}
	best, bestW := 1, -1
	for cut := 1; cut < len(items); cut++ {
		widest := max(visWidth(join(items[:cut])), visWidth(join(items[cut:])))
		if bestW < 0 || widest < bestW {
			best, bestW = cut, widest
		}
	}
	return []string{join(items[:best]), join(items[best:])}
}

// bottomRuleLine is the rule over the status line. With two panes, the half
// under the pane with the cursor is colored like that pane's top rule.
func (s *state) bottomRuleLine(w int) string {
	if !s.split {
		return Dim + strings.Repeat("─", max(w, 1)) + NC
	}
	lw := (w - 1) / 2
	left, right := Dim, Dim
	if s.active == Device {
		left = Cyan
	} else {
		right = Cyan
	}
	return left + strings.Repeat("─", lw) + NC + Dim + "┴" + NC + right + strings.Repeat("─", max(w-1-lw, 0)) + NC
}

// headerRule parts the header (device name and path) from the letter keys
// under it. With two panes it closes the divider between the two headers.
func (s *state) headerRule(w int) string {
	if !s.split {
		return Dim + strings.Repeat("─", max(w, 1)) + NC
	}
	lw := (w - 1) / 2
	return Dim + strings.Repeat("─", lw) + "┴" + strings.Repeat("─", max(w-1-lw, 0)) + NC
}

// nameColor colors a name by what this user may do with it, not by its
// extension (the first that applies):
//
//	red     cannot be read (a folder: cannot be listed)
//	purple  a symbolic link
//	yellow  read-only: copy it out, but not push, rename or delete there
//	blue    a folder
//	green   a file that can be run
//	dim     a hidden file
//
// "" is the terminal's own color.
func nameColor(e Entry) string {
	switch {
	case e.NoRead:
		return Red
	case e.Link:
		return Purple
	case e.ReadOnly:
		return Yellow
	case e.Dir:
		return Blue
	case e.Exec:
		return Green
	case strings.HasPrefix(e.Name, "."):
		return Dim
	}
	return ""
}

// truncateMiddle cuts t to w columns by dropping its middle, so the end of a
// long name (a version, a date, the extension) stays: names that start the
// same ("app-release-v1.0.1-…") still differ on screen.
func truncateMiddle(t string, w int) string {
	if ui.TextWidth(t) <= w {
		return t
	}
	if w < 8 {
		return truncate(t, w)
	}
	tailW := (w - 1) / 2                                      // as much end as start: versions and dates sit at the end
	head := strings.TrimSuffix(truncate(t, w-tailW), "…")     // w-1-tailW columns
	tail := strings.TrimPrefix(truncateLeft(t, tailW+1), "…") // tailW columns
	return head + "…" + tail
}

// Marquee timing for a cursor name longer than its row.
const (
	marqueeWait = time.Second             // before it starts moving
	marqueeStep = 150 * time.Millisecond  // per column
	marqueeHold = 1500 * time.Millisecond // at the end, before starting over
)

// marquee is the w columns of name to show now: the start for marqueeWait
// after the cursor came to this item, then one column further every
// marqueeStep until the end shows, held for marqueeHold, then again.
func (s *state) marquee(side Side, path, name string, w int) string {
	key := fmt.Sprint(side) + "\x00" + path
	if key != s.focusKey {
		s.focusKey, s.focusSince = key, s.now()
	}
	over := ui.TextWidth(name) - w
	if over <= 0 {
		return name
	}
	s.scrolling = true
	cycle := marqueeWait + time.Duration(over)*marqueeStep + marqueeHold
	now := s.now()
	t := now.Sub(s.focusSince) % cycle
	shift := 0
	// scrollNext is when the shown part next changes; the loop redraws only
	// then, not on every tick through the wait and the hold.
	switch moving := t - marqueeWait; {
	case moving < 0:
		s.scrollNext = now.Add(-moving)
	case int(moving/marqueeStep) < over:
		shift = int(moving / marqueeStep)
		s.scrollNext = now.Add(marqueeStep - moving%marqueeStep)
	default:
		shift = over
		s.scrollNext = now.Add(cycle - t)
	}
	return dropColumns(name, shift, w)
}

// dropColumns skips the first n columns of t and keeps the next w (a wide
// character cut in half becomes a space).
func dropColumns(t string, n, w int) string {
	var b strings.Builder
	col, used := 0, 0
	for _, r := range t {
		rw := ui.TextWidth(string(r))
		switch {
		case col+rw <= n:
		case col < n: // straddles the start
			b.WriteString(" ")
			used++
		case used+rw <= w:
			b.WriteRune(r)
			used += rw
		}
		col += rw
		if used >= w {
			break
		}
	}
	return b.String()
}

// clipLine is the row over the status line: the clipboard and how to use
// it, or blank (the row stays so the list does not move).
func (s *state) clipLine() string {
	if s.clip == nil {
		return ""
	}
	// The same rule as the number bar: a key that does nothing right now
	// (the recent view, a task running, Esc taken by the filter) is dim.
	k := func(key, desc string, on bool) string {
		if !on {
			if !ui.Err.Color {
				desc = "×" + desc
			}
			return "  " + Dim + key + " " + desc + NC
		}
		return "  " + Cyan + key + NC + " " + desc
	}
	_, on := s.barLabels()
	escClears := s.task == nil && s.cur().filter == ""
	keys := k("6", "Paste", on[5]) + k("Esc", "Clear", escClears)
	head, src := s.clipParts()
	// A long folder gives way first (its start), so the keys stay on screen.
	room := s.width - 1 - ui.TextWidth(head) - visWidth(keys)
	return " " + head + truncateLeft(src, max(room, 8)) + keys
}
