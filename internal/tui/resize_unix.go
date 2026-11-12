//go:build !windows

package tui

import (
	"os"
	"os/signal"
	"syscall"
)

func watchResize() (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				resized.Store(true)
			case <-done:
				return
			}
		}
	}()
	return func() { signal.Stop(ch); close(done) }
}
