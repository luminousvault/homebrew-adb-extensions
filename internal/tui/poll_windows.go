//go:build windows

package tui

import "time"

// readable on Windows cannot wait on the console: it says yes, and Read
// blocks as before (escape sequences arrive whole from the console).
func readable(int, time.Duration) bool { return true }
