package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb/adbtest"
	"github.com/luminousvault/homebrew-adb-extensions/internal/files"
)

// /sdcard ignores case: report.PDF there is report.pdf, so push renames.
func TestPushRenamesACaseVariant(t *testing.T) {
	dir := pushFiles(t, "report.pdf")
	local := filepath.Join(dir, "report.pdf")
	f := pushFake([]string{"REPORT.PDF"}, map[string]adbtest.Response{local + " /sdcard/Download/report (1).pdf": {}})
	r := ak(t, f, "push", local)
	if r.code != 0 || !strings.Contains(r.err, "Warning: '/sdcard/Download/REPORT.PDF' already exists on Google Pixel 9 Pro, pushing as 'report (1).pdf'\n") {
		t.Errorf("code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
}

// pull does not overwrite a file already in the folder (any case).
func TestPullKeepsExistingLocalFiles(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "REPORT.pdf"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := baseFake()
	f.Responses["-s "+serial+" shell "+recentQuery()] = adbtest.Response{Stdout: "Row: 0 _data=/storage/emulated/0/Download/report.pdf, date_modified=2, _size=10\n"}
	f.Responses["-s "+serial+" pull /storage/emulated/0/Download/report.pdf "+filepath.Join(dest, "report (1).pdf")] = adbtest.Response{}
	r := ak(t, f, "pull", dest, "-l")
	if r.code != 0 || !strings.Contains(r.err, "Warning: 'report.pdf' already exists in '"+dest+"', saving as 'report (1).pdf'") {
		t.Errorf("code %d\n%s\ncalls %q", r.code, r.err, f.Calls)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "REPORT.pdf")); string(b) != "mine" {
		t.Error("the existing file was overwritten")
	}
}

func recentQuery() string { return files.RecentFilesQuery }
