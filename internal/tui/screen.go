package tui

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Screen is a full-screen session for UIs that run their own event loop
// (ak files): raw mode, the alternate screen on stderr, key batches and
// resize events. It follows the same rules as Select (docs/dev/cli-design.md §10).
// Use it from one goroutine; Keys and Resized are channels for a select loop.
type Screen struct {
	inFd, outFd int
	old         *term.State
	out         io.Writer
	keys        chan []Key
	resized     chan struct{}
	stopResize  func()
	closed      bool
}

// OpenScreen enters raw mode and the alternate screen. It fails with
// ErrNoTerminal when stdin or stderr is not a terminal. Call Close afterwards.
func OpenScreen() (*Screen, error) {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stderr.Fd())
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return nil, ErrNoTerminal
	}
	old, err := term.MakeRaw(inFd)
	if err != nil {
		return nil, err
	}
	s := &Screen{inFd: inFd, outFd: outFd, old: old, out: os.Stderr,
		keys: make(chan []Key), resized: make(chan struct{}, 1)}
	s.stopResize = watchResize(func() {
		select {
		case s.resized <- struct{}{}:
		default: // one pending redraw is enough
		}
	})
	enterAlt(s.out)
	go s.readKeys()
	return s, nil
}

// readKeys sends key batches until stdin fails, then closes the channel. The
// goroutine stays blocked in Read after Close; ak exits soon after.
func (s *Screen) readKeys() {
	buf := make([]byte, 0, inputBufferSize)
	chunk := make([]byte, inputBufferSize)
	for {
		n, err := os.Stdin.Read(chunk)
		if err != nil {
			close(s.keys)
			return
		}
		buf = append(buf, chunk[:n]...)
		keys, used := ParseKeys(buf)
		buf = append(buf[:0], buf[used:]...)
		if LoneEsc(buf) {
			keys = append(keys, Key{Kind: KeyEsc})
			buf = buf[:0]
		}
		if len(keys) > 0 {
			s.keys <- keys
		}
	}
}

// Keys delivers key presses in batches; it is closed when stdin fails.
func (s *Screen) Keys() <-chan []Key { return s.keys }

// Resized receives when the terminal size changed (never on Windows, where
// the size is re-read on every Draw instead).
func (s *Screen) Resized() <-chan struct{} { return s.resized }

// Size returns the terminal size, 80x24 when it is unknown.
func (s *Screen) Size() (width, height int) {
	width, height, err := term.GetSize(s.outFd)
	if err != nil || width <= 0 || height <= 0 {
		return 80, 24 // same defaults tput uses for an unsized terminal
	}
	return width, height
}

// Draw replaces the screen with lines, each cut to the terminal width (and
// stripped of color when color is off). clear blanks the screen first, which
// a resize needs. Callers keep len(lines) within the terminal height.
func (s *Screen) Draw(lines []string, clear bool) {
	if clear {
		io.WriteString(s.out, clearScreen)
	}
	width, _ := s.Size()
	writeFrame(s.out, lines, width)
}

// Close leaves the alternate screen (unless Hold is on) and restores the
// terminal. It is safe to call twice.
func (s *Screen) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.stopResize()
	leaveAlt(s.out)
	_ = term.Restore(s.inFd, s.old)
}
