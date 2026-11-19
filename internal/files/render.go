package files

import (
	"fmt"
	"golang.org/x/text/unicode/norm"
	"strings"

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
	l := frameLayout{header: true, topRule: true, bottomRule: true, barGap: true, bar: 1}
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
		return b2i(l.header) + keysRule + b2i(l.topRule) + b2i(l.bottomRule) + len(l.keys) + b2i(l.barGap && l.bar > 0) + l.bar + 1
	}
	// Too short: the gap over the bar, the letter keys (second row, then
	// all), the bar's second row (back to one cut row), the bar, the rules,
	// then the header go, so three list rows and the status line stay.
	for s.height-fixed() < 3 {
		switch {
		case l.barGap:
			l.barGap = false
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
	if s.mode == modeHelp || s.mode == modeFailures {
		// The help and the failure list take everything over the bottom rule.
		rows := b2i(l.header) + b2i(l.header && len(l.keys) > 0) + len(l.keys) + b2i(l.topRule) + l.list
		var lines []string
		if s.mode == modeHelp {
			lines = s.helpPage(w, rows)
		} else {
			lines = s.overlay(failureLines(s.failures), w, rows)
		}
		if l.bottomRule {
			lines = append(lines, s.bottomRuleLine(w))
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
			{"3 Open", "Enter", "Go into a folder or open a file (see How things work)"},
			{"Backspace", "", "Parent folder"},
			{":", "", "Go to a path you type"},
		}},
		{"Select", []helpItem{
			{"Space", "", "Select or unselect, then move down"},
			{"a", "", "Select or unselect everything here (as in the install and pull pickers)"},
			{"/", "", "Filter by name as you type; Esc turns it off"},
			{"", "", "A selection is used by 5 and 8 only; selected items a filter or hidden files hide are left out"},
		}},
		{"Copy", []helpItem{
			{"5 To computer", "g", "On the device side: copy to the computer side's current folder (adb pull)"},
			{"5 To device", "p", "On the computer side: copy to the device side's current folder (adb push)"},
			{"", "", "What: the selected items shown, or else the item under the cursor. The originals stay: there is no move; delete with 8 afterwards"},
			{"o s r", "", "A name already there: overwrite, skip, or rename with ' (N)'. A capital letter does all the rest; Esc or q cancels"},
			{"", "", "Overwrite copies to a temporary name first, checks the size, then swaps it in; if that fails or is canceled, the old item stays"},
			{"Esc", "", "Stops copying (a progress bar shows it) and removes the half-copied item"},
		}},
		{"How things work", []helpItem{
			{"Open", "", "A folder: go into it. A device file is copied to " + tmp + " and opened with its default app; a big file takes a while (progress bar, Esc cancels). Files there are removed on a later start, a day after. A computer file opens at once. With a selection, only the file under the cursor opens"},
			{"Delete", "", "Always asks. On the device it is rm -rf; on the computer it is permanent too, not the Trash"},
			{"Recent", "", "The device's 300 newest files from MediaStore, the list of the gallery and the Files app 'Recent'; about a second the first time. From the computer side too"},
			{"Free space", "", "The device's df, read when a folder opens and after a copy or a delete"},
			{"Device", "", "Reached with adb's shell rights: the main user's storage (user 0) only, not a work profile"},
		}},
		{"Manage", []helpItem{
			{"6 Rename", "r", "Rename the item under the cursor"},
			{"7 Mkdir", "n", "New folder"},
			{"8 Delete", "d", "Delete the selection or the item (Delete key too)"},
		}},
		{"View", []helpItem{
			{"Tab", "", "Device ↔ computer (two panes: the other pane)"},
			{"9 Show computer", "w", "The computer pane beside the device on or off"},
			{"4 Recent", "t", "The device's recent files ↔ its folder"},
			{"2 Sort", "s", "Name → newest → size"},
			{".", "", "Hidden files on or off"},
			{"1 Help", "?", "This help"},
			{"0 Quit", "q", "Quit (Ctrl+C too)"},
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

func failureLines(list []failure) []string {
	lines := []string{" " + Bold + "Failed" + NC, ""}
	for _, f := range list {
		lines = append(lines, " "+Red+"✗"+NC+" "+f.name+"  "+Dim+f.msg+NC)
	}
	return append(lines, "", " "+Dim+"Press any key to close"+NC)
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
	where := norm.NFC.String(displayPath(p.fs, p.dir))
	if p.recent {
		where = "Recent files"
	}
	color := Bold
	if active && s.split {
		color = Cyan + Bold
	}
	// The sort order is on the status line; the header keeps the filter, so
	// with two panes the other pane's filter shows too.
	r := ""
	if p.filter != "" {
		r = "/" + p.filter + " "
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
	nameW := w - 6 - len(right)     // marks take 6 columns
	name := norm.NFC.String(e.Name) // a macOS NFD name shows as its syllables
	if e.Dir && !s.isUp(p, i) {
		name += "/"
	}
	suffix := ""
	if e.Folder != "" {
		suffix = "  " + norm.NFC.String(e.Folder)
	}
	name = truncate(name, max(nameW, 1))
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
	if p.recent {
		parts = append(parts, fmt.Sprintf("Recent %d/%d", pos, n)+free)
	} else {
		parts = append(parts, fmt.Sprintf("%d/%d", pos, n))
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
	left := strings.Join(parts, "  ")
	// The sort order sits at the right end (it used to be in the header).
	sort := s.sortBy.label()
	if p.recent {
		sort = "newest ↓"
	}
	right := Dim + "sort: " + sort + NC
	if gap := w - visWidth(left) - visWidth(right); gap >= 2 {
		return left + strings.Repeat(" ", gap) + right
	}
	return left
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
	if p.count > 1 || t.route != "" {
		head += fmt.Sprintf(" %d/%d", max(p.index, 1), max(p.count, 1))
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
	if p.total <= 0 {
		return string(spinFrames[s.spin%len(spinFrames)]) + " " + join(name, rt, "")
	}
	pct := fmt.Sprintf(" %d%%", min(p.done*100/p.total, 100))
	sizes := fmt.Sprintf(" (%s / %s)", HumanSize(p.done), HumanSize(p.total))
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
				return join(name, rt, bar(min(free, maxBar), p.done, p.total)+pct+sizes)
			}
			room := ui.TextWidth(name) + ui.TextWidth(rt) - (want - free)
			if room < 18 && (withSizes || want > minBar) {
				continue // too tight even cut: a shorter bar, then no sizes
			}
			rw := min(ui.TextWidth(rt), max(room/2, room-ui.TextWidth(name)))
			nw := max(room-rw, 1)
			return join(truncate(name, nw), truncateRoute(rt, max(rw, 1)), bar(want, p.done, p.total)+pct+sizes)
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

// barLabels are the number keys 1-9 and 0 as mc shows its F keys; "" is a
// key that does nothing here.
func (s *state) barLabels() [10]string {
	p := s.cur()
	idle := s.task == nil
	_, onEntry := s.cursorEntry(p)
	l := [10]string{"Help", "Sort", "Open", "Recent", "", "", "", "", "Show computer", "Quit"}
	switch {
	case s.split:
		l[8] = "Hide computer"
	case s.width < minSplitWidth:
		l[8] = "" // too narrow for two panes
	}
	if p.fs.Side() == Device && p.recent {
		l[3] = "Folder"
	}
	if idle {
		l[4] = map[Side]string{Device: "To computer", PC: "To device"}[p.fs.Side()]
		if !p.recent {
			if onEntry {
				l[5] = "Rename"
			}
			l[6] = "Mkdir"
		}
		l[7] = "Delete"
	}
	if len(p.rows) == 0 {
		l[2] = ""
	}
	return l
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
	labels, aliases := s.barLabels(), s.barAliases()
	if rows < 2 {
		return []string{barRow(labels[:], aliases[:], 0, w)}
	}
	return []string{barRow(labels[:5], aliases[:5], 0, w), barRow(labels[5:], aliases[5:], 5, w)}
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
func barRow(labels, aliases []string, first, w int) string {
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
		gap := " " // between the digit and its label; dropped when cells are tiny
		if cw < 6 {
			gap = ""
		}
		used := 1
		if room := cw - 2 - len(gap); label != "" && room > 0 { // and a space before the next digit
			cut := ui.Plain(ui.Fit(label, room))
			cell += gap + cut
			used += len(gap) + ui.TextWidth(cut)
			if alias := " (" + aliases[i] + ")"; aliases[i] != "" && cut == label && used+len(alias) <= cw-1 {
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
// screen, and 1 Help explains them at more length.
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
	out = append(out, k("Enter", "Open"), k("Bksp", "Up"))
	if s.columns() == 2 {
		out = append(out, k("←→", "Column"))
	} else {
		out = append(out, k("←→", "Up/Open"))
	}
	out = append(out, k("Space", "Select"), k("a", "All"), k("/", "Filter"))
	if !p.recent {
		out = append(out, k(":", "Go to"))
	}
	out = append(out, k(".", map[bool]string{true: "Hide hidden", false: "Show hidden"}[s.showHidden]))
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
