package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

// runeWidth returns how many terminal columns r occupies.
func runeWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0x200d || (r >= 0xfe00 && r <= 0xfe0f) {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// Fit cuts line to at most cols visible columns. ANSI escape sequences are kept
// and do not count; a cut line ends with a style reset. Without this, a line
// wider than the terminal wraps, shifts every row below it and breaks redraws.
func Fit(line string, cols int) string {
	var b strings.Builder
	used := 0
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
				j++
			}
			if j < len(line) {
				j++
			}
			b.WriteString(line[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := runeWidth(r)
		if used+w > cols {
			b.WriteString("\033[0m")
			return b.String()
		}
		used += w
		b.WriteString(line[i : i+size])
		i += size
	}
	return b.String()
}

// TextWidth is the number of terminal columns s takes: Hangul, CJK and emoji
// count 2. Use it, not len([]rune(s)), to align columns.
func TextWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// Pad appends spaces so s takes w columns (fmt's %-*s counts runes, not columns).
func Pad(s string, w int) string {
	if d := w - TextWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
