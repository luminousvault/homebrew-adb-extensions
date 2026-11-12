package ui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Kind is a message level, for building lines without printing them.
type Kind int

const (
	KindStep Kind = iota
	KindSuccess
	KindWarn
	KindError
	KindDetail
)

// Line formats text as a message of the given kind for stderr.
func Line(k Kind, text string) string {
	switch k {
	case KindStep:
		return Err.Paint(blue+bold, "==>") + " " + Err.Bold(text)
	case KindSuccess:
		return Err.Green("✓") + " " + text
	case KindWarn:
		return Err.Yellow("Warning:") + " " + text
	case KindError:
		return Err.Red("Error:") + " " + text
	}
	return Err.Dim("  " + text)
}

// Board redraws a block of lines in place on stderr (a status board).
type Board struct{ drawn int }

// Width returns the stderr terminal width (80 if unknown).
func Width() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// Draw replaces the previously drawn lines with lines.
func (b *Board) Draw(lines []string) {
	var s strings.Builder
	if b.drawn > 0 {
		fmt.Fprintf(&s, "\033[%dA", b.drawn)
	}
	w := Width()
	for _, l := range lines {
		s.WriteString("\r\033[K" + Fit(l, w) + "\n")
	}
	Err.Printf("%s", s.String())
	b.drawn = len(lines)
}

// Release keeps the current lines on screen; the next Draw starts below them.
func (b *Board) Release() { b.drawn = 0 }

// HideCursor and ShowCursor toggle the terminal cursor on stderr.
func HideCursor() { Err.Printf("\033[?25l") }
func ShowCursor() { Err.Printf("\033[?25h") }
