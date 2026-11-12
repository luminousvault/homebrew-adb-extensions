//go:build windows

package cli

// interruptGroup is a no-op on Windows: running adb installs finish, and no
// new install starts after installStop is set.
func interruptGroup() {}
