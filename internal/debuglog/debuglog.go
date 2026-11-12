// Package debuglog writes diagnostic logs when AK_DEBUG=1.
package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var file *os.File

// Path returns the log file location: $AK_DEBUG_LOG, or <tmp>/ak-debug.log.
func Path() string {
	if p := os.Getenv("AK_DEBUG_LOG"); p != "" {
		return p
	}
	return filepath.Join(os.TempDir(), "ak-debug.log")
}

// Init truncates the log file when AK_DEBUG=1.
func Init() {
	if os.Getenv("AK_DEBUG") != "1" {
		return
	}
	p := Path()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.Create(p)
	if err != nil {
		return
	}
	file = f
	fmt.Fprintf(file, "=== Debug Log Started at %s ===\nAK_DEBUG=1\nDEBUG_LOG_FILE=%s\n\n", time.Now().Format(time.RFC1123), p)
}

// Logf appends one line with timestamp and caller location.
func Logf(format string, args ...any) {
	if file == nil {
		return
	}
	_, src, line, _ := runtime.Caller(1)
	fmt.Fprintf(file, "[%s] [%s:%d] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), filepath.Base(src), line, fmt.Sprintf(format, args...))
}
