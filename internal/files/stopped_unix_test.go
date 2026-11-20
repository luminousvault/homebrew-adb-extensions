//go:build !windows

package files

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
)

// killedErr is the error of a process ended by SIGKILL, as exec returns it.
func killedErr(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "kill -9 $$").Run()
	if !adb.KilledBySignal(err) {
		t.Fatalf("not a signal: %v", err)
	}
	return err
}

// pullRunner answers adb pull: the first `kills` calls write half the file
// and end as if another program killed adb, the rest write it whole.
type pullRunner struct {
	mu      sync.Mutex
	kills   int
	content string
	killed  error
	pulls   int
}

func (r *pullRunner) Run(args ...string) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(args) != 5 || args[2] != "pull" {
		return "", "", nil // forgetRemoved and the like
	}
	r.pulls++
	dst := args[4]
	if r.pulls <= r.kills {
		_ = os.WriteFile(dst, []byte(r.content[:len(r.content)/2]), 0o644)
		return "", "", r.killed
	}
	return "", "", os.WriteFile(dst, []byte(r.content), 0o644)
}

func (r *pullRunner) Passthrough(args ...string) error { _, _, err := r.Run(args...); return err }

func pullJob(t *testing.T, r *pullRunner) (*copyJob, string) {
	t.Helper()
	old := adb.Default
	adb.Default = r
	t.Cleanup(func() { adb.Default = old })
	dst := t.TempDir()
	write(t, dst, "b1.png") // the item the overwrite replaces
	dev := &deviceFS{serial: serial, name: "Test Phone"}
	src := Entry{Name: "b1.png", Path: "/sdcard/b1.png", Size: int64(len(r.content))}
	return &copyJob{from: dev, to: localFS{PC, computerLabel}, dstDir: dst,
		ops: []copyOp{{src: src, name: "b1.png", overwrite: true}}}, dst
}

// adb killed from outside once (not by Esc): the item is copied again from
// nothing and replaces the existing one.
func TestStoppedAdbIsTriedAgain(t *testing.T) {
	r := &pullRunner{kills: 1, content: "new picture", killed: killedErr(t)}
	job, dst := pullJob(t, r)
	res := job.run(context.Background(), func(progress) {})
	if len(res.failed) != 0 || len(res.done) != 1 || r.pulls != 2 {
		t.Fatalf("pulls %d, %+v", r.pulls, res)
	}
	if got := read(t, filepath.Join(dst, "b1.png")); got != "new picture" {
		t.Fatalf("b1.png = %q", got)
	}
	assertNoTemp(t, dst)
}

// Killed twice: it fails with a message that says what happened, and the
// existing item stays.
func TestStoppedAdbTwiceFails(t *testing.T) {
	r := &pullRunner{kills: 2, content: "new picture", killed: killedErr(t)}
	job, dst := pullJob(t, r)
	res := job.run(context.Background(), func(progress) {})
	if len(res.failed) != 1 || r.pulls != 2 {
		t.Fatalf("pulls %d, %+v", r.pulls, res)
	}
	if msg := res.failed[0].msg; !strings.Contains(msg, "stopped from outside ak") || !strings.Contains(msg, "try again") {
		t.Fatalf("message: %s", msg)
	}
	if got := read(t, filepath.Join(dst, "b1.png")); got != "b1.png" {
		t.Fatalf("the existing item was lost: %q", got)
	}
	assertNoTemp(t, dst)
}
