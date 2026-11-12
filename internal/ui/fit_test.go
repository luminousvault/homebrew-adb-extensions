package ui

import "testing"

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		cols int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\033[0m"},
		{"\033[1;36m➤ abc\033[0m", 3, "\033[1;36m➤ a\033[0m"}, // ANSI kept, not counted
		{"📦 app.apk", 4, "📦 a\033[0m"},                        // emoji is 2 columns
		{"설치 완료", 5, "설치 \033[0m"},                            // Hangul is 2 columns
		{"áb", 2, "áb"},                                     // combining mark is 0 columns
	}
	for _, c := range cases {
		if got := Fit(c.in, c.cols); got != c.want {
			t.Errorf("Fit(%q,%d)=%q want %q", c.in, c.cols, got, c.want)
		}
	}
}
