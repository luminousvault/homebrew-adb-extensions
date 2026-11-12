package cli

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// readKey reads one key without Enter (like `read -rsn1`). Enter returns "".
// Without a terminal it reads a line instead.
func readKey() string {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, _ := stdin.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer func() { _ = term.Restore(fd, old) }()
	b := make([]byte, 1)
	if n, _ := os.Stdin.Read(b); n == 0 || b[0] == '\r' || b[0] == '\n' {
		return ""
	}
	if b[0] == 0x03 {
		_ = term.Restore(fd, old)
		os.Stdout.WriteString("\n")
		exit(130)
	}
	return string(b)
}
