package tui

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/term"

	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// ErrInterrupted is returned when the user presses Ctrl+C (callers exit 130).
var ErrInterrupted = errors.New("interrupted")

// ErrNoTerminal is returned when stdin/stderr is not a terminal.
var ErrNoTerminal = errors.New("interactive selection requires a terminal")

const (
	clearScreen     = "\033[H\033[2J\033[3J"
	hideCursor      = "\033[?25l"
	showCursor      = "\033[?25h"
	pasteModeOn     = "\033[?2004h"
	pasteModeOff    = "\033[?2004l"
	clearBelow      = "\033[J"
	cursorHome      = "\033[H"
	inputBufferSize = 4096
)

var resized atomic.Bool

// Select shows the picker and blocks until the user confirms or presses Ctrl+C.
// The picker stays on screen afterwards, like the Bash version.
func Select(o Options) (Result, error) {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stderr.Fd()) // draw on stderr so stdout stays clean
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return Result{Index: -1}, ErrNoTerminal
	}
	old, err := term.MakeRaw(inFd)
	if err != nil {
		return Result{Index: -1}, err
	}
	stopResize := watchResize()
	out := os.Stderr
	restore := func() {
		stopResize()
		io.WriteString(out, "\r\n\r\n"+showCursor+pasteModeOff) // leave one blank line, like tput cup last+1
		_ = term.Restore(inFd, old)
	}

	io.WriteString(out, pasteModeOn+clearScreen+hideCursor)
	s := newState(o)
	fixedHeight := 0
	buf := make([]byte, 0, inputBufferSize)
	chunk := make([]byte, inputBufferSize)
	for {
		wasResized := resized.Swap(false)
		if wasResized {
			io.WriteString(out, clearScreen)
		}
		width, height, err := term.GetSize(outFd)
		if err != nil || width <= 0 || height <= 0 {
			width, height = 80, 24 // same defaults tput uses for an unsized terminal
		}
		l := s.computeLayout(height, &fixedHeight, wasResized)
		lines := s.frame(l, fixedHeight, width)
		for i := range lines {
			if !ui.Err.Color {
				lines[i] = ui.Plain(lines[i]) // NO_COLOR / --no-color: ➤ and [✓] still mark focus and selection
			}
			lines[i] = ui.Fit(lines[i], width)
		}
		// Raw mode disables output post-processing, so end lines with \r\n.
		io.WriteString(out, cursorHome+strings.Join(lines, "\r\n")+clearBelow)

		act := actNone
		for act == actNone {
			n, err := os.Stdin.Read(chunk)
			if err != nil {
				restore()
				return Result{Index: -1}, err
			}
			buf = append(buf, chunk[:n]...)
			keys, used := parseKeys(buf)
			buf = append(buf[:0], buf[used:]...)
			if len(buf) == 1 && buf[0] == 0x1b && len(keys) == 0 {
				buf = buf[:0] // lone ESC: ignored, as in the Bash version
			}
			for _, k := range keys {
				if act = s.handle(k); act != actNone {
					break
				}
			}
			if len(keys) > 0 {
				break // redraw after each batch of keys
			}
		}
		switch act {
		case actConfirm:
			restore()
			debuglog.Logf("select %q -> %+v", o.Prompt, s.result())
			return s.result(), nil
		case actInterrupt:
			restore()
			return Result{Index: -1}, ErrInterrupted
		}
	}
}
