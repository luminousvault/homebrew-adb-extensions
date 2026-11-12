// Package style holds the ANSI color constants shared by all commands.
// Values match the Bash version so output stays byte-identical.
package style

const (
	Red    = "\033[1;31m"
	Green  = "\033[1;32m"
	Yellow = "\033[1;33m"
	Blue   = "\033[1;34m"
	Purple = "\033[1;35m"
	Cyan   = "\033[1;36m"
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	NC     = "\033[0m"

	BArrow = Blue + "==>" + NC
	GArrow = Green + "==>" + NC
	Error  = Red + "==>" + NC + " " + Bold + "Error:" + NC
)
