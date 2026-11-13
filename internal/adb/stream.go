package adb

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
)

// ErrNoStreaming means the runner cannot start long-running commands (test fakes).
var ErrNoStreaming = errors.New("runner does not support streaming")

// Starter is implemented by runners that can start long-running adb commands.
type Starter interface {
	// Start runs adb in the background. The command does not receive the
	// terminal's Ctrl+C (it gets its own process group), so callers decide how
	// to stop it.
	Start(args ...string) (*exec.Cmd, io.ReadCloser, error)
}

func (e Exec) Start(args ...string) (*exec.Cmd, io.ReadCloser, error) {
	debuglog.Logf("adb (start) %s", strings.Join(args, " "))
	cmd := exec.Command(e.Path, args...)
	detach(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, out, nil
}

// StopSignals are the signals a caller of Start should watch to stop the
// command. A started command is in its own process group, so it outlives ak
// when ak is ended by anything other than Ctrl+C: `timeout` or `kill` (SIGTERM),
// or the terminal closing (SIGHUP).
var StopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// Start starts a long-running adb command with the default runner.
func Start(args ...string) (*exec.Cmd, io.ReadCloser, error) {
	s, ok := Default.(Starter)
	if !ok {
		return nil, nil, ErrNoStreaming
	}
	return s.Start(args...)
}

// Output runs adb and returns raw stdout bytes (for binary data such as screenshots).
func Output(args ...string) ([]byte, error) {
	if e, ok := Default.(Exec); ok {
		debuglog.Logf("adb (output) %s", strings.Join(args, " "))
		cmd := exec.Command(e.Path, args...)
		return cmd.Output()
	}
	out, _, err := Default.Run(args...)
	return []byte(out), err
}
