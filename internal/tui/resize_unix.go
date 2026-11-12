//go:build !windows

package tui

import (
	"os"
	"os/signal"
	"syscall"
)

// watchResize calls onResize when the terminal size changes.
func watchResize(onResize func()) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				onResize()
			case <-done:
				return
			}
		}
	}()
	return func() { signal.Stop(ch); close(done) }
}
