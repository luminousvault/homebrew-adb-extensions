// Package adb runs the adb binary and parses its common outputs.
package adb

import (
	"bytes"
	"os"
	"os/exec"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
)

// Runner executes adb. Tests replace Default with a fake.
type Runner interface {
	// Run executes adb and captures stdout and stderr separately.
	Run(args ...string) (stdout, stderr string, err error)
	// Passthrough executes adb attached to the terminal.
	Passthrough(args ...string) error
}

// Exec runs the adb binary at Path.
type Exec struct{ Path string }

func (e Exec) Run(args ...string) (string, string, error) {
	debuglog.Logf("adb %s", strings.Join(args, " "))
	var out, errb bytes.Buffer
	cmd := exec.Command(e.Path, args...)
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func (e Exec) Passthrough(args ...string) error {
	debuglog.Logf("adb (passthrough) %s", strings.Join(args, " "))
	cmd := exec.Command(e.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// Default is the runner used by all commands.
var Default Runner

// Out runs adb and returns stdout only (stderr discarded), with \r removed.
func Out(args ...string) string {
	out, _, _ := Default.Run(args...)
	return strings.ReplaceAll(out, "\r", "")
}

// Combined runs adb and returns stdout+stderr (like 2>&1) and the exit error.
func Combined(args ...string) (string, error) {
	out, errOut, err := Default.Run(args...)
	return strings.ReplaceAll(out+errOut, "\r", ""), err
}

// Shell runs `adb -s serial shell <cmd>` and returns stdout without \r.
func Shell(serial, cmd string) string {
	return Out("-s", serial, "shell", cmd)
}

// DeviceEntry is one line of `adb devices`.
type DeviceEntry struct {
	Serial string
	State  string
}

// ListDevices parses `adb devices`, skipping the header and blank lines.
func ListDevices() []DeviceEntry {
	return ParseDevices(Out("devices"))
}

// ParseDevices parses `adb devices` output.
func ParseDevices(out string) []DeviceEntry {
	var list []DeviceEntry
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "List") || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "*") {
			continue
		}
		f := strings.Fields(line)
		e := DeviceEntry{Serial: f[0]}
		if len(f) > 1 {
			e.State = f[1]
		}
		list = append(list, e)
	}
	return list
}

// ReadySerials returns serials whose state is "device".
func ReadySerials() []string {
	var s []string
	for _, d := range ListDevices() {
		if d.State == "device" {
			s = append(s, d.Serial)
		}
	}
	return s
}

// ParseGetprop parses `getprop` output ("[key]: [value]") into a map.
func ParseGetprop(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "]: [")
		if !ok {
			continue
		}
		props[k[1:]] = strings.TrimSuffix(v, "]")
	}
	return props
}
