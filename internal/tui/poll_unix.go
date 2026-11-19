//go:build !windows

package tui

import (
	"time"

	"golang.org/x/sys/unix"
)

// readable waits up to d for fd to have input.
func readable(fd int, d time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, int(d/time.Millisecond))
		if err == unix.EINTR {
			continue // a signal (a resize): look again
		}
		return err != nil || n > 0 // on an error let Read report it
	}
}
