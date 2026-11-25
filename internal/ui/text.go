package ui

import (
	"strconv"
	"time"
)

// Quote wraps a name (package, file, folder, a value the user typed) in
// single quotes so it stands apart from the words around a message:
// Installed 'app.apk' on Pixel 7.
func Quote(s string) string { return "'" + s + "'" }

// Duration formats elapsed time for messages: "120ms" under a second,
// otherwise seconds with one decimal ("1.2s"). It never prints "0.0s".
func Duration(d time.Duration) string {
	ms := d.Round(time.Millisecond).Milliseconds()
	if ms < 1000 {
		return strconv.FormatInt(max(ms, 0), 10) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// Plural counts things: "1 APK", "3 APKs", "1 file", "0 files".
// word is the singular; the plural adds "s".
func Plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
