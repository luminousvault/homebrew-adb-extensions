//go:build windows

package tui

// Windows has no SIGWINCH; the size is re-read on every redraw instead.
func watchResize() (stop func()) { return func() {} }
