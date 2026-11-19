//go:build !windows

package tui

import (
	"os"
	"testing"
	"time"
)

func pipeScreen(t *testing.T) (*Screen, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	s := &Screen{in: r, inFd: int(r.Fd()), keys: make(chan []Key, 8), done: make(chan struct{})}
	go s.readKeys()
	return s, w
}

func nextKeys(t *testing.T, s *Screen) []Key {
	t.Helper()
	select {
	case k := <-s.keys:
		return k
	case <-time.After(time.Second):
		t.Fatal("no keys")
		return nil
	}
}

// An arrow key whose bytes arrive in two reads is still one arrow key, not
// Esc followed by letters; ESC alone is Esc after a short wait.
func TestSplitEscapeSequence(t *testing.T) {
	s, w := pipeScreen(t)
	_, _ = w.Write([]byte{0x1b})
	time.Sleep(escWait / 4)
	_, _ = w.Write([]byte("[A"))
	if k := nextKeys(t, s); len(k) != 1 || k[0].Kind != KeyUp {
		t.Errorf("split arrow: %+v", k)
	}
	_, _ = w.Write([]byte{0x1b})
	if k := nextKeys(t, s); len(k) != 1 || k[0].Kind != KeyEsc {
		t.Errorf("lone Esc: %+v", k)
	}
}

// After Close the reader stops without reading what is typed next: those
// bytes are left for the shell.
func TestStoppedReaderLeavesInput(t *testing.T) {
	s, w := pipeScreen(t)
	s.stopped.Store(true)
	close(s.done)
	time.Sleep(2 * readPoll) // the reader notices
	_, _ = w.Write([]byte("ls\n"))
	time.Sleep(2 * readPoll)
	buf := make([]byte, 8)
	_ = s.in.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	n, _ := s.in.Read(buf)
	if string(buf[:n]) != "ls\n" {
		t.Errorf("the input was taken: %q left", buf[:n])
	}
}
