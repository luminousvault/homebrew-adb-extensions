package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
)

const pushListCmd = "if [ ! -e '/sdcard/Download' ] && [ ! -L '/sdcard/Download' ]; then echo ak:missing; " +
	"elif [ ! -d '/sdcard/Download' ]; then echo ak:notdir; " +
	"else cd '/sdcard/Download' && ls -A && echo ak:listed; fi"

// 2: a folder that exists but can't be listed is not created over, and a
// file in its place is an error.
func TestPushDestinationStates(t *testing.T) {
	dir := pushFiles(t, "a.txt")
	local := filepath.Join(dir, "a.txt")
	for _, c := range []struct {
		out, want string
	}{
		{"/system/bin/sh: cd: /sdcard/Download: Permission denied\n", "Error: Couldn't list '/sdcard/Download' on Google Pixel 9 Pro"},
		{"ak:notdir\n", "Error: '/sdcard/Download' on Google Pixel 9 Pro is not a directory"},
	} {
		f := baseFake()
		f.Responses["-s "+serial+" shell "+pushListCmd] = adbtest.Response{Stdout: c.out}
		f.Responses["-s "+serial+" push "+local+" /sdcard/Download/a.txt"] = adbtest.Response{}
		r := ak(t, f, "push", local)
		if r.code != 1 || !strings.Contains(r.err, c.want) || strings.Contains(r.err, "Created") {
			t.Errorf("%q: code %d\n%s", c.out, r.code, r.err)
		}
		for _, call := range f.Calls {
			if strings.Contains(call, " push ") || strings.Contains(call, "mkdir") {
				t.Errorf("%q: must not %s", c.out, call)
			}
		}
	}
}

// 5: the warning names the file that is on the device.
func TestPushWarningNamesTheDevicesFile(t *testing.T) {
	dir := pushFiles(t, "report.pdf")
	local := filepath.Join(dir, "report.pdf")
	f := pushFake([]string{"REPORT.PDF"}, map[string]adbtest.Response{local + " /sdcard/Download/report (1).pdf": {}})
	r := ak(t, f, "push", local)
	if r.code != 0 || !strings.Contains(r.err, "Warning: '/sdcard/Download/REPORT.PDF' already exists on Google Pixel 9 Pro, pushing as 'report (1).pdf'\n") {
		t.Errorf("code %d\n%s", r.code, r.err)
	}
}
