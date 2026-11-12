package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// ErrNoInput means a question could not be asked (stdin is not a terminal and -y was not given).
var ErrNoInput = errors.New("confirmation needed but input is not a terminal")

// Confirm asks a yes/no question on stderr. defaultYes decides what Enter means.
// With -y it answers yes without asking. It reads one key on a terminal.
func Confirm(question string, defaultYes bool) (bool, error) {
	if AssumeYes {
		return true, nil
	}
	if !InTTY {
		return false, ErrNoInput
	}
	choices := "[y/N]"
	if defaultYes {
		choices = "[Y/n]"
	}
	Err.Printf("%s %s ", question, choices)
	k, err := ReadKey()
	if err != nil {
		Err.Println("")
		return false, err
	}
	Err.Println(k)
	switch strings.ToLower(k) {
	case "":
		return defaultYes, nil
	case "y":
		return true, nil
	}
	return false, nil
}

// ErrInterrupted is returned when the user presses Ctrl+C at a prompt.
var ErrInterrupted = errors.New("interrupted")

// ReadKey reads a single key from the terminal without Enter. Enter returns "".
func ReadKey() (string, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		line, rerr := In.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), rerr
	}
	defer func() { _ = term.Restore(fd, old) }()
	b := make([]byte, 1)
	if n, _ := os.Stdin.Read(b); n == 0 {
		return "", nil
	}
	switch b[0] {
	case '\r', '\n':
		return "", nil
	case 0x03:
		return "", ErrInterrupted
	}
	return string(b), nil
}

// ── Spinner ──────────────────────────────────────────

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Spinner shows "⠋ text" on stderr while work runs (terminals only).
type Spinner struct {
	text string
	stop chan struct{}
	done sync.WaitGroup
}

// Spin starts a spinner. Without a terminal it prints "==> text" once.
func Spin(format string, args ...any) *Spinner {
	s := &Spinner{text: fmt.Sprintf(format, args...)}
	if !Err.TTY {
		Step("%s", s.text)
		return s
	}
	s.stop = make(chan struct{})
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			Err.Printf("\r\033[K%s %s", Err.Cyan(string(spinFrames[i%len(spinFrames)])), s.text)
			select {
			case <-s.stop:
				Err.Printf("\r\033[K")
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Stop clears the spinner line.
func (s *Spinner) Stop() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	s.done.Wait()
	s.stop = nil
}

// ── Data output (stdout) ─────────────────────────────

// Cell is a table value with an optional color applied after alignment.
type Cell struct {
	Text  string
	Color func(string) string
}

// Table prints rows aligned in columns under dim bold headers (stdout).
func Table(headers []string, rows [][]Cell) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
		}
	}
	line := func(cells []string, raw []string) {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(c)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(raw[i])+2))
			}
		}
		Out.Println(b.String())
	}
	hs := make([]string, len(headers))
	for i, h := range headers {
		hs[i] = Out.Paint(dim+bold, h)
	}
	line(hs, headers)
	for _, r := range rows {
		cells := make([]string, len(r))
		raw := make([]string, len(r))
		for i, c := range r {
			raw[i] = c.Text
			cells[i] = c.Text
			if c.Color != nil {
				cells[i] = c.Color(c.Text)
			}
		}
		line(cells, raw)
	}
}

// KeyValues prints "  key  value" pairs aligned (stdout).
func KeyValues(pairs [][2]string) {
	w := 0
	for _, p := range pairs {
		w = max(w, len(p[0]))
	}
	for _, p := range pairs {
		Out.Printf("  %-*s  %s\n", w, p[0], p[1])
	}
}

// JSON prints v as indented JSON (stdout).
func JSON(v any) error {
	enc := json.NewEncoder(Out.W)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Choose asks a question answered by one key. choices are lowercase keys; the
// first is the default for Enter and is shown uppercase, e.g. "[Y/n/a]".
// It returns the chosen key. With -y it returns the first choice.
func Choose(question string, choices ...string) (string, error) {
	if AssumeYes {
		return choices[0], nil
	}
	if !InTTY {
		return "", ErrNoInput
	}
	shown := make([]string, len(choices))
	for i, c := range choices {
		shown[i] = c
		if i == 0 {
			shown[i] = strings.ToUpper(c)
		}
	}
	for {
		Err.Printf("%s [%s] ", question, strings.Join(shown, "/"))
		k, err := ReadKey()
		if err != nil {
			Err.Println("")
			return "", err
		}
		Err.Println(k)
		k = strings.ToLower(k)
		if k == "" {
			return choices[0], nil
		}
		for _, c := range choices {
			if k == c {
				return c, nil
			}
		}
	}
}
