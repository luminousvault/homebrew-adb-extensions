//go:build !windows

package cli

import "syscall"

// interruptGroup sends SIGINT to ak's process group, as Ctrl+C on the
// terminal does, so adb commands started by ak stop too.
func interruptGroup() { _ = syscall.Kill(0, syscall.SIGINT) }
