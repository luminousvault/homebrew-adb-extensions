// Package tui implements the interactive list picker used by ak
// (single/multi select, filter, sort, keyboard navigation).
// It is a port of select_interactive from the Bash version.
package tui

import (
	"sort"
	"strings"
)

// Options configures a picker.
type Options struct {
	Multi    bool
	NoFilter bool     // disables filter ('/') and sort ('S')
	Prompt   string   // header text
	Location []string // optional "Location:" entries, joined with " • "
	// Items to show. Text after a tab is a dimmed suffix that is not filtered.
	Items []string
}

// Result is the picker outcome, indices into Options.Items.
type Result struct {
	Index   int   // single mode, -1 if nothing
	Indices []int // multi mode, in selection order
}

type action int

const (
	actNone action = iota
	actConfirm
	actInterrupt
)

const (
	sortTime = 1 // initial order (callers pass newest first)
	sortName = 2
)

type state struct {
	opts         Options
	multi        bool
	enableFilter bool
	enableSort   bool

	initial      []string // items as given
	initialLower []string // lowercased names (suffix removed)
	items        []string // current order (after sort)
	itemsLower   []string
	order        []int // items index -> initial index

	selected []bool // by initial index
	selOrder []int  // initial indices in selection order

	filtered    []int    // indices into items
	highlighted []string // display names for filtered rows

	focused    int
	filterMode bool
	filterText string
	cursor     int
	sortMode   int
}

func newState(o Options) *state {
	s := &state{
		opts:         o,
		multi:        o.Multi,
		enableFilter: !o.NoFilter,
		enableSort:   !o.NoFilter,
		initial:      o.Items,
		items:        o.Items,
		selected:     make([]bool, len(o.Items)),
		sortMode:     sortTime,
	}
	for i, it := range o.Items {
		s.initialLower = append(s.initialLower, asciiLower(name(it)))
		s.order = append(s.order, i)
	}
	s.itemsLower = s.initialLower
	s.applyFilter()
	return s
}

// name strips the tab-separated suffix.
func name(item string) string {
	n, _, _ := strings.Cut(item, "\t")
	return n
}

// asciiLower lowercases ASCII only, keeping byte offsets aligned for highlighting.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

const (
	hlOn  = "\033[43m\033[30m"
	hlOff = "\033[0m"
)

// highlight marks the first case-insensitive match of filter in text.
func highlight(text, lower, filter string) string {
	if filter == "" {
		return text
	}
	i := strings.Index(lower, filter)
	if i < 0 {
		return text
	}
	return text[:i] + hlOn + text[i:i+len(filter)] + hlOff + text[i+len(filter):]
}

func (s *state) applyFilter() {
	s.filtered = s.filtered[:0]
	s.highlighted = s.highlighted[:0]
	if s.filterText == "" {
		for i := range s.items {
			s.filtered = append(s.filtered, i)
			s.highlighted = append(s.highlighted, name(s.items[i]))
		}
		return
	}
	f := asciiLower(s.filterText)
	added := make([]bool, len(s.items))
	add := func(i int) {
		s.filtered = append(s.filtered, i)
		s.highlighted = append(s.highlighted, highlight(name(s.items[i]), s.itemsLower[i], f))
		added[i] = true
	}
	// Multi mode: selected items that do not match stay on top.
	if s.multi {
		for i := range s.items {
			if s.selected[s.order[i]] && !strings.Contains(s.itemsLower[i], f) {
				add(i)
			}
		}
	}
	for i := range s.items {
		if !added[i] && strings.Contains(s.itemsLower[i], f) {
			add(i)
		}
	}
	if len(s.filtered) == 0 || s.focused >= len(s.filtered) {
		s.focused = 0
	}
}

func (s *state) applySort() {
	if s.sortMode != sortName {
		s.items, s.itemsLower = s.initial, s.initialLower
		for i := range s.order {
			s.order[i] = i
		}
		return
	}
	idx := make([]int, len(s.initial))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return s.initial[idx[a]] < s.initial[idx[b]] })
	s.items = make([]string, len(idx))
	s.itemsLower = make([]string, len(idx))
	for i, j := range idx {
		s.items[i], s.itemsLower[i], s.order[i] = s.initial[j], s.initialLower[j], j
	}
}

func (s *state) selectedCount() int {
	n := 0
	for _, v := range s.selected {
		if v {
			n++
		}
	}
	return n
}

// focusedOrig returns the initial index of the focused row.
func (s *state) focusedOrig() int { return s.order[s.filtered[s.focused]] }

func (s *state) toggle(orig int) {
	if !s.selected[orig] {
		s.selected[orig] = true
		s.selOrder = append(s.selOrder, orig)
		return
	}
	s.selected[orig] = false
	kept := s.selOrder[:0]
	for _, v := range s.selOrder {
		if v != orig {
			kept = append(kept, v)
		}
	}
	s.selOrder = kept
}

func (s *state) move(delta int) {
	n := len(s.filtered)
	if n == 0 {
		return
	}
	s.focused = (s.focused + delta + n) % n
}

// confirm handles Enter. In multi mode with nothing selected, the focused row is selected.
func (s *state) confirm() action {
	if len(s.filtered) == 0 {
		return actNone
	}
	if s.multi && s.selectedCount() == 0 {
		s.toggle(s.focusedOrig())
	}
	return actConfirm
}

func (s *state) handle(k key) action {
	if k.kind == keyCtrlC {
		return actInterrupt
	}
	if s.filterMode {
		return s.handleFilter(k)
	}
	switch k.kind {
	case keyUp:
		s.move(-1)
	case keyDown:
		s.move(1)
	case keyEnter:
		return s.confirm()
	case keyRune:
		return s.handleRune(k.r)
	}
	return actNone
}

func (s *state) handleRune(r byte) action {
	switch {
	case r == '/':
		if s.enableFilter {
			s.filterMode = true
		}
	case r == 'a' || r == 'A':
		if s.multi {
			all := s.selectedCount() == len(s.selected)
			s.selOrder = nil
			for i := range s.selected {
				s.selected[i] = !all
				if !all {
					s.selOrder = append(s.selOrder, i)
				}
			}
		}
	case r == 's' || r == 'S':
		if s.enableSort {
			if s.sortMode == sortTime {
				s.sortMode = sortName
			} else {
				s.sortMode = sortTime
			}
			s.applySort()
			s.applyFilter()
		}
	case r == ' ':
		if s.multi && len(s.filtered) > 0 {
			s.toggle(s.focusedOrig())
		}
	case r >= '1' && r <= '9':
		n := int(r - '0')
		count := len(s.initial)
		if n > count || count > 9 {
			return actNone
		}
		if s.multi {
			if n > len(s.filtered) {
				return actNone
			}
			orig := s.order[s.filtered[n-1]]
			for i := range s.selected {
				s.selected[i] = false
			}
			s.selected[orig] = true
			s.selOrder = []int{orig}
			return actConfirm
		}
		s.focused = n - 1
		return actConfirm
	}
	return actNone
}

func (s *state) handleFilter(k key) action {
	switch k.kind {
	case keyUp:
		s.move(-1)
	case keyDown:
		s.move(1)
	case keyRight:
		if s.cursor < len(s.filterText) {
			s.cursor++
		}
	case keyLeft:
		if s.cursor > 0 {
			s.cursor--
		}
	case keyHome, keyCtrlA:
		s.cursor = 0
	case keyEnd, keyCtrlE:
		s.cursor = len(s.filterText)
	case keyPaste:
		s.insert(k.text)
	case keyDelete:
		if s.cursor < len(s.filterText) {
			s.filterText = s.filterText[:s.cursor] + s.filterText[s.cursor+1:]
			s.applyFilter()
		}
	case keyBackspace:
		if s.cursor > 0 {
			s.filterText = s.filterText[:s.cursor-1] + s.filterText[s.cursor:]
			s.cursor--
			s.applyFilter()
		}
	case keyEnter:
		return s.confirm()
	case keyRune:
		switch {
		case k.r == ' ':
			if s.multi && len(s.filtered) > 0 {
				orig := s.focusedOrig()
				wasSelected := s.selected[orig]
				s.toggle(orig)
				if wasSelected {
					s.applyFilter() // unselected rows may drop out of the pinned group
				}
			}
		case k.r == '/':
			s.filterText, s.cursor, s.filterMode = "", 0, false
			s.applyFilter()
		case k.r >= 33 && k.r <= 126:
			s.insert(string(k.r))
		}
	}
	return actNone
}

func (s *state) insert(text string) {
	s.filterText = s.filterText[:s.cursor] + text + s.filterText[s.cursor:]
	s.cursor += len(text)
	s.applyFilter()
}

func (s *state) result() Result {
	if s.multi {
		return Result{Index: -1, Indices: append([]int(nil), s.selOrder...)}
	}
	if len(s.filtered) == 0 {
		return Result{Index: -1}
	}
	return Result{Index: s.focusedOrig()}
}
