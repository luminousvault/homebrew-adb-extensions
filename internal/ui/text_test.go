package ui

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0ms"},
		{40 * time.Microsecond, "0ms"},
		{105 * time.Millisecond, "105ms"},
		{999 * time.Millisecond, "999ms"},
		{999600 * time.Microsecond, "1.0s"}, // rounds up to a second: never "1000ms"
		{1234 * time.Millisecond, "1.2s"},
		{12400 * time.Millisecond, "12.4s"},
		{-time.Second, "0ms"},
	} {
		if got := Duration(c.d); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestPluralAndQuote(t *testing.T) {
	for _, c := range []struct {
		n          int
		word, want string
	}{{0, "file", "0 files"}, {1, "APK", "1 APK"}, {3, "APK", "3 APKs"}, {2, "other APK", "2 other APKs"}} {
		if got := Plural(c.n, c.word); got != c.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", c.n, c.word, got, c.want)
		}
	}
	if got := Quote("my file.txt"); got != "'my file.txt'" {
		t.Errorf("Quote = %q", got)
	}
}

// Off a terminal there is no spinner, so a new step is printed as a "==>"
// line; a clock or counter (Update) and a repeated text are not.
func TestSpinnerOffTerminal(t *testing.T) {
	defer func() { Err = &Stream{W: os.Stderr} }()
	var b bytes.Buffer
	Err = &Stream{W: &b}
	sp := Spin("Recording the screen on Pixel 7 (Ctrl+C to stop)")
	sp.Update("Recording the screen on Pixel 7 (0:01, Ctrl+C to stop)")
	sp.Update("Recording the screen on Pixel 7 (0:02, Ctrl+C to stop)")
	sp.SetText("Stopping the recording on Pixel 7")
	sp.SetText("Stopping the recording on Pixel 7")
	sp.SetText("Saving the recording from Pixel 7")
	sp.Stop()
	want := "==> Recording the screen on Pixel 7 (Ctrl+C to stop)\n" +
		"==> Stopping the recording on Pixel 7\n" +
		"==> Saving the recording from Pixel 7\n"
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}
