package tui

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// ErrInterrupted is returned when the user presses Ctrl+C (callers exit 130).
var ErrInterrupted = errors.New("interrupted")

// ErrNoTerminal is returned when stdin/stderr is not a terminal.
var ErrNoTerminal = errors.New("interactive selection requires a terminal")

const (
	altScreenOn     = "\033[?1049h" // draw on the alternate screen; the shell's screen and scrollback stay intact
	altScreenOff    = "\033[?1049l"
	clearScreen     = "\033[H\033[2J"
	hideCursor      = "\033[?25l"
	showCursor      = "\033[?25h"
	pasteModeOn     = "\033[?2004h"
	pasteModeOff    = "\033[?2004l"
	clearBelow      = "\033[J"
	cursorHome      = "\033[H"
	inputBufferSize = 4096
)

var (
	screenMu sync.Mutex
	held     bool // Hold: stay on the alternate screen between pickers
	onAlt    bool // the alternate screen is open
)

// Hold keeps the alternate screen open when a picker closes, so pickers shown
// one after another (APKs, then devices) look like one screen that changes
// instead of flashing back to the shell between them. Call Release afterwards.
func Hold() {
	screenMu.Lock()
	held = true
	screenMu.Unlock()
}

// Release leaves the alternate screen if a held picker left it open.
func Release() {
	screenMu.Lock()
	defer screenMu.Unlock()
	held = false
	if onAlt {
		io.WriteString(os.Stderr, showCursor+altScreenOff)
		onAlt = false
	}
}

// Select shows the picker on the alternate screen and blocks until the user
// confirms or presses Ctrl+C. Afterwards the terminal shows what it showed
// before, so earlier output (and scrollback) is kept; callers print the choice.
func Select(o Options) (Result, error) {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stderr.Fd()) // draw on stderr so stdout stays clean
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return Result{Index: -1}, ErrNoTerminal
	}
	old, err := term.MakeRaw(inFd)
	if err != nil {
		return Result{Index: -1}, err
	}
	out := os.Stderr
	s := newState(o)
	fixedHeight := 0
	// mu serializes drawing and state changes: a resize redraws from another
	// goroutine. closed stops a resize that was already waiting on mu when the
	// picker closed from drawing over the restored screen.
	var mu sync.Mutex
	closed := false
	draw := func(wasResized bool) {
		if wasResized {
			io.WriteString(out, clearScreen)
		}
		width, height, err := term.GetSize(outFd)
		if err != nil || width <= 0 || height <= 0 {
			width, height = 80, 24 // same defaults tput uses for an unsized terminal
		}
		l := s.computeLayout(height, &fixedHeight, wasResized)
		lines := s.frame(l, fixedHeight, width, height)
		for i := range lines {
			if !ui.Err.Color {
				lines[i] = ui.Plain(lines[i]) // NO_COLOR / --no-color: ➤ and [✓] still mark focus and selection
			}
			lines[i] = ui.Fit(lines[i], width)
		}
		// Raw mode disables output post-processing, so end lines with \r\n.
		io.WriteString(out, cursorHome+strings.Join(lines, "\r\n")+clearBelow)
	}
	stopResize := watchResize(func() {
		mu.Lock()
		defer mu.Unlock()
		if !closed {
			draw(true)
		}
	})
	restore := func() {
		stopResize()
		mu.Lock() // let a resize redraw finish before leaving the alternate screen
		defer mu.Unlock()
		closed = true
		io.WriteString(out, showCursor+pasteModeOff)
		screenMu.Lock()
		if !held {
			io.WriteString(out, altScreenOff)
			onAlt = false
		}
		screenMu.Unlock()
		_ = term.Restore(inFd, old)
	}

	screenMu.Lock()
	if !onAlt {
		io.WriteString(out, altScreenOn)
		onAlt = true
	}
	screenMu.Unlock()
	io.WriteString(out, pasteModeOn+clearScreen+hideCursor)
	buf := make([]byte, 0, inputBufferSize)
	chunk := make([]byte, inputBufferSize)
	mu.Lock()
	draw(false)
	mu.Unlock()
	for {
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
			mu.Lock()
			for _, k := range keys {
				if act = s.handle(k); act != actNone {
					break
				}
			}
			mu.Unlock()
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
		mu.Lock()
		draw(false)
		mu.Unlock()
	}
}
