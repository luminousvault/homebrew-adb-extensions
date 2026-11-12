package tui

import (
	"fmt"
	"strconv"
	"strings"

	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
)

const (
	counterLines   = 1
	helpLines      = 1
	filterBoxLines = 3
	spacingLines   = 1
	clearLine      = "\033[K"
)

// layout is the visible window computed for one frame.
type layout struct {
	maxVisible  int
	windowStart int
	windowEnd   int
}

func (s *state) headerLines() int {
	if len(s.opts.Location) > 0 {
		return 3
	}
	return 2
}

// computeLayout ports the window math of the Bash picker. fixedHeight is the
// list height kept stable while filtering; it is updated outside filter mode
// or after a resize.
func (s *state) computeLayout(height int, fixedHeight *int, resized bool) layout {
	reserved := s.headerLines() + counterLines + helpLines + spacingLines + filterBoxLines
	maxVisible := height - reserved - 1
	if maxVisible < 1 {
		maxVisible = 1
	}
	if !s.filterMode || resized {
		h := maxVisible
		if len(s.initial) < h {
			h = len(s.initial)
		}
		*fixedHeight = h
	}
	count := len(s.filtered)
	if s.filterMode && count < maxVisible {
		maxVisible = count
	}
	l := layout{maxVisible: maxVisible, windowStart: 0, windowEnd: count}
	if count > maxVisible {
		l.windowStart = s.focused - maxVisible/2
		if l.windowStart < 0 {
			l.windowStart = 0
		}
		l.windowEnd = l.windowStart + maxVisible
		if l.windowEnd > count {
			l.windowEnd = count
			l.windowStart = count - maxVisible
			if l.windowStart < 0 {
				l.windowStart = 0
			}
		}
	}
	return l
}

// Line priorities for small terminals: when the frame is taller than the
// terminal, lines with the highest drop priority go first, so items and the
// filter input stay visible down to a few rows.
const (
	keepLine     = iota // items, filter input
	dropPrompt          // "==> Select ..."
	dropCounter         // "3 item(s) | 1 selected"
	dropRule            // filter box rules
	dropHelp            // key help
	dropLocation        // "Location: ..."
	dropSpacer          // blank lines
)

// frame renders the whole picker as lines (without line terminators), at most
// height lines.
func (s *state) frame(l layout, fixedHeight, width, height int) []string {
	type line struct {
		text string
		drop int
	}
	var lines []line
	add := func(drop int, format string, args ...any) {
		lines = append(lines, line{clearLine + fmt.Sprintf(format, args...), drop})
	}

	add(dropPrompt, "%s==> %s%s%s", Blue, Bold, s.opts.Prompt, NC)
	if len(s.opts.Location) > 0 {
		add(dropLocation, "    %sLocation: %s%s", Dim, strings.Join(s.opts.Location, " • "), NC)
	}
	add(dropSpacer, "")
	add(dropCounter, "%s", s.counter(l))
	for i := l.windowStart; i < l.windowEnd && i < len(s.filtered); i++ {
		add(keepLine, "%s", s.row(i))
	}
	if s.filterMode {
		for i := l.windowEnd - l.windowStart; i < fixedHeight; i++ {
			add(dropSpacer, "")
		}
	}
	add(dropSpacer, "")
	add(dropHelp, "%s", s.help())
	if s.filterMode {
		rule := Dim + strings.Repeat("─", max(width-1, 1)) + NC
		add(dropRule, "%s", rule)
		add(keepLine, "> %s", s.filterDisplay())
		add(dropRule, "%s", rule)
	}
	for drop := dropSpacer; len(lines) > height && drop > keepLine; drop-- {
		for i := len(lines) - 1; i >= 0 && len(lines) > height; i-- {
			if lines[i].drop == drop {
				lines = append(lines[:i], lines[i+1:]...)
			}
		}
	}
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		out = append(out, ln.text)
	}
	if height > 0 && len(out) > height {
		out = out[:height] // even items do not fit; never scroll the screen
	}
	return out
}

func (s *state) counter(l layout) string {
	count := len(s.filtered)
	var b strings.Builder
	b.WriteString(Dim)
	if count > l.maxVisible {
		fmt.Fprintf(&b, "Showing %d-%d / %d", l.windowStart+1, l.windowEnd, count)
	} else {
		fmt.Fprintf(&b, "%d item(s)", count)
	}
	if s.filterText != "" {
		fmt.Fprintf(&b, " (filtered from %d)", len(s.initial))
	}
	if s.multi {
		fmt.Fprintf(&b, " | %s%d selected", Green, s.selectedCount())
	}
	b.WriteString(NC)
	return b.String()
}

func (s *state) row(display int) string {
	itemIdx := s.filtered[display]
	orig := s.order[itemIdx]
	checkbox := ""
	if s.multi {
		checkbox = "[ ] "
		if s.selected[orig] {
			checkbox = "[✓] "
		}
	}
	digits := len(strconv.Itoa(len(s.initial)))
	number := fmt.Sprintf("%*d. ", digits, display+1)
	text := s.highlighted[display]
	suffix := ""
	if _, after, ok := strings.Cut(s.items[itemIdx], "\t"); ok {
		suffix = Dim + "  " + after + NC
	}
	switch {
	case display == s.focused:
		return Cyan + "➤ " + checkbox + Bold + number + text + NC + suffix
	case s.multi && s.selected[orig]:
		return "  " + Green + checkbox + NC + number + text + suffix
	default:
		return "  " + checkbox + number + text + suffix
	}
}

func (s *state) help() string {
	pipe := Dim + "│" + NC
	k := func(name, desc string) string { return Cyan + name + NC + ": " + desc + " " + pipe + " " }
	var b strings.Builder
	b.WriteString(Dim + k("↑↓", "Move"))
	if s.filterMode {
		if s.multi {
			b.WriteString(k("Enter", "Confirm") + k("Space", "Toggle"))
		} else {
			b.WriteString(k("Enter", "Select"))
		}
		b.WriteString(k("/", "Exit filter"))
	} else {
		if s.multi {
			b.WriteString(k("Enter", "Confirm") + k("Space", "Toggle") + k("A", "All"))
		} else {
			b.WriteString(k("Enter", "Select"))
		}
		if s.enableSort {
			if s.sortMode == sortTime {
				b.WriteString(k("S", "Time↓"))
			} else {
				b.WriteString(k("S", "Name↑"))
			}
		}
		if s.enableFilter {
			b.WriteString(k("/", "Filter"))
		}
	}
	b.WriteString(Cyan + "^C" + NC + ": Exit" + NC)
	return b.String()
}

func (s *state) filterDisplay() string {
	t, c := s.filterText, s.cursor
	if c >= len(t) {
		return t + "\033[7m \033[0m"
	}
	return t[:c] + "\033[7m" + t[c:c+1] + "\033[0m" + t[c+1:]
}
