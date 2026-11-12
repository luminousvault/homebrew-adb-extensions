//go:build !windows

package adb

import (
	"os/exec"
	"syscall"
)

// detach puts the command in its own process group so Ctrl+C reaches only ak.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
