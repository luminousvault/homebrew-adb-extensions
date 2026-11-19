package tui

import (
	"io"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// Screen is a full-screen session for UIs that run their own event loop
// (ak files): raw mode, the alternate screen on stderr, key batches and
// resize events. It follows the same rules as Select (docs/dev/cli-design.md §10).
// Use it from one goroutine; Keys and Resized are channels for a select loop.
type Screen struct {
	inFd, outFd int
	in          *os.File // stdin (a pipe in tests)
	old         *term.State
	out         io.Writer
	keys        chan []Key
	resized     chan struct{}
	stopResize  func()
	closed      bool
	stopped     atomic.Bool   // Close was called: the key reader stops
	done        chan struct{} // closed by Close
}

// escWait is how long a trailing ESC waits for the rest of an escape
// sequence before it counts as the Esc key: over a slow link (ssh, tmux) the
// bytes of one arrow key can arrive in two reads, and ESC + "[A" read apart
// would be Esc and then the letter A (select all).
const escWait = 40 * time.Millisecond

// readPoll is how often the key reader looks at stdin; between looks it is
// not in a Read, so after Close it stops without taking keys meant for the
// shell (on Unix; Windows has no poll and keeps a blocking Read).
const readPoll = 100 * time.Millisecond

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
	s := &Screen{inFd: inFd, outFd: outFd, in: os.Stdin, old: old, out: os.Stderr,
		keys: make(chan []Key), resized: make(chan struct{}, 1), done: make(chan struct{})}
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

// readKeys sends key batches until stdin fails, then closes the channel. It
// stops when Close is called (on Unix without another Read: see readPoll).
func (s *Screen) readKeys() {
	buf := make([]byte, 0, inputBufferSize)
	chunk := make([]byte, inputBufferSize)
	read := func() bool {
		n, err := s.in.Read(chunk)
		if err != nil {
			close(s.keys)
			return false
		}
		buf = append(buf, chunk[:n]...)
		return true
	}
	for {
		if !readable(s.inFd, readPoll) {
			if s.stopped.Load() {
				return
			}
			continue
		}
		if s.stopped.Load() || !read() {
			return
		}
		var keys []Key
		for {
			k, used := ParseKeys(buf)
			keys = append(keys, k...)
			buf = append(buf[:0], buf[used:]...)
			if !LoneEsc(buf) {
				break
			}
			if readable(s.inFd, escWait) { // the rest of a sequence, or the next key
				if !read() {
					return
				}
				continue
			}
			keys = append(keys, Key{Kind: KeyEsc})
			buf = buf[:0]
			break
		}
		if len(keys) > 0 {
			select {
			case s.keys <- keys:
			case <-s.done:
				return
			}
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
	s.stopped.Store(true)
	close(s.done)
	s.stopResize()
	leaveAlt(s.out)
	_ = term.Restore(s.inFd, s.old)
}
