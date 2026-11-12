//go:build windows

package adb

import (
	"os/exec"
	"syscall"
)

// detach starts the command in a new process group so Ctrl+C reaches only ak.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
