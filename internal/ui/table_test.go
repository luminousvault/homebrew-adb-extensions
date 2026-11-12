package ui

import (
	"bytes"
	"os"
	"testing"
)

func TestTable(t *testing.T) {
	defer func() { Out = &Stream{W: os.Stdout} }()
	rows := [][]Cell{{{Text: "emulator-5554"}, {Text: "Pixel 7"}}, {{Text: "R5CX"}, {Text: "갤럭시"}}}

	// Piped: plain columns, no rule, so awk and the like can read it.
	var b bytes.Buffer
	Out = &Stream{W: &b}
	Table([]string{"SERIAL", "MODEL"}, rows)
	want := "SERIAL         MODEL\nemulator-5554  Pixel 7\nR5CX           갤럭시\n"
	if b.String() != want {
		t.Errorf("piped:\n%q\nwant\n%q", b.String(), want)
	}

	// Terminal: a rule under the headers spans the columns (Hangul counts 2).
	b.Reset()
	Out = &Stream{W: &b, TTY: true}
	Table([]string{"SERIAL", "MODEL"}, rows)
	want = "SERIAL         MODEL\n──────────────────────\nemulator-5554  Pixel 7\nR5CX           갤럭시\n"
	if b.String() != want {
		t.Errorf("terminal:\n%q\nwant\n%q", b.String(), want)
	}
}
