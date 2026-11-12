// Package ui is the single place that writes user-facing output.
// Results go to stdout (Out); messages, prompts and progress go to stderr (Err).
// Colors are used only on terminals, honoring NO_COLOR, --no-color, TERM=dumb
// and CLICOLOR_FORCE. See docs/design/cli-output.md.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ANSI codes. Use them through Stream.Paint so they disappear without color.
const (
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	reset  = "\033[0m"
)

// Stream is one output stream and whether it is a colored terminal.
type Stream struct {
	W     io.Writer
	TTY   bool
	Color bool
}

// Out is stdout (results), Err is stderr (everything else).
var (
	Out = &Stream{W: os.Stdout}
	Err = &Stream{W: os.Stderr}
	In  = bufio.NewReader(os.Stdin)
	// InTTY reports whether stdin is a terminal (prompts and pickers need it).
	InTTY bool
	// AssumeYes answers every confirmation with yes (-y/--yes).
	AssumeYes bool
)

// Setup detects terminals and color support. noColor is the --no-color flag.
func Setup(noColor bool) {
	Out.TTY = isTerminal(os.Stdout)
	Err.TTY = isTerminal(os.Stderr)
	InTTY = isTerminal(os.Stdin)
	Out.Color = colorEnabled(Out.TTY, noColor)
	Err.Color = colorEnabled(Err.TTY, noColor)
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func colorEnabled(tty, noColor bool) bool {
	if noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return true
	}
	return tty
}

// Paint wraps s in the given ANSI codes when the stream is colored.
func (s *Stream) Paint(codes, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return codes + text + reset
}

func (s *Stream) Red(t string) string    { return s.Paint(red, t) }
func (s *Stream) Green(t string) string  { return s.Paint(green, t) }
func (s *Stream) Yellow(t string) string { return s.Paint(yellow, t) }
func (s *Stream) Cyan(t string) string   { return s.Paint(cyan, t) }
func (s *Stream) Bold(t string) string   { return s.Paint(bold, t) }
func (s *Stream) Dim(t string) string    { return s.Paint(dim, t) }

// Printf writes formatted text.
func (s *Stream) Printf(format string, args ...any) { fmt.Fprintf(s.W, format, args...) }

// Println writes a line.
func (s *Stream) Println(line string) { fmt.Fprintln(s.W, line) }

// ── Messages (stderr) ────────────────────────────────

// Step prints a section header: "==> text".
func Step(format string, args ...any) {
	Err.Println(Err.Paint(blue+bold, "==>") + " " + Err.Bold(fmt.Sprintf(format, args...)))
}

// Success prints "✓ text".
func Success(format string, args ...any) {
	Err.Println(Err.Green("✓") + " " + fmt.Sprintf(format, args...))
}

// Info prints an indented line.
func Info(format string, args ...any) { Err.Println("  " + fmt.Sprintf(format, args...)) }

// Detail prints an indented, dimmed line (raw tool output, paths, timings).
func Detail(format string, args ...any) {
	for _, l := range strings.Split(strings.TrimRight(fmt.Sprintf(format, args...), "\n"), "\n") {
		Err.Println(Err.Dim("  " + l))
	}
}

// Warn prints "Warning: text".
func Warn(format string, args ...any) {
	Err.Println(Err.Yellow("Warning:") + " " + fmt.Sprintf(format, args...))
}

// Errorf prints "Error: text".
func Errorf(format string, args ...any) {
	Err.Println(Err.Red("Error:") + " " + fmt.Sprintf(format, args...))
}

// Hint prints "Hint: text".
func Hint(format string, args ...any) {
	Err.Println(Err.Dim("Hint:") + " " + fmt.Sprintf(format, args...))
}

// Plain returns s with ANSI escape sequences removed.
func Plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
